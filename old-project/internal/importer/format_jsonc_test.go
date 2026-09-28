package importer

import (
	"errors"
	"strings"
	"testing"

	"soviettransistors/internal/domain"
)

func TestParseJsonc_CommentsAndTrailingCommas(t *testing.T) {
	source := `{
		// строкный комментарий
		"a": "значение // не комментарий", /* блочный
		комментарий */
		"b": [1, 2, 3,],
		"c": {"d": "e",},
	}`
	parsed, err := parseJsonc([]byte(source))
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if parsed.kind != kindObject {
		t.Fatalf("корень не объект: %v", parsed.kind)
	}
	a, ok := parsed.has("a")
	if !ok || a.str != "значение // не комментарий" {
		t.Errorf("a = %v, ok = %v", a, ok)
	}
	b, _ := parsed.has("b")
	if len(b.items) != 3 {
		t.Errorf("b.items = %d, ожидалось 3", len(b.items))
	}
	c, _ := parsed.has("c")
	if _, ok := c.has("d"); !ok {
		t.Error("c.d отсутствует")
	}
}

// Стандартный JSON без комментариев и висячих запятых — частный случай jsonc.
func TestParseJsonc_PlainJSON(t *testing.T) {
	parsed, err := parseJsonc([]byte(`{"a": [1, 2], "b": null, "c": true}`))
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	a, _ := parsed.has("a")
	if a.kind != kindArray || len(a.items) != 2 {
		t.Errorf("a = %+v", a)
	}
	b, _ := parsed.has("b")
	if b.kind != kindNull {
		t.Errorf("b = %+v, ожидался null", b)
	}
	c, _ := parsed.has("c")
	if c.kind != kindBool || !c.boolean {
		t.Errorf("c = %+v", c)
	}
}

func TestParseJsoncText_ValidFile(t *testing.T) {
	result, err := ParseJsoncText("{\n\t// комментарий\n\t\"transistors\": [\"ГТ115А\"],\n}\n")
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if result.HasErrors() || len(result.Entries) != 1 || result.Entries[0].Transistor.Name() != "ГТ115А" {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
}

func TestParseJsoncText_SyntaxErrorMessage(t *testing.T) {
	_, err := ParseJsoncText("{ не json")
	if err == nil {
		t.Fatal("ожидалась ошибка синтаксиса")
	}
	if !strings.Contains(err.Error(), "файл не является корректным JSONC: ") {
		t.Errorf("текст ошибки = %q", err.Error())
	}
	var userError *domain.UserError
	if !errors.As(err, &userError) {
		t.Error("ожидалась *domain.UserError")
	}
}

// Повторяющиеся ключи — ошибка разбора файла (как в yaml): первый ключ
// не должен молча отбрасывать второй. Номер строки соответствует исходному
// файлу — Standardize заменяет комментарии пробелами, сохраняя переводы строк.
func TestParseJsoncText_DuplicateKeys_Rejected(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "повтор ключа transistors в корне",
			source: "{\n\t\"transistors\": [\"МП39\"],\n\t\"transistors\": [\"КТ315Б\"]\n}\n",
			want:   "файл не является корректным JSONC: повторяющийся ключ «transistors» (строка 3)",
		},
		{
			name:   "повтор name в записи",
			source: "{\n\t\"transistors\": [\n\t\t{\"name\": \"МП39\", \"name\": \"КТ315Б\"}\n\t]\n}\n",
			want:   "файл не является корректным JSONC: повторяющийся ключ «name» (строка 3)",
		},
		{
			name:   "повтор поля во вложенной секции",
			source: "{\n\t\"transistors\": [\n\t\t{\n\t\t\t\"name\": \"КТ315Б\",\n\t\t\t\"ratings\": {\"IkMax\": 50, \"IkMax\": 60}\n\t\t}\n\t]\n}\n",
			want:   "файл не является корректным JSONC: повторяющийся ключ «IkMax» (строка 5)",
		},
		{
			name:   "комментарий перед дубликатом не смещает номер строки",
			source: "{\n\t\"transistors\": [],\n\t// комментарий\n\t\"transistors\": []\n}\n",
			want:   "файл не является корректным JSONC: повторяющийся ключ «transistors» (строка 4)",
		},
	}
	for _, testCase := range cases {
		_, err := ParseJsoncText(testCase.source)
		if err == nil {
			t.Errorf("%s: ожидалась ошибка повторяющегося ключа", testCase.name)
			continue
		}
		if err.Error() != testCase.want {
			t.Errorf("%s: текст ошибки:\n%q\nожидалось:\n%q", testCase.name, err.Error(), testCase.want)
		}
		var userError *domain.UserError
		if !errors.As(err, &userError) {
			t.Errorf("%s: ожидалась *domain.UserError", testCase.name)
		}
	}
}
