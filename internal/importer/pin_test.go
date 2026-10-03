package importer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Пин-тесты этапа 4 (план работ 4.6): ключи секций — из каталога,
// round-trip значений и условий, дословные тексты ошибок. Пины ловят
// рассинхронизацию формата наполнения и каталога-данных.

// TestPinGroupSections — имена секций файла наполнения задаются группами
// каталога (сиды): parameters/ratings/dimensions.
func TestPinGroupSections(t *testing.T) {
	snap := testSnapshot(t)
	var sections []string
	for _, g := range snap.Groups {
		sections = append(sections, g.SectionName)
	}
	got := strings.Join(sections, ",")
	if got != "parameters,ratings,dimensions" {
		t.Fatalf("секции групп каталога: %q, ожидались parameters,ratings,dimensions", got)
	}
	// Каждая секция читается в записи (объединение читателя и каталога).
	root, _ := parseDoc(t, `{"resistors": [ { "name": "С2-33Н",
		"parameters": [], "ratings": [], "dimensions": [] } ]}`)
	doc, issues := ReadDocument(root, snap)
	if len(issues) != 0 || len(doc.Records) != 1 {
		t.Fatalf("секции каталога не читаются: %v", issueMessages(issues))
	}
	if len(doc.Records[0].Input.Sections) != 3 {
		t.Fatalf("заданные пустые секции должны читаться: %+v", doc.Records[0].Input.Sections)
	}
}

// forEachRecord обходит записи документа наполнения в любом формате
// реестра: jsonc/yaml — секции классов с arrayами записей, ndjson —
// string-обёртка на запись (writer.go). Секция catalog пропускается.
func forEachRecord(t *testing.T, label string, data []byte, format Format, fn func(kind string, rec value)) {
	t.Helper()
	if format == FormatNDJSON {
		sc := newNDJSONScanner(strings.NewReader(string(data)))
		for {
			line, number, ok, err := sc.next()
			if err != nil {
				t.Fatalf("%s: line %d: %v", label, number, err)
			}
			if !ok {
				return
			}
			v, err := parseLineJSON(line, number)
			if err != nil {
				t.Fatalf("%s: line %d: %v", label, number, err)
			}
			for _, m := range v.members {
				if m.name == "catalog" {
					continue
				}
				fn(m.name, m.value)
			}
		}
	}
	root, err := parseTree(data, format)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	for _, m := range root.members {
		if m.name == "catalog" {
			continue
		}
		for _, rec := range m.value.items {
			fn(m.name, rec)
		}
	}
}

// TestPinDataKeysFromCatalog — ключи-классы и секции записей data/
// принадлежат каталогу (вид дерева — формат наполнения).
func TestPinDataKeysFromCatalog(t *testing.T) {
	snap := testSnapshot(t)
	sections := map[string]bool{}
	for _, g := range snap.Groups {
		sections[g.SectionName] = true
	}
	kinds := map[string]bool{}
	for _, k := range snap.Kinds {
		kinds[KindSection(k.Code)] = true
	}
	serviceKeys := map[string]bool{
		"name": true, "system": true, "fields": true, "attributes": true,
		"manufacturers": true, "variants": true, "analogs": true,
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "data", "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("data недоступны: %v", err)
	}
	for _, file := range files {
		format, err := FormatByFilename(file)
		if err != nil {
			continue // служебные файлы каталога (например, .gitkeep)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		forEachRecord(t, file, data, format, func(kind string, rec value) {
			if !kinds[kind] {
				t.Errorf("%s: ключ корня «%s» не является классом каталога", file, kind)
			}
			checkRecordKeys(t, file, snap, kinds, sections, serviceKeys, rec)
		})
	}
}

func checkRecordKeys(t *testing.T, file string, snap *catalog.Snapshot,
	kinds, sections, serviceKeys map[string]bool, rec value) {
	t.Helper()
	if rec.kind == kindString {
		return
	}
	if rec.kind != kindObject {
		t.Errorf("%s: запись не object и не string", file)
		return
	}
	for _, m := range rec.members {
		if serviceKeys[m.name] || sections[m.name] {
			continue
		}
		t.Errorf("%s: неизвестный ключ записи «%s»", file, m.name)
	}
	// Значения секций: ключи objectов значений — parameter/value/min/max/
	// text и коды условий каталога.
	for _, m := range rec.members {
		if !sections[m.name] || m.value.kind != kindArray {
			continue
		}
		for _, item := range m.value.items {
			for _, vm := range item.members {
				switch vm.name {
				case "parameter", "value", "min", "max", "text":
				default:
					if _, ok := snap.Condition(vm.name); !ok {
						t.Errorf("%s: ключ «%s» значения — не условие каталога", file, vm.name)
					}
				}
			}
		}
	}
}

// TestPinExportShapeFromCatalog — экспорт пишет только ключи формата
// наполнения во всех форматах реестра: секции — из групп каталога,
// условия значений — из условий каталога (round-trip пин).
func TestPinExportShapeFromCatalog(t *testing.T) {
	app, err := service.Open(context.Background(), service.Config{
		Dialect: "sqlite", DSN: ":memory:", EnsureCreated: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer app.Close() //nolint:errcheck
	importFile(t, app, dataPath(t, "transistors.jsonc"), false)
	importFile(t, app, dataPath(t, "capacitors.jsonc"), false)

	snap, err := app.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("снимок: %v", err)
	}
	sections := map[string]bool{}
	for _, g := range snap.Groups {
		sections[g.SectionName] = true
	}
	kinds := map[string]bool{}
	for _, k := range snap.Kinds {
		kinds[KindSection(k.Code)] = true
	}
	serviceKeys := map[string]bool{
		"name": true, "system": true, "fields": true, "attributes": true,
		"manufacturers": true, "variants": true, "analogs": true,
		"label": true,
	}

	for _, format := range []Format{FormatJSONC, FormatYAML, FormatNDJSON} {
		var buf bytes.Buffer
		if err := New(app).Export(context.Background(), &buf, format, nil); err != nil {
			t.Fatalf("экспорт %s: %v", format, err)
		}
		label := "export." + string(format)
		forEachRecord(t, label, buf.Bytes(), format, func(kind string, rec value) {
			if !kinds[kind] {
				t.Errorf("%s: класс «%s» не является классом каталога", label, kind)
			}
			checkRecordKeys(t, label, snap, nil, sections, serviceKeys, rec)
		})
		// Повторный экспорт байтово совпадает: детерминированность.
		var buf2 bytes.Buffer
		if err := New(app).Export(context.Background(), &buf2, format, nil); err != nil {
			t.Fatalf("повторный экспорт %s: %v", format, err)
		}
		if buf.String() != buf2.String() {
			t.Fatalf("экспорт %s недетерминирован", format)
		}
	}
}

// TestPinVerbatimFormatErrors — дословные тексты ошибок формата.
func TestPinVerbatimFormatErrors(t *testing.T) {
	// Неизвестное расширение — ошибка формата файла.
	_, err := FormatByFilename("data.csv")
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась invalid_import_file, получено %v", err)
	}
	want := "file «data.csv»: cannot determine the format from extension «.csv» " +
		"(allowed: .jsonc/.json, .yaml/.yml, .ndjson — or set --format)"
	if de.Message != want {
		t.Fatalf("текст: %q, ожидался %q", de.Message, want)
	}
	// NDJSON-string с синтаксической ошибкой — номер строки в тексте.
	_, err = parseLineJSON([]byte(`{"transistors": `), 9)
	de, ok = domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась invalid_import_file, получено %v", err)
	}
	if !strings.HasPrefix(de.Message, "line 9: ") {
		t.Fatalf("текст без номера строки: %q", de.Message)
	}
}
