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

// progName — имя утилиты в справке и сообщениях об ошибках.
const progName = "catalogctl"

// Run выполняет команду catalogctl и возвращает код выхода
// (docs/plan/04-module-functionality.md §3). Разбор — по реестру
// commands: strictFlags одним проходом разбирает флаги (контрактная
// лексика и последовательная валидация значений; каждое вхождение на
// месте, повторное — перезаписывает) и собирает позиционные аргументы,
// реестр — диспетчеризация команды/подкоманды и границы числа
// аргументов. Общие опции: --dialect, --db, --dsn, --kind, --system,
// --dry-run, --lang en|ru (локаль вывода — D9, по умолчанию en); опции
// задаются после команды. Ожидаемые ошибки выводятся с префиксом
// «Ошибка: », прочие — «Непредвиденная ошибка: », код выхода 1.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText(i18n.En))
		return 1
	}
	if args[0] == "--help" || args[0] == "-h" {
		args = append([]string{"help"}, args[1:]...)
	}
	name := args[0]
	earlyLang := langFromArgs(args)
	if strings.HasPrefix(name, "-") {
		// Опции до команды («catalogctl --db X count») — не «неизвестная
		// команда», а явная подсказка позиции опций.
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliOptionsAfterCmd, name))
		return 1
	}
	// help — вне реестра commands (справка читает реестр: цикл
	// инициализации); границы числа аргументов проверяет сам исполнитель.
	if name == "help" {
		opts, pos, err := strictFlags(name, args[1:], helpFlags)
		if err != nil {
			PrintError(stderr, earlyLang, err)
			return 1
		}
		return runHelpCmd(context.Background(), opts, pos, stdout, stderr)
	}
	cmd, ok := commands[name]
	if !ok {
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, name))
		return 1
	}
	// Подкоманда обязана идти первой после имени команды (флаги до неё
	// не переносятся к подкоманде и не игнорируются молча).
	full, specs, tail := name, cmd.flags, args[1:]
	var target *command
	switch {
	case cmd.subcommands == nil:
		target = cmd
	case len(tail) > 0:
		sub, known := cmd.subcommands[tail[0]]
		if !known {
			PrintError(stderr, earlyLang, unknownSubcommand(name, tail[0]))
			return 1
		}
		full, specs, tail = name+" "+tail[0], sub.flags, tail[1:]
		target = sub
	}
	opts, pos, err := strictFlags(full, tail, specs)
	if err != nil {
		PrintError(stderr, earlyLang, err)
		return 1
	}
	if target == nil {
		// «catalog» без подкоманды.
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliCatalogSubMissing))
		return 1
	}
	if len(pos) < target.minArgs || (target.maxArgs >= 0 && len(pos) > target.maxArgs) {
		// Строка использования — аргумент-сообщение (use + полный перечень
		// флагов реестра): совпадает с карточкой help <команда>.
		PrintError(stderr, earlyLang, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliArgCount, full, domain.MsgArg(target.use, flagHints(target.flags))))
		return 1
	}
	return target.run(context.Background(), opts, pos, stdout, stderr)
}

// unknownSubcommand — ошибка первой лексемы после «catalog»: флаг до
// подкоманды — неизвестный флаг каталога, прочее — неизвестная
// подкоманда (подкоманда обязана идти первой: флаги до неё не
// переносятся к подкоманде и не игнорируются молча).
func unknownSubcommand(name, sub string) error {
	if sub != "-" && sub != "--" && strings.HasPrefix(sub, "-") {
		if flag := strings.TrimLeft(sub, "-"); flag != "" {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliUnknownFlag, flag, name, name)
		}
	}
	return domain.NewErrorf(domain.CodeValidationFailed,
		domain.MsgCliCatalogSubUnknown, sub)
}

// strictFlags — строгая лексика флагов контракта CLI, последовательная
// валидация их значений и сбор позиционных аргументов, одним проходом:
// форма флага — ровно два дефиса («-имя» и «---имя» — неизвестный флаг
// с этим именем), значение флага не может быть следующим флагом или
// отсутствовать, булев флаг не принимает форму «--имя=значение»,
// «--» — конец флагов (всё после него — позиционные аргументы), токен
// без префикса «-» (и сам «-») — позиционный аргумент; каждое вхождение
// значимого флага проверяется на месте (повторное вхождение перезаписывает
// значение — как в контракте).
func strictFlags(cmd string, args []string, specs []flagSpec) (*options, []string, error) {
	byName := make(map[string]flagSpec, len(specs))
	for _, s := range specs {
		byName[s.name] = s
	}
	o := newOptions()
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			return o, pos, nil
		}
		if !strings.HasPrefix(arg, "--") {
			if strings.HasPrefix(arg, "-") && arg != "-" {
				// одиночный дефис: «-lang» — не форма флага, неизвестный
				// флаг с этим именем (утроенный «---lang» отвергается ниже
				// сам собой: имя после двух дефисов сохраняет лишний).
				return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgCliUnknownFlag, strings.TrimLeft(arg, "-"), cmd, cmd)
			}
			pos = append(pos, arg) // позиционный аргумент
			continue
		}
		name, value := arg[2:], ""
		hasEq := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, value, hasEq = name[:eq], name[eq+1:], true
		}
		spec, known := byName[name]
		if !known {
			return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliUnknownFlag, name, cmd, cmd)
		}
		if !spec.hasValue {
			if hasEq {
				return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgCliFlagNoValue, name)
			}
			if err := o.set(cmd, name, "true"); err != nil {
				return nil, nil, err
			}
			continue
		}
		if !hasEq {
			// Значение обязательно и не может быть следующим флагом
			// («--kind --system» — не kind="--system").
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgCliFlagValueRequired, name)
			}
			i++ // значение флага — не позиция
			value = args[i]
		}
		if err := o.set(cmd, name, value); err != nil {
			return nil, nil, err
		}
	}
	return o, pos, nil
}

// newOptions — пустые опции с инициализированными картами фильтров.
func newOptions() *options {
	return &options{texts: map[string]string{}, nums: map[string]float64{}, seen: map[string]bool{}}
}

// command — команда CLI по реестру: строка использования (сообщение
// каталога: путь и позиционные аргументы, полный перечень флагов
// подставляется генератором справки), спецификации флагов, границы
// числа аргументов и исполнитель. Подкоманды (subcommands != nil) несут
// собственные наборы флагов — объединённый набор принимал бы флаги
// соседних подкоманд и молча игнорировал их.
type command struct {
	use         domain.MsgID
	flags       []flagSpec
	minArgs     int
	maxArgs     int // -1 — без потолка
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
	seen    map[string]bool // явно заданные флаги (семантические конфликты значений)
}

// set присваивает значение флага; числовые флаги и локаль разбираются
// на месте (значения флагов — строки, разбор — в терминах каталога
// сообщений).
func (o *options) set(cmd, name, value string) error {
	o.seen[name] = true
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
	langFlag   = flagSpec{name: "lang", hasValue: true}
)

// commands — реестр команд (полный набор — 04-module-functionality.md
// §3); из него строятся диспетчеризация, справка и строки использования
// ошибок числа аргументов.
var commands = map[string]*command{
	"init": {use: domain.MsgCliUseInit,
		flags: dbFlags, minArgs: 0, maxArgs: 0, run: runInit},
	"parse": {use: domain.MsgCliUseParse,
		flags: append([]flagSpec{kindFlag, systemFlag}, dbFlags...), minArgs: 1, maxArgs: -1, run: runParse},
	"add": {use: domain.MsgCliUseAdd,
		flags: append([]flagSpec{kindFlag, systemFlag}, dbFlags...), minArgs: 1, maxArgs: -1, run: runAdd},
	"import": {use: domain.MsgCliUseImport,
		flags: append([]flagSpec{dryRunFlag}, dbFlags...), minArgs: 1, maxArgs: 1, run: runImport},
	"list": {use: domain.MsgCliUseList,
		flags: append([]flagSpec{
			kindFlag, systemFlag,
			{name: "material", hasValue: true}, {name: "subclass", hasValue: true},
			{name: "adjustment", hasValue: true}, {name: "category", hasValue: true},
			{name: "junctions", hasValue: true}, {name: "group", hasValue: true},
			{name: "series", hasValue: true}, {name: "number", hasValue: true},
			{name: "letters", hasValue: true}, {name: "q", hasValue: true},
			{name: "limit", hasValue: true}, {name: "offset", hasValue: true},
		}, dbFlags...), minArgs: 0, maxArgs: 1, run: runList},
	"info": {use: domain.MsgCliUseInfo, flags: append([]flagSpec{kindFlag}, dbFlags...), minArgs: 1, maxArgs: 1, run: runInfo},
	"find": {use: domain.MsgCliUseFind, flags: append([]flagSpec{kindFlag}, dbFlags...), minArgs: 1, maxArgs: 1, run: runFind},
	"delete": {use: domain.MsgCliUseDelete,
		flags: append([]flagSpec{dryRunFlag, kindFlag}, dbFlags...), minArgs: 1, maxArgs: -1, run: runDelete},
	"count": {use: domain.MsgCliUseCount, flags: append([]flagSpec{kindFlag}, dbFlags...), minArgs: 0, maxArgs: 0, run: runCount},
	"export": {use: domain.MsgCliUseExport,
		flags: append([]flagSpec{kindFlag, {name: "format", hasValue: true}}, dbFlags...), minArgs: 0, maxArgs: 0, run: runExport},
	"version": {use: domain.MsgCliUseVersion, flags: []flagSpec{langFlag}, minArgs: 0, maxArgs: 0, run: runVersion},
	// Подкоманды каталога несут собственные наборы флагов (строгость:
	// флаг соседней подкоманды — ошибка, а не молчаливое игнорирование).
	"catalog": {subcommands: map[string]*command{
		"export": {use: domain.MsgCliUseCatalogExport,
			flags:   append([]flagSpec{{name: "format", hasValue: true}}, dbFlags...),
			minArgs: 0, maxArgs: 1, run: runCatalogExport},
		"import": {use: domain.MsgCliUseCatalogImport,
			flags:   append([]flagSpec{dryRunFlag}, dbFlags...),
			minArgs: 1, maxArgs: 1, run: runCatalogImportCmd},
		"list": {use: domain.MsgCliUseCatalogList,
			flags: dbFlags, minArgs: 0, maxArgs: 0, run: runCatalogListCmd},
	}},
}

// helpFlags — флаги команды help (вне реестра commands: справка читает
// реестр, запись в него замкнула бы цикл инициализации).
var helpFlags = []flagSpec{langFlag}

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
