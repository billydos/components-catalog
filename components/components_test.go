// Контрактный тест фасада: внешний тест-пакет обращается к библиотеке
// только через публичный API пакета components — компилируемость
// поверхности для внешнего потребителя (внутримодульные тесты позволяют
// импорты internal/ и подлинности не доказывают; testutil здесь — только
// DSN одноразовой PostgreSQL, не API библиотеки). Ноги: SQLite (файл)
// и PostgreSQL (CATALOG_TEST_POSTGRES_DSN, прогон опционален).
package components_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/components"
	"github.com/billydos/components-catalog/internal/testutil"
)

// fillJSONC — минимальный файл наполнения на два транзистора (форма —
// docs/fill-format.md; коды параметров/условий — стартовый каталог сидов).
const fillJSONC = `{
	"transistors": [
		{
			"name": "КТ315Б",
			"attributes": {"structure": "npn", "yearFrom": 1967},
			"manufacturers": ["Восход"],
			"parameters": [
				{"parameter": "hfe", "min": 50, "max": 350, "Vce": 10, "Ic": 1}
			],
			"ratings": [
				{"parameter": "VceoMax", "value": 20}
			]
		},
		{
			"name": "BC547B",
			"attributes": {"structure": "npn"},
			"parameters": [
				{"parameter": "hfe", "min": 200, "max": 450, "Vce": 5, "Ic": 2}
			]
		}
	]
}`

// facadeConfig — фабрика конфигураций: каждый прогон получает свежую базу.
type facadeConfig func(t *testing.T) components.Config

func sqliteFileConfig(t *testing.T) components.Config {
	t.Helper()
	return components.Config{
		Dialect:       "sqlite",
		DSN:           filepath.Join(t.TempDir(), "catalog.db"),
		EnsureCreated: true,
	}
}

func postgresConfig(t *testing.T) components.Config {
	t.Helper()
	dsn := testutil.PostgresDSN(t)
	testutil.DropAllTables(t, dsn)
	return components.Config{Dialect: "postgres", DSN: dsn, EnsureCreated: true}
}

func TestFacadeSQLiteFile(t *testing.T) { runFacadeContract(t, sqliteFileConfig) }
func TestFacadePostgres(t *testing.T)   { runFacadeContract(t, postgresConfig) }

// runFacadeContract — полный цикл встраивания через фасад: открытие,
// импорт файла наполнения, поиск, карточка, разбор обозначения, поток
// изменений, агрегаты, REST-обработчик, round-trip экспорт, ошибки-значения.
func runFacadeContract(t *testing.T, factory facadeConfig) {
	ctx := context.Background()
	app, err := components.Open(ctx, factory(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer app.Close() //nolint:errcheck — закрытие в тесте

	// Импорт файла наполнения и идемпотентность повторного прогона.
	rep, err := app.Importer.Import(ctx, strings.NewReader(fillJSONC), "fill.jsonc", false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if rep.Records != 2 || rep.Added != 2 || rep.HasIssues() {
		t.Fatalf("import report: %+v", rep)
	}
	rep, err = app.Importer.Import(ctx, strings.NewReader(fillJSONC), "fill.jsonc", false)
	if err != nil {
		t.Fatalf("reimport: %v", err)
	}
	if rep.Added != 0 || rep.Skipped != 2 {
		t.Fatalf("reimport report: %+v", rep)
	}

	// Прогресс импорта (CC-6): колбэк WithProgress наблюдает чтение
	// и применение; NDJSON — построчно (документ — один вызов чтения).
	var snaps []components.Progress
	rep, err = app.Importer.Import(ctx, strings.NewReader(fillJSONC), "fill.jsonc", false,
		components.WithProgress(func(p components.Progress) { snaps = append(snaps, p) }))
	if err != nil {
		t.Fatalf("import with progress: %v", err)
	}
	want := []components.Progress{
		{Read: 2},
		{Read: 2, Applied: 1},
		{Read: 2, Applied: 2},
	}
	if !slices.Equal(snaps, want) {
		t.Fatalf("progress snapshots: %+v, want %+v", snaps, want)
	}

	// Поиск: подстрока обозначения и параметрический фильтр.
	page, err := app.Services.Devices.Search(ctx, components.SearchQuery{
		Kind:  components.KindTransistor,
		Query: "КТ3",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Designation != "КТ315Б" {
		t.Fatalf("search page: total=%d items=%+v", page.Total, page.Items)
	}
	if page.Items[0].System != components.SystemGost {
		t.Fatalf("search item system: %s", page.Items[0].System)
	}
	id := page.Items[0].ID
	hfeMin := 100.0
	page, err = app.Services.Devices.Search(ctx, components.SearchQuery{
		Kind: components.KindTransistor,
		Parameters: []components.ParameterFilter{
			{Parameter: "hfe", Min: &hfeMin},
		},
	})
	if err != nil {
		t.Fatalf("param search: %v", err)
	}
	if page.Total != 1 || page.Items[0].Designation != "BC547B" {
		t.Fatalf("param search page: total=%d items=%+v", page.Total, page.Items)
	}

	// Карточка: поля разбора по обозначению, значения параметров по группам.
	card, ok, err := app.Services.Devices.Get(ctx, components.KindTransistor, "КТ315Б")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if f, found := card.FieldByName("material"); !found || f.String() != "si" {
		t.Fatalf("card field material: %+v", f)
	}
	var hfeVal *components.CardValue
	for _, g := range card.Groups {
		if g.Section != "parameters" {
			continue
		}
		for i, v := range g.Values {
			if v.Parameter == "hfe" {
				hfeVal = &g.Values[i]
			}
		}
	}
	if hfeVal == nil || hfeVal.Min == nil || hfeVal.Max == nil || *hfeVal.Min != 50 || *hfeVal.Max != 350 {
		t.Fatalf("card hfe value: %+v", hfeVal)
	}
	if len(card.Manufacturers) != 1 || card.Manufacturers[0] != "Восход" {
		t.Fatalf("card manufacturers: %+v", card.Manufacturers)
	}
	byID, ok, err := app.Services.Devices.GetByID(ctx, id)
	if err != nil || !ok || byID.Designation != "КТ315Б" {
		t.Fatalf("get by id: ok=%v err=%v card=%+v", ok, err, byID)
	}

	// Разбор обозначения с автодетектом и автодополнение по префиксу.
	parsed, err := app.Services.Designations.Parse(ctx, "КТ315Б")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Kind != components.KindTransistor || parsed.System != components.SystemGost {
		t.Fatalf("parse: kind=%s system=%s", parsed.Kind, parsed.System)
	}
	sugg, err := app.Services.Designations.Suggest(ctx, "КТ3", components.KindTransistor, 5)
	if err != nil || len(sugg) != 1 || sugg[0].Designation != "КТ315Б" {
		t.Fatalf("suggest: err=%v %+v", err, sugg)
	}

	// Поток изменений: два события created, повторный импорт — без событий.
	changes, err := app.Services.Devices.Changes(ctx, 0, components.ChangesDefaultLimit)
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if len(changes.Events) != 2 {
		t.Fatalf("changes: %+v", changes.Events)
	}
	seen := map[string]bool{}
	for _, ev := range changes.Events {
		if ev.Op != components.ChangeOpCreated {
			t.Fatalf("change event: %+v", ev)
		}
		seen[ev.Designation] = true
	}
	if !seen["КТ315Б"] || !seen["BC547B"] {
		t.Fatalf("change designations: %+v", changes.Events)
	}

	// Агрегаты и ревизии.
	stats, err := app.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Total != 2 || stats.Counts[components.KindTransistor] != 2 {
		t.Fatalf("stats: %+v", stats)
	}
	catalogRev, dataRev, err := app.Revisions(ctx)
	if err != nil || catalogRev < 1 || dataRev < 2 {
		t.Fatalf("revisions: catalog=%d data=%d err=%v", catalogRev, dataRev, err)
	}

	// REST-обработчик: те же данные через /api/v1.
	h := app.Handler()
	restWant(t, h, http.MethodGet, "/api/v1/components?kind=transistor&q=КТ3",
		http.StatusOK, "КТ315Б")
	restWant(t, h, http.MethodGet, "/api/v1/components/id/"+strconv.FormatInt(id, 10),
		http.StatusOK, "КТ315Б")
	restWant(t, h, http.MethodGet, "/api/v1/stats", http.StatusOK, `"total":2`)
	restWant(t, h, http.MethodGet, "/api/v1/parse?designation=КТ315Б",
		http.StatusOK, `"system":"gost"`)
	restWant(t, h, http.MethodGet, "/api/v1/nosuchroute",
		http.StatusNotFound, `"code":"not_found"`)

	// Потоковый импорт NDJSON через REST-обработчик фасада (CC-6):
	// синхронный отчёт после EOF.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import",
		strings.NewReader("{\"transistors\": {\"name\": \"КТ3102А\"}}\n"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"added":1`) {
		t.Fatalf("POST /api/v1/import: статус %d, тело: %s", rec.Code, rec.Body.String())
	}

	// Round-trip экспорт записей.
	var buf bytes.Buffer
	if err := app.Importer.Export(ctx, &buf, components.FormatNDJSON, nil); err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(buf.String(), "КТ315Б") || !strings.Contains(buf.String(), "BC547B") {
		t.Fatalf("export body: %s", buf.String())
	}

	// Ошибки-значения: код домена извлекается из ошибки публичным AsError.
	_, err = app.Services.Devices.Search(ctx, components.SearchQuery{
		Kind: components.KindTransistor,
		Parameters: []components.ParameterFilter{
			{Parameter: "nosuch", Min: &hfeMin},
		},
	})
	de, ok := components.AsError(err)
	if !ok || de.Code != "unknown_parameter" {
		t.Fatalf("search error: %v (domain=%v)", err, de)
	}

	// DisableSuggest: хендлер не монтируется на той же базе.
	noSuggest, err := components.Open(ctx, components.Config{
		Dialect:        "sqlite",
		DSN:            filepath.Join(t.TempDir(), "nosuggest.db"),
		EnsureCreated:  true,
		DisableSuggest: true,
	})
	if err != nil {
		t.Fatalf("open nosuggest: %v", err)
	}
	defer noSuggest.Close() //nolint:errcheck — закрытие в тесте
	restWant(t, noSuggest.Handler(), http.MethodGet, "/api/v1/suggest?q=КТ",
		http.StatusNotFound, `"code":"not_found"`)
}

// restWant — запрос к REST-обработнику с проверкой статуса и подстроки тела.
func restWant(t *testing.T, h http.Handler, method, path string, wantStatus int, wantBody string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: статус %d, ожидался %d; тело: %s",
			method, path, rec.Code, wantStatus, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), wantBody) {
		t.Fatalf("%s %s: тело не содержит %q: %s", method, path, wantBody, rec.Body.String())
	}
}
