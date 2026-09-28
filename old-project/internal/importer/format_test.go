package importer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"soviettransistors/internal/domain"
)

func TestParseFile_UnsupportedExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("ожидалась ошибка неподдерживаемого расширения")
	}
	if !strings.Contains(err.Error(), "неподдерживаемое расширение файла «.txt»") {
		t.Errorf("текст ошибки = %q", err.Error())
	}
}

func TestParseFile_DispatchByExtension(t *testing.T) {
	cases := []struct {
		extension string
		content   string
	}{
		{"yaml", "transistors:\n  - КТ315Б\n"},
		{"yml", "transistors:\n  - КТ315Б\n"},
		{"json", `{"transistors": ["КТ315Б"]}`},
		// jsonc с комментарием и висячей запятой — расширение .jsonc и регистр
		{"jsonc", "{\n\t// комментарий\n\t\"transistors\": [\"КТ315Б\"],\n}"},
		{"JSON", `{"transistors": ["КТ315Б"]}`},
		{"YML", "transistors:\n  - КТ315Б\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.extension, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data."+testCase.extension)
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := ParseFile(path)
			if err != nil {
				t.Fatalf("разбор не удался: %v", err)
			}
			if result.HasErrors() || len(result.Entries) != 1 || result.Entries[0].Transistor.Name() != "КТ315Б" {
				t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
			}
		})
	}
}

func TestParseFile_ReadError(t *testing.T) {
	_, err := ParseFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("ожидалась ошибка чтения отсутствующего файла")
	}
	var userError *domain.UserError
	if errors.As(err, &userError) {
		t.Error("ошибка файловой системы не должна заворачиваться в *domain.UserError")
	}
}
