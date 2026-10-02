package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Config — конфигурация REST API (plan/01-architecture.md §3: без глобальных
// переменных; конфигурацию передаёт сайт). DisableSuggest отключает
// GET /api/v1/suggest — хендлер не монтируется (plan/04-module-
// functionality.md §2); Metrics — хук метрик сайта.
type Config struct {
	Log            *slog.Logger
	DisableSuggest bool
	Metrics        Metrics
}

// Metrics — хук метрик REST (plan/04-module-functionality.md §5): счётчики
// и длительности вызовов по операциям; реализация — на стороне сайта,
// модуль свой экспорт метрик не заводит. errCode — код ошибки домена
// ("" — успешный запрос).
type Metrics interface {
	Observe(op string, status int, errCode string, duration time.Duration)
}

// API — версионируемый REST API /api/v1: тонкий транспорт над сервисным
// слоем приложения (без бизнес-логики), монтируемый в роутер сайта.
// Безопасен для одновременного использования несколькими горутинами.
type API struct {
	app     *service.App
	log     *slog.Logger
	metrics Metrics
	suggest bool
}

// New создаёт API над приложением. Приложение остаётся открытым на стороне
// сайта (App.Close — обязанность владельца).
func New(app *service.App, cfg Config) *API {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &API{app: app, log: log, metrics: cfg.Metrics, suggest: !cfg.DisableSuggest}
}

// Handler возвращает http.Handler REST /api/v1 для монтирования в роутер
// сайта (http.ServeMux: mux.Handle("/api/v1/", api.Handler())).
func (a *API) Handler() http.Handler { return a }

// apiPrefix — версионируемый префикс всех маршрутов.
const apiPrefix = "/api/v1"

// maxBodyBytes — потолок тела записи (POST/PUT): запись наполнения
// заведомо меньше; превышение — 400, а не неограниченное чтение.
const maxBodyBytes = 8 << 20

// route — маршрут: метод, шаблон пути (литеральные сегменты и «{параметр}»)
// и operationId спецификации (совпадение закреплено тестом).
type route struct {
	method  string
	pattern string
	op      string
	handler func(a *API, w *responseWriter, r *http.Request, params map[string]string)
}

// routes — таблица маршрутов /api/v1 (plan/04-module-functionality.md §2);
// порядок значим: литеральный сегмент «id» точнее параметра {kind}.
var routes = []route{
	{method: http.MethodGet, pattern: apiPrefix + "/kinds", op: "kinds", handler: (*API).handleKinds},
	{method: http.MethodGet, pattern: apiPrefix + "/catalog", op: "catalog", handler: (*API).handleCatalog},
	{method: http.MethodGet, pattern: apiPrefix + "/stats", op: "stats", handler: (*API).handleStats},
	{method: http.MethodGet, pattern: apiPrefix + "/components", op: "components_list", handler: (*API).handleSearch},
	{method: http.MethodPost, pattern: apiPrefix + "/components", op: "components_create", handler: (*API).handleCreate},
	{method: http.MethodGet, pattern: apiPrefix + "/components/id/{id}", op: "components_by_id", handler: (*API).handleGetByID},
	{method: http.MethodGet, pattern: apiPrefix + "/components/{kind}/{designation}", op: "components_get", handler: (*API).handleGet},
	{method: http.MethodPut, pattern: apiPrefix + "/components/{kind}/{designation}", op: "components_put", handler: (*API).handlePut},
	{method: http.MethodDelete, pattern: apiPrefix + "/components/{kind}/{designation}", op: "components_delete", handler: (*API).handleDelete},
	{method: http.MethodGet, pattern: apiPrefix + "/suggest", op: "suggest", handler: (*API).handleSuggest},
}

// activeRoutes — маршруты с учётом конфигурации (suggest отключаемый).
func (a *API) activeRoutes() []route {
	if a.suggest {
		return routes
	}
	out := make([]route, 0, len(routes))
	for _, r := range routes {
		if r.op == "suggest" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// opContextKey — ключ slog-контекста запроса (операция маршрута).
type opContextKey struct{}

// contextWithOp — slog-контекст запроса: операция доступна вложенным
// вызовам через контекст.
func contextWithOp(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, opContextKey{}, op)
}

// responseWriter — обёртка для статуса, кода ошибки домена и защиты от
// повторной записи заголовка (recover после частичной записи ответа).
type responseWriter struct {
	http.ResponseWriter
	status  int
	errCode domain.Code
	wrote   bool
}

func (w *responseWriter) WriteHeader(status int) {
	if !w.wrote {
		w.status = status
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// fail — метка ошибки домена для метрик и журнала.
func (w *responseWriter) fail(code domain.Code) { w.errCode = code }

// ServeHTTP — диспетчер: сопоставление пути и метода, slog-контекст
// запроса, хук метрик; JSON-модель ошибок — на всех ответах.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	op := "unknown"
	defer func() {
		if rec := recover(); rec != nil {
			a.log.Error("паника обработки запроса", "op", op, "method", r.Method,
				"path", r.URL.Path, "panic", rec)
			if !rw.wrote {
				a.writeError(rw, http.StatusInternalServerError, domain.CodeInternal, msgPanic)
			}
			rw.fail(domain.CodeInternal)
		}
		a.observe(r, rw, op, start)
	}()

	segments := splitPath(r.URL.Path)
	var params map[string]string
	pathMatched := false
	var handler route
	for _, rt := range a.activeRoutes() {
		m, ok := matchRoute(rt, segments)
		if !ok {
			continue
		}
		pathMatched = true
		if rt.method == r.Method {
			handler, params = rt, m
			break
		}
	}
	switch {
	case handler.handler != nil:
		op = handler.op
		handler.handler(a, rw, r.WithContext(contextWithOp(r.Context(), op)), params)
	case pathMatched:
		w.Header().Set("Allow", strings.Join(a.allowedMethods(segments), ", "))
		a.writeError(rw, http.StatusMethodNotAllowed, domain.CodeMethodNotAllowed, msgMethodNotAllowed)
	default:
		a.writeError(rw, http.StatusNotFound, domain.CodeNotFound, msgRouteNotFound)
	}
}

// observe — журнал запроса и метрики: уровень по статусу (2xx — info,
// 4xx — warn, 5xx — error).
func (a *API) observe(r *http.Request, rw *responseWriter, op string, start time.Time) {
	d := time.Since(start)
	level := slog.LevelInfo
	switch {
	case rw.status >= 500:
		level = slog.LevelError
	case rw.status >= 400:
		level = slog.LevelWarn
	}
	a.log.LogAttrs(r.Context(), level, "REST запрос",
		slog.String("op", op),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("status", rw.status),
		slog.String("error_code", string(rw.errCode)),
		slog.Duration("duration", d),
	)
	if a.metrics != nil {
		a.metrics.Observe(op, rw.status, string(rw.errCode), d)
	}
}

// splitPath — непустые сегменты пути.
func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// matchRoute сопоставляет сегменты пути с шаблоном маршрута; параметры —
// раскодированные сегменты URL (обозначения с «/» прямым путём не
// адресуются — plan/04-module-functionality.md §2).
func matchRoute(rt route, segments []string) (map[string]string, bool) {
	pattern := splitPath(rt.pattern)
	if len(pattern) != len(segments) {
		return nil, false
	}
	var params map[string]string
	for i, seg := range pattern {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if params == nil {
				params = make(map[string]string, 2)
			}
			params[seg[1:len(seg)-1]] = segments[i]
			continue
		}
		if seg != segments[i] {
			return nil, false
		}
	}
	return params, true
}

// allowedMethods — методы маршрутов, совпадающих путём (заголовок Allow).
func (a *API) allowedMethods(segments []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, rt := range a.activeRoutes() {
		if _, ok := matchRoute(rt, segments); ok && !seen[rt.method] {
			seen[rt.method] = true
			out = append(out, rt.method)
		}
	}
	sort.Strings(out)
	return out
}

// Тексты транспортных сообщений — контракт (как тексты домена);
// закреплены тестами дословно.
const (
	msgRouteNotFound    = "неизвестный маршрут запроса"
	msgMethodNotAllowed = "метод не допускается для этого маршрута"
	msgPanic            = "внутренняя ошибка обработки запроса"
	msgBodyTooLarge     = "тело запроса превышает допустимый размер"
	msgNotFoundCard     = "запись «%s» не найдена"
	msgNotFoundByID     = "запись с идентификатором %s не найдена"
	msgAlreadyExists    = "запись «%s» уже существует"
	msgMismatch         = "обозначение тела запроса не совпадает с обозначением в пути"
	msgBadID            = "параметр пути id: ожидается целое число"
	msgNoQ              = "не задан параметр q"
	msgLimitRange       = "параметр limit: ожидается целое от 1 до %d"
	msgOffsetNonNeg     = "параметр offset: ожидается целое неотрицательное число"
	msgSortKey          = "параметр sort: неизвестный ключ сортировки «%s»"
	msgUnknownParam     = "неизвестный параметр запроса «%s»"
	msgNumberParam      = "параметр «%s»: ожидается число"
	msgBoolAttrFilter   = "фильтр атрибута «%s»: логические атрибуты не поддерживаются в фильтрах"
)

// errorBody — модель ошибки REST (plan/04-module-functionality.md §2):
// машиночитаемый код, русский текст и необязательные подробности.
type errorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// writeJSON — JSON-ответ (UTF-8).
func writeJSON(w *responseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Ошибка кодирования после заголовка не перезаписывает статус;
	// фиксируется журналом диспетчера только при панике.
	_ = json.NewEncoder(w).Encode(v)
}

// writeError — транспортная ошибка с кодом домена.
func (a *API) writeError(w *responseWriter, status int, code domain.Code, msg string) {
	w.fail(code)
	writeJSON(w, status, errorBody{Code: string(code), Message: msg})
}

// writeDomainErr — ожидаемая ошибка домена: статус по коду и контексту
// (inQuery — параметры/путь запроса, иначе — тело записи POST/PUT).
func (a *API) writeDomainErr(w *responseWriter, err error, inQuery bool) {
	de, ok := domain.AsError(err)
	if !ok {
		a.writeErr(w, err)
		return
	}
	w.fail(de.Code)
	writeJSON(w, statusFor(de.Code, inQuery), errorBody{Code: string(de.Code), Message: de.Message})
}

// writeErr — неожидаемая ошибка: 500 internal_error, журнал error
// (технический текст сообщения контрактом не является).
func (a *API) writeErr(w *responseWriter, err error) {
	a.log.Error("непредвиденная ошибка REST", "error", err)
	w.fail(domain.CodeInternal)
	writeJSON(w, http.StatusInternalServerError,
		errorBody{Code: string(domain.CodeInternal), Message: err.Error()})
}

// statusFor — HTTP-статус по коду ошибки (план 04 §2: 400/404/409/422/500).
// Контекст query: ошибки параметров запроса и пути — 400 (фильтр по
// неприменимому параметру — 400 по плану); контекст body: синтаксис
// тела — 400, конфликт — 409, семантика записи — 422.
func statusFor(code domain.Code, inQuery bool) int {
	switch code {
	case domain.CodeNotFound:
		return http.StatusNotFound
	case domain.CodeAlreadyExists:
		return http.StatusConflict
	case domain.CodeSchemaVersionMismatch, domain.CodeDatabaseNotInitialized:
		return http.StatusInternalServerError
	case domain.CodeInvalidImportFile:
		return http.StatusBadRequest
	}
	if inQuery {
		switch code {
		case domain.CodeValidationFailed,
			domain.CodeUnknownParameter, domain.CodeUnknownAttribute, domain.CodeUnknownCondition,
			domain.CodeParameterNotApplicable, domain.CodeAttributeNotApplicable,
			domain.CodeInvalidDesignation:
			return http.StatusBadRequest
		}
	}
	return http.StatusUnprocessableEntity
}

// setETag — сильный ETag ревизии («catalog-<n>» / «data-<n>»).
func setETag(w *responseWriter, kind string, rev int64) {
	w.Header().Set("ETag", fmt.Sprintf(`"%s-%d"`, kind, rev))
}

// etagMatch — If-None-Match совпадает с ETag (точное значение либо «*»).
func etagMatch(r *http.Request, etag string) bool {
	header := r.Header.Get("If-None-Match")
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == etag || part == "W/"+etag {
			return true
		}
	}
	return false
}
