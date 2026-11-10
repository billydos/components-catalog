package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Источники данных записи (план 03 §13, D13): корневая секция sources,
// секция записи, семантика замены, идемпотентность, чистка сирот,
// NDJSON-строки и границы форм. Покрытие — через полный конвейер
// импорта (читатель → сервис → хранилище), паритет СУБД — прогонами
// TestSourcesSQLiteMemory/File/Postgres ниже.

const sourcesFileBody = `{
  "sources": [
    { "code": "ba8082", "type": "handbook", "title": "Полупроводниковые приборы",
      "authors": ["А. В. Баюков", "Л. Л. Гуткин"],
      "publisher": "Энергоиздат", "year": 1982, "edition": "2-е" },
    { "code": "ds_kt315b", "type": "datasheet", "title": "Datasheet: КТ315Б (kazus.ru)",
      "urls": ["https://kazus.ru/kt315b"] }
  ],
  "transistors": [
    { "name": "КТ315Б", "system": "gost",
      "attributes": { "description": "малощумный" },
      "sources": ["ds_kt315b", { "code": "ba8082", "note": "таблица 3.14, с. 214" }],
      "parameters": [ { "parameter": "hfe", "min": 50, "max": 350, "Ic": 1, "Vce": 5 } ]
    }
  ]
}`

func runSourcesSuite(t *testing.T, factory configFactory) {
	t.Helper()

	writeSourcesFile := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "sources.jsonc")
		write(t, path, body)
		return path
	}

	t.Run("ApplyAndCard", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, sourcesFileBody)
		rep := importFile(t, app, path, false)
		if rep.HasIssues() || rep.Added != 1 || !rep.SourcesApplied {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		card, found, err := app.Services().Devices.Get(context.Background(),
			domain.KindTransistor, "КТ315Б")
		if err != nil || !found {
			t.Fatalf("карточка: %v %v", found, err)
		}
		if len(card.Sources) != 2 {
			t.Fatalf("источников %d, ожидалось 2", len(card.Sources))
		}
		// Канонический порядок — по коду: ba8082 < ds_kt315b.
		first, second := card.Sources[0], card.Sources[1]
		if first.Code != "ba8082" || first.Note != "таблица 3.14, с. 214" ||
			first.Title != "Полупроводниковые приборы" || first.Type != "handbook" ||
			first.Year == nil || *first.Year != 1982 ||
			len(first.Authors) != 2 || first.Publisher != "Энергоиздат" || first.Edition != "2-е" {
			t.Fatalf("ba8082: %+v", first)
		}
		if second.Code != "ds_kt315b" || second.Type != "datasheet" ||
			len(second.URLs) != 1 || second.URLs[0] != "https://kazus.ru/kt315b" {
			t.Fatalf("ds_kt315b: %+v", second)
		}

		// Идемпотентность повторного импорта: записи Skipped, реестр
		// не меняется (SourcesApplied = false), data_revision стоит.
		_, dataRev, err := app.Revisions(context.Background())
		if err != nil {
			t.Fatalf("ревизии: %v", err)
		}
		rep = importFile(t, app, path, false)
		if rep.HasIssues() || rep.Skipped != 1 || rep.SourcesApplied {
			t.Fatalf("повторный итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		if _, rev2, err := app.Revisions(context.Background()); err != nil || rev2 != dataRev {
			t.Fatalf("ревизии сдвинулись: %d → %d (%v)", dataRev, rev2, err)
		}
	})

	t.Run("DryRunPendingCodes", func(t *testing.T) {
		// Dry-run: реестр не применяется, ссылка на код этого же файла
		// валидна (pending), записи считаются добавленными без записи.
		app := openApp(t, factory)
		path := writeSourcesFile(t, sourcesFileBody)
		rep := importFile(t, app, path, true)
		if rep.HasIssues() || rep.Added != 1 || rep.SourcesApplied {
			t.Fatalf("dry-run: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		n, err := app.Services().Sources.ListSources(context.Background())
		if err != nil || len(n) != 0 {
			t.Fatalf("dry-run применил реестр: %d (%v)", len(n), err)
		}
	})

	t.Run("DryRunUnknownCode", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"transistors": [
			{ "name": "КТ315Б", "sources": ["nowhere"] } ]}`)
		rep := importFile(t, app, path, true)
		if !rep.HasIssues() || rep.Rejected != 1 {
			t.Fatalf("dry-run: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		want := "source «nowhere» not found (define it in the root sources section)"
		if rep.Issues[0].Message != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].Message, want)
		}
	})

	t.Run("SectionSemantics", func(t *testing.T) {
		app := openApp(t, factory)
		importFile(t, app, writeSourcesFile(t, sourcesFileBody), false)

		// Секция не задана — ссылки не меняются.
		rep := importFile(t, app, writeSourcesFile(t, `{"transistors": [
			{ "name": "КТ315Б", "attributes": { "description": "универсальный" } }]}`), false)
		if rep.HasIssues() || rep.Updated != 1 {
			t.Fatalf("обновление без секции: %+v; %v", rep, issueMessages(rep.Issues))
		}
		card := mustCard(t, app, "КТ315Б")
		if len(card.Sources) != 2 {
			t.Fatalf("ссылки изменились: %+v", card.Sources)
		}

		// Задана — замена целиком.
		rep = importFile(t, app, writeSourcesFile(t, `{"sources": [
			{ "code": "gost10862", "type": "standard", "title": "ГОСТ 10862-64" }],
			"transistors": [ { "name": "КТ315Б", "sources": ["gost10862"] }]}`), false)
		if rep.HasIssues() || rep.Updated != 1 {
			t.Fatalf("замена: %+v; %v", rep, issueMessages(rep.Issues))
		}
		card = mustCard(t, app, "КТ315Б")
		if len(card.Sources) != 1 || card.Sources[0].Code != "gost10862" {
			t.Fatalf("замена не применилась: %+v", card.Sources)
		}

		// Пустой список — очистка; сироты вычищаются из реестра.
		rep = importFile(t, app, writeSourcesFile(t, `{"transistors": [
			{ "name": "КТ315Б", "sources": [] }]}`), false)
		if rep.HasIssues() || rep.Updated != 1 {
			t.Fatalf("очистка: %+v; %v", rep, issueMessages(rep.Issues))
		}
		card = mustCard(t, app, "КТ315Б")
		if len(card.Sources) != 0 {
			t.Fatalf("очистка не применилась: %+v", card.Sources)
		}
		rows, err := app.Services().Sources.ListSources(context.Background())
		if err != nil {
			t.Fatalf("реестр: %v", err)
		}
		for _, r := range rows {
			if r.Code == "gost10862" || r.Code == "ds_kt315b" {
				t.Fatalf("сирота %s не вычищен", r.Code)
			}
		}

		// Удаление записи чистит последних сирот (ba8082 был без связей
		// после замены).
		if _, err := app.Services().Devices.Delete(context.Background(),
			domain.KindTransistor, "КТ315Б"); err != nil {
			t.Fatalf("удаление: %v", err)
		}
		rows, err = app.Services().Sources.ListSources(context.Background())
		if err != nil {
			t.Fatalf("реестр: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("после удаления записи в реестре остались сироты: %+v", rows)
		}
	})

	t.Run("UnknownSourceRejectsRecord", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"transistors": [
			{ "name": "КТ315Б", "sources": ["ba8082"] }]}`)
		rep := importFile(t, app, path, false)
		if !rep.HasIssues() || rep.Rejected != 1 || rep.Added != 0 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		if _, found, err := app.Services().Devices.Get(context.Background(),
			domain.KindTransistor, "КТ315Б"); err != nil || found {
			t.Fatalf("запись применена: %v %v", found, err)
		}
	})

	t.Run("DuplicateRefInRecord", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "code": "ba8082", "type": "handbook", "title": "Полупроводниковые приборы" }],
			"transistors": [ { "name": "КТ315Б", "sources": ["ba8082", "ba8082"] }]}`)
		rep := importFile(t, app, path, false)
		if !rep.HasIssues() || rep.Rejected != 1 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		want := "source «ba8082» is listed more than once"
		if rep.Issues[0].Message != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].Message, want)
		}
	})

	t.Run("ValidationVerbatimTexts", func(t *testing.T) {
		app := openApp(t, factory)
		cases := []struct{ body, want string }{
			{`{"sources": [{ "type": "handbook", "title": "X" }]}`,
				"source no. 1: mandatory field \"code\""},
			{`{"sources": [{ "code": "a1", "type": "handbook" }]}`,
				"source no. 1: mandatory field \"title\""},
			{`{"sources": [{ "code": "a1", "title": "X", "type": "site" }]}`,
				"source «a1»: unknown source type «site» (allowed: book, datasheet, handbook, journal, spec, standard, web)"},
			{`{"sources": [{ "code": "A1", "title": "X", "type": "book" }]}`,
				"source code «A1»: only lowercase Latin letters, digits and underscore are allowed"},
			{`{"sources": [{ "code": "a1", "title": "X", "type": "book", "year": 1850 }]}`,
				"source «a1»: year must be within 1900–2100"},
			{`{"sources": [{ "code": "a1", "title": "X", "type": "book", "authors": ["", ""] }]}`,
				"source «a1»: field \"authors\": empty list element"},
			{`{"sources": [{ "code": "a1", "title": "X", "type": "book", "urls": ["u", "u"] }]}`,
				"source «a1»: field \"urls\": element «u» is set more than once"},
			{`{"sources": [
				{ "code": "a1", "title": "X", "type": "book" },
				{ "code": "a1", "title": "Y", "type": "book" }]}`,
				"source code «a1» is defined more than once"},
			{`{"sources": [{ "code": "a1", "title": "X", "type": "book", "isbn": "5-12345-678-9-X" }]`,
				""}, // незакрытый массив — синтаксис, отдельный тест не нужен
		}
		for i, tc := range cases {
			if tc.want == "" {
				continue
			}
			path := writeSourcesFile(t, tc.body)
			rep := importFile(t, app, path, false)
			if !rep.HasIssues() {
				t.Fatalf("случай %d: проблемы не накоплены", i)
			}
			if rep.Issues[0].Message != tc.want {
				t.Fatalf("случай %d: текст %q, ожидался %q", i, rep.Issues[0].Message, tc.want)
			}
			// Реестр не применился.
			rows, err := app.Services().Sources.ListSources(context.Background())
			if err != nil || len(rows) != 0 {
				t.Fatalf("случай %d: реестр применён (%d)", i, len(rows))
			}
		}
	})

	// Не-массивное значение строки sources (число, объект) — проблема
	// строки с номером: итог не зависит от порядка строк, опечатка не
	// исчезает молча; блок источников не применяется, ссылки записей
	// получают unknown_source.
	t.Run("NDJSONSourcesNonArrayValue", func(t *testing.T) {
		badLines := []struct {
			name string
			line string
		}{
			{"число", `{"sources": 42}`},
			{"объект", `{"sources": {"code": "a1", "type": "book", "title": "X"}}`},
			{"логическое значение", `{"sources": true}`},
		}
		for _, bad := range badLines {
			for _, place := range []struct {
				name   string
				prefix string
				lineNo int
			}{
				{"в первой строке", "", 1},
				{"после корректной строки",
					`{"sources": [{"code": "a1", "type": "book", "title": "X"}]}` + "\n", 2},
			} {
				label := bad.name + " " + place.name
				app := openApp(t, factory)
				path := filepath.Join(t.TempDir(), "src.ndjson")
				write(t, path, place.prefix+bad.line+"\n"+
					`{"transistors": {"name": "КТ315Б", "sources": ["a1"]}}`+"\n")
				rep := importFile(t, app, path, false)
				if !rep.HasIssues() || rep.Rejected != 1 || rep.Added != 0 {
					t.Fatalf("%s: итог: %+v; проблемы: %v", label, rep, issueMessages(rep.Issues))
				}
				if rep.SourcesApplied {
					t.Fatalf("%s: блок источников применён", label)
				}
				want := fmt.Sprintf("line %d: \"sources\" must be an array of source definitions "+
					"(objects {code, title, type, …})", place.lineNo)
				if rep.Issues[0].String() != want {
					t.Fatalf("%s: текст: %q, ожидался %q", label, rep.Issues[0].String(), want)
				}
				rows, err := app.Services().Sources.ListSources(context.Background())
				if err != nil || len(rows) != 0 {
					t.Fatalf("%s: реестр применён (%d)", label, len(rows))
				}
			}
		}
	})

	// Проблемы определений накапливаются по всей секции (Issues-модель),
	// а не до первого дефектного: определения после дефектного тоже
	// читаются; секция с проблемами не применяется вовсе — валидное
	// определение среди дефектных в реестр не попадает.
	t.Run("AllDefProblemsAccumulated", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "type": "book", "title": "X" },
			{ "code": "ok1", "type": "book", "title": "Валидное" },
			{ "code": "a3", "title": "Y" }]}`)
		rep := importFile(t, app, path, false)
		if len(rep.Issues) != 2 {
			t.Fatalf("проблем: %d, ожидалось 2: %v", len(rep.Issues), issueMessages(rep.Issues))
		}
		got := issueMessages(rep.Issues)
		want := []string{
			"source no. 1: mandatory field \"code\"",
			"source no. 3: mandatory field \"type\"",
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("проблема %d: %q, ожидалась %q", i+1, got[i], w)
			}
		}
		rows, err := app.Services().Sources.ListSources(context.Background())
		if err != nil || len(rows) != 0 {
			t.Fatalf("реестр применён (%d)", len(rows))
		}
	})

	// Проблемы формы определений не маскируют проблемы значений:
	// валидация сервиса идёт по прочитанным определениям и накопляется
	// вместе с проблемами читателя — полный список за один прогон
	// (как метасхема у секции catalog).
	t.Run("FormAndValueProblemsAccumulated", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "type": "book", "title": "X" },
			{ "code": "a1", "type": "book", "title": "Y" },
			{ "code": "a1", "type": "book", "title": "Z" } ]}`)
		rep := importFile(t, app, path, false)
		if len(rep.Issues) != 2 {
			t.Fatalf("проблем: %d, ожидалось 2: %v", len(rep.Issues), issueMessages(rep.Issues))
		}
		got := issueMessages(rep.Issues)
		want := []string{
			"source no. 1: mandatory field \"code\"",
			"source code «a1» is defined more than once",
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("проблема %d: %q, ожидалась %q", i+1, got[i], w)
			}
		}
		rows, err := app.Services().Sources.ListSources(context.Background())
		if err != nil || len(rows) != 0 {
			t.Fatalf("реестр применён (%d)", len(rows))
		}
	})

	t.Run("ISBNCanonicalized", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "code": "ba8082", "type": "handbook", "title": "Полупроводниковые приборы",
			  "isbn": "5-217-01121-X" }],
			"transistors": [ { "name": "КТ315Б", "sources": ["ba8082"] } ]}`)
		rep := importFile(t, app, path, false)
		if rep.HasIssues() {
			t.Fatalf("проблемы: %v", issueMessages(rep.Issues))
		}
		rows, err := app.Services().Sources.ListSources(context.Background())
		if err != nil || len(rows) != 1 {
			t.Fatalf("реестр: %d (%v)", len(rows), err)
		}
		if rows[0].ISBN != "521701121X" {
			t.Fatalf("isbn: %q", rows[0].ISBN)
		}
	})

	t.Run("ExportRoundTrip", func(t *testing.T) {
		app := openApp(t, factory)
		importFile(t, app, writeSourcesFile(t, sourcesFileBody), false)
		var buf strings.Builder
		if err := New(app).Export(context.Background(), &buf, FormatJSONC, nil); err != nil {
			t.Fatalf("экспорт: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, `"sources": [`) ||
			!strings.Contains(out, `"code": "ba8082"`) ||
			!strings.Contains(out, `"note": "таблица 3.14, с. 214"`) {
			t.Fatalf("экспорт без источников:\n%s", out)
		}
		// Round-trip: повторный импорт экспорта идемпотентен.
		path := filepath.Join(t.TempDir(), "export.jsonc")
		write(t, path, out)
		rep := importFile(t, app, path, false)
		if rep.HasIssues() || rep.Skipped != 1 || rep.SourcesApplied {
			t.Fatalf("round-trip: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}

		// NDJSON: строка sources предшествует записям, повторный импорт
		// экспорта идемпотентен.
		var nd strings.Builder
		if err := New(app).Export(context.Background(), &nd, FormatNDJSON, nil); err != nil {
			t.Fatalf("экспорт ndjson: %v", err)
		}
		ndPath := filepath.Join(t.TempDir(), "export.ndjson")
		write(t, ndPath, nd.String())
		rep = importFile(t, app, ndPath, false)
		if rep.HasIssues() || rep.Skipped != 1 || rep.SourcesApplied {
			t.Fatalf("round-trip ndjson: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
	})

	t.Run("NDJSONSourcesBlock", func(t *testing.T) {
		app := openApp(t, factory)
		body := "{\"sources\": [{\"code\": \"ba8082\", \"type\": \"handbook\", \"title\": \"Полупроводниковые приборы\"}]}\n" +
			"{\"transistors\": {\"name\": \"КТ315Б\", \"sources\": [\"ba8082\"]}}\n"
		path := filepath.Join(t.TempDir(), "src.ndjson")
		write(t, path, body)
		rep := importFile(t, app, path, false)
		if rep.HasIssues() || rep.Added != 1 || !rep.SourcesApplied {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}

		// Строка sources после записи — ошибка порядка.
		path2 := filepath.Join(t.TempDir(), "order.ndjson")
		write(t, path2,
			"{\"transistors\": {\"name\": \"КТ315Б\"}}\n"+
				"{\"sources\": [{\"code\": \"x1\", \"type\": \"book\", \"title\": \"X\"}]}\n")
		rep = importFile(t, app, path2, false)
		if !rep.HasIssues() {
			t.Fatal("ошибка порядка не накоплена")
		}
		want := "the sources block must precede records"
		if rep.Issues[0].Message != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].Message, want)
		}
	})

	t.Run("CatalogImportRejectsSources", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "code": "a1", "type": "book", "title": "X" }]}`)
		_, err := importCatalogFile(t, app, path, false)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeInvalidImportFile {
			t.Fatalf("ожидалась invalid_import_file, получено %v", err)
		}
		want := "the file contains a sources section; catalog import applies to files with the catalog section only"
		if de.Message != want {
			t.Fatalf("текст: %q, ожидался %q", de.Message, want)
		}
	})

	// catalog import отвергает файл с секцией sources ДО применения
	// каталога — в обоих форматах, включая битую форму секции
	// (гейт — по присутствию секции, а не по числу валидных defs).
	t.Run("CatalogImportDoesNotApplyCatalog", func(t *testing.T) {
		cases := []struct {
			name string
			path string
			body string
		}{
			{"jsonc", "mix.jsonc", `{"catalog": {"units": [ { "code": "kx" } ]},
				"sources": [ { "code": "a1", "type": "book", "title": "X" } ]}`},
			{"jsonc-broken", "broken.jsonc", `{"catalog": {"units": [ { "code": "kx" } ]},
				"sources": 5}`},
			{"ndjson", "mix.ndjson",
				"{\"catalog\": {\"units\": [ { \"code\": \"kx\" } ]}}\n" +
					"{\"sources\": [{\"code\": \"a1\", \"type\": \"book\", \"title\": \"X\"}]}\n"},
			{"ndjson-broken", "broken.ndjson",
				"{\"catalog\": {\"units\": [ { \"code\": \"kx\" } ]}}\n" +
					"{\"sources\": 5}\n"},
		}
		for _, tc := range cases {
			app := openApp(t, factory)
			path := filepath.Join(t.TempDir(), tc.path)
			write(t, path, tc.body)
			_, err := importCatalogFile(t, app, path, false)
			de, ok := domain.AsError(err)
			if !ok || de.Code != domain.CodeInvalidImportFile {
				t.Fatalf("%s: ожидалась invalid_import_file, получено %v", tc.name, err)
			}
			// Каталог не применён на отказывающем пути.
			snap, err := app.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("%s: снимок: %v", tc.name, err)
			}
			if _, found := snap.Unit("kx"); found {
				t.Fatalf("%s: каталог применён вопреки отказу (единица kx)", tc.name)
			}
		}
	})

	// Обёртка «sources + соседний ключ» портит блок источников целиком:
	// определения строки не применяются, позднейшая чистая строка sources
	// не реабилитирует блок — ссылки записей получают unknown_source.
	t.Run("NDJSONWrapperTaint", func(t *testing.T) {
		app := openApp(t, factory)
		path := filepath.Join(t.TempDir(), "taint.ndjson")
		write(t, path,
			"{\"sources\": [{\"code\": \"a1\", \"type\": \"book\", \"title\": \"X\"}], \"note\": 1}\n"+
				"{\"sources\": [{\"code\": \"a1\", \"type\": \"book\", \"title\": \"X\"}]}\n"+
				"{\"transistors\": {\"name\": \"КТ315Б\", \"sources\": [\"a1\"]}}\n")
		rep := importFile(t, app, path, false)
		if !rep.HasIssues() || rep.Rejected != 2 || rep.Added != 0 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		found := false
		for _, iss := range rep.Issues {
			if strings.Contains(iss.Message, "source «a1» not found") {
				found = true
			}
		}
		if !found {
			t.Fatalf("нет unknown_source: %v", issueMessages(rep.Issues))
		}
	})

	// Источник, общий для нескольких записей: пакетное чтение карточек
	// (батчи экспорта/GetByIDs) обязано вернуть полные списки (лица,
	// ссылки) в КАЖДОЙ записи — регрессия copy-back разделяемых строк.
	t.Run("SharedSourceBatchCards", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "code": "ba8082", "type": "handbook", "title": "Полупроводниковые приборы",
			  "authors": ["А. В. Баюков", "Л. Л. Гуткин"], "urls": ["https://example.com/x"] }],
			"transistors": [
				{ "name": "КТ315Б", "sources": ["ba8082"] },
				{ "name": "КТ315Г", "sources": ["ba8082"] } ]}`)
		rep := importFile(t, app, path, false)
		if rep.HasIssues() || rep.Added != 2 || !rep.SourcesApplied {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		cards, err := app.Services().Devices.GetByIDs(context.Background(), nil)
		_ = cards
		_ = err
		// Идентификаторы записей — через поиск по классу.
		page, err := app.Services().Devices.Search(context.Background(), service.SearchQuery{
			Kind: domain.KindTransistor, Limit: 10})
		if err != nil || len(page.Items) != 2 {
			t.Fatalf("поиск: %d (%v)", len(page.Items), err)
		}
		ids := []int64{page.Items[0].ID, page.Items[1].ID}
		cards, err = app.Services().Devices.GetByIDs(context.Background(), ids)
		if err != nil || len(cards) != 2 {
			t.Fatalf("GetByIDs: %d (%v)", len(cards), err)
		}
		for _, c := range cards {
			if len(c.Sources) != 1 || c.Sources[0].Code != "ba8082" {
				t.Fatalf("источник карточки %s: %+v", c.Designation, c.Sources)
			}
			src := c.Sources[0]
			if len(src.Authors) != 2 || len(src.URLs) != 1 {
				t.Fatalf("списки источника карточки %s утрачены: %+v", c.Designation, src)
			}
		}
	})

	// Замена заметки места без смены множества ссылок — обновление,
	// не Skipped (заметка входит в слитое состояние).
	t.Run("NoteOnlyUpdate", func(t *testing.T) {
		app := openApp(t, factory)
		path := writeSourcesFile(t, `{"sources": [
			{ "code": "ba8082", "type": "handbook", "title": "Полупроводниковые приборы" }],
			"transistors": [ { "name": "КТ315Б", "sources": ["ba8082"] } ]}`)
		importFile(t, app, path, false)
		path = writeSourcesFile(t, `{"transistors": [
			{ "name": "КТ315Б", "sources": [ { "code": "ba8082", "note": "таблица 3.14" } ] } ]}`)
		rep := importFile(t, app, path, false)
		if rep.HasIssues() || rep.Updated != 1 {
			t.Fatalf("итог: %+v; проблемы: %v", rep, issueMessages(rep.Issues))
		}
		card := mustCard(t, app, "КТ315Б")
		if len(card.Sources) != 1 || card.Sources[0].Note != "таблица 3.14" {
			t.Fatalf("заметка: %+v", card.Sources)
		}
	})

	t.Run("RecordSourceForms", func(t *testing.T) {
		app := openApp(t, factory)
		// Неверная форма элемента: не строка и не объект.
		path := writeSourcesFile(t, `{"sources": [
			{ "code": "a1", "type": "book", "title": "X" }],
			"transistors": [ { "name": "КТ315Б", "sources": [17] }]}`)
		rep := importFile(t, app, path, false)
		if !rep.HasIssues() || rep.Rejected != 1 {
			t.Fatalf("итог: %+v; %v", rep, issueMessages(rep.Issues))
		}
		want := "source no. 1: a string or an object with key \"code\" was expected"
		if rep.Issues[0].Message != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].Message, want)
		}
		// Неизвестное поле ссылки.
		path = writeSourcesFile(t, `{"sources": [
			{ "code": "a1", "type": "book", "title": "X" }],
			"transistors": [ { "name": "КТ315Б", "sources": [{"code": "a1", "page": 3}] }]}`)
		rep = importFile(t, app, path, false)
		if !rep.HasIssues() {
			t.Fatal("неизвестное поле не накоплено")
		}
		want = "source no. 1: unknown field «page» (allowed: code, note)"
		if rep.Issues[0].Message != want {
			t.Fatalf("текст: %q, ожидался %q", rep.Issues[0].Message, want)
		}
	})
}

func TestSourcesSQLiteMemory(t *testing.T) { runSourcesSuite(t, memoryConfig) }
func TestSourcesSQLiteFile(t *testing.T)   { runSourcesSuite(t, fileConfig) }
func TestSourcesPostgres(t *testing.T)     { runSourcesSuite(t, postgresConfig) }

// mustCard — карточка транзистора по обозначению.
func mustCard(t *testing.T, app *service.App, designation string) *service.Card {
	t.Helper()
	card, found, err := app.Services().Devices.Get(context.Background(),
		domain.KindTransistor, designation)
	if err != nil || !found {
		t.Fatalf("карточка %s: %v %v", designation, found, err)
	}
	return card
}

// Незакрытая секция — синтаксическая ошибка файла (жёсткий отказ).
func TestSourcesSyntaxError(t *testing.T) {
	app := openApp(t, memoryConfig)
	path := filepath.Join(t.TempDir(), "broken.jsonc")
	write(t, path, `{"sources": [`)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открытие: %v", err)
	}
	defer f.Close()
	_, err = New(app).Import(context.Background(), f, path, false)
	if _, ok := domain.AsError(err); !ok {
		t.Fatalf("ожидалась доменная ошибка, получено %v", err)
	}
}
