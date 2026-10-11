package cli

import (
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/i18n"
)

// Пин-тесты общей справки (генерация по реестру commands): таблица
// сообщений не расходится с реестром команд и флагов, раскладка — в
// пределах ширины, описание list следует набору фильтров реестра.

// registryCmdKeys — ключи секции команд по реестру: команды commandNames,
// подкоманды каталога — «catalog <подкоманда>», help — последней.
func registryCmdKeys() map[string]bool {
	out := map[string]bool{}
	for _, name := range commandNames() {
		cmd := commands[name]
		if cmd.subcommands == nil {
			out[name] = true
			continue
		}
		for _, sub := range subcommandNames(cmd) {
			out[name+" "+sub] = true
		}
	}
	out["help"] = true
	return out
}

func TestUsageCoversRegistry(t *testing.T) {
	want := registryCmdKeys()
	for key := range usageCmds {
		if !want[key] {
			t.Errorf("справка содержит %q вне реестра commands", key)
		}
	}
	for key := range want {
		if _, ok := usageCmds[key]; !ok {
			t.Errorf("команда реестра %q не покрыта справкой (usageCmds)", key)
		}
	}
	// Строк в секции ровно по одной на ключ usageCmds: дубликатов нет
	// (например, «help» не задвоится, если однажды войдёт в реестр).
	for _, lang := range i18n.Languages() {
		if got := len(usageCmdEntries(lang)); got != len(usageCmds) {
			t.Fatalf("локаль %s: строк секции команд %d, ключей usageCmds %d",
				string(lang), got, len(usageCmds))
		}
	}
}

// registryFlags — флаги всех команд реестра (включая подкоманды).
func registryFlags() map[string]bool {
	out := map[string]bool{}
	for _, name := range commandNames() {
		cmd := commands[name]
		if cmd.subcommands != nil {
			for _, sub := range subcommandNames(cmd) {
				for _, f := range cmd.subcommands[sub].flags {
					out[f.name] = true
				}
			}
			continue
		}
		for _, f := range cmd.flags {
			out[f.name] = true
		}
	}
	return out
}

func TestUsageOptionsAreRegistryFlags(t *testing.T) {
	flags := registryFlags()
	documented := map[string]bool{}
	for _, o := range usageOpts {
		if !flags[o.flag] {
			t.Errorf("общая опция «--%s» не встречается ни в одной команде реестра", o.flag)
		}
		documented[o.flag] = true
	}
	// Опции подключения обязаны быть задокументированы как общие.
	for _, f := range dbFlags {
		if !documented[f.name] {
			t.Errorf("опция подключения «--%s» не документирована в общих опциях", f.name)
		}
	}
}

func TestUsageLayout(t *testing.T) {
	for _, lang := range i18n.Languages() {
		text := usageText(lang)
		for i, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			if n := len([]rune(line)); n > usageWidth {
				t.Errorf("локаль %s: строка %d шире %d колонок (%d): %q",
					string(lang), i+1, usageWidth, n, line)
			}
			if strings.HasSuffix(line, " ") {
				t.Errorf("локаль %s: строка %d завершается пробелами: %q",
					string(lang), i+1, line)
			}
		}
		// Синопсис каждой команды реестра — в справке выбранной локали.
		for key, spec := range usageCmds {
			if syn := i18n.Message(lang, string(spec.syn)); !strings.Contains(text, syn) {
				t.Errorf("локаль %s: синопсис %q команды %s отсутствует в справке",
					string(lang), syn, key)
			}
		}
	}
}

// Описание list следует набору фильтров реестра: флаг команды (кроме
// общих опций подключения) попадает в справку без правки текстов.
func TestUsageListFiltersFollowRegistry(t *testing.T) {
	common := map[string]bool{}
	for _, f := range dbFlags {
		common[f.name] = true
	}
	desc := usageCmdEntry(i18n.En, "list").desc
	for _, f := range commands["list"].flags {
		if common[f.name] {
			if strings.Contains(desc, "--"+f.name) {
				t.Errorf("общая опция «--%s» не должна входить в перечень фильтров list", f.name)
			}
			continue
		}
		if !strings.Contains(desc, "--"+f.name) {
			t.Errorf("фильтр «--%s» команды list не попал в описание справки", f.name)
		}
	}
}

// Футер — вложенные префиксы ошибок каталога сообщений (без дубления
// текстов префиксов).
func TestUsageFooterEmbedsPrefixes(t *testing.T) {
	for _, tc := range []struct {
		lang            i18n.Language
		expected, other string
	}{
		{i18n.En, "«Error: »", "«Unexpected error: »"},
		{i18n.Ru, "«Ошибка: »", "«Непредвиденная ошибка: »"},
	} {
		text := usageText(tc.lang)
		if !strings.Contains(text, tc.expected) || !strings.Contains(text, tc.other) {
			t.Errorf("локаль %s: футер не содержит префиксы ошибок %q/%q",
				string(tc.lang), tc.expected, tc.other)
		}
	}
}
