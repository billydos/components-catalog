package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/importer"
)

// Импорт файла наполнения: NDJSON — потоковый, jsonc/yaml — документ
// целиком; формат — обязательный заголовок Content-Type. Синхронный отчёт
// после EOF (включая прогоны с проблемами), идемпотентность повторного
// запроса, жёсткие ошибки — 400 invalid_import_file, посторонний параметр
// запроса — 400. Тексты транспортных сообщений — контракт.

// ndjsonCT — Content-Type потока NDJSON тела POST /import.
const ndjsonCT = "application/x-ndjson"

// doImport — POST /import с явным Content-Type (формат тела — заголовок;
// contentType "" — запрос без заголовка); возвращает статус, тело и
// декодированную ошибку.
func doImport(t *testing.T, srv *httptest.Server, query, contentType, body string) (int, string, errorBody) {
	t.Helper()
	path := apiPrefix + "/import"
	if query != "" {
		path += "?" + query
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("запрос POST %s: %v", path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("выполнение POST %s: %v", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck — тестовый ответ
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("чтение ответа POST %s: %v", path, err)
	}
	var eb errorBody
	if resp.StatusCode >= 400 {
		if err := json.Unmarshal(data, &eb); err != nil {
			t.Fatalf("декодирование ошибки POST %s: %v (тело %q)", path, err, data)
		}
	}
	return resp.StatusCode, string(data), eb
}

func TestImportReport(t *testing.T) {
	_, srv := newTestAPI(t, Config{})

	// Пустой поток — пустой отчёт (issues — массив, не null).
	status, body, _ := doImport(t, srv, "", ndjsonCT, "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	if !strings.Contains(body, `"records":0`) || !strings.Contains(body, `"issues":[]`) {
		t.Fatalf("пустой отчёт: %s", body)
	}

	// Импорт двух записей: счётчики исходов, применённые записи видны
	// поиском.
	status, body, _ = doImport(t, srv, "", ndjsonCT,
		"{\"transistors\": {\"name\": \"КТ361Б\", \"attributes\": {\"structure\": \"pnp\"}}}\n"+
			"{\"transistors\": {\"name\": \"BC548B\"}}\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep := decode[importReportJSON](t, body)
	if rep.Records != 2 || rep.Added != 2 || rep.Skipped != 0 || rep.Rejected != 0 ||
		rep.CatalogApplied || len(rep.Issues) != 0 {
		t.Fatalf("отчёт: %+v", rep)
	}
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/components?kind=transistor&q="+esc("КТ361"), "")
	if status != http.StatusOK || !strings.Contains(body, "КТ361Б") {
		t.Fatalf("поиск после импорта: %d %s", status, body)
	}

	// Повторный импорт того же тела — Skipped по всем записям.
	status, body, _ = doImport(t, srv, "", ndjsonCT,
		"{\"transistors\": {\"name\": \"КТ361Б\", \"attributes\": {\"structure\": \"pnp\"}}}\n"+
			"{\"transistors\": {\"name\": \"BC548B\"}}\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep = decode[importReportJSON](t, body)
	if rep.Records != 2 || rep.Skipped != 2 || rep.Added != 0 {
		t.Fatalf("повторный отчёт: %+v", rep)
	}

	// dry_run=true: контрольный прогон нового тела — исходы считаются,
	// записи в БД нет (поиск не видит); отчёт помечен dry_run.
	status, body, _ = doImport(t, srv, "dry_run=true", ndjsonCT,
		"{\"transistors\": {\"name\": \"ГТ115Г\"}}\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep = decode[importReportJSON](t, body)
	if !rep.DryRun || rep.Records != 1 || rep.Added != 1 {
		t.Fatalf("dry-run отчёт: %+v", rep)
	}
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/components?kind=transistor&q="+esc("ГТ115"), "")
	if status != http.StatusOK || strings.Contains(body, "ГТ115Г") {
		t.Fatalf("dry-run записал в базу: %d %s", status, body)
	}
	// dry_run=false — явное применение (запись появляется).
	status, body, _ = doImport(t, srv, "dry_run=false", ndjsonCT,
		"{\"transistors\": {\"name\": \"ГТ115Г\"}}\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep = decode[importReportJSON](t, body)
	if rep.DryRun || rep.Added != 1 {
		t.Fatalf("отчёт dry_run=false: %+v", rep)
	}
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/components?kind=transistor&q="+esc("ГТ115"), "")
	if status != http.StatusOK || !strings.Contains(body, "ГТ115Г") {
		t.Fatalf("применение после dry_run=false: %d %s", status, body)
	}

	// Проблемная запись — 200 с отчётом: корректная запись применена,
	// ошибочная — нет; проблема несёт код, позицию и текст (en — каноник).
	// hfe без границ при полном наборе условий — изолированная ошибка
	// формы значения (limits), без наложения на условия измерения.
	status, body, _ = doImport(t, srv, "", ndjsonCT,
		"{\"transistors\": {\"name\": \"ГТ109Г\"}}\n"+
			"{\"transistors\": {\"name\": \"МП39\", \"system\": \"other\", \"parameters\": [{\"parameter\": \"hfe\", \"Vce\": 5, \"Ic\": 5}]}}\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep = decode[importReportJSON](t, body)
	if rep.Records != 2 || rep.Added != 1 || rep.Rejected != 1 || len(rep.Issues) != 1 {
		t.Fatalf("отчёт с проблемой: %+v", rep)
	}
	iss := rep.Issues[0]
	if iss.Record != "МП39" || iss.Line != 2 || iss.Code != "validation_failed" {
		t.Fatalf("проблема: %+v", iss)
	}
	if !strings.Contains(iss.Message, "value type limits requires at least one of keys min and max") {
		t.Fatalf("текст проблемы: %q", iss.Message)
	}

	// Блок catalog строками до записей — каталог применяется (запись
	// уже импортирована выше — Skipped).
	status, body, _ = doImport(t, srv, "", ndjsonCT,
		"{\"catalog\": {\"attributes\": [{\"code\": \"leadFree\", \"type\": \"bool\"}]}}\n"+
			"{\"transistors\": {\"name\": \"ГТ109Г\"}}\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep = decode[importReportJSON](t, body)
	if !rep.CatalogApplied || rep.Skipped != 1 {
		t.Fatalf("отчёт с каталогом: %+v", rep)
	}
}

func TestImportHardErrors(t *testing.T) {
	_, srv := newTestAPI(t, Config{})

	// Синтаксис строки — жёсткая ошибка прогона: 400 invalid_import_file,
	// текст с номером строки (en — канонический).
	status, _, eb := doImport(t, srv, "", ndjsonCT, "{\"transistors\":\n")
	wantError(t, status, eb, http.StatusBadRequest, "invalid_import_file",
		"line 1: EOF")

	// Посторонний параметр запроса — 400; значение dry_run строго
	// true|false (опечатки не молчат); повтор — 400.
	cases := []struct {
		query   string
		message string
	}{
		{"bogus=1", "unknown query parameter «bogus»"},
		{"dry_run=1", "parameter «dry_run»: true or false is expected"},
		{"dry_run=ture", "parameter «dry_run»: true or false is expected"},
		{"dry_run=", "parameter «dry_run»: true or false is expected"},
		{"dry_run=true&dry_run=false", "query parameter «dry_run» is set more than once"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			status, _, eb := doImport(t, srv, tc.query, ndjsonCT, "{}\n")
			wantError(t, status, eb, http.StatusBadRequest, "validation_failed", tc.message)
		})
	}

	// Метод не поддерживается путём: 405 + Allow: POST.
	resp := rawRequest(t, srv, http.MethodGet, apiPrefix+"/import", nil, nil)
	defer resp.Body.Close() //nolint:errcheck — тестовый ответ
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != http.MethodPost {
		t.Fatalf("GET /import: статус %d, Allow %q", resp.StatusCode, resp.Header.Get("Allow"))
	}
}

// Обрыв тела /import — ошибка чтения потока: та же модель ошибок, что
// синтаксис строки (400 invalid_import_file с номером отказной строки);
// осознанное отличие от POST /components, где обрыв тела — api_body_read.
// Хендлер вызывается напрямую (как TestReadRecordBodyReadErrors): по
// живому соединению сервер закрывает его до ответа при ошибке чтения тела.
func TestImportBodyReadAbort(t *testing.T) {
	app, _ := newTestAPI(t, Config{})
	api := New(app, Config{})
	req := httptest.NewRequest(http.MethodPost, apiPrefix+"/import",
		&errReader{data: "{\"transistors\": {\"name\": \"КТ315Б\"}}\n"})
	req.Header.Set("Content-Type", ndjsonCT)
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	api.handleImport(rw, req, nil)
	var eb errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &eb); err != nil {
		t.Fatalf("декодирование ошибки: %v (тело %q)", err, rec.Body.String())
	}
	wantError(t, rec.Code, eb, http.StatusBadRequest, "invalid_import_file",
		"line 2: обрыв соединения")
}

// ru-локаль: сообщения проблем отчёта — из ru-бандла (D9).
func TestImportIssuesLocalized(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	resp := rawRequest(t, srv, http.MethodPost, apiPrefix+"/import",
		strings.NewReader("{\"transistors\": {\"name\": \"\"}}\n"),
		map[string]string{"Accept-Language": "ru", "Content-Type": ndjsonCT})
	defer resp.Body.Close() //nolint:errcheck — тестовый ответ
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус: %d", resp.StatusCode)
	}
	rep := decode[importReportJSON](t, readAll(t, resp.Body))
	if len(rep.Issues) != 1 || rep.Issues[0].Message != "пустое обозначение записи" {
		t.Fatalf("проблема ru: %+v", rep.Issues)
	}
}

// Инфраструктурные проблемы прогона (не-доменные ошибки применения)
// наружу — санитизированным фиксированным текстом internal_error,
// без технических деталей; доменные проблемы проходят как есть.
func TestImportReportSanitizesInternalIssues(t *testing.T) {
	rep := importer.Report{
		Records:  2,
		Rejected: 2,
		Issues: []importer.Issue{
			{
				Record:  "КТ315Б",
				Line:    3,
				Code:    domain.CodeInvalidImportFile,
				MsgID:   domain.MsgInternalError,
				Args:    []any{"pq: connection refused"},
				Message: "internal error: pq: connection refused",
			},
			{
				Record:  "МП39",
				Line:    4,
				Code:    domain.CodeValidationFailed,
				MsgID:   domain.MsgImportRecordEmptyName,
				Message: "empty record designation",
			},
		},
	}
	out := importReportToJSON(i18n.Language("en"), &rep)
	if len(out.Issues) != 2 {
		t.Fatalf("проблемы: %+v", out.Issues)
	}
	san := out.Issues[0]
	if san.Code != string(domain.CodeInternal) ||
		san.Message != "internal request processing error" {
		t.Fatalf("санитизированная проблема: %+v", san)
	}
	if strings.Contains(san.Message, "pq:") || san.Record != "КТ315Б" || san.Line != 3 {
		t.Fatalf("утечка деталей или позиции: %+v", san)
	}
	if dom := out.Issues[1]; dom.Code != "validation_failed" ||
		dom.Message != "empty record designation" {
		t.Fatalf("доменная проблема: %+v", dom)
	}
}

// Согласование формата тела POST /import по Content-Type (план 04 §2):
// три формата реестра — ndjson (поток), jsonc (документ с комментариями)
// и yaml; параметры медиатипа (charset) игнорируются; отсутствие
// и неподдерживаемый заголовок — 400 validation_failed. Тексты — контракт.
func TestImportContentTypes(t *testing.T) {
	_, srv := newTestAPI(t, Config{})

	cases := []struct {
		name        string
		contentType string
		body        string
	}{
		{"ndjson", "application/x-ndjson", "{\"transistors\": {\"name\": \"КТ361Б\"}}\n"},
		{"jsonc с комментариями", "application/json",
			"// наполнение\n{\n  \"transistors\": [\n    {\"name\": \"ГТ115Г\"}\n  ]\n}\n"},
		{"yaml", "application/yaml", "transistors:\n  - name: BC548A\n"},
		{"charset игнорируется", "application/x-ndjson; charset=utf-8",
			"{\"transistors\": {\"name\": \"ГТ109Г\"}}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body, _ := doImport(t, srv, "", tc.contentType, tc.body)
			if status != http.StatusOK {
				t.Fatalf("статус: %d, тело %s", status, body)
			}
			rep := decode[importReportJSON](t, body)
			if rep.Records != 1 || rep.Added != 1 || len(rep.Issues) != 0 {
				t.Fatalf("отчёт: %+v", rep)
			}
		})
	}

	// Заголовок отсутствует — умолчания нет: 400 (en — канонический).
	status, _, eb := doImport(t, srv, "", "", "{}\n")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed",
		"Content-Type header is required: application/x-ndjson, application/json or application/yaml")

	// Неподдерживаемый медиатип — 400, аргумент — разобранный тип.
	status, _, eb = doImport(t, srv, "", "text/plain; charset=utf-8", "x")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed",
		"unsupported Content-Type «text/plain»: application/x-ndjson, application/json or application/yaml is expected")

	// Синтаксически битый заголовок — 400, аргумент — исходное значение.
	status, _, eb = doImport(t, srv, "", "application/", "x")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed",
		"unsupported Content-Type «application/»: application/x-ndjson, application/json or application/yaml is expected")
}

// Документные форматы — тот же конвейер: Issues-модель с позицией
// записи в секции (no, не строка потока), dry_run без записи в БД.
func TestImportDocumentReport(t *testing.T) {
	_, srv := newTestAPI(t, Config{})

	status, body, _ := doImport(t, srv, "", "application/json",
		"{\"transistors\": [{\"name\": \"ГТ109Г\"}, {\"name\": \"\"}]}")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep := decode[importReportJSON](t, body)
	if rep.Records != 2 || rep.Added != 1 || rep.Rejected != 1 || len(rep.Issues) != 1 {
		t.Fatalf("отчёт: %+v", rep)
	}
	iss := rep.Issues[0]
	if iss.Record != "" || iss.Line != 0 || iss.No != 2 || iss.Code != "invalid_import_file" {
		t.Fatalf("проблема: %+v", iss)
	}

	// dry_run документа — контрольный прогон без записи (yaml).
	status, body, _ = doImport(t, srv, "dry_run=true", "application/yaml",
		"transistors:\n  - name: ГТ115Г\n")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	rep = decode[importReportJSON](t, body)
	if !rep.DryRun || rep.Added != 1 {
		t.Fatalf("dry-run отчёт документа: %+v", rep)
	}
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/components?kind=transistor&q="+esc("ГТ115"), "")
	if status != http.StatusOK || strings.Contains(body, "ГТ115Г") {
		t.Fatalf("dry-run записал в базу: %d %s", status, body)
	}

	// Синтаксис документа — жёсткая ошибка: 400 invalid_import_file.
	status, _, eb := doImport(t, srv, "", "application/json", "{\"transistors\": [")
	wantError(t, status, eb, http.StatusBadRequest, "invalid_import_file",
		"the file is not valid JSONC: hujson: line 1, column 18: parsing value: unexpected EOF")
}

// Документное тело сверх потолка — 400 invalid_import_file с потолком
// в тексте (та же модель ошибок, что у строки NDJSON сверх лимита
// сканера). Хендлер вызывается напрямую (как TestImportBodyReadAbort):
// по живому соединению сервер закрывает соединение до ответа (сигнал
// requestTooLarge MaxBytesReader).
func TestImportBodyTooLarge(t *testing.T) {
	app, _ := newTestAPI(t, Config{})
	api := New(app, Config{})
	req := httptest.NewRequest(http.MethodPost, apiPrefix+"/import",
		bytes.NewReader(make([]byte, maxImportBodyBytes+1)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	api.handleImport(rw, req, nil)
	var eb errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &eb); err != nil {
		t.Fatalf("декодирование ошибки: %v (тело %q)", err, rec.Body.String())
	}
	wantError(t, rec.Code, eb, http.StatusBadRequest, "invalid_import_file",
		"import request body exceeds the allowed size of 16 MiB; use application/x-ndjson for larger files")
}

// rawRequest — запрос с нестандартным телом/заголовками (за пределами
// помощника do): r может быть io.Reader или nil; hdr — Accept-Language.
func rawRequest(t *testing.T, srv *httptest.Server, method, path string,
	body io.Reader, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatalf("запрос %s %s: %v", method, path, err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("выполнение %s %s: %v", method, path, err)
	}
	return resp
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("чтение тела: %v", err)
	}
	return string(data)
}
