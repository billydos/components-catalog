package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/service"
)

// Run выполняет команду catalogctl и возвращает код выхода
// (docs/plan/04-module-functionality.md §3). Общие опции: --dialect, --db,
// --dsn, --kind, --system, --dry-run, --lang en|ru (локаль вывода — D9,
// по умолчанию en); опции задаются после команды. Ожидаемые ошибки
// выводятся с префиксом «Ошибка: », прочие — «Непредвиденная ошибка: »,
// код выхода 1.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText(i18n.En))
		return 1
	}
	name := args[0]
	if name == "help" || name == "--help" || name == "-h" {
		return runHelp(args[1:], stdout, stderr)
	}
	earlyLang := langFromArgs(args)
	if strings.HasPrefix(name, "-") {
		// Опции до команды («catalogctl --db X count») — не «неизвестная
		// команда», а явная подсказка позиции опций.
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliOptionsAfterCmd, name))
		return 1
	}
	cmd, ok := commands[name]
	if !ok {
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, name))
		return 1
	}
	if cmd.subcommands != nil {
		return runSubcommands(name, cmd, args[1:], stdout, stderr)
	}
	opts, pos, err := parseArgs(name, cmd.flags, args[1:])
	if err != nil {
		PrintError(stderr, earlyLang, err)
		return 1
	}
	tooFew := len(pos) < cmd.minArgs
	tooMany := cmd.maxArgs >= 0 && len(pos) > cmd.maxArgs
	if tooFew || tooMany {
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliArgCount, name, cmd.usage))
		return 1
	}
	ctx := context.Background()
	return cmd.run(ctx, opts, pos, stdout, stderr)
}

// command — команда CLI: разбор аргументов и исполнение. Подкоманды
// (subcommands != nil) разбираются по собственным наборам флагов —
// объединённый набор принимал бы флаги соседних подкоманд и молча
// игнорировал их.
type command struct {
	usage       string
	flags       []flagSpec
	minArgs     int
	maxArgs     int
	subcommands map[string]*command
	run         func(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int
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
	lang    string
	dryRun  bool
	q       string
	limit   int
	offset  int
	texts   map[string]string // текстовые фильтры полей обозначения
	nums    map[string]float64
}

// langOf — локаль вывода отображаемых строк (--lang; по умолчанию en — D9).
func (o *options) langOf() i18n.Language {
	if l, ok := i18n.ParseLanguage(o.lang); ok {
		return l
	}
	return i18n.En
}

// Базовые флаги подключения и подбора.
var (
	dbFlags = []flagSpec{
		{name: "dialect", hasValue: true},
		{name: "db", hasValue: true},
		{name: "dsn", hasValue: true},
		{name: "lang", hasValue: true},
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
	"list": {usage: "list [--kind] [--system] [--material] [--subclass] [--adjustment] [--category] [--junctions] [--group] [--series] [--number] [--letters] [--q] [--limit] [--offset] [--db]",
		flags: append([]flagSpec{
			kindFlag, systemFlag,
			{name: "material", hasValue: true}, {name: "subclass", hasValue: true},
			{name: "adjustment", hasValue: true}, {name: "category", hasValue: true},
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
	// Подкоманды каталога несут собственные наборы флагов (строгость:
	// флаг соседней подкоманды — ошибка, а не молчаливое игнорирование).
	"catalog": {usage: "catalog export [<файл>] [--format jsonc|yaml|ndjson] [--db] | catalog import <файл> [--dry-run] [--db] | catalog list [--db]",
		subcommands: map[string]*command{
			"export": {usage: "catalog export [<файл>] [--format jsonc|yaml|ndjson] [--db]",
				flags:   append([]flagSpec{{name: "format", hasValue: true}}, dbFlags...),
				minArgs: 0, maxArgs: 1, run: runCatalog},
			"import": {usage: "catalog import <файл> [--dry-run] [--db]",
				flags:   append([]flagSpec{dryRunFlag}, dbFlags...),
				minArgs: 1, maxArgs: 1, run: runCatalog},
			"list": {usage: "catalog list [--db]",
				flags: dbFlags, minArgs: 0, maxArgs: 0, run: runCatalog},
		}},
}

// runSubcommands — разбор команды с подкомандами: первая позиционная
// лексема выбирает подкоманду (флаги могут стоять и до неё — пропускаются
// по объединению наборов), остаток разбирается набором подкоманды.
func runSubcommands(name string, cmd *command, args []string, stdout, stderr io.Writer) int {
	earlyLang := langFromArgs(args)
	union := make([]flagSpec, 0, 8)
	seen := map[string]bool{}
	addAll := func(specs []flagSpec) {
		for _, s := range specs {
			if !seen[s.name] {
				seen[s.name] = true
				union = append(union, s)
			}
		}
	}
	for _, sub := range cmd.subcommands {
		addAll(sub.flags)
	}
	subName, rest, ok := firstPositional(union, args)
	if !ok {
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliArgCount, name, cmd.usage))
		return 1
	}
	sub, known := cmd.subcommands[subName]
	if !known {
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, name+" "+subName))
		return 1
	}
	full := name + " " + subName
	opts, pos, err := parseArgs(full, sub.flags, rest)
	if err != nil {
		PrintError(stderr, earlyLang, err)
		return 1
	}
	tooFew := len(pos) < sub.minArgs
	tooMany := sub.maxArgs >= 0 && len(pos) > sub.maxArgs
	if tooFew || tooMany {
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliArgCount, full, sub.usage))
		return 1
	}
	pos = append([]string{subName}, pos...)
	return sub.run(context.Background(), opts, pos, stdout, stderr)
}

// firstPositional ищет первую позиционную лексему (подкоманду), пропуская
// флаги: значение флага, принимающего значение, — не позиция; после «--»
// всё позиционно.
func firstPositional(specs []flagSpec, args []string) (string, []string, bool) {
	byName := make(map[string]bool, len(specs))
	hasValue := make(map[string]bool, len(specs))
	for _, s := range specs {
		byName[s.name] = true
		hasValue[s.name] = s.hasValue
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+2 < len(args) {
				return args[i+1], args[i+2:], true
			}
			return "", nil, false
		}
		if !strings.HasPrefix(arg, "--") {
			return arg, args[i+1:], true
		}
		name := strings.TrimPrefix(arg, "--")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if hasValue[name] && !strings.Contains(arg, "=") {
			i++ // значение флага — не позиция
		}
	}
	return "", nil, false
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
		if !strings.HasPrefix(arg, "--") {
			if strings.HasPrefix(arg, "-") && arg != "-" {
				// одиночный либо утроенный дефис: «-lang», «---lang» —
				// не форма флага, неизвестный флаг с этим именем.
				return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgCliUnknownFlag, strings.TrimLeft(arg, "-"), cmd, cmd)
			}
			pos = append(pos, arg)
			continue
		}
		name := arg[2:]
		value := ""
		hasValue := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value, hasValue = name[eq+1:], true
			name = name[:eq]
		}
		spec, known := byName[name]
		if !known {
			return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliUnknownFlag, name, cmd, cmd)
		}
		if spec.hasValue {
			if !hasValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					// Значение обязательно и не может быть следующим
					// флагом («--kind --system» — не kind="--system").
					return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
						domain.MsgCliFlagValueRequired, name)
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
			return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliFlagNoValue, name)
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
	case "lang":
		if _, ok := i18n.ParseLanguage(value); !ok {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliLangInvalid, value)
		}
		o.lang = value
	case "dry-run":
		o.dryRun = value == "true"
	case "q":
		o.q = value
	case "limit", "offset":
		n, err := strconv.Atoi(value)
		if err != nil {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliFlagNonNegative, name, value)
		}
		if name == "limit" {
			// Диапазон limit симметричен REST (api_limit_range): целое
			// от 1 до потолка поиска; 0 — ошибка, а не «по умолчанию».
			if n < 1 || n > service.SearchLimitMax {
				return domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgApiLimitRange, service.SearchLimitMax)
			}
			o.limit = n
		} else {
			if n < 0 {
				return domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgCliFlagNonNegative, name, value)
			}
			o.offset = n
		}
	case "material", "subclass", "adjustment", "category", "series", "letters":
		o.texts[name] = value
	case "junctions", "group", "number":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliFlagNumber, name, value)
		}
		o.nums[name] = f
	default:
		return domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownFlagSet, name, cmd)
	}
	return nil
}

// dsnOf — строка подключения: --dsn приоритетнее --db (путь sqlite);
// по умолчанию catalog.db в текущем каталоге. --dsn postgres://… без
// --dialect выводит диалект из схемы DSN, а противоречие с явным
// --dialect sqlite — ошибка с подсказкой: молчаливая трактовка строки
// postgres как пути sqlite-файла даёт непрозрачную ошибку драйвера.
func (o *options) dsnOf() (string, string, error) {
	dialect := o.dialect
	if dialect == "" {
		dialect = "sqlite"
	}
	dsn := o.dsn
	if dsn == "" {
		dsn = o.db
	}
	if dsn == "" && dialect == "sqlite" {
		dsn = "catalog.db"
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		if o.dialect == "" {
			dialect = "postgres"
		} else if dialect != "postgres" {
			return "", "", domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliDsnDialect, dialect)
		}
	}
	return dialect, dsn, nil
}

// hasDB — явно задана база (флаги --db/--dsn/--dialect).
func (o *options) hasDB() bool { return o.db != "" || o.dsn != "" || o.dialect != "" }

// openApp открывает приложение: команды записи выполняют EnsureCreated
// (создание схемы и сиды на пустую базу), команды чтения требуют
// инициализированную базу.
func openApp(ctx context.Context, o *options, ensure bool) (*service.App, error) {
	dialect, dsn, err := o.dsnOf()
	if err != nil {
		return nil, err
	}
	if dialect != "sqlite" && dialect != "postgres" {
		return nil, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliDialectUnknown, dialect)
	}
	return service.Open(ctx, service.Config{
		Dialect: dialect, DSN: dsn, EnsureCreated: ensure,
	})
}

// kindFlagOf — значение --kind (пустое — не задано).
func (o *options) kindFlagOf() domain.Kind { return domain.Kind(o.kind) }

// langFromArgs — локаль ранних ошибок разбора аргументов: значение флага
// --lang, если задано до ошибки (по умолчанию en — D9).
func langFromArgs(args []string) i18n.Language {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--lang" {
			if l, ok := i18n.ParseLanguage(args[i+1]); ok {
				return l
			}
		}
		if strings.HasPrefix(args[i], "--lang=") {
			if l, ok := i18n.ParseLanguage(strings.TrimPrefix(args[i], "--lang=")); ok {
				return l
			}
		}
	}
	return i18n.En
}

// fail — печать ошибки с контрактным префиксом; возвращает код 1.
func fail(stderr io.Writer, lang i18n.Language, err error) int {
	PrintError(stderr, lang, err)
	return 1
}
