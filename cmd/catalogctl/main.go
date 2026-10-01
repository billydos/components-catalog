// Команда catalogctl — консольная утилита администрирования справочника
// (init, import, add, list, info, find, delete, count, parse, export, catalog).
// Точка входа CLI: только main, вся логика — в internal/cli поверх сервисного
// слоя (plan/01-architecture.md §1).
package main

import (
	"os"

	"github.com/billydos/components-catalog/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
