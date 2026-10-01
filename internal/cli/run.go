package cli

import (
	"fmt"
	"io"
)

// Run выполняет команду catalogctl и возвращает код выхода.
//
// Скелет этапа 0 (plan/05-work-plan.md): реестр команд появляется на
// этапе 4; сейчас заглушка, чтобы бинарник собирался, а контракт вывода
// ошибок был закреплён тестами.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "catalogctl: команда не задана")
		return 1
	}
	fmt.Fprintf(stderr, "catalogctl: команда %q ещё не реализована\n", args[0])
	return 1
}
