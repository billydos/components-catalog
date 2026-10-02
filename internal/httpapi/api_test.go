package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/service"
	"github.com/goccy/go-yaml"
)

// Тесты хендлеров /api/v1 через httptest (план работ 5.3): статусы, тела,
// коды ошибок, ETag; дословные тексты транспортных сообщений — контракт.
// Маршруты сверяются со спецификацией api/openapi.yaml (критерий этапа 5).

// collector — реализация Metrics для проверки хука.
type collector struct {
	entries []string
}

func (c *collector) Observe(op string, status int, errCode string, duration time.Duration) {
	c.entries = append(c.entries, fmt.Sprintf("%s %d %s", op, status, errCode))
}

// newTestAPI открывает приложение (sqlite в памяти, сиды каталога),
// наполняет базовыми записями и поднимает тестовый сервер (журнал —
// в никуда: тексты журнала проверяются отдельными тестами хука метрик).
func newTestAPI(t *testing.T, cfg Config) (*service.App, *httptest.Server) {
	t.Helper()
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	app, err := service.Open(context.Background(), service.Config{
		Dialect: "sqlite", DSN: ":memory:", EnsureCreated: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { app.Close() }) //nolint:errcheck — тестовая база
	seedDevices(t, app)
	api := New(app, cfg)
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return app, srv
}

func ptr[T any](v T) *T { return &v }

func seedDevices(t *testing.T, app *service.App) {
	t.Helper()
	upsert := func(in service.DeviceInput) {
		t.Helper()
		if _, err := app.Services().Devices.Upsert(context.Background(), in); err != nil {
			t.Fatalf("upsert %q: %v", in.Name, err)
		}
	}
	upsert(service.DeviceInput{
		Name: "BC547B",
		Attributes: []catalog.AttributeValue{
			{Attribute: "structure", Text: ptr("npn")},
		},
		Sections: []service.SectionInput{
			{Section: "parameters", Values: []catalog.ParameterValue{
				{Parameter: "h21e", Min: ptr(200.0), Max: ptr(450.0), Conditions: []catalog.ConditionValue{
					{Condition: "Uke", Value: 5}, {Condition: "Ik", Value: 2},
				}},
			}},
		},
	})
	upsert(service.DeviceInput{
		Name: "КТ315Б",
		Attributes: []catalog.AttributeValue{
			{Attribute: "structure", Text: ptr("npn")},
			{Attribute: "yearFrom", Num: ptr(1967.0)},
		},
		Sections: []service.SectionInput{
			{Section: "parameters", Values: []catalog.ParameterValue{
				{Parameter: "h21e", Min: ptr(50.0), Max: ptr(350.0), Conditions: []catalog.ConditionValue{
					{Condition: "Uke", Value: 10}, {Condition: "Ik", Value: 1},
				}},
			}},
			{Section: "ratings", Values: []catalog.ParameterValue{
				{Parameter: "UkeoMax", Exact: ptr(20.0)},
				{Parameter: "TempMin", Exact: ptr(-60.0)},
			}},
		},
		Manufacturers: &[]string{"Восход", "Терма"},
		Analogs:       &[]service.AnalogInput{{Designation: "BC547B"}},
	})
	upsert(service.DeviceInput{
		Name: "МЛТ-0.5",
		Sections: []service.SectionInput{
			{Section: "parameters", Values: []catalog.ParameterValue{
				{Parameter: "Rnom", Min: ptr(1.0), Max: ptr(5100000.0)},
			}},
			{Section: "ratings", Values: []catalog.ParameterValue{
				{Parameter: "Pnom", Exact: ptr(0.5)},
			}},
		},
	})
	upsert(service.DeviceInput{
		Name: "К50-35",
		Attributes: []catalog.AttributeValue{
			{Attribute: "polarized", Bool: ptr(true)},
		},
		Variants: &[]service.VariantInput{
			{Label: "160 В", Sections: []service.SectionInput{
				{Section: "parameters", Values: []catalog.ParameterValue{
					{Parameter: "Unom", Exact: ptr(160.0)},
					{Parameter: "Cnom", Min: ptr(1000000.0), Max: ptr(10000000.0)},
				}},
				{Section: "dimensions", Values: []catalog.ParameterValue{
					{Parameter: "diameter", Exact: ptr(8.0)},
					{Parameter: "leadLength", Exact: ptr(12.0)},
				}},
			}},
			{Label: "25 В", Sections: []service.SectionInput{
				{Section: "parameters", Values: []catalog.ParameterValue{
					{Parameter: "Unom", Exact: ptr(25.0)},
					{Parameter: "Cnom", Min: ptr(47000000.0), Max: ptr(4700000000.0)},
				}},
				{Section: "dimensions", Values: []catalog.ParameterValue{
					{Parameter: "diameter", Exact: ptr(10.0)},
					{Parameter: "leadLength", Exact: ptr(16.0)},
				}},
			}},
		},
	})
}

// do — запрос к серверу; возвращает статус, тело и декодированную ошибку.
func do(t *testing.T, srv *httptest.Server, method, path, body string) (int, string, errorBody) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("запрос %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("выполнение %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("чтение ответа %s %s: %v", method, path, err)
	}
	var eb errorBody
	if resp.StatusCode >= 400 {
		if err := json.Unmarshal(data, &eb); err != nil {
			t.Fatalf("декодирование ошибки %s %s: %v (тело %q)", method, path, err, data)
		}
	}
	return resp.StatusCode, string(data), eb
}

func decode[T any](t *testing.T, body string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("декодирование %T: %v (тело %q)", v, err, body)
	}
	return v
}

// esc — URL-кодирование сегмента пути (обозначения с «/» прямым путём
// не адресуются).
func esc(s string) string { return url.PathEscape(s) }

func wantError(t *testing.T, status int, eb errorBody, wantStatus int, code, message string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("статус: %d, ожидался %d (тело %+v)", status, wantStatus, eb)
	}
	if eb.Code != code {
		t.Fatalf("код: %q, ожидался %q", eb.Code, code)
	}
	if eb.Message != message {
		t.Fatalf("текст: %q, ожидался %q", eb.Message, message)
	}
}

func TestKinds(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/kinds", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	resp := decode[kindsResponseJSON](t, body)
	codes := make([]string, 0, len(resp.Kinds))
	for _, k := range resp.Kinds {
		codes = append(codes, k.Code)
	}
	if !slices.Equal(codes, []string{"capacitor", "diode", "resistor", "transistor"}) {
		t.Fatalf("классы: %+v", resp.Kinds)
	}
}

func TestCatalogAndETag(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	req, _ := http.NewRequest(http.MethodGet, srv.URL+apiPrefix+"/catalog", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус: %d", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("ETag отсутствует")
	}
	snap := decode[catalogSnapshotJSON](t, string(data))
	if snap.Revision == 0 || len(snap.Parameters) == 0 || len(snap.Systems) == 0 {
		t.Fatalf("снимок каталога пуст: revision=%d params=%d systems=%d",
			snap.Revision, len(snap.Parameters), len(snap.Systems))
	}
	if snap.SeriesFamilies[0].Series == "" || snap.SeriesFamilies[0].TailSemantic != "" &&
		snap.SeriesFamilies[0].TailSemantic != "power" {
		t.Fatalf("series_families: %+v", snap.SeriesFamilies[0])
	}

	// If-None-Match → 304 с тем же ETag.
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+apiPrefix+"/catalog", nil)
	req2.Header.Set("If-None-Match", etag)
	resp2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("статус 304 ожидался, получен %d", resp2.StatusCode)
	}
	if resp2.Header.Get("ETag") != etag {
		t.Fatalf("ETag 304: %q, ожидался %q", resp2.Header.Get("ETag"), etag)
	}
}

func TestStats(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/stats", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	st := decode[statsJSON](t, body)
	if st.SchemaVersion != 1 || st.Total != 4 || st.Kinds["transistor"] != 2 || st.Kinds["capacitor"] != 1 {
		t.Fatalf("статистика: %+v", st)
	}
	if st.CatalogRevision == 0 || st.DataRevision == 0 {
		t.Fatalf("ревизии: %+v", st)
	}
}

func TestSearch(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	cases := []struct {
		name   string
		query  string
		want   []string
		hasETa bool
	}{
		{"все", "", []string{"К50-35", "МЛТ-0.5", "BC547B", "КТ315Б"}, true},
		{"класс", "kind=transistor", []string{"BC547B", "КТ315Б"}, false},
		{"система", "system=pro", []string{"BC547B"}, false},
		{"подстрока", "q=315", []string{"КТ315Б"}, false},
		{"поле обозначения", "material=%D0%BA%D1%80%D0%B5%D0%BC%D0%BD%D0%B8%D0%B9", []string{"BC547B", "КТ315Б"}, false},
		{"параметр", "par.h21e.min=150", []string{"BC547B"}, false},
		{"точный параметр", "par.Pnom.exact=0.5", []string{"МЛТ-0.5"}, false},
		{"атрибут", "attr.structure=npn", []string{"BC547B", "КТ315Б"}, false},
		{"атрибут число", "attr.yearFrom=1967", []string{"КТ315Б"}, false},
		{"сортировка", "kind=transistor&sort=-designation", []string{"КТ315Б", "BC547B"}, false},
		{"пагинация", "sort=designation&limit=2&offset=2", []string{"КТ315Б", "МЛТ-0.5"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := apiPrefix + "/components"
			if tc.query != "" {
				path += "?" + tc.query
			}
			req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			data, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("статус: %d, тело %s", resp.StatusCode, data)
			}
			if resp.Header.Get("ETag") == "" {
				t.Fatal("ETag отсутствует")
			}
			page := decode[searchPageJSON](t, string(data))
			got := make([]string, 0, len(page.Items))
			for _, it := range page.Items {
				got = append(got, it.Designation)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("результат: %v, ожидался %v", got, tc.want)
			}
		})
	}

	// total — по фильтрам, а не по странице.
	_, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/components?limit=2", "")
	page := decode[searchPageJSON](t, body)
	if page.Total != 4 || len(page.Items) != 2 || page.Limit != 2 {
		t.Fatalf("пагинация: %+v", page)
	}

	// вариантный параметрический фильтр — в пределах одного исполнения:
	// Unom=25 И diameter≤9 — у К50-35 диаметр 10 на 25 В → пусто.
	_, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/components?kind=capacitor&par.Unom.exact=25&par.diameter.max=9", "")
	page = decode[searchPageJSON](t, body)
	if len(page.Items) != 0 {
		t.Fatalf("вариантный фильтр дал сочетание разных исполнений: %v", page.Items)
	}
	_, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/components?kind=capacitor&par.Unom.exact=25&par.diameter.max=10", "")
	page = decode[searchPageJSON](t, body)
	if len(page.Items) != 1 || page.Items[0].Designation != "К50-35" {
		t.Fatalf("вариантный фильтр: %v", page.Items)
	}
}

func TestSearchETagChangesOnWrite(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	get := func() string {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+apiPrefix+"/components", nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body) //nolint:errcheck — только ETag
		return resp.Header.Get("ETag")
	}
	first := get()
	status, _, _ := do(t, srv, http.MethodPost, apiPrefix+"/components", `{"name":"ГТ109Г"}`)
	if status != http.StatusCreated {
		t.Fatalf("создание: %d", status)
	}
	if second := get(); second == first {
		t.Fatalf("ETag не сменился после записи: %s", second)
	}
}

func TestSearchErrors(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	cases := []struct {
		query   string
		code    string
		message string
	}{
		{"kind=bogus", "validation_failed", "неизвестный класс приборов «bogus»"},
		{"system=bogus", "validation_failed", "неизвестная система обозначений «bogus»"},
		{"par.bogus.min=1", "unknown_parameter", "неизвестный параметр «bogus»"},
		{"attr.bogus=npn", "unknown_attribute", "неизвестный атрибут «bogus»"},
		{"kind=resistor&par.h21e.min=1", "parameter_not_applicable",
			"параметр «h21e» неприменим к классу resistor"},
		{"kind=resistor&attr.structure=npn", "attribute_not_applicable",
			"атрибут «structure» неприменим к классу resistor"},
		{"attr.polarized=true", "validation_failed",
			"фильтр атрибута «polarized»: логические атрибуты не поддерживаются в фильтрах"},
		{"attr.structure=1967", "validation_failed", "фильтр атрибута «structure»: ожидается текстовое значение"},
		{"par.h21e.avg=5", "validation_failed", "неизвестный параметр запроса «par.h21e.avg»"},
		{"limit=0", "validation_failed", "параметр limit: ожидается целое от 1 до 200"},
		{"limit=201", "validation_failed", "параметр limit: ожидается целое от 1 до 200"},
		{"limit=abc", "validation_failed", "параметр limit: ожидается целое от 1 до 200"},
		{"offset=-1", "validation_failed", "параметр offset: ожидается целое неотрицательное число"},
		{"sort=bogus", "validation_failed", "параметр sort: неизвестный ключ сортировки «bogus»"},
		{"bogus=1", "validation_failed", "неизвестный параметр запроса «bogus»"},
		{"junctions=abc", "validation_failed", "параметр «junctions»: ожидается число"},
		{"par.h21e.min=abc", "validation_failed", "параметр «par.h21e.min»: ожидается число"},
		{"par.h21e.text=1", "validation_failed", "фильтр параметра «h21e»: ожидается число"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/components?"+tc.query, "")
			wantError(t, status, eb, http.StatusBadRequest, tc.code, tc.message)
		})
	}
}

func TestCard(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodGet,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"), "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	card := decode[cardJSON](t, body)
	if card.Designation != "КТ315Б" || card.Kind != "transistor" ||
		card.System != "gost" || card.KindName == "" || card.SystemName == "" {
		t.Fatalf("карточка: %+v", card)
	}
	if card.ID == 0 || len(card.Fields) == 0 {
		t.Fatalf("поля разбора отсутствуют: %+v", card)
	}
	if len(card.Attributes) == 0 || card.Attributes[0].Code != "structure" ||
		*card.Attributes[0].Text != "npn" {
		t.Fatalf("атрибуты: %+v", card.Attributes)
	}
	if len(card.Groups) != 2 || card.Groups[0].Section != "parameters" ||
		card.Groups[0].Values[0].Parameter != "h21e" ||
		*card.Groups[0].Values[0].Min != 50 || len(card.Groups[0].Values[0].Conditions) != 2 {
		t.Fatalf("группы значений: %+v", card.Groups)
	}
	if len(card.Manufacturers) != 2 || len(card.Analogs) != 1 ||
		card.Analogs[0].Designation != "BC547B" {
		t.Fatalf("производители/аналоги: %+v %+v", card.Manufacturers, card.Analogs)
	}

	// Встречная ссылка у цели аналога (направленность — D8).
	_, body, _ = do(t, srv, http.MethodGet, apiPrefix+"/components/transistor/BC547B", "")
	back := decode[cardJSON](t, body)
	if len(back.Backlinks) != 1 || back.Backlinks[0].Designation != "КТ315Б" {
		t.Fatalf("встречные ссылки: %+v", back.Backlinks)
	}

	// Исполнения: матрица «ёмкость × напряжение → габариты».
	_, body, _ = do(t, srv, http.MethodGet, apiPrefix+"/components/capacitor/"+esc("К50-35"), "")
	capCard := decode[cardJSON](t, body)
	if len(capCard.Variants) != 2 || capCard.Variants[0].Label != "160 В" {
		t.Fatalf("исполнения: %+v", capCard.Variants)
	}
	found := false
	for _, g := range capCard.Variants[0].Groups {
		for _, v := range g.Values {
			if v.Parameter == "diameter" && *v.Exact == 8 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("габариты исполнения отсутствуют: %+v", capCard.Variants[0])
	}
}

func TestCardNotFoundAndBadKind(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/components/transistor/"+esc("КТ999А"), "")
	wantError(t, status, eb, http.StatusNotFound, "not_found", "запись «КТ999А» не найдена")

	status, _, eb = do(t, srv, http.MethodGet, apiPrefix+"/components/bogus/"+esc("КТ315Б"), "")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed",
		"неизвестный класс приборов «bogus»")
}

func TestCardByID(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	_, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/components/transistor/"+esc("КТ315Б"), "")
	id := decode[cardJSON](t, body).ID
	status, body, _ := do(t, srv, http.MethodGet, fmt.Sprintf("%s/components/id/%d", apiPrefix, id), "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	if card := decode[cardJSON](t, body); card.Designation != "КТ315Б" {
		t.Fatalf("карточка по id: %+v", card)
	}

	status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/components/id/abc", "")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed",
		"параметр пути id: ожидается целое число")

	status, _, eb = do(t, srv, http.MethodGet, apiPrefix+"/components/id/999999", "")
	wantError(t, status, eb, http.StatusNotFound, "not_found",
		"запись с идентификатором 999999 не найдена")
}

func TestSuggest(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/suggest?q="+esc("КТ3"), "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	resp := decode[suggestResponseJSON](t, body)
	if len(resp.Items) != 1 || resp.Items[0].Designation != "КТ315Б" {
		t.Fatalf("подсказки: %+v", resp.Items)
	}

	status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/suggest", "")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed", "не задан параметр q")

	status, _, eb = do(t, srv, http.MethodGet, apiPrefix+"/suggest?q=%D0%9A&limit=51", "")
	wantError(t, status, eb, http.StatusBadRequest, "validation_failed",
		"параметр limit: ожидается целое от 1 до 50")
}

func TestSuggestDisabled(t *testing.T) {
	_, srv := newTestAPI(t, Config{DisableSuggest: true})
	status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/suggest?q="+esc("КТ3"), "")
	wantError(t, status, eb, http.StatusNotFound, "not_found", "неизвестный маршрут запроса")
}

func TestCreate(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodPost, apiPrefix+"/components",
		`{"name":"ГТ109Г","attributes":{"structure":"pnp"}}`)
	if status != http.StatusCreated {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp := decode[upsertResponseJSON](t, body)
	if resp.Outcome != "Added" || resp.Card.Designation != "ГТ109Г" ||
		resp.Card.Kind != "transistor" {
		t.Fatalf("ответ создания: %+v", resp)
	}

	// Повтор — 409 already_exists.
	status, _, eb := do(t, srv, http.MethodPost, apiPrefix+"/components", `{"name":"ГТ109Г"}`)
	wantError(t, status, eb, http.StatusConflict, "already_exists", "запись «ГТ109Г» уже существует")

	// Строка-обозначение как тело.
	status, body, _ = do(t, srv, http.MethodPost, apiPrefix+"/components", `"2Т312А"`)
	if status != http.StatusCreated {
		t.Fatalf("строка-обозначение: %d, тело %s", status, body)
	}

	// Синтаксис: битый JSON — 400 invalid_import_file (текст — от hujson,
	// контрактом не является).
	status, _, eb = do(t, srv, http.MethodPost, apiPrefix+"/components", `{"name":`)
	if status != http.StatusBadRequest || eb.Code != "invalid_import_file" {
		t.Fatalf("битый JSON: %d %+v", status, eb)
	}
	// Дубликаты ключей — тихая потеря данных недопустима (дословный текст).
	status, _, eb = do(t, srv, http.MethodPost, apiPrefix+"/components",
		`{"name":"ГТ402Г","name":"ГТ402Г"}`)
	wantError(t, status, eb, http.StatusBadRequest, "invalid_import_file",
		"файл не является корректным JSONC: повторяющийся ключ «name» (строка 1)")

	// Форма записи: неизвестная секция — 400 (код читателя формата).
	status, _, eb = do(t, srv, http.MethodPost, apiPrefix+"/components",
		`{"name":"ГТ402Г","bogus":[]}`)
	wantError(t, status, eb, http.StatusBadRequest, "invalid_import_file",
		"неизвестное поле «bogus» (допустимы: name, system, attributes, manufacturers, variants, analogs и секции групп: parameters, ratings, dimensions)")

	// Невалидное обозначение — 422 invalid_designation.
	status, _, eb = do(t, srv, http.MethodPost, apiPrefix+"/components", `{"name":"@@@"}`)
	if status != http.StatusUnprocessableEntity || eb.Code != "invalid_designation" {
		t.Fatalf("невалидное обозначение: %d %+v", status, eb)
	}

	// Система other без класса — создание адресуется PUT (документировано);
	// POST честно отказывает.
	status, _, eb = do(t, srv, http.MethodPost, apiPrefix+"/components",
		`{"name":"MJE340","system":"other"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("other без класса: %d %+v", status, eb)
	}
}

func TestPut(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	// Замена секции производителей целиком.
	status, body, _ := do(t, srv, http.MethodPut,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"),
		`{"name":"КТ315Б","manufacturers":["Оникс"]}`)
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp := decode[upsertResponseJSON](t, body)
	if resp.Outcome != "UpdatedExisting" || len(resp.Card.Manufacturers) != 1 ||
		resp.Card.Manufacturers[0] != "Оникс" {
		t.Fatalf("upsert: %+v", resp)
	}

	// Семантика «отсутствует — не менять» + идемпотентность: исход Skipped.
	status, body, _ = do(t, srv, http.MethodPut,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"), `{"name":"КТ315Б"}`)
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	if resp := decode[upsertResponseJSON](t, body); resp.Outcome != "Skipped" {
		t.Fatalf("повторный upsert: %+v", resp)
	}

	// Очистка секции.
	status, body, _ = do(t, srv, http.MethodPut,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"),
		`{"name":"КТ315Б","manufacturers":[]}`)
	if status != http.StatusOK {
		t.Fatalf("статус: %d", status)
	}
	if resp := decode[upsertResponseJSON](t, body); len(resp.Card.Manufacturers) != 0 {
		t.Fatalf("очистка производителей: %+v", resp.Card.Manufacturers)
	}

	// Несовпадение обозначения — 422 designation_mismatch.
	status, _, eb := do(t, srv, http.MethodPut,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"), `{"name":"КТ315В"}`)
	wantError(t, status, eb, http.StatusUnprocessableEntity, "designation_mismatch",
		"обозначение тела запроса не совпадает с обозначением в пути")

	// PUT создаёт (класс в пути переопределяет автодетект — способ записи
	// для системы other).
	status, body, _ = do(t, srv, http.MethodPut,
		apiPrefix+"/components/diode/"+esc("Д226"),
		`{"name":"Д226","system":"series","attributes":{"category":"выпрямительный"}}`)
	if status != http.StatusOK {
		t.Fatalf("создание PUT: %d, тело %s", status, body)
	}
	if resp := decode[upsertResponseJSON](t, body); resp.Outcome != "Added" ||
		resp.Card.Designation != "Д226" {
		t.Fatalf("создание PUT: %+v", resp)
	}
}

func TestDelete(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodDelete,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"), "")
	if status != http.StatusNoContent && body != "" {
		t.Fatalf("удаление: %d %q", status, body)
	}
	if status != http.StatusNoContent {
		t.Fatalf("статус: %d", status)
	}
	status, _, _ = do(t, srv, http.MethodGet, apiPrefix+"/components/transistor/"+esc("КТ315Б"), "")
	if status != http.StatusNotFound {
		t.Fatalf("после удаления: %d", status)
	}
	status, _, eb := do(t, srv, http.MethodDelete,
		apiPrefix+"/components/transistor/"+esc("КТ315Б"), "")
	wantError(t, status, eb, http.StatusNotFound, "not_found", "запись «КТ315Б» не найдена")

	// Встречная ссылка исчезла у цели аналога (каскад).
	_, body, _ = do(t, srv, http.MethodGet, apiPrefix+"/components/transistor/BC547B", "")
	if back := decode[cardJSON](t, body); len(back.Backlinks) != 0 {
		t.Fatalf("встречные ссылки после удаления: %+v", back.Backlinks)
	}
}

func TestRouteErrors(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+apiPrefix+"/components", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("статус: %d", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "GET, POST" {
		t.Fatalf("Allow: %q", allow)
	}
	var eb errorBody
	if err := json.Unmarshal(data, &eb); err != nil || eb.Code != "method_not_allowed" ||
		eb.Message != "метод не допускается для этого маршрута" {
		t.Fatalf("ошибка 405: %q → %+v (%v)", data, eb, err)
	}

	status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/bogus", "")
	wantError(t, status, eb, http.StatusNotFound, "not_found", "неизвестный маршрут запроса")
}

func TestInternalError(t *testing.T) {
	app, srv := newTestAPI(t, Config{})
	app.Close() //nolint:errcheck — проверка деградации транспорта
	status, _, eb := do(t, srv, http.MethodGet, apiPrefix+"/kinds", "")
	if status != http.StatusInternalServerError || eb.Code != "internal_error" ||
		eb.Message != "внутренняя ошибка обработки запроса" {
		t.Fatalf("внутренняя ошибка: %d %+v", status, eb)
	}
}

// TestRequestLog — slog-контекст запросов: структурированная запись
// с операцией, методом, путём, статусом и кодом ошибки.
func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{Log: slog.New(slog.NewTextHandler(&buf, nil))}
	_, srv := newTestAPI(t, cfg)
	do(t, srv, http.MethodGet, apiPrefix+"/kinds", "")
	line := buf.String()
	for _, want := range []string{"op=kinds", "method=GET", "status=200", "error_code="} {
		if !strings.Contains(line, want) {
			t.Fatalf("журнал запроса не содержит %q: %q", want, line)
		}
	}
}

func TestMetricsHook(t *testing.T) {
	c := &collector{}
	_, srv := newTestAPI(t, Config{Metrics: c})
	do(t, srv, http.MethodGet, apiPrefix+"/kinds", "")
	do(t, srv, http.MethodGet, apiPrefix+"/components/transistor/"+esc("КТ999А"), "")
	want := []string{"kinds 200 ", "components_get 404 not_found"}
	if !slices.Equal(c.entries, want) {
		t.Fatalf("метрики: %v, ожидались %v", c.entries, want)
	}
}

// TestRoutesMatchSpec — спецификация и реализация не расходятся
// (критерий этапа 5): маршруты/методы/operationId совпадают дословно.
func TestRoutesMatchSpec(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("чтение спецификации: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("разбор спецификации: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("спецификация без путей")
	}
	type endpoint struct{ method, path string }
	specEndpoints := make(map[endpoint]string)
	for path, ops := range spec.Paths {
		for method, op := range ops {
			specEndpoints[endpoint{strings.ToUpper(method), path}] = op.OperationID
		}
	}
	implEndpoints := make(map[endpoint]string)
	for _, rt := range routes {
		implEndpoints[endpoint{rt.method, rt.pattern}] = rt.op
	}
	for ep, opID := range implEndpoints {
		want, ok := specEndpoints[ep]
		if !ok {
			t.Errorf("маршрут %s %s отсутствует в спецификации", ep.method, ep.path)
			continue
		}
		if want != opID {
			t.Errorf("operationId %s %s: спецификация %q, реализация %q",
				ep.method, ep.path, want, opID)
		}
	}
	for ep := range specEndpoints {
		if _, ok := implEndpoints[ep]; !ok {
			t.Errorf("маршрут спецификации %s %s не реализован", ep.method, ep.path)
		}
	}
}
