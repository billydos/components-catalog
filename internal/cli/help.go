package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Общая справка собирается динамически: состав и порядок команд — реестр
// commands (commandNames/subcommandNames, help — последней), общие опции —
// таблица usageOpts; строки (синопсисы, описания, заголовки секций, футер) —
// каталог сообщений по локали вывода (--lang), раскладка — выравнивание
// колонок и перенос описаний — вычисляется по фактическим длинам строк
// локали. Справка не может разойтись с реестром: полнота против commands
// и флагов реестра закреплена тестом help_test.go.
const (
	usageIndent = 2  // отступ строк секций справки
	usageGap    = 2  // промежуток между колонками секции
	usageWidth  = 80 // полная ширина строки секции (перенос описаний)
	usagePara   = 72 // ширина абзацев — заголовок и футер (кавычные обороты префиксов не разрываются)
)

// usageEntry — строка двухколоночной секции справки: ключ (синопсис
// команды либо спецификация опции) и описание.
type usageEntry struct {
	key  string
	desc string
}

// usageCmdSpec — сообщения справки команды реестра: синопсис, описание
// и аргументы описания (list — перечень фильтров из собственного набора
// флагов реестра; nil — описание без аргументов).
type usageCmdSpec struct {
	syn, desc domain.MsgID
	descArgs  func() []any
}

// usageCmds — сообщения справки по командам; ключи — имена команд
// реестра, подкоманды каталога — «catalog <подкоманда>», help — последняя
// (вне реестра: справка его читает). Полнота против commands — тестом.
var usageCmds = map[string]usageCmdSpec{
	"init":           {syn: domain.MsgCliUsageCmdInit, desc: domain.MsgCliUsageCmdInitDesc},
	"parse":          {syn: domain.MsgCliUsageCmdParse, desc: domain.MsgCliUsageCmdParseDesc},
	"add":            {syn: domain.MsgCliUsageCmdAdd, desc: domain.MsgCliUsageCmdAddDesc},
	"import":         {syn: domain.MsgCliUsageCmdImport, desc: domain.MsgCliUsageCmdImportDesc},
	"list":           {syn: domain.MsgCliUsageCmdList, desc: domain.MsgCliUsageCmdListDesc, descArgs: listFilterFlags},
	"info":           {syn: domain.MsgCliUsageCmdInfo, desc: domain.MsgCliUsageCmdInfoDesc},
	"find":           {syn: domain.MsgCliUsageCmdFind, desc: domain.MsgCliUsageCmdFindDesc},
	"delete":         {syn: domain.MsgCliUsageCmdDelete, desc: domain.MsgCliUsageCmdDeleteDesc},
	"count":          {syn: domain.MsgCliUsageCmdCount, desc: domain.MsgCliUsageCmdCountDesc},
	"export":         {syn: domain.MsgCliUsageCmdExport, desc: domain.MsgCliUsageCmdExportDesc},
	"catalog export": {syn: domain.MsgCliUsageCmdCatalogExport, desc: domain.MsgCliUsageCmdCatalogExportDesc},
	"catalog import": {syn: domain.MsgCliUsageCmdCatalogImport, desc: domain.MsgCliUsageCmdCatalogImportDesc},
	"catalog list":   {syn: domain.MsgCliUsageCmdCatalogList, desc: domain.MsgCliUsageCmdCatalogListDesc},
	"help":           {syn: domain.MsgCliUsageCmdHelp, desc: domain.MsgCliUsageCmdHelpDesc},
}

// usageOpts — общие опции справки: имя флага (полнота против реестра —
// тестом), спецификация и описание.
var usageOpts = []struct {
	flag      string
	syn, desc domain.MsgID
}{
	{"dialect", domain.MsgCliUsageOptDialect, domain.MsgCliUsageOptDialectDesc},
	{"db", domain.MsgCliUsageOptDb, domain.MsgCliUsageOptDbDesc},
	{"dsn", domain.MsgCliUsageOptDsn, domain.MsgCliUsageOptDsnDesc},
	{"kind", domain.MsgCliUsageOptKind, domain.MsgCliUsageOptKindDesc},
	{"system", domain.MsgCliUsageOptSystem, domain.MsgCliUsageOptSystemDesc},
	{"lang", domain.MsgCliUsageOptLang, domain.MsgCliUsageOptLangDesc},
	{"dry-run", domain.MsgCliUsageOptDryRun, domain.MsgCliUsageOptDryRunDesc},
	{"format", domain.MsgCliUsageOptFormat, domain.MsgCliUsageOptFormatDesc},
}

// usageText — общая справка (catalogctl без аргументов и catalogctl help)
// по локали вывода (--lang): заголовок, секции команд и общих опций (две
// колонки — выравнивание по широкому ключу секций, перенос описаний),
// футер о префиксах ошибок и коде выхода.
func usageText(lang i18n.Language) string {
	cmds := usageCmdEntries(lang)
	opts := usageOptEntries(lang)
	keyWidth := max(widestKey(cmds), widestKey(opts))
	var b strings.Builder
	writeParagraph(&b, lang, domain.MsgCliUsageHeader)
	b.WriteString("\n")
	b.WriteString(i18n.Message(lang, string(domain.MsgCliUsageCommands)) + "\n")
	writeEntries(&b, cmds, keyWidth)
	b.WriteString("\n")
	b.WriteString(i18n.Message(lang, string(domain.MsgCliUsageOptions)) + "\n")
	writeEntries(&b, opts, keyWidth)
	b.WriteString("\n")
	writeParagraph(&b, lang, domain.MsgCliUsageFooter,
		domain.MsgArg(domain.MsgCliErrPrefix), domain.MsgArg(domain.MsgCliUnexpectedPrefix))
	return b.String()
}

// widestKey — ширина широкого ключа секции (рун): общая колонка описаний
// обеих секций выравнивается по нему.
func widestKey(entries []usageEntry) int {
	width := 0
	for _, e := range entries {
		width = max(width, len([]rune(e.key)))
	}
	return width
}

// usageCmdEntries — строки секции команд: реестр commands в порядке
// commandNames (подкоманды каталога — за catalog в порядке
// subcommandNames), help — последней.
func usageCmdEntries(lang i18n.Language) []usageEntry {
	out := make([]usageEntry, 0, len(usageCmds))
	for _, name := range commandNames() {
		cmd := commands[name]
		if cmd.subcommands != nil {
			for _, sub := range subcommandNames(cmd) {
				out = append(out, usageCmdEntry(lang, name+" "+sub))
			}
			continue
		}
		out = append(out, usageCmdEntry(lang, name))
	}
	return append(out, usageCmdEntry(lang, "help"))
}

// usageCmdEntry — строка команды: синопсис и описание по сообщениям
// справки (описание с аргументами — list: перечень фильтров реестра).
func usageCmdEntry(lang i18n.Language, name string) usageEntry {
	spec := usageCmds[name]
	var args []any
	if spec.descArgs != nil {
		args = spec.descArgs()
	}
	return usageEntry{
		key:  i18n.Message(lang, string(spec.syn)),
		desc: i18n.Message(lang, string(spec.desc), args...),
	}
}

// usageOptEntries — строки секции общих опций.
func usageOptEntries(lang i18n.Language) []usageEntry {
	out := make([]usageEntry, 0, len(usageOpts))
	for _, o := range usageOpts {
		out = append(out, usageEntry{
			key:  i18n.Message(lang, string(o.syn)),
			desc: i18n.Message(lang, string(o.desc)),
		})
	}
	return out
}

// listFilterFlags — перечень фильтров команды list для описания в
// справке: флаги реестра list, кроме общих опций подключения и локали
// (dbFlags — своя секция общих опций).
func listFilterFlags() []any {
	common := make(map[string]bool, len(dbFlags))
	for _, f := range dbFlags {
		common[f.name] = true
	}
	flags := make([]string, 0, len(commands["list"].flags))
	for _, f := range commands["list"].flags {
		if !common[f.name] {
			flags = append(flags, "--"+f.name)
		}
	}
	return []any{strings.Join(flags, " ")}
}

// writeEntries — строки секции: отступ, ключ, описания выровнены в
// колонку (ширина широкого ключа обеих секций + usageGap), перенос
// описаний по словам (полная ширина строки — usageWidth), строка
// продолжения — с отступом колонки описаний.
func writeEntries(b *strings.Builder, entries []usageEntry, keyWidth int) {
	descWidth := max(usageWidth-usageIndent-keyWidth-usageGap, 1)
	for _, e := range entries {
		indent := strings.Repeat(" ", usageIndent)
		key := indent + e.key + strings.Repeat(" ", keyWidth-len([]rune(e.key))+usageGap)
		rows := wrapWords(e.desc, descWidth)
		b.WriteString(key + rows[0] + "\n")
		cont := indent + strings.Repeat(" ", keyWidth+usageGap)
		for _, row := range rows[1:] {
			b.WriteString(cont + row + "\n")
		}
	}
}

// writeParagraph — абзац (заголовок, футер): перенос по словам до
// usagePara колонок.
func writeParagraph(b *strings.Builder, lang i18n.Language, id domain.MsgID, args ...any) {
	for _, line := range wrapWords(i18n.Message(lang, string(id), args...), usagePara) {
		b.WriteString(line + "\n")
	}
}

// wrapWords — перенос текста по словам: строки не шире width рун;
// слово длиннее width занимает строку целиком (не разрывается).
func wrapWords(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	out := make([]string, 0, len(words))
	line := words[0]
	for _, w := range words[1:] {
		if len([]rune(line))+1+len([]rune(w)) <= width {
			line += " " + w
			continue
		}
		out = append(out, line)
		line = w
	}
	return append(out, line)
}

// runHelpCmd — help [команда] [--lang en|ru] (команда реестра; разбор
// флагов — urfave/cli): без позиционного аргумента — общая справка,
// с аргументом — справка команды из реестра; «help help» — справка о
// самой справке (usage её рекламирует). Лишние позиционные аргументы и
// неизвестная команда — локализованные ошибки выбранной локали.
func runHelpCmd(_ context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	lang := opts.langOf()
	if len(pos) == 0 {
		fmt.Fprint(stdout, usageText(lang))
		return 0
	}
	if len(pos) > 1 {
		PrintError(stderr, lang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliHelpArgs))
		return 1
	}
	if pos[0] == "help" {
		fmt.Fprintf(stdout, "%s catalogctl %s\n",
			i18n.Message(lang, string(domain.MsgCliUsageWord)),
			i18n.Message(lang, string(domain.MsgCliHelpUsage)))
		return 0
	}
	cmd, ok := commands[pos[0]]
	if !ok {
		PrintError(stderr, lang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, pos[0]))
		return 1
	}
	fmt.Fprintf(stdout, "%s catalogctl %s\n", i18n.Message(lang, string(domain.MsgCliUsageWord)), cmd.usage)
	return 0
}

// commandNames/subcommandNames (sorted) — детерминированный порядок
// построения дерева urfave/cli и секции команд общей справки;
// commandNames используется тестом полноты справки.
func commandNames() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func subcommandNames(cmd *command) []string {
	names := make([]string, 0, len(cmd.subcommands))
	for name := range cmd.subcommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
