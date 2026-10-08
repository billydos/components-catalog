package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
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
	etag := fmt.Sprintf(`"catalog-%d"`, snap.Revision)
	setETag(w, "catalog", snap.Revision)
	if etagMatch(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, catalogToJSON(requestLang(r), snap))
}

// handleStats — GET /api/v1/stats: количества по классам, версия схемы,
// ревизии.
func (a *API) handleStats(w *responseWriter, r *http.Request, _ map[string]string) {
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

// handleSearch — GET /api/v1/components: подстрока обозначения, система,
// фильтры полей/атрибутов/параметров, сортировка, пагинация; ETag по
// data_revision.
func (a *API) handleSearch(w *responseWriter, r *http.Request, _ map[string]string) {
	snap, err := a.app.Snapshot(r.Context())
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	q, err := parseSearchQuery(r, snap)
	if err != nil {
		a.writeDomainErr(w, r, err, true)
		return
	}
	_, dataRev, err := a.app.Revisions(r.Context())
	if err != nil {
		a.writeErr(w, err)
		return
	}
	etag := fmt.Sprintf(`"data-%d"`, dataRev)
	setETag(w, "data", dataRev)
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
	q := r.URL.Query().Get("q")
	if q == "" {
		a.writeError(w, r, http.StatusBadRequest, domain.CodeValidationFailed, domain.MsgApiQMissing)
		return
	}
	kind := domain.Kind(r.URL.Query().Get("kind"))
	limit, exists, err := queryLimit(r.URL.Query()["limit"], service.SuggestLimitMax)
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

// handleCreate — POST /api/v1/components: создание; тело — запись
// наполнения (name/system + секции), класс — автодетект по обозначению
// (для system other создание адресуется PUT с классом в пути).
// Существующий (kind, designation) — 409 already_exists.
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
	if _, found, gerr := a.app.Services().Devices.Get(r.Context(), p.Kind, p.Designation); gerr != nil {
		a.writeErr(w, gerr)
		return
	} else if found {
		a.writeError(w, r, http.StatusConflict, domain.CodeAlreadyExists,
			domain.MsgApiAlreadyExists, p.Designation)
		return
	}
	outcome, err := a.app.Services().Devices.Upsert(r.Context(), in)
	if err != nil {
		a.writeDomainErr(w, r, err, false)
		return
	}
	if outcome == service.OutcomeSkipped {
		// Гонка создания между проверкой и применением: запись уже есть.
		a.writeError(w, r, http.StatusConflict, domain.CodeAlreadyExists,
			domain.MsgApiAlreadyExists, p.Designation)
		return
	}
	card, found, err := a.app.Services().Devices.Get(r.Context(), p.Kind, p.Designation)
	if err != nil || !found {
		a.writeErr(w, err)
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
	if err != nil || !found {
		a.writeErr(w, err)
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
func (a *API) readRecordBody(w *responseWriter, r *http.Request) (service.DeviceInput, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
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
