package httpapi

import (
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

// Потоковый импорт NDJSON (features.log CC-6): синхронный отчёт после
// EOF (включая прогоны с проблемами), идемпотентность повторного запроса,
// жёсткие ошибки — 400 invalid_import_file, посторонний параметр запроса —
// 400. Тексты транспортных сообщений — контракт.

func TestImportReport(t *testing.T) {
	_, srv := newTestAPI(t, Config{})

	// Пустой поток — пустой отчёт (issues — массив, не null).
	status, body, _ := do(t, srv, http.MethodPost, apiPrefix+"/import", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	if !strings.Contains(body, `"records":0`) || !strings.Contains(body, `"issues":[]`) {
		t.Fatalf("пустой отчёт: %s", body)
	}

	// Импорт двух записей: счётчики исходов, применённые записи видны
	// поиском.
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/import",
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
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/import",
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
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/import?dry_run=true",
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
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/import?dry_run=false",
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
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/import",
		"{\"transistors\": {\"name\": \"ГТ109Г\"}}\n"+
			"{\"transistors\": {\"name\": \"МП39\", \"system\": \"other\", \"parameters\": [{\"parameter\": \"hfe\", \"max\": 50}]}}\n")
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
	if !strings.Contains(iss.Message, "hfe") {
		t.Fatalf("текст проблемы: %q", iss.Message)
	}

	// Блок catalog строками до записей — каталог применяется (запись
	// уже импортирована выше — Skipped).
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/import",
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
	status, _, eb := do(t, srv, http.MethodPost, apiPrefix+"/import", "{\"transistors\":\n")
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
			status, _, eb := do(t, srv, http.MethodPost, apiPrefix+"/import?"+tc.query, "{}\n")
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
		map[string]string{"Accept-Language": "ru"})
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
