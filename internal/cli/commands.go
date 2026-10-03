package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/importer"
	"github.com/billydos/components-catalog/internal/service"
)

// Команды CLI — тонкие транспорты над сервисным слоем (справка из
// каталога — catalog list). Вывод детерминирован для закрепления
// тестами.

func runInit(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, true)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck — закрытие при выходе
	_, dsn := opts.dsnOf()
	fmt.Fprintf(stdout, "%s\n", i18n.Message(opts.langOf(), "cli_db_initialized", dsn, appDialect(opts)))
	return 0
}

func appDialect(opts *options) string {
	dialect, _ := opts.dsnOf()
	return dialect
}

// runParse — разбор обозначений с автодетектом класса и системы;
// без --db/--dsn работает над стартовым реестром домена, с базой —
// над расширенным реестром семейств каталога.
func runParse(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	exit := 0
	if opts.hasDB() {
		app, err := openApp(ctx, opts, false)
		if err != nil {
			return fail(stderr, opts.langOf(), err)
		}
		defer app.Close() //nolint:errcheck
		for _, arg := range pos {
			p, err := app.Services().Designations.ParseForSystem(ctx, arg, domain.System(opts.system), opts.kindFlagOf())
			if err != nil {
				PrintError(stderr, opts.langOf(), err)
				exit = 1
				continue
			}
			printParsed(stdout, opts.langOf(), p)
		}
		return exit
	}
	for _, arg := range pos {
		p, err := domain.ParseDesignationForSystem(arg, domain.System(opts.system), opts.kindFlagOf())
		if err != nil {
			PrintError(stderr, opts.langOf(), err)
			exit = 1
			continue
		}
		printParsed(stdout, opts.langOf(), p)
	}
	return exit
}

func runAdd(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, true)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	exit := 0
	for _, arg := range pos {
		in := service.DeviceInput{Name: arg, System: domain.System(opts.system), Kind: opts.kindFlagOf()}
		outcome, err := app.Services().Devices.Upsert(ctx, in)
		if err != nil {
			PrintError(stderr, opts.langOf(), fmt.Errorf("%s: %w", arg, err))
			exit = 1
			continue
		}
		fmt.Fprintf(stdout, "%s: %s\n", arg, outcomeText(opts.langOf(), outcome))
	}
	return exit
}

func outcomeText(lang i18n.Language, o service.Outcome) string {
	switch o {
	case service.OutcomeAdded:
		return i18n.Message(lang, "cli_outcome_added")
	case service.OutcomeUpdatedExisting:
		return i18n.Message(lang, "cli_outcome_updated")
	default:
		return i18n.Message(lang, "cli_outcome_skipped")
	}
}

// runImport — импорт файла наполнения (формат — по расширению);
// «-» — стандартный ввод.
func runImport(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, true)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	f, closeFile, err := openInput(pos[0])
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer closeFile() //nolint:errcheck
	rep, err := importer.New(app).Import(ctx, f, pos[0], opts.dryRun)
	if err != nil {
		return fail(stderr, opts.langOf(), fmt.Errorf("%s: %w", pos[0], err))
	}
	return printReport(stdout, stderr, opts.langOf(), &rep)
}

// runCatalog — catalog export|import|list.
func runCatalog(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	if len(pos) == 0 {
		PrintError(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliCatalogSubMissing))
		return 1
	}
	sub, rest := pos[0], pos[1:]
	switch sub {
	case "export":
		if len(rest) > 1 {
			PrintError(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliCatalogExportArgs))
			return 1
		}
		return runCatalogExport(ctx, opts, stdout, stderr)
	case "import":
		if len(rest) != 1 {
			PrintError(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliCatalogImportArgs))
			return 1
		}
		return runCatalogImport(ctx, opts, rest[0], stdout, stderr)
	case "list":
		if len(rest) > 0 {
			PrintError(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgCliCatalogListArgs))
			return 1
		}
		return runCatalogList(ctx, opts, stdout, stderr)
	}
	PrintError(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
		domain.MsgCliCatalogSubUnknown, sub))
	return 1
}

func runCatalogExport(ctx context.Context, opts *options, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	format, err := exportFormat(opts)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	if err := importer.New(app).ExportCatalog(ctx, stdout, format); err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	return 0
}

func runCatalogImport(ctx context.Context, opts *options, name string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, true)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	f, closeFile, err := openInput(name)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer closeFile() //nolint:errcheck
	rep, err := importer.New(app).ImportCatalogFile(ctx, f, name, opts.dryRun)
	if err != nil {
		return fail(stderr, opts.langOf(), fmt.Errorf("%s: %w", name, err))
	}
	return printReport(stdout, stderr, opts.langOf(), &rep)
}

// runCatalogList — справка из каталога: классы, системы, группы/секции,
// параметры и атрибуты (для составления файлов наполнения).
func runCatalogList(ctx context.Context, opts *options, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	snap, err := app.Snapshot(ctx)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	lang := opts.langOf()
	fmt.Fprintln(stdout, i18n.Message(lang, "cli_h_kinds"))
	for _, k := range snap.Kinds {
		fmt.Fprintf(stdout, "  %s — %s\n", k.Code, i18n.KindName(lang, string(k.Code)))
	}
	fmt.Fprintln(stdout, i18n.Message(lang, "cli_h_systems"))
	for _, s := range snap.Systems {
		fmt.Fprintf(stdout, "  %s — %s\n", s.Code, i18n.SystemName(lang, string(s.Code)))
	}
	fmt.Fprintln(stdout, i18n.Message(lang, "cli_h_groups"))
	for _, g := range snap.Groups {
		fmt.Fprintln(stdout, i18n.Message(lang, "cli_group_line", g.SectionName, i18n.GroupName(lang, g.Code), g.Code))
	}
	fmt.Fprintln(stdout, i18n.Message(lang, "cli_h_params"))
	for _, p := range snap.Parameters {
		unit := p.Unit
		if unit == "" {
			unit = "—"
		}
		fmt.Fprintln(stdout, i18n.Message(lang, "cli_param_line",
			p.Code, i18n.ParameterName(lang, p.Code), sectionOf(snap, p.Group), string(p.ValueType), unit))
	}
	fmt.Fprintln(stdout, i18n.Message(lang, "cli_h_attrs"))
	for _, a := range snap.Attributes {
		fmt.Fprintln(stdout, i18n.Message(lang, "cli_attr_list_line",
			a.Code, i18n.AttributeName(lang, a.Code), string(a.Type)))
	}
	return 0
}

func sectionOf(snap *catalog.Snapshot, group string) string {
	if g, ok := snap.Group(group); ok {
		return g.SectionName
	}
	return group
}

func runList(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck

	query := service.SearchQuery{
		Kind:   opts.kindFlagOf(),
		System: domain.System(opts.system),
		Query:  opts.q,
		Limit:  opts.limit,
		Offset: opts.offset,
	}
	for _, name := range []string{"material", "subclass", "series", "letters"} {
		if v, ok := opts.texts[name]; ok {
			if name == "material" {
				// Фильтр материала: вход канонизируется к коду словаря
				// (код либо отображаемое название локали — D9).
				code, okCode := i18n.MaterialCode(v)
				if !okCode {
					return fail(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
						domain.MsgUnknownMaterial, v))
				}
				v = code
			}
			query.Fields = append(query.Fields, service.FieldFilter{Field: name, Text: v})
		}
	}
	for _, name := range []string{"junctions", "group", "number"} {
		if v, ok := opts.nums[name]; ok {
			field := name
			if field == "number" {
				field = "dev_number"
			}
			query.Fields = append(query.Fields, service.FieldFilter{Field: field, Num: v, HasNum: true, Op: service.OpEq})
		}
	}
	page, err := app.Services().Devices.Search(ctx, query)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	tw := tabwriter.NewWriter(stdout, 2, 8, 2, ' ', 0)
	for _, item := range page.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Designation, string(item.Kind), string(item.System))
	}
	if err := tw.Flush(); err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	return 0
}

func runInfo(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	card, found, err := app.Services().Devices.Get(ctx, opts.kindFlagOf(), pos[0])
	if err != nil {
		return fail(stderr, opts.langOf(), fmt.Errorf("%s: %w", pos[0], err))
	}
	if !found {
		return fail(stderr, opts.langOf(), notFound(pos[0]))
	}
	snap, err := app.Snapshot(ctx)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	printCard(stdout, opts.langOf(), card, snap)
	return 0
}

func notFound(designation string) error {
	return domain.NewErrorf(domain.CodeNotFound, domain.MsgCliRecordNotFound, designation)
}

func runFind(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	res, err := app.Services().Devices.Find(ctx, opts.kindFlagOf(), pos[0])
	if err != nil {
		return fail(stderr, opts.langOf(), fmt.Errorf("%s: %w", pos[0], err))
	}
	if res.Found != nil {
		snap, err := app.Snapshot(ctx)
		if err != nil {
			return fail(stderr, opts.langOf(), err)
		}
		printCard(stdout, opts.langOf(), res.Found, snap)
		return 0
	}
	if res.Suggestion != nil {
		snap, err := app.Snapshot(ctx)
		if err != nil {
			return fail(stderr, opts.langOf(), err)
		}
		fmt.Fprintf(stdout, "%s\n", i18n.Message(opts.langOf(), "cli_find_suggestion",
			pos[0], res.Suggestion.Designation))
		printCard(stdout, opts.langOf(), res.Suggestion, snap)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", i18n.Message(opts.langOf(), "cli_find_not_found", pos[0]))
	return 1
}

func runDelete(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	exit := 0
	for _, arg := range pos {
		if opts.dryRun {
			// Контрольный прогон: проверяется существование записи,
			// удаления не происходит.
			_, found, err := app.Services().Devices.Get(ctx, opts.kindFlagOf(), arg)
			if err != nil {
				PrintError(stderr, opts.langOf(), fmt.Errorf("%s: %w", arg, err))
				exit = 1
				continue
			}
			if !found {
				PrintError(stderr, opts.langOf(), notFound(arg))
				exit = 1
				continue
			}
			fmt.Fprintf(stdout, "%s\n", i18n.Message(opts.langOf(), "cli_delete_dry_run", arg))
			continue
		}
		deleted, err := app.Services().Devices.Delete(ctx, opts.kindFlagOf(), arg)
		if err != nil {
			PrintError(stderr, opts.langOf(), fmt.Errorf("%s: %w", arg, err))
			exit = 1
			continue
		}
		if !deleted {
			PrintError(stderr, opts.langOf(), notFound(arg))
			exit = 1
			continue
		}
		fmt.Fprintf(stdout, "%s\n", i18n.Message(opts.langOf(), "cli_deleted", arg))
	}
	return exit
}

func runCount(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	var kind *domain.Kind
	if opts.kind != "" {
		k := opts.kindFlagOf()
		snap, err := app.Snapshot(ctx)
		if err != nil {
			return fail(stderr, opts.langOf(), err)
		}
		if _, ok := snap.Kind(k); !ok {
			return fail(stderr, opts.langOf(), domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgKindUnknown, string(k)))
		}
		kind = &k
	}
	n, err := app.Services().Devices.Count(ctx, kind)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	fmt.Fprintln(stdout, strconv.Itoa(n))
	return 0
}

func runExport(ctx context.Context, opts *options, pos []string, stdout, stderr io.Writer) int {
	app, err := openApp(ctx, opts, false)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	defer app.Close() //nolint:errcheck
	format, err := exportFormat(opts)
	if err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	var kind *domain.Kind
	if opts.kind != "" {
		k := opts.kindFlagOf()
		kind = &k
	}
	if err := importer.New(app).Export(ctx, stdout, format, kind); err != nil {
		return fail(stderr, opts.langOf(), err)
	}
	return 0
}

// exportFormat — формат экспорта (--format, по умолчанию jsonc).
func exportFormat(opts *options) (importer.Format, error) {
	if opts.format == "" {
		return importer.FormatJSONC, nil
	}
	return importer.ParseFormat(opts.format)
}

// printReport печатает итог импорта и проблемы; код 1 при проблемах.
// Итог и проблемы локализуются каталогом сообщений (import_summary*,
// import_issue*), позиция проблемы — из структурных полей Issue.
func printReport(stdout, stderr io.Writer, lang i18n.Language, rep *importer.Report) int {
	head := ""
	if rep.DryRun {
		head = i18n.Message(lang, "cli_dry_run_head")
	}
	summary := i18n.Message(lang, "import_summary", rep.Records, rep.Added, rep.Updated, rep.Skipped)
	if rep.Rejected > 0 {
		summary += i18n.Message(lang, "import_summary_rejected", rep.Rejected)
	}
	if rep.CatalogApplied {
		summary += i18n.Message(lang, "import_summary_catalog")
	}
	fmt.Fprintf(stdout, "%s%s: %s\n", head, rep.Name, summary)
	for _, issue := range rep.Issues {
		fmt.Fprintf(stderr, "%s\n", i18n.Message(lang, "cli_issue_line", issueString(lang, issue)))
	}
	if rep.HasIssues() {
		return 1
	}
	return 0
}

// issueString — строка проблемы по локали: позиция (запись/строка/номер)
// и локализованное сообщение.
func issueString(lang i18n.Language, iss importer.Issue) string {
	msg := i18n.Message(lang, string(iss.MsgID), iss.Args...)
	switch {
	case iss.Record != "" && iss.Line > 0:
		return i18n.Message(lang, "import_issue_record_line", iss.Record, iss.Line, msg)
	case iss.Record != "":
		return i18n.Message(lang, "import_issue_record", iss.Record, msg)
	case iss.Line > 0:
		return i18n.Message(lang, "import_issue_line", iss.Line, msg)
	case iss.No > 0:
		return i18n.Message(lang, "import_issue_no", iss.No, msg)
	}
	return msg
}

// openInput открывает файл импорта («-» — стандартный ввод); вторым
// значением — функция закрытия (no-op для stdin).
func openInput(name string) (io.Reader, func(), error) {
	if name == "-" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, domain.NewErrorf(domain.CodeInvalidImportFile,
			domain.MsgCliFileOpen, name, err)
	}
	return f, func() { f.Close() }, nil //nolint:errcheck — закрытие при выходе
}
