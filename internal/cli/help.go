package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// usageText — общая справка (catalogctl без аргументов и catalogctl help)
// по локали вывода (--lang; строки каталога сообщений cli_usage).
func usageText(lang i18n.Language) string {
	return i18n.Message(lang, string(domain.MsgCliUsage))
}

// runHelp — справка по командам (help [команда] [--lang en|ru]): разбор
// аргументов — общий parseArgs (флаги в любой позиции, формы «--lang X» и
// «--lang=X», как у прочих команд), команда — первый позиционный аргумент;
// неизвестная команда и лишние позиционные аргументы — локализованные
// ошибки выбранной локали.
func runHelp(args []string, stdout, stderr io.Writer) int {
	opts, pos, err := parseArgs("help", []flagSpec{{name: "lang", hasValue: true}}, args)
	if err != nil {
		PrintError(stderr, langFromArgs(args), err)
		return 1
	}
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
	cmd, ok := commands[pos[0]]
	if !ok {
		PrintError(stderr, lang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, pos[0]))
		return 1
	}
	fmt.Fprintf(stdout, "%s catalogctl %s\n", i18n.Message(lang, string(domain.MsgCliUsageWord)), cmd.usage)
	return 0
}

// командаСправка sorted — используется тестом полноты справки.
func commandNames() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
