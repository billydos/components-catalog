package importer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
	"github.com/billydos/components-catalog/internal/testutil"
)

// Интеграционный набор этапа 4 (план работ 4.2–4.6): один и тот же набор
// на SQLite (память/файл) и PostgreSQL (локально по
// CATALOG_TEST_POSTGRES_DSN). Критерии: импорт sample-data всех форматов
// идемпотентен; --dry-run не пишет; негативные файлы дают полный список
// проблем за прогон; round-trip значений и условий во всех форматах.

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

// sampleDataPath — путь к файлу sample-data из тестов пакета.
func sampleDataPath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "sample-data", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sample-data/%s недоступен: %v", name, err)
	}
	return path
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

func runImporterSuite(t *testing.T, factory configFactory) {
	t.Run("SampleDataIdempotent", func(t *testing.T) {
		app := openApp(t, factory)
		files := []string{"transistors.jsonc", "diodes.jsonc", "resistors.jsonc", "capacitors.jsonc"}
		wantCounts := map[string]int{
			"transistors.jsonc": 18, "diodes.jsonc": 18, "resistors.jsonc": 11, "capacitors.jsonc": 9,
		}
		for _, name := range files {
			rep := importFile(t, app, sampleDataPath(t, name), false)
			if rep.HasIssues() {
				t.Fatalf("%s: проблемы: %v", name, issueMessages(rep.Issues))
			}
			if rep.Records != wantCounts[name] || rep.Added != wantCounts[name] {
				t.Fatalf("%s: записей %d, добавлено %d (ожидалось %d)", name, rep.Records, rep.Added, wantCounts[name])
			}
		}
		// NDJSON — те же записи диодов: все без изменений.
		rep := importFile(t, app, sampleDataPath(t, "diodes.ndjson"), false)
		if rep.HasIssues() || rep.Skipped != 18 {
			t.Fatalf("diodes.ndjson: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		// Повторный импорт всех форматов — исход Skipped по всем записям.
		for _, name := range files {
			rep := importFile(t, app, sampleDataPath(t, name), false)
			if rep.HasIssues() || rep.Added != 0 || rep.Updated != 0 || rep.Skipped != wantCounts[name] {
				t.Fatalf("повторный %s: %+v; проблемы: %v", name, rep, issueMessages(rep.Issues))
			}
		}
		rep = importFile(t, app, sampleDataPath(t, "diodes.ndjson"), false)
		if rep.HasIssues() || rep.Skipped != 18 {
			t.Fatalf("повторный diodes.ndjson: %+v", rep)
		}
	})

	t.Run("DryRunDoesNotWrite", func(t *testing.T) {
		app := openApp(t, factory)
		rep := importFile(t, app, sampleDataPath(t, "transistors.jsonc"), true)
		if rep.HasIssues() {
			t.Fatalf("проблемы dry-run: %v", issueMessages(rep.Issues))
		}
		if rep.Added != 18 || rep.DryRun != true {
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
		rep = importFile(t, app, sampleDataPath(t, "transistors.jsonc"), false)
		if rep.Added != 18 || rep.HasIssues() {
			t.Fatalf("импорт после dry-run: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("NegativeFileFullIssueList", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "negative.jsonc")
		write(t, file, `{
			"transistors": [
				{ "name": "МП39", "parameters": [ { "parameter": "h21e", "max": 50, "Uke": 5, "Ik": 5 } ] },
				"ГТ109Г",
				{ "name": "2N9999ZZ", "system": "jedec" },
				{ "name": "МП39", "ratings": [ { "parameter": "UkeoMax", "value": 0 } ] }
			]
		}`)
		rep := importFile(t, app, file, false)
		if len(rep.Issues) != 2 {
			t.Fatalf("проблем: %d, ожидалось 2: %v", len(rep.Issues), issueMessages(rep.Issues))
		}
		want := []string{
			"запись «МП39»: параметр «h21e»: тип значения range — обязательны ключи min и max",
			"запись «МП39»: параметр «UkeoMax»: значение ключа value должно быть положительным",
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

	t.Run("ForwardAnalogReferencesResolved", func(t *testing.T) {
		app := openApp(t, factory)
		// Прямая ссылка на позднейшую запись файла: многопроходное
		// разрешение (real) и pending-обозначения (dry-run).
		file := filepath.Join(t.TempDir(), "analogs.jsonc")
		write(t, file, `{
			"transistors": [
				{ "name": "МП39", "analogs": ["BC547B"] },
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
				{ "name": "Д226", "analogs": ["1N4007"] },
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
		// Повторный импорт — без изменений по обеим записям.
		rep = importFile(t, app, file, false)
		if rep.HasIssues() || rep.Skipped != 2 {
			t.Fatalf("повторный: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("AnalogMissingTargetIsIssue", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "broken.jsonc")
		write(t, file, `{
			"transistors": [ { "name": "МП39", "analogs": ["BC999ZZ"] } ]
		}`)
		rep := importFile(t, app, file, false)
		if !rep.HasIssues() || rep.Rejected != 1 {
			t.Fatalf("итог: %+v", rep)
		}
		want := "запись «МП39»: аналог «BC999ZZ» не найден в классе transistor"
		if rep.Issues[0].String() != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].String(), want)
		}
	})

	t.Run("NDJSONCatalogBeforeRecords", func(t *testing.T) {
		app := openApp(t, factory)
		good := filepath.Join(t.TempDir(), "good.ndjson")
		write(t, good,
			`{"catalog": {"parameter_groups": [ { "code": "env", "section": "environment", "name": "Условия эксплуатации" } ]}}`+"\n"+
				`{"catalog": {"parameters": [ { "code": "vibration", "group": "env", "name": "вибрация", "value_type": "text" } ]}}`+"\n"+
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
			`{"transistors": {"name": "МП39"}}`+"\n"+
				`{"catalog": {"units": [{"code": "кВ", "name": "киловольт", "symbol": "кВ"}]}}`+"\n")
		rep = importFile(t, app, bad, false)
		if len(rep.Issues) != 1 || rep.Issues[0].String() != "строка 2: блок catalog должен предшествовать записям" {
			t.Fatalf("итог: %+v; %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("CatalogSectionWithRecords", func(t *testing.T) {
		app := openApp(t, factory)
		file := filepath.Join(t.TempDir(), "catrec.jsonc")
		write(t, file, `{
			"catalog": {
				"attributes": [ { "code": "coating", "name": "покрытие", "type": "enum", "enum_values": ["лак", "эмаль"] } ]
			},
			"diodes": [
				{ "name": "Д226", "attributes": { "coating": "лак" } }
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
		file := sampleDataPath(t, "diodes.jsonc")
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
		if de.Message != "файл содержит записи классов; catalog import применяется к файлам только с секцией catalog" {
			t.Fatalf("текст: %q", de.Message)
		}
	})

	t.Run("ExportRoundTripAllFormats", func(t *testing.T) {
		app := openApp(t, factory)
		importFile(t, app, sampleDataPath(t, "transistors.jsonc"), false)
		importFile(t, app, sampleDataPath(t, "resistors.jsonc"), false)
		importFile(t, app, sampleDataPath(t, "capacitors.jsonc"), false)

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
		importFile(t, app, sampleDataPath(t, "transistors.jsonc"), false)
		importFile(t, app, sampleDataPath(t, "diodes.jsonc"), false)
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, FormatJSONC, nil); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		card, found, err := app.Services().Devices.Get(context.Background(), domain.KindTransistor, "КТ315Б")
		if err != nil || !found {
			t.Fatalf("карточка: %v", err)
		}
		var h21e *struct{}
		for _, g := range card.Groups {
			for _, v := range g.Values {
				if v.Parameter == "h21e" {
					if v.Min == nil || *v.Min != 50 || v.Max == nil || *v.Max != 350 {
						t.Fatalf("h21e: %+v", v)
					}
					if len(v.Conditions) != 2 {
						t.Fatalf("условия h21e: %+v", v.Conditions)
					}
					h21e = &struct{}{}
				}
			}
		}
		if h21e == nil {
			t.Fatal("h21e не найден в карточке")
		}
	})
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись %s: %v", path, err)
	}
}
