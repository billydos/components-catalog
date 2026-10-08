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

// runHelp — справка по командам (help [команда] [--lang en|ru]).
func runHelp(args []string, stdout, stderr io.Writer) int {
	lang := i18n.En
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--lang" {
			if l, ok := i18n.ParseLanguage(args[i+1]); ok {
				lang = l
			}
		}
	}
	if len(args) == 0 {
		fmt.Fprint(stdout, usageText(lang))
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		PrintError(stderr, i18n.En, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, args[0]))
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
