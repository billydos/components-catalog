package importer

import (
	"errors"
	"strings"
	"testing"

	"soviettransistors/internal/domain"
)

func TestParseYamlText_SyntaxErrorMessage(t *testing.T) {
	_, err := ParseYamlText("transistors: [\n")
	if err == nil {
		t.Fatal("ожидалась ошибка синтаксиса")
	}
	if !strings.Contains(err.Error(), "файл не является корректным YAML: ") {
		t.Errorf("текст ошибки = %q", err.Error())
	}
	var userError *domain.UserError
	if !errors.As(err, &userError) {
		t.Error("ожидалась *domain.UserError")
	}
}

// Проблемы структуры корня — Issues уровня файла, накапливаются вместе
// (ANALYSIS-02 B3); жёсткая ошибка — только синтаксис и несколько
// yaml-документов.
func TestParseYamlText_RootProblems(t *testing.T) {
	cases := []struct {
		name   string
		source string
		issues []string
	}{
		{"пустой файл — null, не объект", "", []string{`корневой элемент должен быть объектом вида { "transistors": [ ... ] }`}},
		{"корень не объект", "- 1\n- 2\n", []string{`корневой элемент должен быть объектом вида { "transistors": [ ... ] }`}},
		{"нет ключа transistors", "{}", []string{`отсутствует обязательный ключ "transistors"`}},
		{"transistors не массив", "transistors: {}", []string{`"transistors" должен быть массивом`}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := ParseYamlText(testCase.source)
			if err != nil {
				t.Fatalf("разбор не удался: %v", err)
			}
			if len(result.Entries) != 0 || len(result.Issues) != len(testCase.issues) {
				t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
			}
			for i, want := range testCase.issues {
				if got := result.Issues[i].Description; got != want {
					t.Errorf("текст проблемы %d = %q, ожидалось %q", i+1, got, want)
				}
			}
		})
	}
	if _, err := ParseYamlText("transistors: []\n---\ntransistors: []\n"); err == nil {
		t.Error("ожидалась ошибка: несколько yaml-документов в одном файле")
	}
}

func TestParseYamlText_UnknownRootKey_ProducesIssue(t *testing.T) {
	result, err := ParseYamlText("# комментарий\nextra: 1\ntransistors: [] # хвостовой комментарий\n")
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if !result.HasErrors() {
		t.Fatal("ожидалась проблема")
	}
	want := "неизвестный ключ корневого объекта «extra» (допустим только \"transistors\")"
	if got := result.Issues[0].Description; got != want {
		t.Errorf("текст проблемы = %q, ожидалось %q", got, want)
	}
}

func TestParseYamlText_StringEntry_AndKindDescription(t *testing.T) {
	result, err := ParseYamlText("transistors:\n  - КТ315Б\n  - 42\n")
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Transistor.Name() != "КТ315Б" {
		t.Fatalf("entries = %+v", result.Entries)
	}
	want := "запись должна быть строкой (обозначение) или объектом, получено: число"
	if got := result.Issues[0].Description; got != want {
		t.Errorf("текст проблемы = %q, ожидалось %q", got, want)
	}
}

func TestParseYamlText_DesignationFormEntry(t *testing.T) {
	text := `transistors:
  - material: Г
    subclass: Т
    assembly: false
    feature: 1
    number: 15
    letters: А
    ratings: {IkMax: 50, tempMin: -40, tempMax: 55}
`
	result, err := ParseYamlText(text)
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	entry := result.Entries[0]
	if entry.Transistor.Name() != "ГТ115А" {
		t.Errorf("обозначение = %q", entry.Transistor.Name())
	}
	if entry.Ratings == nil || entry.Ratings.IkMax == nil || *entry.Ratings.IkMax != 50 {
		t.Errorf("ratings = %+v", entry.Ratings)
	}
}

func TestParseYamlText_ManufacturersNull_DoesNotChange(t *testing.T) {
	text := "transistors:\n  - name: КТ315Б\n    attributes:\n      manufacturers: null\n      structure: npn\n"
	result, err := ParseYamlText(text)
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	entry := result.Entries[0]
	if entry.Attributes == nil || entry.Attributes.Structure == nil || *entry.Attributes.Structure != "npn" {
		t.Fatalf("attributes = %+v", entry.Attributes)
	}
	if entry.Manufacturers != nil {
		t.Errorf("manufacturers = %v, ожидалось nil", entry.Manufacturers)
	}
}

func TestParseYamlText_EmptySections_Clear(t *testing.T) {
	result, err := ParseYamlText("transistors:\n  - name: КТ315Б\n    parameters: []\n    ratings: {}\n")
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	entry := result.Entries[0]
	if entry.Parameters == nil || len(entry.Parameters) != 0 {
		t.Errorf("parameters = %+v, ожидалась пустая непустая-nil секция", entry.Parameters)
	}
	if entry.Ratings == nil {
		t.Error("ratings = nil, ожидалась пустая непустая-nil секция")
	}
}

func TestParseYamlText_DuplicateKeys_Rejected(t *testing.T) {
	if _, err := ParseYamlText("transistors: []\ntransistors: []\n"); err == nil {
		t.Error("ожидалась ошибка повторяющегося ключа")
	}
	if _, err := ParseYamlText("transistors:\n  - name: МП39\n    name: КТ315Б\n"); err == nil {
		t.Error("ожидалась ошибка повторяющегося ключа в записи")
	}
	if _, err := ParseYamlText("transistors:\n  - name: КТ315Б\n    ratings: {IkMax: 50, IkMax: 60}\n"); err == nil {
		t.Error("ожидалась ошибка повторяющегося ключа во вложенной секции")
	}
}

func TestParseYamlText_FloatValue(t *testing.T) {
	result, err := ParseYamlText("transistors:\n  - name: КТ315Б\n    attributes:\n      massMax: 0.85\n")
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if result.HasErrors() || len(result.Entries) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	attributes := result.Entries[0].Attributes
	if attributes == nil || attributes.MassMax == nil || *attributes.MassMax != 0.85 {
		t.Errorf("massMax = %v, ожидалось 0.85", attributes)
	}
}

// .inf/.nan попадают в дерево как текст специальных чисел и отвергаются при
// чтении значения; закрепляет текущее поведение (неточность текста — C7).
func TestParseYamlText_InfinityAndNan_Rejected(t *testing.T) {
	result, err := ParseYamlText("transistors:\n  - name: КТ315Б\n    ratings: {IkMax: .inf, tempMax: .nan}\n")
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if len(result.Entries) != 0 || len(result.Issues) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	description := result.Issues[0].Description
	if !contains(description, `"IkMax" должно быть числом`) || !contains(description, `"tempMax" должно быть числом`) {
		t.Errorf("текст проблемы = %q", description)
	}
}

// Переиспользование блока между записями: якорь на секции первой записи,
// алиас во второй — разворачивается в то же значение.
func TestParseYamlText_AnchorReuseBetweenEntries(t *testing.T) {
	text := "transistors:\n" +
		"  - name: КТ315Б\n" +
		"    attributes: &attrs\n" +
		"      structure: npn\n" +
		"      manufacturers: [Завод]\n" +
		"  - name: ГТ115А\n" +
		"    attributes: *attrs\n"
	result, err := ParseYamlText(text)
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if result.HasErrors() || len(result.Entries) != 2 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	for index, entry := range result.Entries {
		if entry.Attributes == nil || entry.Attributes.Structure == nil || *entry.Attributes.Structure != "npn" {
			t.Errorf("запись %d: attributes = %+v, ожидалось structure npn", index+1, entry.Attributes)
		}
		if len(entry.Manufacturers) != 1 || entry.Manufacturers[0] != "Завод" {
			t.Errorf("запись %d: manufacturers = %v", index+1, entry.Manufacturers)
		}
	}
}

func TestParseYaml_TreeConversions(t *testing.T) {
	t.Run("якорь и алиас", func(t *testing.T) {
		parsed, err := parseYaml([]byte("base: &x 10\nalias: *x\n"))
		if err != nil {
			t.Fatalf("разбор не удался: %v", err)
		}
		base, _ := parsed.has("base")
		alias, _ := parsed.has("alias")
		if base.kind != kindNumber || base.num != "10" || alias.kind != kindNumber || alias.num != "10" {
			t.Errorf("base = %+v, alias = %+v, ожидались числа 10", base, alias)
		}
	})
	t.Run("алиас отображения", func(t *testing.T) {
		parsed, err := parseYaml([]byte("first: &attrs {structure: npn}\nsecond: *attrs\n"))
		if err != nil {
			t.Fatalf("разбор не удался: %v", err)
		}
		second, _ := parsed.has("second")
		structure, ok := second.has("structure")
		if second.kind != kindObject || !ok || structure.str != "npn" {
			t.Errorf("second = %+v, ожидалось отображение {structure: npn}", second)
		}
	})
	t.Run("неизвестный алиас", func(t *testing.T) {
		if _, err := parseYaml([]byte("a: *nope\n")); err == nil || !contains(err.Error(), "неизвестный алиас «*nope»") {
			t.Errorf("err = %v, ожидалась ошибка неизвестного алиаса", err)
		}
	})
	t.Run("циклический алиас", func(t *testing.T) {
		if _, err := parseYaml([]byte("a: &x [*x]\n")); err == nil || !contains(err.Error(), "циклическая ссылка") {
			t.Errorf("err = %v, ожидалась ошибка циклической ссылки", err)
		}
	})
	t.Run("тег", func(t *testing.T) {
		parsed, err := parseYaml([]byte("tagged: !!str 5\n"))
		if err != nil {
			t.Fatalf("разбор не удался: %v", err)
		}
		// тег разворачивается в помеченный узел как есть (сам тег игнорируется)
		tagged, _ := parsed.has("tagged")
		if tagged.kind != kindNumber || tagged.num != "5" {
			t.Errorf("tagged = %+v, ожидалось число 5", tagged)
		}
	})
	t.Run("литеральный блок", func(t *testing.T) {
		parsed, err := parseYaml([]byte("note: |\n  примечание\n"))
		if err != nil {
			t.Fatalf("разбор не удался: %v", err)
		}
		note, _ := parsed.has("note")
		if note.kind != kindString || !contains(note.str, "примечание") {
			t.Errorf("note = %+v, ожидался текст блока", note)
		}
	})
	t.Run("явная форма ключа", func(t *testing.T) {
		parsed, err := parseYaml([]byte("? key\n: 1\n"))
		if err != nil {
			t.Fatalf("разбор не удался: %v", err)
		}
		key, _ := parsed.has("key")
		if key.kind != kindNumber || key.num != "1" {
			t.Errorf("key = %+v, ожидалось число 1", key)
		}
	})
	t.Run("числовой ключ", func(t *testing.T) {
		if _, err := parseYaml([]byte("1: x\n")); err == nil || !contains(err.Error(), "неверный ключ объекта") {
			t.Errorf("err = %v, ожидалась ошибка «неверный ключ объекта»", err)
		}
	})
	t.Run("merge-ключ", func(t *testing.T) {
		if _, err := parseYaml([]byte("<<: x\n")); err == nil || !contains(err.Error(), "неверный ключ объекта") {
			t.Errorf("err = %v, ожидалась ошибка «неверный ключ объекта»", err)
		}
	})
	t.Run("числовой ключ во вложенном отображении", func(t *testing.T) {
		if _, err := parseYaml([]byte("outer:\n  1: x\n")); err == nil || !contains(err.Error(), "неверный ключ объекта") {
			t.Errorf("err = %v, ожидалась ошибка «неверный ключ объекта»", err)
		}
	})
	t.Run("числовой ключ в элементе последовательности", func(t *testing.T) {
		if _, err := parseYaml([]byte("list:\n  - 1: x\n")); err == nil || !contains(err.Error(), "неверный ключ объекта") {
			t.Errorf("err = %v, ожидалась ошибка «неверный ключ объекта»", err)
		}
	})
}
