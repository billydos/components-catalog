package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Справка собирается динамически: состав и порядок команд — реестр
// commands (commandNames/subcommandNames, help — последней), общие
// опции — перечень usageOpts, описания флагов — таблица usageFlagMsgs;
// строки (синопсисы, описания, заголовки секций, футер) — каталог
// сообщений по локали вывода (--lang), раскладка — выравнивание колонок
// и перенос описаний — вычисляется по фактическим длинам строк локали.
// Справка не может разойтись с реестром: полнота против commands и
// флагов реестра закреплена тестом help_test.go.
const (
	usageIndent = 2  // отступ строк секций справки
	usageGap    = 2  // промежуток между колонками секции
	usageWidth  = 80 // полная ширина строки справки (перенос описаний)
	usagePara   = 72 // ширина абзацев — заголовок и футер; подобрана под текущие тексты (при 80 перенос разрывает «Unexpected error: » и «Непредвиденная ошибка: »)
)

// usageEntry — строка двухколоночной секции справки: ключ (синопсис
// команды, спецификация опции, команда примера) и описание.
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
	"version":        {syn: domain.MsgCliUsageCmdVersion, desc: domain.MsgCliUsageCmdVersionDesc},
	"catalog export": {syn: domain.MsgCliUsageCmdCatalogExport, desc: domain.MsgCliUsageCmdCatalogExportDesc},
	"catalog import": {syn: domain.MsgCliUsageCmdCatalogImport, desc: domain.MsgCliUsageCmdCatalogImportDesc},
	"catalog list":   {syn: domain.MsgCliUsageCmdCatalogList, desc: domain.MsgCliUsageCmdCatalogListDesc},
	"help":           {syn: domain.MsgCliUsageCmdHelp, desc: domain.MsgCliUsageCmdHelpDesc},
}

// usageFlagMsgs — сообщения флагов (спецификация и описание) по имени
// для общих опций и карточек команд; полнота против флагов реестра —
// тестом.
var usageFlagMsgs = map[string]struct{ syn, desc domain.MsgID }{
	"dialect":    {domain.MsgCliUsageOptDialect, domain.MsgCliUsageOptDialectDesc},
	"db":         {domain.MsgCliUsageOptDb, domain.MsgCliUsageOptDbDesc},
	"dsn":        {domain.MsgCliUsageOptDsn, domain.MsgCliUsageOptDsnDesc},
	"kind":       {domain.MsgCliUsageOptKind, domain.MsgCliUsageOptKindDesc},
	"system":     {domain.MsgCliUsageOptSystem, domain.MsgCliUsageOptSystemDesc},
	"lang":       {domain.MsgCliUsageOptLang, domain.MsgCliUsageOptLangDesc},
	"dry-run":    {domain.MsgCliUsageOptDryRun, domain.MsgCliUsageOptDryRunDesc},
	"format":     {domain.MsgCliUsageOptFormat, domain.MsgCliUsageOptFormatDesc},
	"material":   {domain.MsgCliUsageOptMaterial, domain.MsgCliUsageOptMaterialDesc},
	"subclass":   {domain.MsgCliUsageOptSubclass, domain.MsgCliUsageOptSubclassDesc},
	"adjustment": {domain.MsgCliUsageOptAdjustment, domain.MsgCliUsageOptAdjustmentDesc},
	"category":   {domain.MsgCliUsageOptCategory, domain.MsgCliUsageOptCategoryDesc},
	"junctions":  {domain.MsgCliUsageOptJunctions, domain.MsgCliUsageOptJunctionsDesc},
	"group":      {domain.MsgCliUsageOptGroup, domain.MsgCliUsageOptGroupDesc},
	"series":     {domain.MsgCliUsageOptSeries, domain.MsgCliUsageOptSeriesDesc},
	"number":     {domain.MsgCliUsageOptNumber, domain.MsgCliUsageOptNumberDesc},
	"letters":    {domain.MsgCliUsageOptLetters, domain.MsgCliUsageOptLettersDesc},
	"q":          {domain.MsgCliUsageOptQ, domain.MsgCliUsageOptQDesc},
	"limit":      {domain.MsgCliUsageOptLimit, domain.MsgCliUsageOptLimitDesc},
	"offset":     {domain.MsgCliUsageOptOffset, domain.MsgCliUsageOptOffsetDesc},
}

// usageOpts — порядок общих опций в секции общей справки (подмножество
// флагов реестра, сообщения — usageFlagMsgs).
var usageOpts = []string{"dialect", "db", "dsn", "kind", "system", "lang", "dry-run", "format"}

// usageExamples — примеры общей справки: команда (локаль-нейтральная)
// и короткое описание.
var usageExamples = []struct {
	cmd  string
	desc domain.MsgID
}{
	{"catalogctl init --db catalog.db", domain.MsgCliUsageExInit},
	{"catalogctl import transistors.jsonc", domain.MsgCliUsageExImport},
	{"catalogctl list --kind transistor --material si", domain.MsgCliUsageExList},
	{"catalogctl info КТ315Б", domain.MsgCliUsageExInfo},
}

// usageText — общая справка (catalogctl без аргументов и catalogctl help)
// по локали вывода (--lang): заголовок, секции команд, общих опций и
// примеров (две колонки — выравнивание по широкому ключу секций, перенос
// описаний), примечание о позиции опций, футер о префиксах ошибок и коде
// выхода.
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
	writeExamples(&b, lang)
	b.WriteString("\n")
	writeParagraph(&b, lang, domain.MsgCliUsagePosition)
	b.WriteString("\n")
	writeParagraph(&b, lang, domain.MsgCliUsageFooter,
		domain.MsgArg(domain.MsgCliErrPrefix), domain.MsgArg(domain.MsgCliUnexpectedPrefix))
	return b.String()
}

// usageCard — карточка команды (help <команда>): строка использования с
// полным набором флагов реестра и секция опций с описаниями.
func usageCard(lang i18n.Language, use domain.MsgID, flags []flagSpec) string {
	var b strings.Builder
	writeUseLine(&b, lang, use, flagHints(flags))
	b.WriteString("\n")
	writeFlagSection(&b, lang, flags)
	return b.String()
}

// usageSubLines — карточка команды с подкомандами (help catalog): строки
// использования каждой подкоманды (наборы флагов у подкоманд собственные,
// общая секция опций невозможна).
func usageSubLines(lang i18n.Language, cmd *command) string {
	var b strings.Builder
	for _, name := range subcommandNames(cmd) {
		sub := cmd.subcommands[name]
		writeUseLine(&b, lang, sub.use, flagHints(sub.flags))
	}
	return b.String()
}

// writeUseLine — строка использования с префиксом «формат: catalogctl»:
// перенос по словам до usageWidth, продолжение — с отступа префикса.
func writeUseLine(b *strings.Builder, lang i18n.Language, use domain.MsgID, hints string) {
	prefix := i18n.Message(lang, string(domain.MsgCliUsageWord)) + " " + progName + " "
	rows := wrapWords(i18n.Message(lang, string(use), hints), max(usageWidth-len([]rune(prefix)), 1))
	b.WriteString(prefix + rows[0] + "\n")
	indent := strings.Repeat(" ", len([]rune(prefix)))
	for _, row := range rows[1:] {
		b.WriteString(indent + row + "\n")
	}
}

// writeFlagSection — секция опций карточки: спецификации и описания
// флагов (сообщения usageFlagMsgs), две колонки как в общей справке.
func writeFlagSection(b *strings.Builder, lang i18n.Language, specs []flagSpec) {
	entries := make([]usageEntry, 0, len(specs))
	for _, s := range specs {
		m := usageFlagMsgs[s.name]
		entries = append(entries, usageEntry{
			key:  i18n.Message(lang, string(m.syn)),
			desc: i18n.Message(lang, string(m.desc)),
		})
	}
	b.WriteString(i18n.Message(lang, string(domain.MsgCliUsageFlags)) + "\n")
	writeEntries(b, entries, widestKey(entries))
}

// writeExamples — секция примеров: ширина колонки — по командам примеров.
func writeExamples(b *strings.Builder, lang i18n.Language) {
	entries := make([]usageEntry, 0, len(usageExamples))
	for _, ex := range usageExamples {
		entries = append(entries, usageEntry{
			key:  ex.cmd,
			desc: i18n.Message(lang, string(ex.desc)),
		})
	}
	b.WriteString(i18n.Message(lang, string(domain.MsgCliUsageExamples)) + "\n")
	writeEntries(b, entries, widestKey(entries))
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

// usageOptEntries — строки секции общих опций (порядок — usageOpts,
// сообщения — usageFlagMsgs).
func usageOptEntries(lang i18n.Language) []usageEntry {
	out := make([]usageEntry, 0, len(usageOpts))
	for _, name := range usageOpts {
		m := usageFlagMsgs[name]
		out = append(out, usageEntry{
			key:  i18n.Message(lang, string(m.syn)),
			desc: i18n.Message(lang, string(m.desc)),
		})
	}
	return out
}

// listFilterFlags — перечень фильтров команды list для описания в
// справке: флаги реестра list, кроме общих опций подключения и локали
// (dbFlags — своя секция общих опций).
func listFilterFlags() []any {
	flags := make([]string, 0, len(commands["list"].flags))
	for _, f := range commands["list"].flags {
		if !slices.ContainsFunc(dbFlags, func(c flagSpec) bool { return c.name == f.name }) {
			flags = append(flags, "--"+f.name)
		}
	}
	return []any{strings.Join(flags, " ")}
}

// flagHints — полный перечень флагов реестра для строки использования:
// «[--имя]» через пробел.
func flagHints(specs []flagSpec) string {
	hints := make([]string, 0, len(specs))
	for _, s := range specs {
		hints = append(hints, "[--"+s.name+"]")
	}
	return strings.Join(hints, " ")
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

// writeEntries — строки секции: отступ, ключ, описания выровнены в
// колонку (ширина широкого ключа секции + usageGap), перенос описаний
// по словам (полная ширина строки — usageWidth), строка продолжения —
// с отступа колонки описаний.
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
// флагов — strictFlags): без позиционного аргумента — общая справка, с
// аргументом — карточка команды реестра (строка использования с полным
// набором флагов и описания опций); «catalog <подкоманда>» — карточка
// подкоманды, «catalog» — строки использования подкоманд, «help help» —
// карточка самой справки. Лишние позиционные аргументы и неизвестная
// команда — локализованные ошибки выбранной локали.
// helpArgsError — ошибка лишних позиционных аргументов help: строка
// использования — аргумент-сообщение, тот же рендер, что в карточке
// «help help» и ошибках числа аргументов прочих команд.
func helpArgsError() error {
	return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgCliHelpArgs,
		domain.MsgArg(domain.MsgCliUseHelp, flagHints(helpFlags)))
}

func runHelpCmd(_ context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	lang := opts.langOf()
	if len(pos) == 0 {
		fmt.Fprint(stdout, usageText(lang))
		return 0
	}
	if len(pos) > 2 {
		PrintError(stderr, lang, helpArgsError())
		return 1
	}
	// «help help» — карточка самой справки (вне реестра: справка его
	// читает; флаги — helpFlags).
	if pos[0] == "help" {
		if len(pos) == 2 {
			PrintError(stderr, lang, helpArgsError())
			return 1
		}
		fmt.Fprint(stdout, usageCard(lang, domain.MsgCliUseHelp, helpFlags))
		return 0
	}
	cmd, ok := commands[pos[0]]
	if !ok {
		PrintError(stderr, lang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, pos[0]))
		return 1
	}
	if cmd.subcommands != nil {
		if len(pos) == 1 {
			fmt.Fprint(stdout, usageSubLines(lang, cmd))
			return 0
		}
		sub, known := cmd.subcommands[pos[1]]
		if !known {
			PrintError(stderr, lang, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliCatalogSubUnknown, pos[1]))
			return 1
		}
		fmt.Fprint(stdout, usageCard(lang, sub.use, sub.flags))
		return 0
	}
	if len(pos) == 2 {
		PrintError(stderr, lang, helpArgsError())
		return 1
	}
	fmt.Fprint(stdout, usageCard(lang, cmd.use, cmd.flags))
	return 0
}

// commandNames/subcommandNames (sorted) — детерминированный порядок
// диспетчеризации по реестру и секции команд общей справки;
// commandNames используется тестом полноты справки.
func commandNames() []string {
	return slices.Sorted(maps.Keys(commands))
}

func subcommandNames(cmd *command) []string {
	return slices.Sorted(maps.Keys(cmd.subcommands))
}
