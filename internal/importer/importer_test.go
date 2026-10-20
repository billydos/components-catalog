package importer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
	"github.com/billydos/components-catalog/internal/storage"
	"github.com/billydos/components-catalog/internal/testutil"
)

// Интеграционный набор этапа 4 (план работ 4.2–4.6): один и тот же набор
// на SQLite (память/файл) и PostgreSQL (локально по
// CATALOG_TEST_POSTGRES_DSN). Критерии: --dry-run не пишет; негативные
// файлы дают полный список проблем за прогон; round-trip значений и
// условий во всех форматах. Идемпотентность импорта выверенного
// наполнения data/ (все классы, обе СУБД) — verified_test.go; NDJSON
// покрывается round-trip экспортом (string-обёртка на запись).

type configFactory func(t *testing.T) service.Config

func memoryConfig(t *testing.T) service.Config {
	t.Helper()
	return service.Config{Dialect: "sqlite", DSN: ":memory:", EnsureCreated: true}
}

func fileConfig(t *testing.T) service.Config {
	t.Helper()
	return service.Config{Dialect: "sqlite", DSN: filepath.Join(t.TempDir(), "catalog.db"), EnsureCreated: true}
}

func postgresConfig(t *testing.T) service.Config {
	t.Helper()
	dsn := testutil.PostgresDSN(t)
	testutil.DropAllTables(t, dsn)
	return service.Config{Dialect: "postgres", DSN: dsn, EnsureCreated: true}
}

func TestIntegrationSQLiteMemory(t *testing.T) { runImporterSuite(t, memoryConfig) }
func TestIntegrationSQLiteFile(t *testing.T)   { runImporterSuite(t, fileConfig) }
func TestIntegrationPostgres(t *testing.T)     { runImporterSuite(t, postgresConfig) }

func openApp(t *testing.T, factory configFactory) *service.App {
	t.Helper()
	app, err := service.Open(context.Background(), factory(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

func importFile(t *testing.T, app *service.App, path string, dryRun bool) Report {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открытие %s: %v", path, err)
	}
	defer f.Close()
	rep, err := New(app).Import(context.Background(), f, path, dryRun)
	if err != nil {
		t.Fatalf("импорт %s: %v", path, err)
	}
	return rep
}

// importFileErr — импорт с жёсткой ошибкой прогона (синтаксис файла).
func importFileErr(t *testing.T, app *service.App, path string, dryRun bool) (Report, error) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открытие %s: %v", path, err)
	}
	defer f.Close()
	return New(app).Import(context.Background(), f, path, dryRun)
}

func runImporterSuite(t *testing.T, factory configFactory) {
	t.Run("DryRunDoesNotWrite", func(t *testing.T) {
		app := openApp(t, factory)
		rep := importFile(t, app, dataPath(t, "transistors.jsonc"), true)
		if rep.HasIssues() {
			t.Fatalf("проблемы dry-run: %v", issueMessages(rep.Issues))
		}
		if rep.Added != 23 || rep.DryRun != true {
			t.Fatalf("dry-run: %+v", rep)
		}
		n, err := app.Services().Devices.Count(context.Background(), nil)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("dry-run записал в базу: записей %d", n)
		}
		// Прогон после dry-run применяет всё то же самое.
		rep = importFile(t, app, dataPath(t, "transistors.jsonc"), false)
		if rep.Added != 23 || rep.HasIssues() {
			t.Fatalf("импорт после dry-run: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("NegativeFileFullIssueList", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "negative.jsonc")
		write(t, file, `{
			"transistors": [
				{ "name": "МП39", "system": "other", "parameters": [ { "parameter": "hfe", "max": 50, "Vce": 5, "Ic": 5 } ] },
				"ГТ109Г",
				{ "name": "2N9999ZZ", "system": "jedec" },
				{ "name": "МП39", "system": "other", "ratings": [ { "parameter": "VceoMax", "value": 0 } ] }
			]
		}`)
		rep := importFile(t, app, file, false)
		if len(rep.Issues) != 2 {
			t.Fatalf("проблем: %d, ожидалось 2: %v", len(rep.Issues), issueMessages(rep.Issues))
		}
		want := []string{
			"record «МП39»: parameter «hfe»: value type range requires keys min and max",
			"record «МП39»: parameter «VceoMax»: value of key value must be positive",
		}
		got := issueMessages(rep.Issues)
		for _, w := range want {
			if !containsMessage(got, w) {
				t.Errorf("проблема отсутствует: %q (есть: %v)", w, got)
			}
		}
		// Корректные записи применены, ошибочные — нет: полный список
		// за один прогон при частичном применении.
		if rep.Added != 2 || rep.Rejected != 2 || rep.Records != 4 {
			t.Fatalf("итог: %+v", rep)
		}
	})

	t.Run("VariantAttributesRoundTrip", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "vars.jsonc")
		write(t, file, `{
			"resistors": [
				{
					"name": "СП3-33",
					"attributes": { "spec": "ОЖ0.467.093ТУ" },
					"variants": [
						{ "label": "24",
							"attributes": { "functionalChar": "А", "package": "DIP" },
							"ratings": [ { "parameter": "Pnom", "value": 0.125 } ] },
						{ "label": "28",
							"attributes": { "functionalChar": "Б" },
							"ratings": [ { "parameter": "Pnom", "value": 0.25 } ] }
					]
				}
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 1 {
			t.Fatalf("импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, FormatJSONC, nil); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		if !strings.Contains(buf.String(), `"functionalChar"`) {
			t.Fatalf("экспорт без атрибутов исполнения:\n%s", buf.String())
		}
		// Повторный импорт экспорта — Skipped по всем записям:
		// round-trip идемпотентен и с атрибутами исполнений.
		exp := filepath.Join(t.TempDir(), "export.jsonc")
		write(t, exp, buf.String())
		rep = importFile(t, app, exp, false)
		if rep.HasIssues() || rep.Skipped != 1 {
			t.Fatalf("повторный импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("ForwardAnalogReferencesResolved", func(t *testing.T) {
		app := openApp(t, factory)
		// Прямая ссылка на позднейшую запись файла: многопроходное
		// разрешение (real) и pending-обозначения (dry-run).
		file := filepath.Join(t.TempDir(), "analogs.jsonc")
		write(t, file, `{
			"transistors": [
				{ "name": "МП39", "system": "other", "analogs": ["BC547B"] },
				{ "name": "BC547B" }
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 2 {
			t.Fatalf("импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, "МП39")
		if err != nil || !found {
			t.Fatalf("карточка МП39: %v %v", found, err)
		}
		if len(card.Analogs) != 1 || card.Analogs[0].Designation != "BC547B" {
			t.Fatalf("аналоги МП39: %+v", card.Analogs)
		}
		card, found, err = app.Services().Devices.Get(context.Background(), domain.KindTransistor, "BC547B")
		if err != nil || !found {
			t.Fatalf("карточка BC547B: %v %v", found, err)
		}
		if len(card.Backlinks) != 1 || card.Backlinks[0].Designation != "МП39" {
			t.Fatalf("встречные ссылки BC547B: %+v", card.Backlinks)
		}
	})

	t.Run("MutualAnalogReferences", func(t *testing.T) {
		// Взаимные ссылки (A → B и B → A в одном файле, 03 §8: импорт
		// воспроизводит состояние независимо от порядка записей):
		// цикл разрывается холостым созданием без секции аналогов.
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "mutual.jsonc")
		write(t, file, `{
			"diodes": [
				{ "name": "Д226", "system": "other", "analogs": ["1N4007"] },
				{ "name": "1N4007", "analogs": ["Д226"] }
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 2 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindDiode, "Д226")
		if err != nil || !found {
			t.Fatalf("карточка Д226: %v %v", found, err)
		}
		if len(card.Analogs) != 1 || card.Analogs[0].Designation != "1N4007" {
			t.Fatalf("исходящие Д226: %+v", card.Analogs)
		}
		if len(card.Backlinks) != 1 || card.Backlinks[0].Designation != "1N4007" {
			t.Fatalf("встречные Д226: %+v", card.Backlinks)
		}
		card, found, err = app.Services().Devices.Get(context.Background(), domain.KindDiode, "1N4007")
		if err != nil || !found {
			t.Fatalf("карточка 1N4007: %v %v", found, err)
		}
		if len(card.Analogs) != 1 || card.Analogs[0].Designation != "Д226" {
			t.Fatalf("исходящие 1N4007: %+v", card.Analogs)
		}
		// Повторный импорт — unchanged по обеим записям.
		rep = importFile(t, app, file, false)
		if rep.HasIssues() || rep.Skipped != 2 {
			t.Fatalf("повторный: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("AnalogMissingTargetIsIssue", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "broken.jsonc")
		write(t, file, `{
			"transistors": [ { "name": "МП39", "system": "other", "analogs": ["BC999ZZ"] } ]
		}`)
		rep := importFile(t, app, file, false)
		if !rep.HasIssues() || rep.Rejected != 1 {
			t.Fatalf("итог: %+v", rep)
		}
		want := "record «МП39»: analog «BC999ZZ» not found in kind transistor"
		if rep.Issues[0].String() != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].String(), want)
		}
	})

	// Поток изменений (change feed): применённые записи оставляют события,
	// повторный импорт (Skipped) — ничего; отвергнутая запись с холостым
	// созданием компенсируется удалением — фид видит и создание,
	// и компенсационное deleted.
	t.Run("ChangeFeedEvents", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "feed.jsonc")
		write(t, file, `{
			"transistors": [
				{ "name": "МП39", "system": "other", "attributes": { "structure": "npn" } },
				"ГТ109Г"
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 2 {
			t.Fatalf("импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
		page, err := app.Services().Devices.Changes(context.Background(), 0, service.ChangesLimitMax)
		if err != nil {
			t.Fatalf("changes: %v", err)
		}
		if len(page.Events) != 2 {
			t.Fatalf("событий: %d, ожидалось 2: %+v", len(page.Events), page.Events)
		}
		for i, want := range []struct {
			op          service.ChangeOp
			designation string
		}{{service.ChangeOpCreated, "МП39"}, {service.ChangeOpCreated, "ГТ109Г"}} {
			e := page.Events[i]
			if e.Op != want.op || e.Designation != want.designation ||
				e.Kind != domain.KindTransistor || e.DeviceID == 0 {
				t.Fatalf("событие %d: %+v, ожидалось %s %s", i, e, want.op, want.designation)
			}
		}

		// Повторный импорт — Skipped по всем записям: событий нет,
		// data_revision не выросла.
		_, dataBefore, err := app.Revisions(context.Background())
		if err != nil {
			t.Fatalf("ревизии: %v", err)
		}
		rep = importFile(t, app, file, false)
		if rep.HasIssues() || rep.Skipped != 2 {
			t.Fatalf("повторный импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
		page, err = app.Services().Devices.Changes(context.Background(),
			page.Events[len(page.Events)-1].ID, service.ChangesLimitMax)
		if err != nil || len(page.Events) != 0 {
			t.Fatalf("события при повторном импорте: %+v err=%v", page.Events, err)
		}
		if _, dataAfter, _ := app.Revisions(context.Background()); dataAfter != dataBefore {
			t.Fatalf("data_revision при Skipped: %d → %d", dataBefore, dataAfter)
		}
	})

	t.Run("ChangeFeedCompensatingDelete", func(t *testing.T) {
		// Аналог указывает на запись, которая сама отвергается: холостое
		// создание разрывает ожидание аналога, затем отвергнутая запись
		// компенсируется удалением — фид: created, deleted (компенсация).
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "compensate.jsonc")
		write(t, file, `{
			"transistors": [
				{ "name": "МП39", "system": "other", "analogs": ["ГТ109Г"] },
				{ "name": "ГТ109Г", "ratings": [ { "parameter": "VceoMax", "value": 0 } ] }
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.Rejected != 2 || rep.Added != 0 {
			t.Fatalf("итог: %+v; %v", rep, issueMessages(rep.Issues))
		}
		page, err := app.Services().Devices.Changes(context.Background(), 0, service.ChangesLimitMax)
		if err != nil {
			t.Fatalf("changes: %v", err)
		}
		if len(page.Events) != 2 {
			t.Fatalf("событий: %d, ожидалось 2 (created + компенсационный deleted): %+v",
				len(page.Events), page.Events)
		}
		first, last := page.Events[0], page.Events[1]
		if first.Op != service.ChangeOpCreated || first.Designation != "МП39" ||
			last.Op != service.ChangeOpDeleted || last.Designation != "МП39" ||
			first.DeviceID != last.DeviceID {
			t.Fatalf("события компенсации: %+v", page.Events)
		}
		// Компенсационное удаление — позже создания: ревизия не убывает.
		if last.Revision <= first.Revision {
			t.Fatalf("ревизии компенсации: %d → %d", first.Revision, last.Revision)
		}
		// Отвергнутая запись событий не оставила.
		for _, e := range page.Events {
			if e.Designation == "ГТ109Г" {
				t.Fatalf("отвергнутая запись оставила событие: %+v", e)
			}
		}
	})

	t.Run("NDJSONCatalogBeforeRecords", func(t *testing.T) {
		app := openApp(t, factory)
		good := filepath.Join(t.TempDir(), "good.ndjson")
		write(t, good,
			`{"catalog": {"parameter_groups": [ { "code": "env", "section": "environment" } ]}}`+"\n"+
				`{"catalog": {"parameters": [ { "code": "vibration", "group": "env", "value_type": "text" } ]}}`+"\n"+
				`{"transistors": { "name": "КТ315Б", "environment": [ { "parameter": "vibration", "text": "до 10 g" } ] }}`+"\n")
		rep := importFile(t, app, good, false)
		if rep.HasIssues() || rep.CatalogApplied != true || rep.Added != 1 {
			t.Fatalf("итог: %+v; %v", rep, issueMessages(rep.Issues))
		}
		if _, err := app.Snapshot(context.Background()); err != nil {
			t.Fatalf("снимок: %v", err)
		}

		// catalog после записи — проблема корневого уровня.
		bad := filepath.Join(t.TempDir(), "bad.ndjson")
		write(t, bad,
			`{"transistors": {"name": "МП39", "system": "other"}}`+"\n"+
				`{"catalog": {"units": [{"code": "kV"}]}}`+"\n")
		rep = importFile(t, app, bad, false)
		if len(rep.Issues) != 1 || rep.Issues[0].String() != "line 2: the catalog block must precede records" {
			t.Fatalf("итог: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("CatalogSectionWithRecords", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "catrec.jsonc")
		write(t, file, `{
			"catalog": {
				"attributes": [ { "code": "coating", "type": "enum", "enum_values": ["лак", "эмаль"] } ]
			},
			"diodes": [
				{ "name": "Д226", "system": "other", "attributes": { "coating": "лак" } }
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 1 || !rep.CatalogApplied {
			t.Fatalf("итог: %+v; %v", rep, issueMessages(rep.Issues))
		}
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindDiode, "Д226")
		if err != nil || !found {
			t.Fatalf("карточка: %v %v", found, err)
		}
		if len(card.Attributes) != 1 || card.Attributes[0].Code != "coating" {
			t.Fatalf("атрибут нового каталога не применён: %+v", card.Attributes)
		}
	})

	t.Run("CatalogImportFileRejectsRecords", func(t *testing.T) {
		app := openApp(t, factory)
		catRev, dataRev, err := app.Revisions(context.Background())
		if err != nil {
			t.Fatalf("ревизии: %v", err)
		}
		file := dataPath(t, "diodes.jsonc")
		f, err := os.Open(file)
		if err != nil {
			t.Fatalf("открытие: %v", err)
		}
		defer f.Close()
		_, err = New(app).ImportCatalogFile(context.Background(), f, file, false)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeInvalidImportFile {
			t.Fatalf("ожидалась invalid_import_file, получено %v", err)
		}
		if de.Message != "the file contains kind records; catalog import applies to files with the catalog section only" {
			t.Fatalf("текст: %q", de.Message)
		}
		// Отказ до любых записей в БД: записей нет, ревизии не двигались.
		n, err := app.Services().Devices.Count(context.Background(), nil)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("отказ оставил записи в базе: %d", n)
		}
		catRev2, dataRev2, err := app.Revisions(context.Background())
		if err != nil {
			t.Fatalf("ревизии: %v", err)
		}
		if catRev2 != catRev || dataRev2 != dataRev {
			t.Fatalf("отказ двинул ревизии: catalog %d→%d, data %d→%d",
				catRev, catRev2, dataRev, dataRev2)
		}
	})

	t.Run("CatalogImportMixedFileAppliesNothing", func(t *testing.T) {
		app := openApp(t, factory)
		catRev, dataRev, err := app.Revisions(context.Background())
		if err != nil {
			t.Fatalf("ревизии: %v", err)
		}
		// Смешанный документ: каталог вводит новый класс, файл содержит
		// записи (в том числе под вводимым классом).
		file := filepath.Join(t.TempDir(), "mixed.jsonc")
		write(t, file, `{
			"catalog": { "kinds": [ { "code": "sensor" } ] },
			"transistors": [ { "name": "КТ315Б" } ],
			"sensors": [ { "name": "DHT11" } ]
		}`)
		rep, err := importCatalogFile(t, app, file, false)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeInvalidImportFile {
			t.Fatalf("ожидалась invalid_import_file, получено %v", err)
		}
		if de.Message != "the file contains kind records; catalog import applies to files with the catalog section only" {
			t.Fatalf("текст: %q", de.Message)
		}
		if rep.CatalogApplied {
			t.Fatalf("каталог применён вопреки отказу: %+v", rep)
		}
		n, err := app.Services().Devices.Count(context.Background(), nil)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("отказ оставил записи в базе: %d", n)
		}
		snap, err := app.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("снимок: %v", err)
		}
		if _, ok := snap.Kind(domain.Kind("sensor")); ok {
			t.Fatal("отказ оставил расширение каталога (класс sensor)")
		}
		catRev2, dataRev2, err := app.Revisions(context.Background())
		if err != nil {
			t.Fatalf("ревизии: %v", err)
		}
		if catRev2 != catRev || dataRev2 != dataRev {
			t.Fatalf("отказ двинул ревизии: catalog %d→%d, data %d→%d",
				catRev, catRev2, dataRev, dataRev2)
		}

		// Смешанный NDJSON: строки catalog + строки записей.
		nd := filepath.Join(t.TempDir(), "mixed.ndjson")
		write(t, nd,
			`{"catalog": {"kinds": [ { "code": "sensor" } ]}}`+"\n"+
				`{"transistors": { "name": "МП39" }}`+"\n")
		rep, err = importCatalogFile(t, app, nd, false)
		if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeInvalidImportFile {
			t.Fatalf("ndjson: ожидалась invalid_import_file, получено %v", err)
		}
		if rep.CatalogApplied {
			t.Fatalf("ndjson: каталог применён вопреки отказу: %+v", rep)
		}
		snap, err = app.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("снимок: %v", err)
		}
		if _, ok := snap.Kind(domain.Kind("sensor")); ok {
			t.Fatal("ndjson: отказ оставил расширение каталога (класс sensor)")
		}
	})

	t.Run("NDJSONCatalogAndRecordInOneLine", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "drop.ndjson")
		write(t, file,
			`{"catalog": {"units": [ { "code": "kV" } ]}, "transistors": [ { "name": "КТ315Б" } ]}`+"\n")
		rep := importFile(t, app, file, false)
		// Обёртка «catalog + класс» — отвергнутая запись для сводки и
		// гейта смешанного файла; проблема формы строки каталога портит
		// каталог целиком (как проблема формы секции catalog документа).
		if !rep.HasIssues() || rep.Records != 1 || rep.Rejected != 1 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		want := "line 1: the wrapper line must contain exactly one key — a kind or catalog"
		if rep.Issues[0].String() != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].String(), want)
		}
		if rep.CatalogApplied {
			t.Fatalf("каталог применён: %+v", rep)
		}
		snap, err := app.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("снимок: %v", err)
		}
		if _, ok := snap.Unit("kV"); ok {
			t.Fatal("единица kV введена из проблемной строки")
		}
		if n, err := app.Services().Devices.Count(context.Background(), nil); err != nil || n != 0 {
			t.Fatalf("count: %v %d", err, n)
		}
	})

	t.Run("NDJSONCatalogImportWrapperIsMixedFile", func(t *testing.T) {
		app := openApp(t, factory)
		// catalog import: каталог чистой строки + обёртка с классовым
		// ключом — файл смешанный, отказ до применения каталога; запись
		// обёртки не теряется молча (видна гейту и сводке).
		file := filepath.Join(t.TempDir(), "mixed.ndjson")
		write(t, file,
			`{"catalog": {"units": [ { "code": "kV" } ]}}`+"\n"+
				`{"catalog": {"units": [ { "code": "GV" } ]}, "transistors": [ { "name": "КТ315Б" } ]}`+"\n")
		rep, err := importCatalogFile(t, app, file, false)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeInvalidImportFile ||
			de.MsgID != domain.MsgImportCatalogRecordsMixed {
			t.Fatalf("ожидалась invalid_import_file (records mixed), получено %v", err)
		}
		if rep.Records != 1 || rep.Rejected != 1 {
			t.Fatalf("итог: %+v", rep)
		}
		if rep.CatalogApplied {
			t.Fatalf("каталог применён вопреки отказу: %+v", rep)
		}
		snap, err := app.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("снимок: %v", err)
		}
		if _, ok := snap.Unit("kV"); ok {
			t.Fatal("отказ оставил расширение каталога (единица kV)")
		}
		if n, err := app.Services().Devices.Count(context.Background(), nil); err != nil || n != 0 {
			t.Fatalf("count: %v %d", err, n)
		}
	})

	t.Run("NDJSONCleanCatalogPlusWrapperTaintsCatalog", func(t *testing.T) {
		app := openApp(t, factory)
		// Обычный import: чистая строка catalog + обёртка портят каталог
		// целиком (он не применяется), чистые записи применяются по
		// текущему снимку.
		file := filepath.Join(t.TempDir(), "taint.ndjson")
		write(t, file,
			`{"catalog": {"units": [ { "code": "kV" } ]}}`+"\n"+
				`{"catalog": {"units": [ { "code": "GV" } ]}, "transistors": [ { "name": "КТ315Б" } ]}`+"\n"+
				`{"transistors": { "name": "КТ361А" }}`+"\n")
		rep := importFile(t, app, file, false)
		if !rep.HasIssues() || rep.Records != 2 || rep.Rejected != 1 || rep.Added != 1 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		if rep.CatalogApplied {
			t.Fatalf("каталог применён: %+v", rep)
		}
		snap, err := app.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("снимок: %v", err)
		}
		if _, ok := snap.Unit("kV"); ok {
			t.Fatal("каталог применён вопреки испорченной обёртке")
		}
		if n, err := app.Services().Devices.Count(context.Background(), nil); err != nil || n != 1 {
			t.Fatalf("count: %v %d", err, n)
		}
	})

	t.Run("DeactivatedParameterSkippedReimport", func(t *testing.T) {
		app := openApp(t, factory)
		// Деактивация параметра делает значения существующих записей
		// «неактивными»: побайтово тот же повторный импорт обязан
		// оставаться Skipped (сравнение до валидации), запись в БД нет.
		file := filepath.Join(t.TempDir(), "dev.ndjson")
		write(t, file, `{"transistors": { "name": "КТ315Б", "ratings": [ { "parameter": "VceoMax", "value": 20 } ] }}`+"\n")
		rep := importFile(t, app, file, false)
		if rep.Added != 1 || rep.Skipped != 0 {
			t.Fatalf("первичный импорт: %+v", rep)
		}
		cat := filepath.Join(t.TempDir(), "deact.jsonc")
		write(t, cat, `{"catalog": {"parameters": [ { "code": "VceoMax", "group": "limiting",
	"unit": "V", "value_type": "exact", "is_active": false } ]}}`)
		if rep, err := importCatalogFile(t, app, cat, false); err != nil || rep.CatalogApplied != true {
			t.Fatalf("деактивация: %v %+v", err, rep)
		}
		rep2 := importFile(t, app, file, false)
		if rep2.Records != 1 || rep2.Skipped != 1 || rep2.Rejected != 0 {
			t.Fatalf("повторный импорт после деактивации: %+v; проблемы: %v", rep2, issueMessages(rep2.Issues))
		}
	})

	t.Run("NDJSONInvalidTailRejected", func(t *testing.T) {
		app := openApp(t, factory)
		// Хвост после полного значения строки — жёсткая ошибка
		// синтаксиса, запись не принимается молча.
		for _, line := range []string{
			`{"transistors": { "name": "МП39" }} xyz`,
			`{"transistors": { "name": "МП39" }}}`,
		} {
			file := filepath.Join(t.TempDir(), "tail.ndjson")
			write(t, file, line+"\n")
			_, err := importFileErr(t, app, file, false)
			de, ok := domain.AsError(err)
			if !ok || de.Code != domain.CodeInvalidImportFile {
				t.Fatalf("ожидалась invalid_import_file для %q, получено %v", line, err)
			}
			if n, err := app.Services().Devices.Count(context.Background(), nil); err != nil || n != 0 {
				t.Fatalf("count: %v %d", err, n)
			}
		}
	})

	t.Run("CatalogFormIssuesBlockApplication", func(t *testing.T) {
		app := openApp(t, factory)
		for _, tc := range []struct {
			name string
			body string
			want string
		}{
			{
				name: "unit не строка",
				body: `{"catalog": {"conditions": [ { "code": "ta", "unit": 5 } ]}}`,
				want: `catalog: section conditions, «ta»: field "unit" must be a string`,
			},
			{
				name: "catalog не объект",
				body: `{"catalog": []}`,
				want: `catalog: catalog must be an object with subsections (allowed: kinds, ` +
					`designation_systems, designation_system_kinds, units, ` +
					`categories, conditions, parameter_groups, parameters, attributes, ` +
					`validation_rules, kind_validation_rules)`,
			},
			{
				name: "битая строка среди валидных",
				body: `{"catalog": {"conditions": [ { "code": "ta" }, { "code": "tb", "unit": 5 } ]}}`,
				want: `catalog: section conditions, «tb»: field "unit" must be a string`,
			},
			{
				name: "sort_order вне диапазона целых",
				body: `{"catalog": {"parameter_groups": [ { "code": "gx", "section": "envx", "sort_order": 1e19 } ]}}`,
				want: `catalog: section parameter_groups, «gx»: field "sort_order" must be an integer`,
			},
			{
				name: "sort_order дробный",
				body: `{"catalog": {"parameter_groups": [ { "code": "gx", "section": "envx", "sort_order": 1.5 } ]}}`,
				want: `catalog: section parameter_groups, «gx»: field "sort_order" must be an integer`,
			},
		} {
			catRev, _, err := app.Revisions(context.Background())
			if err != nil {
				t.Fatalf("ревизии: %v", err)
			}
			file := filepath.Join(t.TempDir(), "form.jsonc")
			write(t, file, tc.body)
			rep, err := importCatalogFile(t, app, file, false)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if !rep.HasIssues() || rep.CatalogApplied {
				t.Fatalf("%s: итог %+v; проблемы: %v", tc.name, rep, issueMessages(rep.Issues))
			}
			if rep.Issues[0].String() != tc.want {
				t.Fatalf("%s: текст: %q, ожидался %q", tc.name, rep.Issues[0].String(), tc.want)
			}
			snap, err := app.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("%s: снимок: %v", tc.name, err)
			}
			if _, ok := snap.Condition("ta"); ok {
				t.Fatalf("%s: секция применена частично (условие ta)", tc.name)
			}
			if _, ok := snap.Condition("tb"); ok {
				t.Fatalf("%s: секция применена (условие tb)", tc.name)
			}
			catRev2, _, err := app.Revisions(context.Background())
			if err != nil {
				t.Fatalf("%s: ревизии: %v", tc.name, err)
			}
			if catRev2 != catRev {
				t.Fatalf("%s: catalog_revision двинулась %d→%d", tc.name, catRev, catRev2)
			}
		}
	})

	t.Run("CatalogConditionSetTypoBlocked", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "typo.jsonc")
		write(t, file, `{"catalog": {"parameters": [ {
			"code": "vib", "group": "env", "value_type": "text",
			"condition_sets": [ { "items": [
				{ "condition": "ta", "mode": "required", "fxed_value": 0.1 }
			] } ]
		} ] }}`)
		rep, err := importCatalogFile(t, app, file, false)
		if err != nil {
			t.Fatalf("импорт: %v", err)
		}
		if !rep.HasIssues() || rep.CatalogApplied {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		want := "catalog: section parameters, «vib»: unknown field «fxed_value» " +
			"(allowed: condition, mode, fixed_value)"
		if rep.Issues[0].String() != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].String(), want)
		}
		snap, err := app.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("снимок: %v", err)
		}
		if _, ok := snap.Parameter("vib"); ok {
			t.Fatal("параметр с опечаткой применён")
		}
	})

	t.Run("IdleCreationCompensatedOnLaterFailure", func(t *testing.T) {
		app := openApp(t, factory)
		// Взаимные аналоги: цикл разрывается холостым созданием Д226;
		// полное применение после создания проваливается не-аналоговой
		// ошибкой (дубликат цели в списке аналогов) — холостая запись
		// компенсируется удалением, в отчёте проблема.
		file := filepath.Join(t.TempDir(), "deadlock.jsonc")
		write(t, file, `{
			"diodes": [
				{ "name": "Д226", "system": "other", "analogs": ["1N4007", "1N4007"] },
				{ "name": "1N4007", "analogs": ["Д226"] }
			]
		}`)
		rep := importFile(t, app, file, false)
		if rep.Added != 1 || rep.Rejected != 1 || len(rep.Issues) != 1 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		want := "record «Д226»: analog «1N4007» is set more than once"
		if rep.Issues[0].String() != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].String(), want)
		}
		if _, found, err := app.Services().Devices.Get(context.Background(), domain.KindDiode, "Д226"); err != nil || found {
			t.Fatalf("холостое создание не компенсировано: found=%v err=%v", found, err)
		}
		if _, found, err := app.Services().Devices.Get(context.Background(), domain.KindDiode, "1N4007"); err != nil || !found {
			t.Fatalf("валидная запись не применена: found=%v err=%v", found, err)
		}
	})

	t.Run("NDJSONCatalogMergeSoftIssues", func(t *testing.T) {
		app := openApp(t, factory)
		cases := []struct {
			name string
			line string
			want string
		}{
			{
				name: "вторая строка catalog не объект",
				line: `{"catalog": 5}`,
				want: "line 2: catalog must be an object with subsections",
			},
			{
				name: "подраздел не массив",
				line: `{"catalog": {"units": 5}}`,
				want: "line 2: catalog subsection «units» must be an array in every catalog line",
			},
		}
		for _, tc := range cases {
			file := filepath.Join(t.TempDir(), "merge.ndjson")
			write(t, file,
				`{"catalog": {"units": [ { "code": "kV" } ]}}`+"\n"+tc.line+"\n")
			rep := importFile(t, app, file, false)
			// Ошибка слияния — мягкая проблема строки, не жёсткий abort:
			// прогона без применения каталога достаточно.
			if !rep.HasIssues() || rep.CatalogApplied {
				t.Fatalf("%s: итог: %+v; проблемы: %v", tc.name, rep, issueMessages(rep.Issues))
			}
			if rep.Issues[0].String() != tc.want {
				t.Fatalf("%s: текст: %q, ожидался %q", tc.name, rep.Issues[0].String(), tc.want)
			}
			snap, err := app.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("%s: снимок: %v", tc.name, err)
			}
			if _, ok := snap.Unit("kV"); ok {
				t.Fatalf("%s: каталог применён вопреки проблеме слияния", tc.name)
			}
		}
	})

	t.Run("ExportOrderStableAcrossBatches", func(t *testing.T) {
		app := openApp(t, factory)
		// Больше одной пачки выгрузки (200): порядок (kind, designation)
		// и полнота не зависят от разбивки на пачки.
		const total = 260
		var b strings.Builder
		for i := 1000; i < 1000+total; i++ {
			fmt.Fprintf(&b, `{"transistors": { "name": "2N%d" }}`+"\n", i)
		}
		file := filepath.Join(t.TempDir(), "bulk.ndjson")
		write(t, file, b.String())
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != total {
			t.Fatalf("импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, FormatNDJSON, nil); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		var names []string
		sc := newNDJSONScanner(strings.NewReader(buf.String()))
		for {
			line, _, ok, err := sc.next()
			if err != nil {
				t.Fatalf("чтение: %v", err)
			}
			if !ok {
				break
			}
			v, err := parseLineJSON(line, 0)
			if err != nil {
				t.Fatalf("разбор: %v", err)
			}
			m := v.members[0]
			nameVal, _ := m.value.has("name")
			names = append(names, nameVal.str)
		}
		if len(names) != total {
			t.Fatalf("полнота: выгружено %d из %d", len(names), total)
		}
		for i := 1; i < len(names); i++ {
			if names[i-1] >= names[i] {
				t.Fatalf("порядок нарушен на %d: %q >= %q", i, names[i-1], names[i])
			}
		}
		var buf2 bytes.Buffer
		if err := New(app).Export(context.Background(), &buf2, FormatNDJSON, nil); err != nil {
			t.Fatalf("повторный экспорт: %v", err)
		}
		if buf.String() != buf2.String() {
			t.Fatal("экспорт недетерминирован")
		}
	})

	t.Run("ExportRoundTripAllFormats", func(t *testing.T) {
		app := openApp(t, factory)
		importFile(t, app, dataPath(t, "transistors.jsonc"), false)
		importFile(t, app, dataPath(t, "resistors.jsonc"), false)
		importFile(t, app, dataPath(t, "capacitors.jsonc"), false)

		for _, format := range []Format{FormatJSONC, FormatYAML, FormatNDJSON} {
			var buf bytes.Buffer
			if err := New(app).Export(context.Background(), &buf, format, nil); err != nil {
				t.Fatalf("экспорт %s: %v", format, err)
			}
			name := "export." + string(format)
			if format == FormatJSONC {
				name = "export.jsonc"
			}
			file := filepath.Join(t.TempDir(), name)
			write(t, file, buf.String())

			rep := importFile(t, app, file, false)
			if rep.HasIssues() || rep.Skipped != rep.Records {
				t.Fatalf("round-trip %s: %+v; %v", format, rep, issueMessages(rep.Issues))
			}
			// Повторный экспорт байтово совпадает: детерминированность.
			var buf2 bytes.Buffer
			if err := New(app).Export(context.Background(), &buf2, format, nil); err != nil {
				t.Fatalf("повторный экспорт %s: %v", format, err)
			}
			if buf.String() != buf2.String() {
				t.Fatalf("экспорт %s недетерминирован", format)
			}
		}
	})

	t.Run("NotesTextListRoundTrip", func(t *testing.T) {
		// Заметки (notes, text_list): массив строк в наполнении каждого
		// формата реестра, экспорт пишет массивом, повторный импорт —
		// Skipped по всем записям.
		sources := map[Format]string{
			FormatJSONC: `{
			"resistors": [
				{ "name": "С2-33Н", "attributes": {
					"notes": ["снижение мощности при нагреве", "серия 05"] } }
			]
		}`,
			FormatYAML: "resistors:\n" +
				"  - name: С2-33Н\n" +
				"    attributes:\n" +
				"      notes:\n" +
				"        - снижение мощности при нагреве\n" +
				"        - серия 05\n",
			FormatNDJSON: `{"resistors": { "name": "С2-33Н", "attributes": {` +
				` "notes": ["снижение мощности при нагреве", "серия 05"] } }}` + "\n",
		}
		for _, format := range []Format{FormatJSONC, FormatYAML, FormatNDJSON} {
			app := openApp(t, factory)
			file := filepath.Join(t.TempDir(), "notes."+string(format))
			write(t, file, sources[format])
			rep := importFile(t, app, file, false)
			if rep.HasIssues() || rep.Added != 1 {
				t.Fatalf("импорт %s: %+v; %v", format, rep, issueMessages(rep.Issues))
			}
			var buf bytes.Buffer
			if err := New(app).Export(context.Background(), &buf, format, nil); err != nil {
				t.Fatalf("экспорт %s: %v", format, err)
			}
			exported := buf.String()
			if format == FormatJSONC {
				// Массив заметок — построчный формат экспорта: проверка
				// структурной метки и элементов (склейка одной строкой — потеря).
				if !strings.Contains(exported, `"notes": [`) ||
					!strings.Contains(exported, `"снижение мощности при нагреве"`) ||
					!strings.Contains(exported, `"серия 05"`) {
					t.Fatalf("экспорт не содержит заметки массивом: %s", exported)
				}
			}
			file2 := filepath.Join(t.TempDir(), "notes-export."+string(format))
			write(t, file2, exported)
			rep = importFile(t, app, file2, false)
			if rep.HasIssues() || rep.Skipped != rep.Records || rep.Records != 1 {
				t.Fatalf("повторный импорт %s: %+v; %v", format, rep, issueMessages(rep.Issues))
			}
		}
	})

	t.Run("CatalogExportRoundTrip", func(t *testing.T) {
		app := openApp(t, factory)
		for _, format := range []Format{FormatJSONC, FormatYAML, FormatNDJSON} {
			var buf bytes.Buffer
			if err := New(app).ExportCatalog(context.Background(), &buf, format); err != nil {
				t.Fatalf("экспорт каталога %s: %v", format, err)
			}
			name := "catalog-export." + string(format)
			if format == FormatJSONC {
				name = "catalog-export.jsonc"
			}
			file := filepath.Join(t.TempDir(), name)
			write(t, file, buf.String())

			f, err := os.Open(file)
			if err != nil {
				t.Fatalf("открытие: %v", err)
			}
			rep, err := New(app).ImportCatalogFile(context.Background(), f, file, false)
			f.Close()
			if err != nil {
				t.Fatalf("импорт каталога %s: %v", format, err)
			}
			if rep.HasIssues() || rep.Records != 0 {
				t.Fatalf("импорт каталога %s: %+v; %v", format, rep, issueMessages(rep.Issues))
			}
			var buf2 bytes.Buffer
			if err := New(app).ExportCatalog(context.Background(), &buf2, format); err != nil {
				t.Fatalf("повторный экспорт каталога %s: %v", format, err)
			}
			if !strings.EqualFold(buf.String(), buf2.String()) && buf.String() != buf2.String() {
				t.Fatalf("экспорт каталога %s недетерминирован", format)
			}
		}
	})

	t.Run("ValuesAndConditionsRoundTrip", func(t *testing.T) {
		app := openApp(t, factory)
		// Значения всех форм и условия сохраняются экспортом без потерь.
		importFile(t, app, dataPath(t, "transistors.jsonc"), false)
		importFile(t, app, dataPath(t, "diodes.jsonc"), false)
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, FormatJSONC, nil); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, "КТ315Б")
		if err != nil || !found {
			t.Fatalf("карточка: %v", err)
		}
		var hfe *struct{}
		for _, g := range card.Groups {
			for _, v := range g.Values {
				if v.Parameter == "hfe" {
					if v.Min == nil || *v.Min != 50 || v.Max == nil || *v.Max != 350 {
						t.Fatalf("hfe: %+v", v)
					}
					if len(v.Conditions) != 2 {
						t.Fatalf("условия hfe: %+v", v.Conditions)
					}
					hfe = &struct{}{}
				}
			}
		}
		if hfe == nil {
			t.Fatal("hfe не найден в карточке")
		}
	})

	t.Run("ClassificationFieldsRoundTrip", func(t *testing.T) {
		app := openApp(t, factory)
		importFile(t, app, dataPath(t, "transistors.jsonc"), false)
		// Явные классификационные поля: карточка, сквозной поиск по коду,
		// round-trip экспорта и идемпотентность повторного импорта.
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, "MJE340")
		if err != nil || !found {
			t.Fatalf("карточка MJE340: %v (found=%v)", err, found)
		}
		f, ok := card.FieldByName("material")
		if !ok || f.Text != "si" {
			t.Fatalf("material MJE340: %v", f)
		}
		if f, ok := card.FieldByName("subclass"); !ok || f.Text != "bjt" {
			t.Fatalf("subclass MJE340: %v", f)
		}
		// gost-запись: подкласс — продукт парсера (код словаря).
		card, found, err = app.Services().Devices.Get(context.Background(), domain.KindTransistor, "КТ315Б")
		if err != nil || !found {
			t.Fatalf("карточка КТ315Б: %v (found=%v)", err, found)
		}
		if f, ok := card.FieldByName("subclass"); !ok || f.Text != "bjt" {
			t.Fatalf("subclass КТ315Б: %v", f)
		}
		// Сквозной фильтр: other и gost в одном запросе по коду словаря.
		page, err := app.Services().Devices.Search(context.Background(), service.SearchQuery{
			Kind:   domain.KindTransistor,
			Fields: []service.FieldFilter{{Field: "subclass", Text: "bjt"}},
			Limit:  200,
		})
		if err != nil {
			t.Fatalf("поиск: %v", err)
		}
		names := map[string]bool{}
		for _, item := range page.Items {
			names[item.Designation] = true
		}
		if !names["КТ315Б"] || !names["MJE340"] || !names["2N2222A"] || names["IRF540"] {
			t.Fatalf("subclass=bjt: %v", names)
		}
		// Экспорт пишет секцию fields, повторный импорт — без изменений.
		kind := domain.KindTransistor
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, FormatJSONC, &kind); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		if !strings.Contains(buf.String(), `"fields": {`) {
			t.Fatalf("экспорт: секция fields отсутствует:\n%s", buf.String())
		}
		file := filepath.Join(t.TempDir(), "export.jsonc")
		write(t, file, buf.String())
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 0 || rep.Updated != 0 {
			t.Fatalf("повторный импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	// Синонимы: импорт секции, find по историческому названию,
	// round-trip экспорта и идемпотентность повторного импорта.
	t.Run("SynonymsRoundTrip", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "synonyms.jsonc")
		write(t, file, `{
  "resistors": [
    {
      "name": "С5-35",
      "synonyms": [
        "ПЭВ",
        { "name": "ПЭВР", "note": "обозначение до 1968", "yearTo": 1968 }
      ]
    }
  ]
}`)
		rep := importFile(t, app, file, false)
		if rep.HasIssues() || rep.Added != 1 {
			t.Fatalf("импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
		res, err := app.Services().Devices.Find(context.Background(), "", "ПЭВ")
		if err != nil || res.Found == nil ||
			res.Found.Designation != "С5-35" || res.MatchedBySynonym != "ПЭВ" {
			t.Fatalf("find по синониму: err=%v found=%v matched=%q", err, res.Found, res.MatchedBySynonym)
		}
		kind := domain.KindResistor
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, FormatJSONC, &kind); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		exported := buf.String()
		if !strings.Contains(exported, `"synonyms": [`) ||
			!strings.Contains(exported, `"name": "ПЭВР"`) ||
			!strings.Contains(exported, `"yearTo": 1968`) {
			t.Fatalf("экспорт: секция synonyms отсутствует или неполна:\n%s", exported)
		}
		reimport := filepath.Join(t.TempDir(), "reexport.jsonc")
		write(t, reimport, exported)
		rep = importFile(t, app, reimport, false)
		if rep.HasIssues() || rep.Added != 0 || rep.Updated != 0 {
			t.Fatalf("повторный импорт: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	// Конфликты уникальности синонимов внутри одного файла видны dry-run'у
	// (заявки ранее проверенных записей) и реальному импорту одинаково.
	t.Run("SynonymConflictsSameFile", func(t *testing.T) {
		app := openApp(t, factory)
		dup := filepath.Join(t.TempDir(), "synonym-dup.jsonc")
		write(t, dup, `{
  "resistors": [
    { "name": "С5-35", "synonyms": ["ПЭВ"] },
    { "name": "С5-37", "synonyms": ["ПЭВ"] }
  ]
}`)
		for _, dry := range []bool{true, false} {
			rep := importFile(t, app, dup, dry)
			if rep.Rejected != 1 || rep.Added != 1 || rep.Records != 2 {
				t.Fatalf("конфликт синонима (dry=%v): %+v; %v", dry, rep, issueMessages(rep.Issues))
			}
			if !containsMessage(issueMessages(rep.Issues),
				"record «С5-37»: synonym «ПЭВ» is already assigned to record «С5-35»") {
				t.Fatalf("текст конфликта (dry=%v): %v", dry, issueMessages(rep.Issues))
			}
		}

		// Синоним, совпадающий с обозначением записи этого же файла,
		// и запись под чужой заявкой синонима — отказ в одной и той же
		// записи в обоих режимах (последовательная симуляция dry-run:
		// С5-16МВ применяется, МЛТ отвергается — как в реальном прогоне).
		shadow := filepath.Join(t.TempDir(), "synonym-shadow.jsonc")
		write(t, shadow, `{
  "resistors": [
    { "name": "С5-16МВ", "synonyms": ["МЛТ"] },
    { "name": "МЛТ", "system": "other" }
  ]
}`)
		for _, dry := range []bool{true, false} {
			rep := importFile(t, app, shadow, dry)
			if rep.Rejected != 1 || rep.Added != 1 {
				t.Fatalf("затенение синонима (dry=%v): %+v; %v", dry, rep, issueMessages(rep.Issues))
			}
			if !containsMessage(issueMessages(rep.Issues),
				"record «МЛТ»: designation «МЛТ» is taken by a synonym of record «С5-16МВ»") {
				t.Fatalf("текст затенения (dry=%v): %v", dry, issueMessages(rep.Issues))
			}
		}

		// Отвергнутая запись не заявляет синонимы: её конфликт не мешает
		// корректной записи того же файла (годы в обратном порядке —
		// запись С5-42В отвергнута целиком, заявка ПЭВТ не регистрируется).
		after := filepath.Join(t.TempDir(), "synonym-after.jsonc")
		write(t, after, `{
  "resistors": [
    { "name": "С5-42В", "synonyms": [ { "name": "ПЭВТ", "yearFrom": 1975, "yearTo": 1968 } ] },
    { "name": "С5-47В", "synonyms": ["ПЭВТ"] }
  ]
}`)
		rep := importFile(t, app, after, true)
		if rep.Rejected != 1 || rep.Added != 1 {
			t.Fatalf("порядок заявок (dry-run): %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	// Прогресс импорта: колбэк WithProgress наблюдает построчное
	// чтение NDJSON и итоговые исходы применения (включая dry-run);
	// документные форматы отмечают чтение одним вызовом после разбора.
	t.Run("ProgressCallback", func(t *testing.T) {
		app := openApp(t, factory)
		ndjson := "{\"transistors\": {\"name\": \"КТ315Б\"}}\n" +
			"{\"transistors\": {\"name\": \"BC547B\"}}\n" +
			"{\"transistors\": {\"name\": \"МП39\", \"system\": \"other\"}}\n"
		var snap []Progress
		rep, err := New(app).Import(context.Background(),
			strings.NewReader(ndjson), "progress.ndjson", false,
			WithProgress(func(p Progress) { snap = append(snap, p) }))
		if err != nil || rep.HasIssues() || rep.Added != 3 {
			t.Fatalf("импорт: err=%v %+v; %v", err, rep, issueMessages(rep.Issues))
		}
		want := []Progress{
			{Read: 1, Line: 1},
			{Read: 2, Line: 2},
			{Read: 3, Line: 3},
			{Read: 3, Applied: 1, Line: 3},
			{Read: 3, Applied: 2, Line: 3},
			{Read: 3, Applied: 3, Line: 3},
		}
		if !slices.Equal(snap, want) {
			t.Fatalf("снимки прогресса: %+v, ожидались %+v", snap, want)
		}

		// Dry-run: чтение и исходы те же, записи в базу нет.
		snap = nil
		rep, err = New(app).Import(context.Background(),
			strings.NewReader(ndjson), "progress.ndjson", true,
			WithProgress(func(p Progress) { snap = append(snap, p) }))
		if err != nil || rep.HasIssues() || rep.Skipped != 3 {
			t.Fatalf("dry-run: err=%v %+v; %v", err, rep, issueMessages(rep.Issues))
		}
		if len(snap) != 6 || snap[len(snap)-1] != (Progress{Read: 3, Applied: 3, Line: 3}) {
			t.Fatalf("снимки dry-run: %+v", snap)
		}

		// Документ (jsonc): чтение — один вызов, далее — исходы записей.
		snap = nil
		rep, err = New(app).Import(context.Background(),
			strings.NewReader(`{"transistors": ["КТ315Б", "BC547B"]}`),
			"progress.jsonc", false,
			WithProgress(func(p Progress) { snap = append(snap, p) }))
		if err != nil || rep.HasIssues() || rep.Skipped != 2 {
			t.Fatalf("документ: err=%v %+v; %v", err, rep, issueMessages(rep.Issues))
		}
		want = []Progress{
			{Read: 2},
			{Read: 2, Applied: 1},
			{Read: 2, Applied: 2},
		}
		if !slices.Equal(snap, want) {
			t.Fatalf("снимки документа: %+v, ожидались %+v", snap, want)
		}

		// Прямая ссылка аналога на позднейшую запись (requeue без исхода
		// не считается) и запись, отвергнутая чтением (в Read входит,
		// в Applied — нет).
		snap = nil
		rep, err = New(app).Import(context.Background(), strings.NewReader(
			"{\"diodes\": {\"name\": \"Д226\", \"system\": \"other\", \"analogs\": [\"1N4007\"]}}\n"+
				"{\"diodes\": {\"name\": \"1N4007\"}}\n"+
				"{\"diodes\": 42}\n"),
			"progress.ndjson", false,
			WithProgress(func(p Progress) { snap = append(snap, p) }))
		if err != nil || rep.Added != 2 || rep.Rejected != 1 {
			t.Fatalf("аналоги/отказ: err=%v %+v; %v", err, rep, issueMessages(rep.Issues))
		}
		want = []Progress{
			{Read: 1, Line: 1},
			{Read: 2, Line: 2},
			{Read: 3, Line: 3},
			{Read: 3, Applied: 1, Line: 3},
			{Read: 3, Applied: 2, Line: 3},
		}
		if !slices.Equal(snap, want) {
			t.Fatalf("снимки аналогов: %+v, ожидались %+v", snap, want)
		}
	})
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись %s: %v", path, err)
	}
}

// Строка NDJSON сверх потолка сканера (16 МиБ) — доменная ошибка
// invalid_import_file (REST — 400): проблема файла, а не инфраструктуры.
func TestNDJSONLineTooLongIsDomainError(t *testing.T) {
	app := openApp(t, memoryConfig)
	long := strings.Repeat("a", 17*1024*1024)
	_, err := New(app).Import(context.Background(),
		strings.NewReader("{\"transistors\": {\"name\": \""+long+"\"}}"),
		"big.ndjson", false)
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ошибка: %v (domain=%+v)", err, de)
	}
}

func importCatalogFile(t *testing.T, app *service.App, path string, dryRun bool) (Report, error) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открытие %s: %v", path, err)
	}
	defer f.Close()
	return New(app).ImportCatalogFile(context.Background(), f, path, dryRun)
}

// Неразбираемая запись (эволюция каталога/грамматики) не прерывает
// экспорт: секция fields записи пропускается, остальные записи и секции
// выгружаются; жёсткий отказ — только ошибки чтения БД.
func TestExportSkipsUnparseableRecordFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	app := openApp(t, func(t *testing.T) service.Config {
		t.Helper()
		return service.Config{Dialect: "sqlite", DSN: path, EnsureCreated: true}
	})
	file := filepath.Join(t.TempDir(), "good.jsonc")
	write(t, file, `{"transistors": [ {
		"name": "MJE340", "system": "other",
		"fields": { "material": "si", "subclass": "bjt" }
	} ]}`)
	rep := importFile(t, app, file, false)
	if rep.HasIssues() || rep.Added != 1 {
		t.Fatalf("импорт: %+v; %v", rep, issueMessages(rep.Issues))
	}

	// Напрямую через storage — запись с системой gost и обозначением,
	// не разбираемым грамматикой: текущим каталогом не разбирается.
	db, err := storage.Open(context.Background(), storage.Config{Dialect: "sqlite", DSN: path})
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	if _, _, err := tx.InsertDevice(context.Background(),
		domain.KindTransistor, domain.SystemGost, "ЪЪЪ5"); err != nil {
		t.Fatalf("вставка: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	var buf bytes.Buffer
	if err := New(app).Export(context.Background(), &buf, FormatJSONC, nil); err != nil {
		t.Fatalf("экспорт: %v", err)
	}
	root, err := parseJSONC(buf.Bytes())
	if err != nil {
		t.Fatalf("разбор выгрузки: %v", err)
	}
	sec, _ := root.has("transistors")
	if len(sec.items) != 2 {
		t.Fatalf("выгружено записей: %d, ожидалось 2:\n%s", len(sec.items), buf.String())
	}
	exported := map[string]value{}
	for _, item := range sec.items {
		nameVal, _ := item.has("name")
		exported[nameVal.str] = item
	}
	if _, ok := exported["MJE340"]; !ok {
		t.Fatalf("разбираемая запись потеряна:\n%s", buf.String())
	}
	broken := exported["ЪЪЪ5"]
	if _, has := broken.has("fields"); has {
		t.Fatalf("неразбираемая запись выгружена с fields:\n%s", buf.String())
	}
	if sysVal, _ := broken.has("system"); sysVal.str != string(domain.SystemGost) {
		t.Fatalf("система неразбираемой записи: %v", sysVal)
	}
}
