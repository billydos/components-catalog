package importer

import (
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Фронтенды форматов: синтаксис (жёсткая ошибка — только он), дубликаты
// ключей — ошибка разбора файла (тихая потеря данных недопустима),
// порядок ключей и числа без потери точности. Тексты ошибок — контракт.

func TestParseJSONCCommentsAndTrailingCommas(t *testing.T) {
	v, err := parseJSONC([]byte(`{
  // комментарий
  "transistors": [
    { "name": "КТ315Б", "system": "gost", }, /* блочный */
  ],
}`))
	if err != nil {
		t.Fatalf("jsonc с комментариями: %v", err)
	}
	if v.kind != kindObject {
		t.Fatalf("корень должен быть objectом: %v", v.kind)
	}
	section, ok := v.has("transistors")
	if !ok || section.kind != kindArray || len(section.items) != 1 {
		t.Fatalf("секция transistors не прочитана: %+v", section)
	}
	name, _ := section.items[0].has("name")
	if name.str != "КТ315Б" {
		t.Fatalf("порядок/значение ключей потеряны: %q", name.str)
	}
}

func TestParseJSONCDuplicateKeyIsHardError(t *testing.T) {
	_, err := parseJSONC([]byte("{\n  \"name\": \"КТ315Б\",\n  \"name\": \"ГТ109Г\"\n}"))
	de, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("ожидалась *domain.Error, получено %v", err)
	}
	if de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("код: %s, ожидался invalid_import_file", de.Code)
	}
	want := "the file is not valid JSONC: duplicate key «name» (line 3)"
	if de.Message != want {
		t.Fatalf("текст: %q, ожидался %q", de.Message, want)
	}
}

func TestParseJSONCSyntaxError(t *testing.T) {
	_, err := parseJSONC([]byte("{ \"name\": "))
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась ошибка invalid_import_file, получено %v", err)
	}
	if !strings.Contains(de.Message, "the file is not valid JSONC") {
		t.Fatalf("текст без указания формата: %q", de.Message)
	}
}

func TestParseJSONCTrailingData(t *testing.T) {
	if _, err := parseJSONC([]byte(`{} {}`)); err == nil {
		t.Fatal("данные после корневого значения должны отвергаться")
	}
}

func TestParseJSONCNumberPrecision(t *testing.T) {
	v, err := parseJSONC([]byte(`{ "min": 10000000000, "max": 0.125, "e": 1e3 }`))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	minv, _ := v.has("min")
	if minv.num != "10000000000" {
		t.Fatalf("number сохранено неточно: %q", minv.num)
	}
	maxv, _ := v.has("max")
	if maxv.num != "0.125" {
		t.Fatalf("дробь сохранена неточно: %q", maxv.num)
	}
}

func TestParseYAMLAnchorsAndAliases(t *testing.T) {
	v, err := parseYAML([]byte("gost: &g\n  system: gost\nrecords:\n  - name: КТ315Б\n    base: *g\n"))
	if err != nil {
		t.Fatalf("якоря и алиасы: %v", err)
	}
	records, _ := v.has("records")
	base, _ := records.items[0].has("base")
	if base.kind != kindObject {
		t.Fatalf("алиас не раскрыт в значение якоря: %+v", base)
	}
	sys, _ := base.has("system")
	if sys.kind != kindString || sys.str != "gost" {
		t.Fatalf("значение якоря потеряно: %+v", sys)
	}
}

func TestParseYAMLMergeKeyRejected(t *testing.T) {
	// merge-ключ «<<» отвергается как ключ objectа — сознательно:
	// семантика слияния не входит в формат наполнения.
	if _, err := parseYAML([]byte("base: &b\n  a: 1\nrec:\n  <<: *b\n  name: X\n")); err == nil {
		t.Fatal("merge-ключ должен отвергаться")
	}
}

func TestParseYAMLUnknownAliasRejected(t *testing.T) {
	_, err := parseYAML([]byte("a: *nope\n"))
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась ошибка invalid_import_file, получено %v", err)
	}
	if !strings.Contains(de.Message, "неизвестный алиас") {
		t.Fatalf("текст: %q", de.Message)
	}
}

func TestParseYAMLKeyOrderAndScalars(t *testing.T) {
	v, err := parseYAML([]byte("name: КТ315Б\nsystem: gost\nyear: 1967\nok: true\nnil: null\n"))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	var names []string
	for _, m := range v.members {
		names = append(names, m.name)
	}
	if strings.Join(names, ",") != "name,system,year,ok,nil" {
		t.Fatalf("порядок ключей потерян: %v", names)
	}
	year, _ := v.has("year")
	if year.kind != kindNumber || year.num != "1967" {
		t.Fatalf("number: %+v", year)
	}
	okv, _ := v.has("ok")
	if okv.kind != kindBool || !okv.boolean {
		t.Fatalf("булево: %+v", okv)
	}
	nilv, _ := v.has("nil")
	if nilv.kind != kindNull {
		t.Fatalf("null: %+v", nilv)
	}
}

func TestParseYAMLMultipleDocumentsRejected(t *testing.T) {
	_, err := parseYAML([]byte("a: 1\n---\nb: 2\n"))
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась ошибка invalid_import_file, получено %v", err)
	}
	if !strings.Contains(de.Message, "несколько yaml-документов") {
		t.Fatalf("текст: %q", de.Message)
	}
}

func TestParseYAMLSyntaxError(t *testing.T) {
	_, err := parseYAML([]byte("a: [1, 2\n"))
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась ошибка invalid_import_file, получено %v", err)
	}
	if !strings.Contains(de.Message, "the file is not valid YAML") {
		t.Fatalf("текст без указания формата: %q", de.Message)
	}
}

func TestParseLineJSONDuplicateKeyWithLineNumber(t *testing.T) {
	_, err := parseLineJSON([]byte(`{"transistors": {"name": "A", "name": "B"}}`), 7)
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась ошибка invalid_import_file, получено %v", err)
	}
	want := "line 7: duplicate key «name» (line 1)"
	if de.Message != want {
		t.Fatalf("текст: %q, ожидался %q", de.Message, want)
	}
}

func TestFormatByFilename(t *testing.T) {
	cases := []struct {
		name string
		want Format
		ok   bool
	}{
		{"fill.jsonc", FormatJSONC, true},
		{"fill.json", FormatJSONC, true},
		{"fill.JSONC", FormatJSONC, true},
		{"fill.yaml", FormatYAML, true},
		{"fill.yml", FormatYAML, true},
		{"fill.ndjson", FormatNDJSON, true},
		{"fill.txt", "", false},
		{"fill", "", false},
	}
	for _, tc := range cases {
		f, err := FormatByFilename(tc.name)
		if tc.ok && (err != nil || f != tc.want) {
			t.Errorf("%s: %v, %v", tc.name, f, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: ожидалась ошибка формата", tc.name)
		}
	}
	_, err := FormatByFilename("fill.txt")
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidImportFile {
		t.Fatalf("ожидалась ошибка invalid_import_file, получено %v", err)
	}
}

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"jsonc", "yaml", "ndjson"} {
		f, err := ParseFormat(s)
		if err != nil || string(f) != s {
			t.Errorf("ParseFormat(%q) = %v, %v", s, f, err)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("неизвестный формат должен отвергаться")
	}
}

func TestNDJSONScannerSkipsEmptyLines(t *testing.T) {
	sc := newNDJSONScanner(strings.NewReader("\n  \n1\n\n2\n"))
	var got []string
	for {
		line, _, ok, err := sc.next()
		if err != nil {
			t.Fatalf("чтение: %v", err)
		}
		if !ok {
			break
		}
		got = append(got, string(line))
	}
	if strings.Join(got, ",") != "1,2" {
		t.Fatalf("пустые строки не пропущены: %v", got)
	}
}
