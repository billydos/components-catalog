package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/cli"
)

// Сценарные прогоны CLI (этап 4): полный контур команд на временной базе
// sqlite, префиксы ошибок и коды выхода, справка, round-trip экспорта.

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func testDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "catalog.db")
}

// dataFile — путь к файлу выверенного наполнения data/ из тестов пакета.
func dataFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "data", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("data/%s: %v", name, err)
	}
	return path
}

func TestNoArgsPrintsUsage(t *testing.T) {
	stdout, stderr, code := run(t)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "Commands:") {
		t.Fatalf("код %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, code := run(t, "frobnicate")
	if code != 1 || !strings.HasPrefix(stderr, "Error: ") {
		t.Fatalf("код %d, stderr %q", code, stderr)
	}
	if stderr != "Error: unknown command «frobnicate»; help: catalogctl help\n" {
		t.Fatalf("текст: %q", stderr)
	}
}

func TestHelp(t *testing.T) {
	stdout, _, code := run(t, "help")
	if code != 0 || !strings.Contains(stdout, "catalog import <file>") {
		t.Fatalf("справка: код %d", code)
	}
	stdout, _, code = run(t, "help", "import")
	if code != 0 || !strings.Contains(stdout, "usage: catalogctl import") {
		t.Fatalf("справка import: код %d, %q", code, stdout)
	}
	_, errOut, code := run(t, "help", "frobnicate")
	if code != 1 || !strings.HasPrefix(errOut, "Error: ") {
		t.Fatalf("справка об ошибке: код %d, %q", code, errOut)
	}
}

func TestParseWithoutDatabase(t *testing.T) {
	// Локаль по умолчанию — en (канонический язык, D9).
	stdout, _, code := run(t, "parse", "КТ315Б")
	if code != 0 {
		t.Fatalf("код %d", code)
	}
	want := "КТ315Б: transistor (transistor), system GOST (gost)\n" +
		"  material: silicon\n  subclass: bipolar transistor\n  assembly: device\n" +
		"  development number: 315\n  letters: Б\n"
	if stdout != want {
		t.Fatalf("вывод:\n got:  %q\n want: %q", stdout, want)
	}
	// --lang ru — отображаемые строки ru-бандла.
	stdout, _, code = run(t, "parse", "КТ315Б", "--lang", "ru")
	if code != 0 {
		t.Fatalf("код %d", code)
	}
	want = "КТ315Б: транзисторы (transistor), система ГОСТ (gost)\n" +
		"  материал: кремний\n  подкласс: биполярный транзистор\n  сборка: прибор\n" +
		"  номер разработки: 315\n  буквы: Б\n"
	if stdout != want {
		t.Fatalf("вывод (ru):\n got:  %q\n want: %q", stdout, want)
	}
	_, errOut, code := run(t, "parse", "XX42")
	if code != 1 || !strings.HasPrefix(errOut, "Error: ") {
		t.Fatalf("разбор ошибки: код %d, %q", code, errOut)
	}
	// Неверное значение --lang — ошибка валидации.
	_, errOut, code = run(t, "parse", "КТ315Б", "--lang", "de")
	if code != 1 || !strings.HasPrefix(errOut, "Error: flag «--lang» accepts en or ru") {
		t.Fatalf("--lang de: код %d, %q", code, errOut)
	}
}

func TestUnknownFlag(t *testing.T) {
	_, stderr, code := run(t, "list", "--bogus")
	if code != 1 || !strings.HasPrefix(stderr, "Error: unknown flag «--bogus»") {
		t.Fatalf("код %d, stderr %q", code, stderr)
	}
}

func TestFullCycle(t *testing.T) {
	db := testDB(t)

	stdout, stderr, code := run(t, "init", "--db", db)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "database initialized:") {
		t.Fatalf("init: код %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	stdout, stderr, code = run(t, "import", dataFile(t, "transistors.jsonc"), "--db", db)
	if code != 0 || stderr != "" {
		t.Fatalf("import: код %d, stderr %q", code, stderr)
	}
	want := dataFile(t, "transistors.jsonc") + ": records: 23, added: 23, updated: 0, unchanged: 0\n"
	if stdout != want {
		t.Fatalf("итог импорта:\n got:  %q\n want: %q", stdout, want)
	}

	// Идемпотентность повторного импорта.
	stdout, _, code = run(t, "import", dataFile(t, "transistors.jsonc"), "--db", db)
	if code != 0 || !strings.Contains(stdout, "unchanged: 23") {
		t.Fatalf("повторный import: код %d, %q", code, stdout)
	}

	stdout, stderr, code = run(t, "count", "--db", db)
	if code != 0 || stdout != "23\n" || stderr != "" {
		t.Fatalf("count: код %d, %q, %q", code, stdout, stderr)
	}

	stdout, stderr, code = run(t, "list", "--kind", "transistor", "--material", "кремний", "--db", db)
	if code != 0 || stderr != "" {
		t.Fatalf("list: код %d, %q", code, stderr)
	}
	// tabwriter выравнивает колонки пробелами — проверяем построчно.
	hasLine := func(parts ...string) bool {
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 3 {
				match := true
				for _, p := range parts {
					if !strings.Contains(line, p) {
						match = false
					}
				}
				if match {
					return true
				}
			}
		}
		return false
	}
	if !hasLine("КТ315Б", "transistor", "gost") || !hasLine("КТ805А", "transistor", "gost") {
		t.Fatalf("list --material кремний: кремниевые записи отсутствуют: %q", stdout)
	}
	if hasLine("ГТ109Г") {
		t.Fatalf("list --material кремний: германиевая запись попала в выборку: %q", stdout)
	}
	stdout, stderr, code = run(t, "info", "КТ315Б", "--db", db)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "КТ315Б — transistor") ||
		!strings.Contains(stdout, "Analogs: 2N3904") {
		t.Fatalf("info: код %d, %q, %q", code, stdout, stderr)
	}

	// Сквозные словарные фильтры: подкласс (код и ru-название), категория.
	stdout, stderr, code = run(t, "list", "--kind", "transistor", "--subclass", "bjt", "--db", db)
	if code != 0 || stderr != "" {
		t.Fatalf("list --subclass bjt: код %d, %q", code, stderr)
	}
	if !hasLine("КТ315Б", "transistor", "gost") || !hasLine("MJE340", "transistor", "other") {
		t.Fatalf("list --subclass bjt: %q", stdout)
	}
	if hasLine("IRF540") {
		t.Fatalf("list --subclass bjt: полевой транзистор в выборке: %q", stdout)
	}
	stdout, stderr, code = run(t, "list", "--kind", "transistor", "--subclass", "биполярный транзистор", "--db", db)
	if code != 0 || !hasLine("КТ315Б") {
		t.Fatalf("list --subclass (ru): код %d, %q, %q", code, stdout, stderr)
	}
	stdout, stderr, code = run(t, "list", "--kind", "transistor", "--category", "high_voltage", "--db", db)
	if code != 0 || stderr != "" || !hasLine("MJE340") || hasLine("КТ315Б") {
		t.Fatalf("list --category high_voltage: код %d, %q, %q", code, stdout, stderr)
	}
	_, stderr, code = run(t, "list", "--kind", "transistor", "--subclass", "bogus", "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Error: unknown subclass «bogus»") {
		t.Fatalf("list --subclass bogus: код %d, %q", code, stderr)
	}

	// Карточка записи с явными полями: локализованные значения словарей.
	stdout, stderr, code = run(t, "info", "MJE340", "--lang", "ru", "--db", db)
	if code != 0 || stderr != "" ||
		!strings.Contains(stdout, "материал: кремний") ||
		!strings.Contains(stdout, "подкласс: биполярный транзистор") ||
		!strings.Contains(stdout, "категория: высоковольтный") {
		t.Fatalf("info MJE340 (ru): код %d, %q, %q", code, stdout, stderr)
	}

	stdout, stderr, code = run(t, "find", "КТ315Б", "--db", db)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "КТ315Б — transistor") {
		t.Fatalf("find: код %d, %q, %q", code, stdout, stderr)
	}

	// find по отсутствующей записи с равнозначной подсказкой (материал).
	stdout, _, code = run(t, "find", "2Т315Б", "--db", db)
	if code != 1 || !strings.Contains(stdout, "equivalent by material: КТ315Б") {
		t.Fatalf("find 2Т315Б: код %d, %q", code, stdout)
	}

	stdout, stderr, code = run(t, "delete", "КТ315Б", "--dry-run", "--db", db)
	if code != 0 || stdout != "КТ315Б: will be deleted\n" || stderr != "" {
		t.Fatalf("delete --dry-run: код %d, %q, %q", code, stdout, stderr)
	}
	stdout, _, code = run(t, "count", "--db", db)
	if code != 0 || stdout != "23\n" {
		t.Fatalf("dry-run удалил запись: %q", stdout)
	}

	stdout, stderr, code = run(t, "delete", "КТ315Б", "--db", db)
	if code != 0 || stdout != "КТ315Б: deleted\n" || stderr != "" {
		t.Fatalf("delete: код %d, %q, %q", code, stdout, stderr)
	}
	stdout, _, code = run(t, "count", "--db", db)
	if code != 0 || stdout != "22\n" {
		t.Fatalf("count после удаления: %q", stdout)
	}

	_, stderr, code = run(t, "info", "КТ315Б", "--db", db)
	if code != 1 || stderr != "Error: record «КТ315Б» not found\n" {
		t.Fatalf("info отсутствующей: код %d, %q", code, stderr)
	}
}

func TestImportIssuesReportedToStderr(t *testing.T) {
	db := testDB(t)
	if _, _, code := run(t, "init", "--db", db); code != 0 {
		t.Fatalf("init: %d", code)
	}
	bad := filepath.Join(t.TempDir(), "bad.jsonc")
	content := `{"transistors": [
		{ "name": "МП39", "ratings": [ { "parameter": "UkeoMax", "value": -1 } ] },
		"ГТ109Г"
	]}`
	if err := os.WriteFile(bad, []byte(content), 0o644); err != nil {
		t.Fatalf("запись: %v", err)
	}
	stdout, stderr, code := run(t, "import", bad, "--db", db)
	if code != 1 {
		t.Fatalf("код: %d (ожидался 1 при проблемах)", code)
	}
	if !strings.Contains(stdout, "records: 2, added: 1") {
		t.Fatalf("итог: %q", stdout)
	}
	want := "Problem: record «МП39»: parameter «UkeoMax»: value of key value must be positive\n"
	if stderr != want {
		t.Fatalf("stderr:\n got:  %q\n want: %q", stderr, want)
	}
}

func TestImportDryRunReportAndNoChanges(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	stdout, stderr, code := run(t, "import", dataFile(t, "diodes.jsonc"), "--db", db, "--dry-run")
	if code != 0 || stderr != "" {
		t.Fatalf("dry-run: код %d, %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "dry run (no database write): ") ||
		!strings.Contains(stdout, "added: 25") {
		t.Fatalf("итог dry-run: %q", stdout)
	}
	_, _, code = run(t, "count", "--db", db)
	stdout2, _, _ := run(t, "count", "--db", db)
	if code != 0 && stdout2 != "0\n" {
		t.Fatalf("после dry-run count: %q", stdout2)
	}
}

func TestExportRoundTripAllFormats(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	run(t, "import", dataFile(t, "resistors.jsonc"), "--db", db)
	for _, format := range []string{"jsonc", "yaml", "ndjson"} {
		stdout, stderr, code := run(t, "export", "--kind", "resistor", "--format", format, "--db", db)
		if code != 0 || stderr != "" || !strings.Contains(stdout, "С2-33Н") {
			t.Fatalf("export %s: код %d, %q, %q", format, code, stdout, stderr)
		}
		ext := map[string]string{"jsonc": ".jsonc", "yaml": ".yaml", "ndjson": ".ndjson"}[format]
		file := filepath.Join(t.TempDir(), "export"+ext)
		if err := os.WriteFile(file, []byte(stdout), 0o644); err != nil {
			t.Fatalf("запись: %v", err)
		}
		out, stderr, code := run(t, "import", file, "--db", db)
		if code != 0 || stderr != "" || !strings.Contains(out, "unchanged: 9") {
			t.Fatalf("round-trip %s: код %d, %q, %q", format, code, out, stderr)
		}
	}
}

func TestCatalogCommands(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)

	stdout, stderr, code := run(t, "catalog", "export", "--db", db)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "\"catalog\"") {
		t.Fatalf("catalog export: код %d, %q", code, stdout)
	}
	catFile := filepath.Join(t.TempDir(), "catalog.jsonc")
	if err := os.WriteFile(catFile, []byte(stdout), 0o644); err != nil {
		t.Fatalf("запись: %v", err)
	}

	db2 := testDB(t)
	stdout, stderr, code = run(t, "catalog", "import", catFile, "--db", db2)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "catalog extended") {
		t.Fatalf("catalog import: код %d, %q, %q", code, stdout, stderr)
	}

	// catalog import отвергает файл с записями классов.
	_, stderr, code = run(t, "catalog", "import", dataFile(t, "diodes.jsonc"), "--db", db2)
	if code != 1 || !strings.Contains(stderr, "Error: ") ||
		!strings.Contains(stderr, "the file contains kind records; catalog import applies to files with the catalog section only") {
		t.Fatalf("catalog import записей: код %d, %q", code, stderr)
	}

	stdout, stderr, code = run(t, "catalog", "list", "--db", db2)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Parameters:") ||
		!strings.Contains(stdout, "h21e —") || !strings.Contains(stdout, "Attributes:") {
		t.Fatalf("catalog list: код %d, %q", code, stdout)
	}
}

func TestAddRequiresKindForOther(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	// Обозначение вне строгих систем: обязателен --kind (и --system other —
	// автодетекта для other нет).
	_, stderr, code := run(t, "add", "MJE340", "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Error: ") {
		t.Fatalf("add без --kind (other): код %d, %q", code, stderr)
	}
	_, stderr, code = run(t, "add", "MJE340", "--kind", "transistor", "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Error: ") {
		t.Fatalf("add без --system other: код %d, %q", code, stderr)
	}
	stdout, stderr, code := run(t, "add", "MJE340", "--kind", "transistor", "--system", "other", "--db", db)
	if code != 0 || stderr != "" || stdout != "MJE340: added\n" {
		t.Fatalf("add с --kind/--system: код %d, %q, %q", code, stdout, stderr)
	}
}

func TestLimitFlagValidation(t *testing.T) {
	_, stderr, code := run(t, "list", "--limit", "много")
	if code != 1 || !strings.HasPrefix(stderr, "Error: flag «--limit» requires a non-negative number") {
		t.Fatalf("код %d, %q", code, stderr)
	}
}
