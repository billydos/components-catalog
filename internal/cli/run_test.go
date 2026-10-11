package cli_test

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/cli"
	"github.com/billydos/components-catalog/internal/testutil"
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
	// Примеры и примечание о позиции опций — в общей справке.
	if !strings.Contains(stdout, "catalogctl init --db catalog.db") ||
		!strings.Contains(stdout, "«--»") {
		t.Fatalf("общая справка без примеров/примечания: %q", stdout)
	}
	stdout, _, code = run(t, "help", "import")
	if code != 0 || !strings.HasPrefix(stdout, "usage: catalogctl import") {
		t.Fatalf("справка import: код %d, %q", code, stdout)
	}
	// Карточка локализована и содержит полный набор флагов реестра.
	if !strings.Contains(stdout, "import <file> [--dry-run] [--dialect] [--db] [--dsn] [--lang]") ||
		strings.Contains(stdout, "<файл>") {
		t.Fatalf("карточка import: %q", stdout)
	}
	_, errOut, code := run(t, "help", "frobnicate")
	if code != 1 || !strings.HasPrefix(errOut, "Error: ") {
		t.Fatalf("справка об ошибке: код %d, %q", code, errOut)
	}
}

// Карточки команд: строка использования с полным набором флагов и
// описания опций (в том числе фильтры list); подкоманды каталога —
// «help catalog <подкоманда>», «help catalog» — строки использования.
func TestHelpCommandCards(t *testing.T) {
	stdout, _, code := run(t, "help", "list")
	if code != 0 || !strings.HasPrefix(stdout, "usage: catalogctl list") {
		t.Fatalf("карточка list: код %d, %q", code, stdout)
	}
	for _, want := range []string{
		"Options:", "--kind <code>", "--material <code>",
		"--q <text>", "--limit <n>", "1–200", "--offset <n>",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("карточка list без %q:\n%s", want, stdout)
		}
	}

	stdout, _, code = run(t, "help", "catalog", "export")
	if code != 0 || !strings.HasPrefix(stdout, "usage: catalogctl catalog export") ||
		!strings.Contains(stdout, "--format jsonc|yaml|ndjson") {
		t.Fatalf("карточка catalog export: код %d, %q", code, stdout)
	}

	stdout, _, code = run(t, "help", "catalog")
	if code != 0 ||
		strings.Count(stdout, "usage: catalogctl catalog") != 3 {
		t.Fatalf("карточка catalog (три подкоманды): код %d, %q", code, stdout)
	}

	// Карточка на локали: плейсхолдеры локализованы.
	stdout, _, code = run(t, "help", "parse", "--lang", "ru")
	if code != 0 || !strings.HasPrefix(stdout, "формат: catalogctl parse") ||
		!strings.Contains(stdout, "<обозначение>…") || strings.Contains(stdout, "<designation>") {
		t.Fatalf("карточка parse (ru): код %d, %q", code, stdout)
	}

	// Неизвестная подкоманда и лишний аргумент — локализованные ошибки.
	_, errOut, code := run(t, "help", "catalog", "bogus")
	if code != 1 || !strings.HasPrefix(errOut, "Error: ") {
		t.Fatalf("help catalog bogus: код %d, %q", code, errOut)
	}
	_, errOut, code = run(t, "help", "catalog", "export", "extra")
	if code != 1 || !strings.HasPrefix(errOut, "Error: ") {
		t.Fatalf("help catalog export extra: код %d, %q", code, errOut)
	}
}

// version — версия сборки (локальная сборка — «(devel)»); в общей
// справке команда присутствует.
func TestVersionCommand(t *testing.T) {
	stdout, stderr, code := run(t, "version")
	if code != 0 || stderr != "" || stdout != "catalogctl (devel)\n" {
		t.Fatalf("version: код %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	stdout, _, code = run(t, "help")
	if code != 0 || !strings.Contains(stdout, "  version") {
		t.Fatalf("общая справка без version: %q", stdout)
	}
}

// Строка использования в ошибке числа аргументов — аргумент-сообщение:
// канонический en и локализованный ru совпадают с карточкой команды.
func TestArgCountUsageLocalized(t *testing.T) {
	_, stderr, code := run(t, "import", "a", "b")
	if code != 1 ||
		!strings.Contains(stderr, "usage: import <file> [--dry-run] [--dialect] [--db] [--dsn] [--lang]") ||
		strings.Contains(stderr, "<файл>") {
		t.Fatalf("import a b (en): код %d, %q", code, stderr)
	}
	_, stderr, code = run(t, "import", "a", "b", "--lang", "ru")
	if code != 1 || !strings.Contains(stderr, "формат: import <файл> [--dry-run]") {
		t.Fatalf("import a b (ru): код %d, %q", code, stderr)
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

func TestParseUnknownKindSystem(t *testing.T) {
	// Неизвестные --kind/--system — ожидаемые ошибки валидации:
	// префикс «Error: »/«Ошибка: », код выхода 1.
	_, errOut, code := run(t, "parse", "КТ315Б", "--kind", "foo")
	if code != 1 || errOut != "Error: unknown device kind «foo»\n" {
		t.Fatalf("--kind foo: код %d, %q", code, errOut)
	}
	_, errOut, code = run(t, "parse", "КТ315Б", "--system", "bar")
	if code != 1 || errOut != "Error: unknown designation system «bar»\n" {
		t.Fatalf("--system bar: код %d, %q", code, errOut)
	}
	_, errOut, code = run(t, "parse", "КТ315Б", "--kind", "foo", "--lang", "ru")
	if code != 1 || errOut != "Ошибка: неизвестный класс приборов «foo»\n" {
		t.Fatalf("--kind foo ru: код %d, %q", code, errOut)
	}
	_, errOut, code = run(t, "parse", "КТ315Б", "--system", "bar", "--lang", "ru")
	if code != 1 || errOut != "Ошибка: неизвестная система обозначений «bar»\n" {
		t.Fatalf("--system bar ru: код %d, %q", code, errOut)
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
		{ "name": "ГТ109Г", "ratings": [ { "parameter": "UkeoMax", "value": -1 } ] },
		"КТ315Б"
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
	want := "Problem: record «ГТ109Г»: parameter «UkeoMax»: value of key value must be positive\n"
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

func TestHelpLangVariants(t *testing.T) {
	// Флаг --lang до команды, после команды и в форме «--lang=» — разбор
	// общий с прочими командами; справка на выбранной локали.
	for _, args := range [][]string{
		{"help", "--lang", "ru", "parse"},
		{"help", "--lang=ru", "parse"},
		{"help", "parse", "--lang", "ru"},
	} {
		stdout, _, code := run(t, args...)
		if code != 0 || !strings.HasPrefix(stdout, "формат: catalogctl parse") {
			t.Fatalf("help %v: код %d, справка %q", args, code, stdout)
		}
	}
	// help без команды, но с флагом — общая справка на локали.
	stdout, _, code := run(t, "help", "--lang", "ru")
	if code != 0 || !strings.HasPrefix(stdout, "catalogctl — консольная") {
		t.Fatalf("help --lang ru: код %d, справка %q", code, stdout)
	}
	// Неизвестная команда — ошибка на выбранной локали.
	_, stderr, code := run(t, "help", "--lang", "ru", "nosuch")
	if code != 1 || stderr != "Ошибка: неизвестная команда «nosuch»; справка: catalogctl help\n" {
		t.Fatalf("help nosuch (ru): код %d, %q", code, stderr)
	}
	// Лишний позиционный аргумент после команды — ошибка; строка
	// использования — та же, что в карточке «help help».
	_, stderr, code = run(t, "help", "parse", "extra")
	if code != 1 ||
		stderr != "Error: wrong number of arguments; usage: catalogctl help [command] [--lang]\n" {
		t.Fatalf("help лишний аргумент: код %d, %q", code, stderr)
	}
	_, stderr, code = run(t, "help", "--lang", "ru", "parse", "extra")
	if code != 1 ||
		stderr != "Ошибка: неверное число аргументов; формат: catalogctl help [команда] [--lang]\n" {
		t.Fatalf("help лишний аргумент (ru): код %d, %q", code, stderr)
	}
}

// --help/-h — маршрутизация в команду help: общая справка в stdout,
// код 0; опции после флага разбираются командой help (карточка).
func TestHelpFlagRouting(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		stdout, stderr, code := run(t, flag)
		if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "catalogctl — ") {
			t.Fatalf("%s: код %d, stdout %q, stderr %q", flag, code, stdout, stderr)
		}
	}
	stdout, _, code := run(t, "--help", "list")
	if code != 0 || !strings.HasPrefix(stdout, "usage: catalogctl list") {
		t.Fatalf("--help list: код %d, %q", code, stdout)
	}
}

// Строгая лексика флагов у самой справки и версии: посторонний флаг —
// неизвестный флаг, а не молчаливое игнорирование.
func TestHelpAndVersionFlagLexing(t *testing.T) {
	_, stderr, code := run(t, "help", "--bogus")
	if code != 1 || !strings.HasPrefix(stderr, "Error: unknown flag «--bogus»") {
		t.Fatalf("help --bogus: код %d, %q", code, stderr)
	}
	_, stderr, code = run(t, "version", "--db", "x.db")
	if code != 1 || !strings.HasPrefix(stderr, "Error: unknown flag «--db»") {
		t.Fatalf("version --db: код %d, %q", code, stderr)
	}
}

func TestCatalogExportToFile(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	want, _, code := run(t, "catalog", "export", "--db", db)
	if code != 0 || !strings.Contains(want, "\"catalog\"") {
		t.Fatalf("catalog export (stdout): код %d, %q", code, want)
	}

	// Аргумент — файл назначения: выгрузка записывается в файл (формат —
	// по --format, не по расширению), stdout пуст.
	file := filepath.Join(t.TempDir(), "catalog-export.txt")
	stdout, stderr, code := run(t, "catalog", "export", file, "--db", db)
	if code != 0 || stderr != "" || stdout != "" {
		t.Fatalf("catalog export <файл>: код %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("файл выгрузки не создан: %v", err)
	}
	if string(data) != want {
		t.Fatalf("файл выгрузки отличается от stdout-выгрузки:\n got:  %q\n want: %q", data, want)
	}
	db2 := testDB(t)
	run(t, "init", "--db", db2)
	// Формат выгрузки — jsonc (по --format), а не по расширению: для
	// round-trip импорта копия получает распознаваемое расширение.
	roundTrip := filepath.Join(t.TempDir(), "catalog-round-trip.jsonc")
	if err := os.WriteFile(roundTrip, data, 0o644); err != nil {
		t.Fatalf("запись: %v", err)
	}
	out, stderr, code := run(t, "catalog", "import", roundTrip, "--db", db2)
	if code != 0 || stderr != "" || !strings.Contains(out, "catalog extended") {
		t.Fatalf("round-trip каталога: код %d, %q, %q", code, out, stderr)
	}

	// «-» — выгрузка в stdout.
	stdout, stderr, code = run(t, "catalog", "export", "-", "--db", db)
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("catalog export -: код %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// Ошибка создания файла — доменная ошибка cli_file_write.
	bad := filepath.Join(t.TempDir(), "no-such-dir", "catalog.jsonc")
	_, stderr, code = run(t, "catalog", "export", bad, "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Error: cannot write file «"+bad+"»") {
		t.Fatalf("catalog export в недоступный путь: код %d, %q", code, stderr)
	}

	// Два аргумента — ошибка.
	_, stderr, code = run(t, "catalog", "export", file, file, "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Error: ") {
		t.Fatalf("catalog export с двумя аргументами: код %d, %q", code, stderr)
	}
}

func TestInitMasksDsnPassword(t *testing.T) {
	// sqlite-путь печатается как есть (маскировке не подлежит).
	db := testDB(t)
	stdout, stderr, code := run(t, "init", "--db", db)
	if code != 0 || stderr != "" || !strings.Contains(stdout, db) {
		t.Fatalf("init sqlite: код %d, %q, %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, "***") {
		t.Fatalf("init sqlite исказил путь: %q", stdout)
	}
}

func TestInitMasksPostgresPassword(t *testing.T) {
	dsn := testutil.PostgresDSN(t)
	secret := dsnSecret(dsn)
	if secret == "" {
		t.Skip("DSN без пароля — маскирование недоступно для проверки")
	}
	testutil.DropAllTables(t, dsn)
	stdout, stderr, code := run(t, "init", "--dialect", "postgres", "--dsn", dsn)
	if code != 0 || stderr != "" {
		t.Fatalf("init postgres: код %d, %q, %q", code, stdout, stderr)
	}
	// Утечка — исходный DSN в выводе. Голое значение пароля не годится как
	// признак утечки: оно может совпадать с другими частями DSN (CI:
	// postgres://postgres:postgres@…) и присутствовать в замаскированном
	// выводе законно.
	if strings.Contains(stdout, dsn) {
		t.Fatalf("init печатает исходный DSN postgres: %q", stdout)
	}
	if !strings.Contains(stdout, "***") {
		t.Fatalf("init не маскирует пароль postgres: %q", stdout)
	}
}

// dsnSecret — пароль DSN (URL-форма userinfo либо ключевая форма
// password=); пусто — пароля нет.
func dsnSecret(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		pw, _ := u.User.Password()
		return pw
	}
	lower := strings.ToLower(dsn)
	i := strings.Index(lower, "password=")
	if i < 0 {
		return ""
	}
	rest := dsn[i+len("password="):]
	if rest != "" && (rest[0] == '\'' || rest[0] == '"') {
		quote := rest[0]
		if end := strings.IndexByte(rest[1:], quote); end >= 0 {
			return rest[1 : 1+end]
		}
		return rest[1:]
	}
	if end := strings.IndexAny(rest, " \t"); end >= 0 {
		return rest[:end]
	}
	return rest
}

func TestLimitRangeValidation(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	// Диапазон --limit симметричен REST: 0 и выше потолка — ошибка
	// api_limit_range (не тихий клампинг и не «0 = по умолчанию»).
	for _, v := range []string{"0", "201"} {
		_, stderr, code := run(t, "list", "--limit", v, "--db", db)
		want := "Error: parameter limit: an integer from 1 to 200 is expected\n"
		if code != 1 || stderr != want {
			t.Fatalf("--limit %s: код %d, %q (want %q)", v, code, stderr, want)
		}
	}
	_, stderr, code := run(t, "list", "--limit", "200", "--lang", "ru", "--db", db)
	if code != 0 || stderr != "" {
		t.Fatalf("--limit 200: код %d, %q", code, stderr)
	}
	_, stderr, code = run(t, "list", "--limit", "0", "--lang", "ru", "--db", db)
	want := "Ошибка: параметр limit: ожидается целое от 1 до 200\n"
	if code != 1 || stderr != want {
		t.Fatalf("--limit 0 (ru): код %d, %q (want %q)", code, stderr, want)
	}
}

// TestListAll — позиционный аргумент «all»: все записи без пагинации
// (CLI итерирует страницы поиска; потолок limit ≤ 200 — контракт сервиса).
// Датасет 205 записей перекрывает потолок — проверяется сшивка страниц;
// «all» несовместим с --limit/--offset, иной позиционный аргумент — ошибка.
func TestListAll(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	var ds strings.Builder
	for i := 0; i < 205; i++ {
		fmt.Fprintf(&ds, "{\"transistors\":{\"name\":\"2N%d\",\"system\":\"jedec\"}}\n", 1000+i)
	}
	path := filepath.Join(t.TempDir(), "many.ndjson")
	if err := os.WriteFile(path, []byte(ds.String()), 0o600); err != nil {
		t.Fatalf("датасет: %v", err)
	}
	stdout, stderr, code := run(t, "import", path, "--db", db)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "records: 205, added: 205") {
		t.Fatalf("import: код %d, %q, %q", code, stdout, stderr)
	}

	// Без «all» — страница по умолчанию (50 записей).
	stdout, stderr, code = run(t, "list", "--db", db)
	if code != 0 || stderr != "" || strings.Count(stdout, "\n") != 50 {
		t.Fatalf("list без all: код %d, строк %d, %q", code, strings.Count(stdout, "\n"), stderr)
	}

	// «all» — все 205 записей (200 + хвост второй страницы), без
	// дублей и пропусков, порядок детерминирован (kind, designation).
	stdout, stderr, code = run(t, "list", "all", "--db", db)
	if code != 0 || stderr != "" {
		t.Fatalf("list all: код %d, %q", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 205 {
		t.Fatalf("list all: строк %d, ожидается 205", len(lines))
	}
	seen := make(map[string]bool, len(lines))
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[1] != "transistor" || fields[2] != "jedec" {
			t.Fatalf("строка %d не из трёх полей записи: %q", i+1, line)
		}
		if seen[fields[0]] {
			t.Fatalf("дубликат обозначения: %q", fields[0])
		}
		seen[fields[0]] = true
	}
	if fields := strings.Fields(lines[0]); fields[0] != "2N1000" {
		t.Fatalf("первая строка: %q", lines[0])
	}
	if fields := strings.Fields(lines[204]); fields[0] != "2N1204" {
		t.Fatalf("последняя строка: %q", lines[204])
	}

	// Фильтры совместно с «all».
	stdout, stderr, code = run(t, "list", "all", "--kind", "transistor", "--db", db)
	if code != 0 || stderr != "" || strings.Count(stdout, "\n") != 205 {
		t.Fatalf("list all --kind: код %d, строк %d, %q", code, strings.Count(stdout, "\n"), stderr)
	}

	// «all» несовместим с --limit/--offset — в том числе с явным
	// no-op значением --offset 0 (молчаливое игнорирование запрещено).
	for _, args := range [][]string{
		{"--limit", "10"},
		{"--offset", "0"},
	} {
		_, stderr, code = run(t, append([]string{"list", "all"}, args...)...)
		if code != 1 ||
			!strings.HasPrefix(stderr, "Error: argument «all» cannot be combined with --limit/--offset") {
			t.Fatalf("list all %v: код %d, %q", args, code, stderr)
		}
	}
	_, stderr, code = run(t, "list", "all", "--offset", "5", "--lang", "ru", "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Ошибка: аргумент «all» несовместим с --limit/--offset") {
		t.Fatalf("list all --offset (ru): код %d, %q", code, stderr)
	}

	// Единственный допустимый позиционный аргумент — «all».
	_, stderr, code = run(t, "list", "everything", "--db", db)
	if code != 1 ||
		stderr != "Error: unknown argument «everything» of list; the only accepted argument is «all»\n" {
		t.Fatalf("list everything: код %d, %q", code, stderr)
	}
	_, stderr, code = run(t, "list", "all", "all", "--db", db)
	if code != 1 || !strings.HasPrefix(stderr, "Error: wrong number of arguments for command list") {
		t.Fatalf("list all all: код %d, %q", code, stderr)
	}
}

// «help help» — справка о самой справке, а не «неизвестная команда».
func TestHelpHelp(t *testing.T) {
	stdout, stderr, code := run(t, "help", "help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "help [command]") {
		t.Fatalf("код %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// Опции до команды — явная подсказка позиции опций, а не «неизвестная
// команда --db».
func TestOptionsBeforeCommandHinted(t *testing.T) {
	_, stderr, code := run(t, "--db", "/tmp/x.db", "count")
	if code != 1 || !strings.HasPrefix(stderr, "Error: ") ||
		!strings.Contains(stderr, "options must follow the command") {
		t.Fatalf("код %d, stderr %q", code, stderr)
	}
}

// Флаги соседних подкоманд catalog не принимаются молча.
func TestCatalogSubcommandFlagsStrict(t *testing.T) {
	db := testDB(t)
	run(t, "init", "--db", db)
	_, stderr, code := run(t, "catalog", "list", "--db", db, "--dry-run")
	if code != 1 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("catalog list --dry-run: код %d, stderr %q", code, stderr)
	}
	_, stderr, code = run(t, "catalog", "export", "--db", db, "--dry-run")
	if code != 1 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("catalog export --dry-run: код %d, stderr %q", code, stderr)
	}
	// Свои флаги работают.
	cat := filepath.Join(t.TempDir(), "cat.jsonc")
	if err := os.WriteFile(cat, []byte(`{"catalog": {"units": [ { "code": "kV" } ]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = run(t, "catalog", "import", cat, "--db", db, "--dry-run")
	if code != 0 || stderr != "" {
		t.Fatalf("catalog import --dry-run: код %d, stderr %q", code, stderr)
	}
}

// Форма флага — ровно два дефиса; значение не может быть следующим
// флагом.
func TestFlagLexingStrict(t *testing.T) {
	db := testDB(t)
	_, stderr, code := run(t, "count", "-kind", "transistor", "--db", db)
	if code != 1 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("-kind: код %d, stderr %q", code, stderr)
	}
	_, stderr, code = run(t, "count", "---kind", "transistor", "--db", db)
	if code != 1 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("---kind: код %d, stderr %q", code, stderr)
	}
	_, stderr, code = run(t, "list", "--kind", "--system", "--db", db)
	if code != 1 || !strings.Contains(stderr, "requires a value") {
		t.Fatalf("--kind --system: код %d, stderr %q", code, stderr)
	}
}

// «--» — конец флагов: всё после него — позиционные аргументы (похожие
// на флаги токены разбираются как обозначения, повторный «--» — тоже
// позиционный); одиночный «-» — позиционный аргумент, не форма флага.
func TestDoubleDashPositionals(t *testing.T) {
	// Обозначение после «--» разбирается как обычно.
	stdout, _, code := run(t, "parse", "--", "КТ315Б")
	if code != 0 || !strings.Contains(stdout, "КТ315Б: transistor") {
		t.Fatalf("parse -- КТ315Б: код %d, stdout %q", code, stdout)
	}
	// Флаги до «--» применяются; «--» не входит в позиционные.
	stdout, _, code = run(t, "parse", "--lang", "ru", "--", "КТ315Б")
	if code != 0 || !strings.Contains(stdout, "номер разработки: 315") {
		t.Fatalf("parse --lang ru -- КТ315Б: код %d, stdout %q", code, stdout)
	}
	// Токены после «--» — не флаги: «--lang» и «ru» разбираются как
	// обозначения (ошибки), обозначение до «--» — корректно.
	_, stderr, code := run(t, "parse", "КТ315Б", "--", "--lang", "ru")
	if code != 1 || !strings.Contains(stderr, "«--LANG»") || !strings.Contains(stderr, "«RU»") {
		t.Fatalf("parse КТ315Б -- --lang ru: код %d, stderr %q", code, stderr)
	}
	// Повторный «--» — сам позиционный аргумент.
	_, stderr, code = run(t, "parse", "КТ315Б", "--", "--", "КТ315Б")
	if code != 1 || !strings.Contains(stderr, "designation «--»") {
		t.Fatalf("parse КТ315Б -- -- КТ315Б: код %d, stderr %q", code, stderr)
	}
	// Одиночный «-» — позиционный аргумент (не неизвестный флаг).
	_, stderr, code = run(t, "parse", "-", "КТ315Б")
	if code != 1 || !strings.Contains(stderr, "designation «-»") ||
		strings.Contains(stderr, "unknown flag") {
		t.Fatalf("parse - КТ315Б: код %d, stderr %q", code, stderr)
	}
}

// --dsn postgres://… без --dialect выводит диалект из схемы DSN;
// противоречие с явным --dialect sqlite — подсказка, а не непрозрачная
// ошибка драйвера.
func TestDsnDialectDerivation(t *testing.T) {
	_, stderr, code := run(t, "init", "--dsn", "postgres://u:p@127.0.0.1:1/db")
	if code == 1 && strings.Contains(stderr, "unable to open database file") {
		t.Fatalf("dsn postgres молча трактован как sqlite-путь: %s", stderr)
	}

	_, stderr, code = run(t, "init", "--dsn", "postgres://u:p@127.0.0.1:1/db",
		"--dialect", "sqlite")
	if code != 1 || !strings.Contains(stderr, "--dialect") ||
		!strings.Contains(stderr, "postgres") {
		t.Fatalf("противоречие dsn/dialect: код %d, stderr %q", code, stderr)
	}
}
