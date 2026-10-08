// Сценарии REST: qa/restsrv (минимальный хост /api/v1) поднимается на
// свободном порту ноги прогона, проверки идут через net/http — контракт
// статусов, коды ошибок, ETag/304, CRUD без внешних инструментов.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// restSuite прогоняет сценарии REST на ноге (база — та же, что у CLI-ноги).
func restSuite(ctx context.Context, r *report, srv, tmp string, l leg) {
	r.section("phase: rest " + l.name)

	port, err := freePort()
	if err != nil {
		r.check("rest/ready", "restsrv: свободный порт", false, err.Error())
		return
	}
	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)
	logPath := filepath.Join(tmp, "restsrv-"+l.name+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		r.check("rest/ready", "restsrv: лог хоста", false, err.Error())
		return
	}
	defer logFile.Close() //nolint:errcheck — лог прогона живёт до конца ноги

	srvCmd := exec.Command(srv, append([]string{fmt.Sprintf("--addr=127.0.0.1:%d", port)}, l.dbArgs...)...)
	srvCmd.Stderr = logFile
	if err := srvCmd.Start(); err != nil {
		r.check("rest/ready", "restsrv: запуск хоста", false, err.Error())
		return
	}
	stopped := make(chan struct{})
	go func() { _ = srvCmd.Wait(); close(stopped) }()
	defer func() {
		_ = srvCmd.Process.Kill()
		<-stopped
	}()

	client := &http.Client{Timeout: requestTimeout}
	cl := restClient{base: base, client: client}

	// Готовность: /kinds отвечает 200 (до 20 с — холодный старт postgres);
	// успешный запрос выходит сразу, ранний выход — отмена прогона или
	// падение хоста.
	ready := false
	deadline := time.Now().Add(20 * time.Second)
	for !ready && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
		case <-stopped:
		case <-time.After(100 * time.Millisecond):
			if status, _, _ := cl.do(ctx, http.MethodGet, "/kinds", "", nil); status == http.StatusOK {
				ready = true
			}
			continue
		}
		break
	}
	if !r.check("rest/ready", "restsrv: готовность", ready, tailFile(logPath, 15)...) {
		return
	}

	// GET /kinds: 200, классы.
	status, body, _ := cl.do(ctx, http.MethodGet, "/kinds", "", nil)
	r.check("rest/kinds", "GET /kinds: 200, классы",
		status == http.StatusOK && strings.Contains(body, "transistor"),
		detail(status, body)...)

	// GET /catalog: 200, снимок каталога; ETag → 304.
	status, body, hdr := cl.do(ctx, http.MethodGet, "/catalog", "", nil)
	r.check("rest/catalog", "GET /catalog: 200, снимок каталога",
		status == http.StatusOK && strings.Contains(body, `"parameters"`),
		detail(status, body)...)
	etag := hdr.Get("Etag")
	status, _, _ = cl.do(ctx, http.MethodGet, "/catalog", "", map[string]string{"If-None-Match": etag})
	r.check("rest/catalog-etag", "GET /catalog: ETag → 304", status == http.StatusNotModified,
		detail(status, "")...)

	// GET /stats: 200, ревизии.
	status, body, _ = cl.do(ctx, http.MethodGet, "/stats", "", nil)
	r.check("rest/stats", "GET /stats: 200, ревизии",
		status == http.StatusOK && strings.Contains(body, "data_revision"),
		detail(status, body)...)

	// GET /components: поиск с фильтрами и ETag → 304.
	status, body, _ = cl.do(ctx, http.MethodGet,
		"/components?kind=capacitor&"+url.Values{"q": {"К50"}}.Encode(), "", nil)
	r.check("rest/components-search", "GET /components: поиск с фильтрами",
		status == http.StatusOK && strings.Contains(body, "К50-35"),
		detail(status, body)...)
	_, _, hdr = cl.do(ctx, http.MethodGet, "/components?kind=capacitor", "", nil)
	status, _, _ = cl.do(ctx, http.MethodGet, "/components?kind=capacitor", "", map[string]string{"If-None-Match": hdr.Get("Etag")})
	r.check("rest/components-etag", "GET /components: ETag → 304",
		status == http.StatusNotModified, detail(status, "")...)

	// GET /components: параметрический фильтр и негативные случаи
	// (400 unknown_parameter / validation_failed).
	status, body, _ = cl.do(ctx, http.MethodGet, "/components?kind=transistor&par.h21e.min=50", "", nil)
	r.check("rest/components-parfilter", "GET /components: параметрический фильтр",
		status == http.StatusOK, detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodGet, "/components?kind=transistor&par.bogus.min=1", "", nil)
	r.check("rest/components-unknown-par", "GET /components: неизвестный параметр — 400, код unknown_parameter",
		status == http.StatusBadRequest && strings.Contains(body, "unknown_parameter"),
		detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodGet, "/components?kind=transistor&bogus=1", "", nil)
	r.check("rest/components-unknown-key", "GET /components: неизвестный ключ запроса — 400, код validation_failed",
		status == http.StatusBadRequest && strings.Contains(body, "validation_failed"),
		detail(status, body)...)

	// Карточка по ключу (с исполнениями) и по стабильному id.
	status, body, _ = cl.do(ctx, http.MethodGet,
		"/components/capacitor/"+url.PathEscape("К50-35"), "", nil)
	r.check("rest/card", "GET /components/{kind}/{designation}: карточка с исполнениями",
		status == http.StatusOK && strings.Contains(body, `"variants"`),
		detail(status, body)...)
	id := searchFirstID(ctx, cl)
	if id == "" {
		r.check("rest/card-by-id", "GET /components/id/{id}: карточка по стабильному id",
			false, "не найден id записи поиска")
	} else {
		status, body, _ = cl.do(ctx, http.MethodGet, "/components/id/"+id, "", nil)
		r.check("rest/card-by-id", "GET /components/id/{id}: карточка по стабильному id",
			status == http.StatusOK, detail(status, body)...)
	}

	// CRUD: создание 201 Added, повтор 409 already_exists, PUT секций 200,
	// несовпадение обозначения 422, DELETE 204, после удаления 404.
	cl.do(ctx, http.MethodDelete, "/components/transistor/2N9013", "", nil) // остаток прошлого прогона
	status, body, _ = cl.do(ctx, http.MethodPost, "/components", `{"name":"2N9013"}`, nil)
	r.check("rest/post-create", "POST /components: создание — 201 Added",
		status == http.StatusCreated && strings.Contains(body, "Added"),
		detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodPost, "/components", `{"name":"2N9013"}`, nil)
	r.check("rest/post-duplicate", "POST /components: повторное создание — 409 already_exists",
		status == http.StatusConflict && strings.Contains(body, "already_exists"),
		detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodPut, "/components/transistor/2N9013",
		`{"name":"2N9013","attributes":{"package":"TO-18"}}`, nil)
	r.check("rest/put-sections", "PUT /components: семантика секций — 200, секция применена",
		status == http.StatusOK && strings.Contains(body, "TO-18"),
		detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodPut, "/components/transistor/2N9013",
		`{"name":"2N9014"}`, nil)
	r.check("rest/put-mismatch", "PUT /components: несовпадение обозначения — 422 designation_mismatch",
		status == http.StatusUnprocessableEntity && strings.Contains(body, "designation_mismatch"),
		detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodDelete, "/components/transistor/2N9013", "", nil)
	r.check("rest/delete", "DELETE /components: удаление — 204",
		status == http.StatusNoContent, detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodGet, "/components/transistor/2N9013", "", nil)
	r.check("rest/get-after-delete", "GET после DELETE — 404",
		status == http.StatusNotFound, detail(status, body)...)

	// suggest по префиксу и неизвестный маршрут.
	status, body, _ = cl.do(ctx, http.MethodGet,
		"/suggest?"+url.Values{"q": {"КТ3"}, "kind": {"transistor"}}.Encode(), "", nil)
	r.check("rest/suggest", "GET /suggest: автодополнение по префиксу",
		status == http.StatusOK && strings.Contains(body, "designation"),
		detail(status, body)...)
	status, body, _ = cl.do(ctx, http.MethodGet, "/bogus", "", nil)
	r.check("rest/route-404", "неизвестный маршрут — 404",
		status == http.StatusNotFound, detail(status, body)...)
}

// jsonCT — Content-Type JSON-тел запросов.
const jsonCT = "application/json"

// restClient — HTTP-клиент ноги REST.
type restClient struct {
	base   string
	client *http.Client
}

// do выполняет запрос к пути path (с запросом) и возвращает статус, тело
// и заголовки ответа.
func (c restClient) do(ctx context.Context, method, path, body string, headers map[string]string) (int, string, http.Header) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequestWithContext(reqCtx, method, c.base+path, reader)
	if err != nil {
		return 0, "запрос: " + err.Error(), nil
	}
	if body != "" {
		req.Header.Set("Content-Type", jsonCT)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, "запрос: " + err.Error(), nil
	}
	defer resp.Body.Close() //nolint:errcheck — тело читается целиком
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "чтение тела: " + err.Error(), resp.Header
	}
	return resp.StatusCode, string(data), resp.Header
}

// searchFirstID — стабильный id первой записи поиска транзисторов.
func searchFirstID(ctx context.Context, c restClient) string {
	status, body, _ := c.do(ctx, http.MethodGet, "/components?kind=transistor&limit=1", "", nil)
	if status != http.StatusOK {
		return ""
	}
	var page struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil || len(page.Items) == 0 {
		return ""
	}
	return fmt.Sprintf("%d", page.Items[0].ID)
}

// detail — детали HTTP-проверки: статус и фрагмент тела; обрезка — по границе
// рун (обозначения в телах кириллические, байтовая обрезка порвала бы символ).
func detail(status int, body string) []string {
	d := fmt.Sprintf("status=%d", status)
	if rs := []rune(body); len(rs) > 300 {
		body = string(rs[:300]) + "…"
	}
	if body = strings.TrimSpace(body); body != "" {
		d += ", body: " + body
	}
	return []string{d}
}

// tailFile — последние n непустых строк файла (лог restsrv при отказе).
func tailFile(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := trimSpaceLines(string(data))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
