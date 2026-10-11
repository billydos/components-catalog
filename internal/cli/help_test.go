package cli

import (
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Пин-тесты справки (генерация по реестру commands): таблицы сообщений
// не расходятся с реестром команд и флагов, карточки и общая справка — в
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

// leafCommands — листовые команды реестра (без подкоманд) и подкоманды.
func leafCommands() map[string]*command {
	out := map[string]*command{}
	for _, name := range commandNames() {
		cmd := commands[name]
		if cmd.subcommands != nil {
			for _, sub := range subcommandNames(cmd) {
				out[name+" "+sub] = cmd.subcommands[sub]
			}
			continue
		}
		out[name] = cmd
	}
	return out
}

func TestUsageOptionsAreRegistryFlags(t *testing.T) {
	flags := registryFlags()
	for _, name := range usageOpts {
		if !flags[name] {
			t.Errorf("общая опция «--%s» не встречается ни в одной команде реестра", name)
		}
	}
	// Опции подключения обязаны быть задокументированы как общие.
	for _, f := range dbFlags {
		if !contains(usageOpts, f.name) {
			t.Errorf("опция подключения «--%s» не документирована в общих опциях", f.name)
		}
	}
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Каждый флаг реестра и helpFlags описан в usageFlagMsgs; каждая листовая
// команда имеет строку использования — новый флаг или команда не
// останутся без описаний справки.
func TestUsageFlagsAndUseLinesCovered(t *testing.T) {
	flags := registryFlags()
	for _, f := range helpFlags {
		flags[f.name] = true
	}
	for name := range flags {
		if _, ok := usageFlagMsgs[name]; !ok {
			t.Errorf("флаг «--%s» реестра не описан в usageFlagMsgs", name)
		}
	}
	for name, cmd := range leafCommands() {
		if cmd.use == "" {
			t.Errorf("команда %q без строки использования (use)", name)
		}
	}
}

// Карточки команд: раскладка в пределах ширины, без хвостовых пробелов,
// в обеих локалях (листовые команды, подкоманды и родитель catalog).
func TestUsageCardsLayout(t *testing.T) {
	check := func(what, text string, lang i18n.Language) {
		t.Helper()
		for i, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			if n := len([]rune(line)); n > usageWidth {
				t.Errorf("локаль %s: %s, строка %d шире %d колонок (%d): %q",
					string(lang), what, i+1, usageWidth, n, line)
			}
			if strings.HasSuffix(line, " ") {
				t.Errorf("локаль %s: %s, строка %d завершается пробелами: %q",
					string(lang), what, i+1, line)
			}
		}
	}
	for _, lang := range i18n.Languages() {
		for name, cmd := range leafCommands() {
			check("карточка "+name, usageCard(lang, cmd.use, cmd.flags), lang)
		}
		check("карточка help", usageCard(lang, domain.MsgCliUseHelp, helpFlags), lang)
		check("каталог", usageSubLines(lang, commands["catalog"]), lang)
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
		// Примеры и примечание о позиции опций — в справке.
		for _, ex := range usageExamples {
			if !strings.Contains(text, ex.cmd) {
				t.Errorf("локаль %s: пример %q отсутствует в справке", string(lang), ex.cmd)
			}
		}
		if !strings.Contains(text, "«--»") {
			t.Errorf("локаль %s: примечание о «--» отсутствует в справке", string(lang))
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
