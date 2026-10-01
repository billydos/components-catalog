package importer

import (
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

// TestPinSampleDataKeysFromCatalog — ключи-классы и секции записей
// sample-data принадлежат каталогу (вид дерева — формат наполнения).
func TestPinSampleDataKeysFromCatalog(t *testing.T) {
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
		"name": true, "system": true, "attributes": true,
		"manufacturers": true, "variants": true, "analogs": true,
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "sample-data", "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("sample-data недоступны: %v", err)
	}
	for _, file := range files {
		if _, err := FormatByFilename(file); err != nil {
			continue // служебные файлы каталога (например, .gitkeep)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		format, err := FormatByFilename(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var root value
		if format == FormatNDJSON {
			sc := newNDJSONScanner(strings.NewReader(string(data)))
			for {
				line, number, ok, err := sc.next()
				if err != nil {
					t.Fatalf("%s: строка %d: %v", file, number, err)
				}
				if !ok {
					break
				}
				v, err := parseLineJSON(line, number)
				if err != nil {
					t.Fatalf("%s: строка %d: %v", file, number, err)
				}
				root = v
				checkRecordWrapperKeys(t, file, snap, kinds, sections, serviceKeys, v)
			}
			continue
		}
		root, err = parseTree(data, format)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, m := range root.members {
			if m.name == "catalog" {
				continue
			}
			if !kinds[m.name] {
				t.Errorf("%s: ключ корня «%s» не является классом каталога", file, m.name)
			}
			for _, rec := range m.value.items {
				checkRecordKeys(t, file, snap, kinds, sections, serviceKeys, rec)
			}
		}
	}
}

func checkRecordWrapperKeys(t *testing.T, file string, snap *catalog.Snapshot,
	kinds, sections, serviceKeys map[string]bool, wrapper value) {
	t.Helper()
	for _, m := range wrapper.members {
		if m.name == "catalog" {
			continue
		}
		if !kinds[m.name] {
			t.Errorf("%s: ключ-класс «%s» не является классом каталога", file, m.name)
		}
		checkRecordKeys(t, file, snap, kinds, sections, serviceKeys, m.value)
	}
}

func checkRecordKeys(t *testing.T, file string, snap *catalog.Snapshot,
	kinds, sections, serviceKeys map[string]bool, rec value) {
	t.Helper()
	if rec.kind == kindString {
		return
	}
	if rec.kind != kindObject {
		t.Errorf("%s: запись не объект и не строка", file)
		return
	}
	for _, m := range rec.members {
		if serviceKeys[m.name] || sections[m.name] {
			continue
		}
		t.Errorf("%s: неизвестный ключ записи «%s»", file, m.name)
	}
	// Значения секций: ключи объектов значений — parameter/value/min/max/
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
// наполнения: секции — из групп каталога, условия значений — из условий
// каталога (round-trip пин).
func TestPinExportShapeFromCatalog(t *testing.T) {
	app, err := service.Open(context.Background(), service.Config{
		Dialect: "sqlite", DSN: ":memory:", EnsureCreated: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer app.Close() //nolint:errcheck
	importFile(t, app, sampleDataPath(t, "transistors.jsonc"), false)
	importFile(t, app, sampleDataPath(t, "capacitors.jsonc"), false)

	snap, err := app.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("снимок: %v", err)
	}
	sections := map[string]bool{}
	for _, g := range snap.Groups {
		sections[g.SectionName] = true
	}
	serviceKeys := map[string]bool{
		"name": true, "system": true, "attributes": true,
		"manufacturers": true, "variants": true, "analogs": true,
		"label": true,
	}

	var buf strings.Builder
	if err := New(app).Export(context.Background(), &bufWriter{&buf}, FormatJSONC, nil); err != nil {
		t.Fatalf("экспорт: %v", err)
	}
	root, err := parseJSONC([]byte(buf.String()))
	if err != nil {
		t.Fatalf("разбор экспорта: %v", err)
	}
	for _, sec := range root.members {
		for _, rec := range sec.value.items {
			for _, m := range rec.members {
				if !serviceKeys[m.name] && !sections[m.name] {
					t.Errorf("экспорт: неизвестный ключ записи «%s»", m.name)
				}
				if !sections[m.name] || m.value.kind != kindArray {
					continue
				}
				for _, item := range m.value.items {
					for _, vm := range item.members {
						switch vm.name {
						case "parameter", "value", "min", "max", "text":
						default:
							if _, ok := snap.Condition(vm.name); !ok {
								t.Errorf("экспорт: ключ «%s» значения — не условие каталога", vm.name)
							}
						}
					}
				}
			}
		}
	}
}

type bufWriter struct{ b *strings.Builder }

func (w *bufWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// TestPinVerbatimFormatErrors — дословные тексты ошибок формата.
func TestPinVerbatimFormatErrors(t *testing.T) {
	// Неизвестное расширение — ошибка формата файла.
	_, err := FormatByFilename("data.csv")
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась invalid_import_file, получено %v", err)
	}
	want := "файл «data.csv»: не удалось определить формат по расширению «.csv» " +
		"(допустимы: .jsonc/.json, .yaml/.yml, .ndjson — либо задайте --format)"
	if de.Message != want {
		t.Fatalf("текст: %q, ожидался %q", de.Message, want)
	}
	// NDJSON-строка с синтаксической ошибкой — номер строки в тексте.
	_, err = parseLineJSON([]byte(`{"transistors": `), 9)
	de, ok = domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась invalid_import_file, получено %v", err)
	}
	if !strings.HasPrefix(de.Message, "строка 9: ") {
		t.Fatalf("текст без номера строки: %q", de.Message)
	}
}
