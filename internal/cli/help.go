package cli

import (
	"context"
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
// построения дерева urfave/cli; commandNames используется тестом
// полноты справки.
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
