package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Run выполняет команду catalogctl и возвращает код выхода
// (docs/plan/04-module-functionality.md §3). Общие опции: --dialect, --db,
// --dsn, --kind, --system, --dry-run; ожидаемые ошибки выводятся
// с префиксом «Ошибка: », прочие — «Непредвиденная ошибка: »,
// код выхода 1.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 1
	}
	name := args[0]
	if name == "help" || name == "--help" || name == "-h" {
		return runHelp(args[1:], stdout, stderr)
	}
	cmd, ok := commands[name]
	if !ok {
		PrintError(stderr, domain.NewError(domain.CodeValidationFailed,
			fmt.Sprintf("неизвестная команда «%s»; справка: catalogctl help", name)))
		return 1
	}
	opts, pos, err := parseArgs(name, cmd.flags, args[1:])
	if err != nil {
		PrintError(stderr, err)
		return 1
	}
	tooFew := len(pos) < cmd.minArgs
	tooMany := cmd.maxArgs >= 0 && len(pos) > cmd.maxArgs
	if tooFew || tooMany {
		PrintError(stderr, domain.NewError(domain.CodeValidationFailed,
			"неверное число аргументов команды "+name+"; формат: "+cmd.usage))
		return 1
	}
	ctx := context.Background()
	return cmd.run(ctx, opts, pos, stdout, stderr)
}

// command — команда CLI: разбор аргументов и исполнение.
type command struct {
	usage   string
	flags   []flagSpec
	minArgs int
	maxArgs int
	run     func(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int
}

// flagSpec — флаг команды: hasValue — флаг принимает значение
// (иначе булев флаг).
type flagSpec struct {
	name     string
	hasValue bool
}

// options — разобранные опции команд.
type options struct {
	dialect string
	db      string
	dsn     string
	kind    string
	system  string
	format  string
	dryRun  bool
	q       string
	limit   int
	offset  int
	texts   map[string]string // текстовые фильтры полей обозначения
	nums    map[string]float64
}

// Базовые флаги подключения и подбора.
var (
	dbFlags = []flagSpec{
		{name: "dialect", hasValue: true},
		{name: "db", hasValue: true},
		{name: "dsn", hasValue: true},
	}
	kindFlag   = flagSpec{name: "kind", hasValue: true}
	systemFlag = flagSpec{name: "system", hasValue: true}
	dryRunFlag = flagSpec{name: "dry-run"}
)

// commands — реестр команд (полный набор — 04-module-functionality.md §3).
var commands = map[string]*command{
	"init": {usage: "init [--dialect sqlite|postgres] [--db <путь>] [--dsn <строка>]",
		flags: dbFlags, minArgs: 0, maxArgs: 0, run: runInit},
	"parse": {usage: "parse <обозначение>… [--kind] [--system] [--db]",
		flags: append([]flagSpec{kindFlag, systemFlag}, dbFlags...), minArgs: 1, maxArgs: -1, run: runParse},
	"add": {usage: "add <обозначение>… [--kind] [--system] [--db]",
		flags: append([]flagSpec{kindFlag, systemFlag}, dbFlags...), minArgs: 1, maxArgs: -1, run: runAdd},
	"import": {usage: "import <файл> [--dry-run] [--db]",
		flags: append([]flagSpec{dryRunFlag}, dbFlags...), minArgs: 1, maxArgs: 1, run: runImport},
	"list": {usage: "list [--kind] [--system] [--material] [--subclass] [--junctions] [--group] [--series] [--number] [--letters] [--q] [--limit] [--offset] [--db]",
		flags: append([]flagSpec{
			kindFlag, systemFlag,
			{name: "material", hasValue: true}, {name: "subclass", hasValue: true},
			{name: "junctions", hasValue: true}, {name: "group", hasValue: true},
			{name: "series", hasValue: true}, {name: "number", hasValue: true},
			{name: "letters", hasValue: true}, {name: "q", hasValue: true},
			{name: "limit", hasValue: true}, {name: "offset", hasValue: true},
		}, dbFlags...), minArgs: 0, maxArgs: 0, run: runList},
	"info": {usage: "info <обозначение> [--kind] [--db]", flags: append([]flagSpec{kindFlag}, dbFlags...), minArgs: 1, maxArgs: 1, run: runInfo},
	"find": {usage: "find <обозначение> [--kind] [--db]", flags: append([]flagSpec{kindFlag}, dbFlags...), minArgs: 1, maxArgs: 1, run: runFind},
	"delete": {usage: "delete <обозначение>… [--dry-run] [--kind] [--db]",
		flags: append([]flagSpec{dryRunFlag, kindFlag}, dbFlags...), minArgs: 1, maxArgs: -1, run: runDelete},
	"count": {usage: "count [--kind] [--db]", flags: append([]flagSpec{kindFlag}, dbFlags...), minArgs: 0, maxArgs: 0, run: runCount},
	"export": {usage: "export [--kind] [--format jsonc|yaml|ndjson] [--db]",
		flags: append([]flagSpec{kindFlag, {name: "format", hasValue: true}}, dbFlags...), minArgs: 0, maxArgs: 0, run: runExport},
	"catalog": {usage: "catalog export [--format] [--db] | catalog import <файл> [--dry-run] [--db] | catalog list [--db]",
		flags:   append([]flagSpec{{name: "format", hasValue: true}, dryRunFlag}, dbFlags...),
		minArgs: 1, maxArgs: 2, run: runCatalog},
}

// parseArgs разбирает аргументы команды: флаги в любой позиции
// (использование вида «import <файл> --dry-run»), «--» — конец флагов.
func parseArgs(cmd string, specs []flagSpec, args []string) (*options, []string, error) {
	byName := make(map[string]flagSpec, len(specs))
	for _, s := range specs {
		byName[s.name] = s
	}
	opts := &options{texts: map[string]string{}, nums: map[string]float64{}}
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			pos = append(pos, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value := ""
		hasValue := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value, hasValue = name[eq+1:], true
			name = name[:eq]
		}
		spec, known := byName[name]
		if !known {
			return nil, nil, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("неизвестный флаг «--%s» (команда %s; справка: catalogctl help %s)", name, cmd, cmd))
		}
		if spec.hasValue {
			if !hasValue {
				if i+1 >= len(args) {
					return nil, nil, domain.NewError(domain.CodeValidationFailed,
						fmt.Sprintf("флаг «--%s» требует значение", name))
				}
				i++
				value = args[i]
			}
			if err := opts.set(cmd, name, value); err != nil {
				return nil, nil, err
			}
			continue
		}
		if hasValue {
			return nil, nil, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("флаг «--%s» не принимает значение", name))
		}
		if err := opts.set(cmd, name, "true"); err != nil {
			return nil, nil, err
		}
	}
	return opts, pos, nil
}

// set присваивает значение флага; числовые флаги разбираются на месте.
func (o *options) set(cmd, name, value string) error {
	switch name {
	case "dialect":
		o.dialect = value
	case "db":
		o.db = value
	case "dsn":
		o.dsn = value
	case "kind":
		o.kind = value
	case "system":
		o.system = value
	case "format":
		o.format = value
	case "dry-run":
		o.dryRun = value == "true"
	case "q":
		o.q = value
	case "limit", "offset":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || (name == "limit" && n == 0) {
			return domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("флаг «--%s» требует неотрицательное число, получено «%s»", name, value))
		}
		if name == "limit" {
			o.limit = n
		} else {
			o.offset = n
		}
	case "material", "subclass", "series", "letters":
		o.texts[name] = value
	case "junctions", "group", "number":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("флаг «--%s» требует число, получено «%s»", name, value))
		}
		o.nums[name] = f
	default:
		return domain.NewError(domain.CodeValidationFailed,
			fmt.Sprintf("неизвестный флаг «--%s» (команда %s)", name, cmd))
	}
	return nil
}

// dsnOf — строка подключения: --dsn приоритетнее --db (путь sqlite);
// по умолчанию catalog.db в текущем каталоге.
func (o *options) dsnOf() (dialect, dsn string) {
	dialect = o.dialect
	if dialect == "" {
		dialect = "sqlite"
	}
	dsn = o.dsn
	if dsn == "" {
		dsn = o.db
	}
	if dsn == "" && dialect == "sqlite" {
		dsn = "catalog.db"
	}
	return dialect, dsn
}

// hasDB — явно задана база (флаги --db/--dsn/--dialect).
func (o *options) hasDB() bool { return o.db != "" || o.dsn != "" || o.dialect != "" }

// openApp открывает приложение: команды записи выполняют EnsureCreated
// (создание схемы и сиды на пустую базу), команды чтения требуют
// инициализированную базу.
func openApp(ctx context.Context, o *options, ensure bool) (*service.App, error) {
	dialect, dsn := o.dsnOf()
	if dialect != "sqlite" && dialect != "postgres" {
		return nil, domain.NewError(domain.CodeValidationFailed,
			fmt.Sprintf("неизвестный диалект «%s» (допустимы: sqlite, postgres)", dialect))
	}
	return service.Open(ctx, service.Config{
		Dialect: dialect, DSN: dsn, EnsureCreated: ensure,
	})
}

// kindFlagOf — значение --kind (пустое — не задано).
func (o *options) kindFlagOf() domain.Kind { return domain.Kind(o.kind) }

// fail — печать ошибки с контрактным префиксом; возвращает код 1.
func fail(stderr io.Writer, err error) int {
	PrintError(stderr, err)
	return 1
}
