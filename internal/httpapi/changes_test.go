package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/service"
)

// Change feed: happy path, значения по умолчанию, курсор, невалидные
// after/limit, дубликаты и посторонние параметры — 400; пустой фид —
// пустой массив (не null). Тексты транспортных сообщений — контракт.
func TestChanges(t *testing.T) {
	_, srv := newTestAPI(t, Config{})

	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/changes", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp := decode[changesResponseJSON](t, body)
	// Сиды создали 5 записей — 5 событий created (дефолтные after=0/limit=100).
	if len(resp.Events) != 5 {
		t.Fatalf("событий: %d, ожидалось 5: %+v", len(resp.Events), resp.Events)
	}
	if resp.DataRevision == 0 {
		t.Fatalf("data_revision: %+v", resp)
	}
	ops := map[string]int{}
	for i, e := range resp.Events {
		if e.Op != "created" {
			t.Fatalf("операция сида: %+v", e)
		}
		if e.Kind == "" || e.Designation == "" || e.DeviceID == 0 || e.Revision == 0 {
			t.Fatalf("поля события: %+v", e)
		}
		if i > 0 && e.ID <= resp.Events[i-1].ID {
			t.Fatalf("порядок id не возрастает: %+v", resp.Events)
		}
		ops[e.Designation]++
	}
	if ops["КТ315Б"] != 1 || ops["BC547B"] != 1 {
		t.Fatalf("события сидов: %+v", resp.Events)
	}

	// Курсор: после последнего события — пустой массив.
	last := resp.Events[len(resp.Events)-1].ID
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/changes?after="+strconv.FormatInt(last, 10), "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	if tail := decode[changesResponseJSON](t, body); len(tail.Events) != 0 {
		t.Fatalf("хвост за концом: %+v", tail.Events)
	}
	// За концом фида по заведомо большому курсору — тоже пустой массив.
	status, body, _ = do(t, srv, http.MethodGet, apiPrefix+"/changes?after=999999", "")
	if status != http.StatusOK || !strings.Contains(body, `"events":[]`) {
		t.Fatalf("пустой фид: %d %s", status, body)
	}

	// limit ограничивает страницу.
	status, body, _ = do(t, srv, http.MethodGet, apiPrefix+"/changes?limit=2", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	if head := decode[changesResponseJSON](t, body); len(head.Events) != 2 {
		t.Fatalf("страница по лимиту: %+v", head.Events)
	}

	// Событие updated после PUT и deleted после DELETE.
	status, _, _ = do(t, srv, http.MethodPut,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"),
		`{"name":"КТ315Б","manufacturers":["Оникс"]}`)
	if status != http.StatusOK {
		t.Fatalf("put: %d", status)
	}
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/changes?after="+strconv.FormatInt(last, 10), "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	page := decode[changesResponseJSON](t, body)
	if len(page.Events) != 1 || page.Events[0].Op != "updated" ||
		page.Events[0].Designation != "КТ315Б" {
		t.Fatalf("событие updated: %+v", page.Events)
	}
	status, _, _ = do(t, srv, http.MethodDelete,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"), "")
	if status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/changes?after="+strconv.FormatInt(page.Events[0].ID, 10), "")
	page = decode[changesResponseJSON](t, body)
	if len(page.Events) != 1 || page.Events[0].Op != "deleted" ||
		page.Events[0].Designation != "КТ315Б" || page.Events[0].DeviceID == 0 {
		t.Fatalf("событие deleted: %+v", page.Events)
	}

	// Невалидные параметры — 400 с дословными текстами.
	cases := []struct {
		query   string
		message string
	}{
		{"after=-1", "parameter after: a non-negative integer is expected"},
		{"after=abc", "parameter after: a non-negative integer is expected"},
		{"after=1&after=2", "query parameter «after» is set more than once"},
		{"limit=0", "parameter limit: an integer from 1 to 1000 is expected"},
		{"limit=1001", "parameter limit: an integer from 1 to 1000 is expected"},
		{"limit=abc", "parameter limit: an integer from 1 to 1000 is expected"},
		{"limit=1&limit=2", "query parameter «limit» is set more than once"},
		{"bogus=1", "unknown query parameter «bogus»"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/changes?"+tc.query, "")
			wantError(t, status, eb, http.StatusBadRequest, "validation_failed", tc.message)
		})
	}
}

// Пустой фид (база без записей) — events: [], не null.
func TestChangesEmptyFeed(t *testing.T) {
	app, err := service.Open(context.Background(), service.Config{
		Dialect: "sqlite", DSN: ":memory:", EnsureCreated: true,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { app.Close() }) //nolint:errcheck — тестовая база
	api := New(app, Config{})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/changes", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	if !strings.Contains(body, `"events":[]`) {
		t.Fatalf("пустой фид — не пустой массив: %s", body)
	}
	if resp := decode[changesResponseJSON](t, body); resp.DataRevision != 1 {
		t.Fatalf("data_revision пустой базы: %+v", resp)
	}
}
