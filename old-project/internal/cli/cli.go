// Package cli — разбор аргументов командной строки и вывод: команды справочника
// советских транзисторов. Ожидаемые ошибки домена печатаются с префиксом
// «Ошибка: », прочие — «Непредвиденная ошибка: »; код выхода 1.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"soviettransistors/internal/domain"
)

const defaultDatabaseFile = "transistors.db"

// Run выполняет команду; возвращаемое значение — код выхода процесса.
func Run(args []string) int {
	var command string
	var rest []string
	var dbPath string
	dbSet := false
	dryRun := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Ошибка: после --db ожидается путь к файлу базы")
				return 1
			}
			i++
			dbPath = args[i]
			dbSet = true
		case strings.HasPrefix(arg, "--db="):
			dbPath = strings.TrimPrefix(arg, "--db=")
			dbSet = true
		case arg == "--dry-run":
			dryRun = true
		case command == "":
			command = strings.ToLower(arg)
		default:
			rest = append(rest, arg)
		}
		if dbSet && dbPath == "" {
			fmt.Fprintln(os.Stderr, "Ошибка: путь к файлу базы не может быть пустым")
			return 1
		}
	}

	if command == "" {
		printHelp()
		return 1
	}

	code, err := execute(command, rest, dbPath, dbSet, dryRun)
	if err != nil {
		var userError *domain.UserError
		if errors.As(err, &userError) {
			fmt.Fprintln(os.Stderr, "Ошибка: "+err.Error())
		} else {
			fmt.Fprintln(os.Stderr, "Непредвиденная ошибка: "+err.Error())
		}
		return 1
	}
	return code
}

func execute(command string, rest []string, dbPath string, dbSet bool, dryRun bool) (int, error) {
	switch command {
	case "help", "-h", "--help":
		return printHelp(), nil
	case "init":
		return cmdInit(dbPath, dbSet)
	case "parse":
		return cmdParse(rest)
	case "add":
		return cmdAdd(rest, dbPath, dbSet, dryRun)
	case "import":
		return cmdImport(rest, dbPath, dbSet, dryRun)
	case "list":
		return cmdList(rest, dbPath, dbSet)
	case "info":
		return cmdInfo(rest, dbPath, dbSet)
	case "find":
		return cmdFind(rest, dbPath, dbSet)
	case "delete":
		return cmdDelete(rest, dbPath, dbSet, dryRun)
	case "count":
		return cmdCount(dbPath, dbSet)
	}
	fmt.Fprintln(os.Stderr, "Неизвестная команда: "+command)
	printHelp()
	return 1, nil
}
