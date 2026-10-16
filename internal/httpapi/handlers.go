package httpapi

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/importer"
	"github.com/billydos/components-catalog/internal/service"
)

// Хендлеры /api/v1 — тонкие транспорты над сервисным слоем: разбор
// запроса, вызов сервиса, вывод (docs/plan/04-module-functionality.md §2).

// handleKinds — GET /api/v1/kinds: классы приборов.
func (a *API) handleKinds(w *responseWriter, r *http.Request, _ map[string]string) {
	snap, err := a.app.Snapshot(r.Context())
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	lang := requestLang(r)
	resp := kindsResponseJSON{Kinds: make([]kindDefJSON, 0, len(snap.Kinds))}
	for _, k := range snap.Kinds {
		resp.Kinds = append(resp.Kinds, kindDefJSON{
			Code: string(k.Code), Name: i18n.KindName(lang, string(k.Code)),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCatalog — GET /api/v1/catalog: снимок каталога для построения
// форм и фильтров; ETag по catalog_revision (сайт кэширует до смены).
func (a *API) handleCatalog(w *responseWriter, r *http.Request, _ map[string]string) {
	snap, err := a.app.Snapshot(r.Context())
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	etag := catalogETag(snap.Revision)
	w.Header().Set("ETag", etag)
	if etagMatch(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, catalogToJSON(requestLang(r), snap))
}

// handleStats — GET /api/v1/stats: количества по классам, версия схемы,
// ревизии; с kind — фасетные агрегаты значений параметров класса (CC-5):
// min/max численных параметров и частоты текстовых/enum значений.
// Повтор разбираемого параметра и посторонний ключ запроса — 400.
func (a *API) handleStats(w *responseWriter, r *http.Request, _ map[string]string) {
	values, err := parseStrictQuery(r)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if err := rejectDuplicateParams(values, "kind"); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if name != "kind" {
			a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed,
				domain.MsgApiQueryParamUnknown, name)
			return
		}
	}
	if kind := values.Get("kind"); kind != "" {
		ps, err := a.app.Services().Devices.ParameterStats(r.Context(), domain.Kind(kind))
		if err != nil {
			a.writeDomainErr(w, r, err, true)
			return
		}
		writeJSON(w, http.StatusOK, parameterStatsToJSON(requestLang(r), ps))
		return
	}
	st, err := a.app.Stats(r.Context())
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	kinds := make(map[string]int, len(st.Counts))
	for k, n := range st.Counts {
		kinds[string(k)] = n
	}
	writeJSON(w, http.StatusOK, statsJSON{
		SchemaVersion:   st.SchemaVersion,
		CatalogRevision: st.CatalogRevision,
		DataRevision:    st.DataRevision,
		Total:           st.Total,
		Kinds:           kinds,
	})
}

// parseStrictQuery разбирает строку запроса строго: url.Values.Query()
// молча отбрасывает всю строку при ошибке разбора («?kind=transistor;q=x»
// даёт пустые фильтры и 200 без фильтрования) — здесь ошибка разбора
// становится 400 (строгий контракт параметров запроса).
func parseStrictQuery(r *http.Request) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgApiQueryInvalid, err.Error())
	}
	return values, nil
}

// handleSearch — GET /api/v1/components: подстрока обозначения, система,
// фильтры полей/атрибутов/параметров, сортировка, пагинация; ETag —
// data_revision и хэш канонической строки запроса (разные фильтры —
// разные ETag). Разбор параметров и сервисная валидация запроса — строго
// до проверки ETag: ошибки 400 (транспортные и семантические) приоритетнее
// отсечения 304, в том числе для If-None-Match: *.
func (a *API) handleSearch(w *responseWriter, r *http.Request, _ map[string]string) {
	snap, err := a.app.Snapshot(r.Context())
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	values, err := parseStrictQuery(r)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	q, err := parseSearchQuery(values, snap)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if err := a.app.Services().Devices.ValidateSearch(r.Context(), q); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	_, dataRev, err := a.app.Revisions(r.Context())
	if err != nil {
		a.writeErr(w, r, err)
		return
	}
	etag := searchETag(dataRev, values)
	w.Header().Set("ETag", etag)
	if etagMatch(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	page, err := a.app.Services().Devices.Search(r.Context(), q)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	writeJSON(w, http.StatusOK, searchPageToJSON(page))
}

// handleGet — GET /api/v1/components/{kind}/{designation}: карточка
// (поля, атрибуты, группы значений, исполнения, аналоги, производители).
func (a *API) handleGet(w *responseWriter, r *http.Request, params map[string]string) {
	kind := domain.Kind(params["kind"])
	if err := checkKind(r, a.app, kind); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	card, found, err := a.app.Services().Devices.Get(r.Context(), kind, params["designation"])
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if !found {
		a.writeError(w, r, http.StatusNotFound, domain.CodeNotFound,
			domain.MsgApiCardNotFound, params["designation"])
		return
	}
	writeJSON(w, http.StatusOK, cardToJSON(requestLang(r), card))
}

// handleGetByID — GET /api/v1/components/id/{id}: карточка по стабильному
// id (обозначения с «/» адресуются этим путём или поиском).
func (a *API) handleGetByID(w *responseWriter, r *http.Request, params map[string]string) {
	id, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil {
		a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed, domain.MsgApiBadID)
		return
	}
	card, found, err := a.app.Services().Devices.GetByID(r.Context(), id)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if !found {
		a.writeError(w, r, http.StatusNotFound, domain.CodeNotFound,
			domain.MsgApiIdNotFound, params["id"])
		return
	}
	writeJSON(w, http.StatusOK, cardToJSON(requestLang(r), card))
}

// handleSuggest — GET /api/v1/suggest: автодополнение по префиксу
// обозначения (хендлер отключается конфигурацией — не монтируется).
func (a *API) handleSuggest(w *responseWriter, r *http.Request, _ map[string]string) {
	values, err := parseStrictQuery(r)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if err := rejectDuplicateParams(values, "q", "kind", "limit"); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	q := values.Get("q")
	if q == "" {
		a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed, domain.MsgApiQMissing)
		return
	}
	kind := domain.Kind(values.Get("kind"))
	limit, exists, err := queryLimit(values["limit"], service.SuggestLimitMax)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if !exists {
		limit = service.SuggestDefaultLimit
	}
	suggests, err := a.app.Services().Designations.Suggest(r.Context(), q, kind, limit)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	resp := suggestResponseJSON{Items: make([]suggestionJSON, 0, len(suggests))}
	for _, s := range suggests {
		resp.Items = append(resp.Items, suggestionJSON{
			Kind: string(s.Kind), System: string(s.System), Designation: s.Designation,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleParse — GET /api/v1/parse: разбор обозначения (автодетект либо
// явная система/класс) с перечнем кандидатов строгих систем — основа UX
// ввода обозначения (features.log CC-1); разбор — только сервисный слой.
func (a *API) handleParse(w *responseWriter, r *http.Request, _ map[string]string) {
	values, err := parseStrictQuery(r)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if err := rejectDuplicateParams(values, "designation", "system", "kind"); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	designation := values.Get("designation")
	if designation == "" {
		a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed, domain.MsgApiDesignationMissing)
		return
	}
	p, candidates, err := a.app.Services().Designations.ParseWithCandidates(
		r.Context(), designation,
		domain.System(values.Get("system")), domain.Kind(values.Get("kind")))
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	writeJSON(w, http.StatusOK, parseToJSON(p, candidates))
}

// handleChanges — GET /api/v1/changes: поток событий изменений записей
// (change feed — features.log CC-2) для инкрементальной синхронизации
// производных индексов сайта; курсор — id последнего полученного события
// (after). Фид покрывает только изменения данных (data_revision):
// изменения каталога отслеживаются ETag GET /catalog. Повтор разбираемого
// параметра и посторонний ключ запроса — 400 (опечатки курсора не молчат).
func (a *API) handleChanges(w *responseWriter, r *http.Request, _ map[string]string) {
	values, err := parseStrictQuery(r)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if err := rejectDuplicateParams(values, "after", "limit"); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if name != "after" && name != "limit" {
			a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed,
				domain.MsgApiQueryParamUnknown, name)
			return
		}
	}
	after := int64(0)
	if vs := values["after"]; len(vs) > 0 {
		n, perr := strconv.ParseInt(vs[0], 10, 64)
		if perr != nil || n < 0 {
			a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed,
				domain.MsgApiAfterNonNeg)
			return
		}
		after = n
	}
	limit, exists, err := queryLimit(values["limit"], service.ChangesLimitMax)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if !exists {
		limit = service.ChangesDefaultLimit
	}
	page, err := a.app.Services().Devices.Changes(r.Context(), after, limit)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	writeJSON(w, http.StatusOK, changesToJSON(page))
}

// handleImport — POST /api/v1/import: потоковый импорт файла наполнения
// NDJSON (features.log CC-6). Тело читается построчно без загрузки
// целиком и без потолка тела запроса — память ограничена потолком строки
// сканера (16 МиБ); ответ — синхронный отчёт после EOF (Issues-модель:
// корректные записи применяются, ошибочные — нет, отчёт — 200 даже
// с проблемами). Жёсткая ошибка — только синтаксис формата и чтения
// потока (400 invalid_import_file). Параметров запроса нет: любой ключ —
// 400 (опечатки не молчат). Прогресс по REST не отдаётся — запрос
// синхронный (наблюдение — хук метрик); запись монтируется сайтом
// только в защищённые маршруты.
func (a *API) handleImport(w *responseWriter, r *http.Request, _ map[string]string) {
	values, err := parseStrictQuery(r)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if names := slices.Sorted(maps.Keys(values)); len(names) > 0 {
		a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed,
			domain.MsgApiQueryParamUnknown, names[0])
		return
	}
	rep, err := importer.New(a.app).Import(r.Context(), r.Body, importBodyName, false)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	// Инфраструктурные проблемы прогона (не-доменные ошибки применения)
	// наружу отдаются санитизированным фиксированным текстом
	// (importReportToJSON); технические детали — только здесь.
	for _, iss := range rep.Issues {
		if iss.MsgID == domain.MsgInternalError {
			a.log.Error("импорт: непредвиденная проблема записи",
				"record", iss.Record, "line", iss.Line, "error", iss.Message)
		}
	}
	writeJSON(w, http.StatusOK, importReportToJSON(requestLang(r), &rep))
}

// importBodyName — условное имя тела запроса POST /import: формат —
// NDJSON по расширению, в ответ имя не входит (файла нет — тело).
const importBodyName = "import.ndjson"

// handleCreate — POST /api/v1/components: создание; тело — запись
// наполнения (name/system + секции), класс — автодетект по обозначению
// (для system other создание адресуется PUT с классом в пути).
// Существующий (kind, designation) — 409 already_exists без изменения
// записи: сервисный Create атомарно отказывает (гонка параллельных POST
// не применяет секций проигравшего к чужой записи).
func (a *API) handleCreate(w *responseWriter, r *http.Request, _ map[string]string) {
	in, err := a.readRecordBody(w, r)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	p, err := a.app.Services().Designations.ParseForSystem(r.Context(), in.Name, in.System, in.Kind)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	outcome, err := a.app.Services().Devices.Create(r.Context(), in)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	card, found, err := a.app.Services().Devices.Get(r.Context(), p.Kind, p.Designation)
	if err != nil {
		a.writeErr(w, r, err)
		return
	}
	if !found {
		a.writeError(w, r, http.StatusNotFound, domain.CodeNotFound,
			domain.MsgApiCardNotFound, p.Designation)
		return
	}
	writeJSON(w, http.StatusCreated, upsertResponseJSON{Outcome: string(outcome), Card: cardToJSON(requestLang(r), card)})
}

// handlePut — PUT /api/v1/components/{kind}/{designation}: upsert
// с семантикой секций; name тела обязан совпадать с designation пути
// (422 designation_mismatch).
func (a *API) handlePut(w *responseWriter, r *http.Request, params map[string]string) {
	kind := domain.Kind(params["kind"])
	if err := checkKind(r, a.app, kind); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	in, err := a.readRecordBody(w, r)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	pathDesignation, err := domain.Canonicalize(params["designation"])
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	bodyDesignation, err := domain.Canonicalize(in.Name)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	if bodyDesignation != pathDesignation {
		a.writeError(w, r, http.StatusUnprocessableEntity, domain.CodeDesignationMismatch, domain.MsgApiDesignationMismatch)
		return
	}
	in.Kind = kind
	outcome, err := a.app.Services().Devices.Upsert(r.Context(), in)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	card, found, err := a.app.Services().Devices.Get(r.Context(), kind, pathDesignation)
	if err != nil {
		a.writeErr(w, r, err)
		return
	}
	if !found {
		// Запись удалена между применением и чтением карточки — 404,
		// как соседние пути (не внутренняя ошибка с nil-текстом).
		a.writeError(w, r, http.StatusNotFound, domain.CodeNotFound,
			domain.MsgApiCardNotFound, pathDesignation)
		return
	}
	writeJSON(w, http.StatusOK, upsertResponseJSON{Outcome: string(outcome), Card: cardToJSON(requestLang(r), card)})
}

// handleDelete — DELETE /api/v1/components/{kind}/{designation}:
// удаление (каскад из devices по дочерним таблицам, чистка сирот).
func (a *API) handleDelete(w *responseWriter, r *http.Request, params map[string]string) {
	kind := domain.Kind(params["kind"])
	if err := checkKind(r, a.app, kind); err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	deleted, err := a.app.Services().Devices.Delete(r.Context(), kind, params["designation"])
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	if !deleted {
		a.writeError(w, r, http.StatusNotFound, domain.CodeNotFound,
			domain.MsgApiCardNotFound, params["designation"])
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readRecordBody читает и разбирает тело записи наполнения
// (общий читатель формата: дубликаты ключей — ошибка разбора).
// MaxBytesReader получает исходный ResponseWriter (не обёртку): сигнал
// requestTooLarge — закрытие соединения сервером — срабатывает только
// на оригинальном writer'е net/http.
func (a *API) readRecordBody(w *responseWriter, r *http.Request) (service.DeviceInput, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w.ResponseWriter, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return service.DeviceInput{}, domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgApiBodyTooLarge)
		}
		return service.DeviceInput{}, domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgApiBodyRead)
	}
	snap, err := a.app.Snapshot(r.Context())
	if err != nil {
		return service.DeviceInput{}, err
	}
	return importer.ReadRecordJSON(data, snap)
}

// requestLang — локаль отображаемых строк ответа: Accept-Language
// (q-значения, базовый подтег), по умолчанию en (D9).
func requestLang(r *http.Request) i18n.Language {
	return i18n.Negotiate(r.Header.Get("Accept-Language"))
}

// checkKind — класс пути существует в каталоге (текст ошибки — общий
// с сервисным слоем контракт).
func checkKind(r *http.Request, app *service.App, kind domain.Kind) error {
	snap, err := app.Snapshot(r.Context())
	if err != nil {
		return err
	}
	if _, ok := snap.Kind(kind); !ok {
		return domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgKindUnknown, string(kind))
	}
	return nil
}
