// Команда runner — кроссплатформенный прогон QA справочника (Linux, macOS,
// Windows; регламент — qa/README.md). Фазы: определение окружения и железа,
// сборка catalogctl/restsrv, статические проверки (go build/vet, gofmt,
// go test), сквозные сценарии CLI и REST на SQLite (всегда) и PostgreSQL
// (при CATALOG_TEST_POSTGRES_DSN либо -postgres-dsn), опциональная
// нагрузочная прикидка (-scale). Итог — структурный отчёт qa-log/1
// (qa/reports/*.log); код выхода 0 — все проверки зелёные.
//
// Прогон целиком на Go: внешние команды — только go и собранные бинарники
// проекта; никаких sh/bat/cmd-скриптов.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func main() {
	var (
		reportPath  = flag.String("report", "", "путь файла отчёта (по умолчанию qa/reports/qa-<дата>-<goos>-<goarch>.log)")
		postgresDSN = flag.String("postgres-dsn", "",
			"DSN PostgreSQL для ноги postgres (по умолчанию CATALOG_TEST_POSTGRES_DSN; пусто — нога опущена)")
		resetPostgres = flag.Bool("reset-postgres", false,
			"очистить таблицы модуля в базе -postgres-dsn перед ногой (флаг явно подтверждает одноразовость базы; env-конвенция CATALOG_TEST_POSTGRES_DSN чистится всегда)")
		dialect = flag.String("dialect", envDialect(),
			"прогнать только одну ногу: sqlite | postgres (по умолчанию CATALOG_QA_DIALECT; пусто — все доступные)")
		scale = flag.Int("scale", envScale(), "масштаб нагрузочной прикидки, записей (0 — фаза опущена)")
		keep  = flag.Bool("keep", false, "не удалять временный каталог прогона (диагностика)")
	)
	flag.Parse()
	if *scale < 0 {
		fmt.Fprintf(os.Stderr, "runner: -scale: масштаб не может быть отрицательным (%d); 0 — пропустить фазу load\n", *scale)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch *dialect {
	case "", "sqlite", "postgres":
	default:
		fmt.Fprintf(os.Stderr, "runner: неизвестный диалект %q (sqlite | postgres)\n", *dialect)
		os.Exit(2)
	}

	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "runner: %v\n", err)
		os.Exit(2)
	}

	opts := runOptions{dialect: *dialect, scale: *scale, keep: *keep}
	// DSN из env — конвенция одноразовой базы (как в internal/testutil):
	// сброс автоматический. DSN из флага — явное указание произвольной
	// базы: сброс только с отдельного подтверждения -reset-postgres,
	// без него нога идёт как есть (грязная база падает на проверках
	// видимым образом — безопасный отказ вместо разрушения данных).
	opts.postgresDSN = *postgresDSN
	opts.resetPostgres = *resetPostgres
	if opts.postgresDSN == "" {
		opts.postgresDSN = os.Getenv("CATALOG_TEST_POSTGRES_DSN")
		opts.resetPostgres = true
	}
	if opts.dialect == "postgres" && opts.postgresDSN == "" {
		fmt.Fprintf(os.Stderr, "runner: диалект postgres требует -postgres-dsn / CATALOG_TEST_POSTGRES_DSN\n")
		os.Exit(2)
	}

	path := *reportPath
	if path == "" {
		// Наносекунды в имени исключают коллизию повторных прогонов в
		// пределах секунды (os.Create молча обрезал бы чужой отчёт).
		now := time.Now()
		path = filepath.Join(root, "qa", "reports", fmt.Sprintf("qa-%s-%09d-%s-%s.log",
			now.Format("20060102-150405"), now.Nanosecond(), runtime.GOOS, runtime.GOARCH))
	}
	r, err := newReport(path, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runner: отчёт %s: %v\n", path, err)
		os.Exit(2)
	}

	// Первый сигнал отменяет прогон: прерванные и оставшиеся шаги
	// помечаются заметками «отменён», итог — код выхода 130. stop()
	// возвращает SIGINT/SIGTERM умолчание — повторный Ctrl+C завершает
	// процесс немедленно, не дожидаясь остановки команд (WaitDelay).
	go func() {
		<-ctx.Done()
		stop()
	}()
	r.onCancel(func() bool { return ctx.Err() != nil })

	run(ctx, r, root, opts)
	verdict := r.finish()
	fmt.Printf("отчёт: %s (%s)\n", path, verdict)
	if r.isCancelled() {
		os.Exit(130)
	}
	if verdict != "PASS" {
		os.Exit(1)
	}
}

// runOptions — параметры прогона.
type runOptions struct {
	dialect       string // "" | sqlite | postgres — фильтр ноги сценариев и нагрузки
	postgresDSN   string
	resetPostgres bool
	scale         int
	keep          bool
}

// run выполняет фазы прогона; отказ фазы не прерывает последующие.
// Сборка бинарников нужна только сценариям cli/rest: статические
// проверки и нагрузочная прикидка выполняются независимо (свои go-вызовы),
// при провале сборки сценарии помечаются пропуском с причиной, а не
// молча пропускаются. Отмена сигналом завершает прогон: оставшиеся фазы
// и шаги помечаются пропуском, а не FAIL.
func run(ctx context.Context, r *report, root string, opts runOptions) {
	writeEnvSection(r, root)
	if opts.scale == 0 {
		// envScale молча возвращает 0 на нераспознанном значении — фаза load
		// опускается; причина фиксируется заметкой (прямой `go test` в том же
		// окружении падает с явной ошибкой разбора).
		if v := os.Getenv("CATALOG_QA_SCALE"); v != "" && v != "0" {
			r.note("CATALOG_QA_SCALE=" + v + " не распознана — фаза load опущена (масштаб 0)")
		}
	}
	if opts.postgresDSN != "" {
		r.kv("postgres_server", pgVersion(ctx, opts.postgresDSN))
	} else {
		r.note("нога PostgreSQL опущена: CATALOG_TEST_POSTGRES_DSN / -postgres-dsn не заданы")
	}
	r.kv("sqlite_server", sqliteVersion())
	if opts.dialect != "" {
		r.kv("dialect", opts.dialect)
	}

	tmp, err := os.MkdirTemp("", "catalog-qa-")
	if err != nil {
		r.section("phase: prepare")
		r.check("prepare/tmp", "временный каталог прогона", false, err.Error())
		return
	}
	if opts.keep {
		r.note("временный каталог прогона: " + tmp)
	} else {
		defer os.RemoveAll(tmp) //nolint:errcheck — одноразовый каталог прогона
	}

	if cancelled(ctx, r) {
		return
	}
	ctl, srv, buildOK := buildBinaries(ctx, r, root, tmp)
	staticPhase(ctx, r, root, opts)
	if cancelled(ctx, r) {
		return
	}

	if buildOK {
		runLegs(ctx, r, ctl, srv, root, tmp, opts)
	} else {
		r.note("сценарии cli/rest пропущены: сборка catalogctl/restsrv не прошла (фаза build)")
	}

	if opts.scale > 0 && !cancelled(ctx, r) {
		loadPhase(ctx, r, root, opts)
	}
}

// runLegs — сценарные фазы cli/rest по ногам прогона (включая ограждённую
// подготовку ноги postgres).
func runLegs(ctx context.Context, r *report, ctl, srv, root, tmp string, opts runOptions) {
	legs := []leg{{name: "sqlite", dbArgs: []string{"--db", filepath.Join(tmp, "qa.db")}}}
	// Нога postgres — только когда не исключена -dialect sqlite: сброс
	// базы деструктивен, исключённая нога не должна его выполнять.
	if opts.postgresDSN != "" && opts.dialect != "sqlite" {
		r.section("phase: prepare")
		if opts.resetPostgres {
			if resetPostgres(ctx, r, opts.postgresDSN) {
				legs = append(legs, leg{
					name: "postgres",
					dbArgs: []string{
						"--dialect", "postgres", "--dsn", opts.postgresDSN,
					},
				})
			}
		} else {
			r.note("очистка базы -postgres-dsn опущена: только с явным -reset-postgres; " +
				"нога идёт по текущему состоянию базы (одноразовая база — env CATALOG_TEST_POSTGRES_DSN)")
			legs = append(legs, leg{
				name: "postgres",
				dbArgs: []string{
					"--dialect", "postgres", "--dsn", opts.postgresDSN,
				},
			})
		}
	} else if opts.postgresDSN != "" {
		r.note("нога PostgreSQL исключена: -dialect sqlite (база не сбрасывается)")
	}
	// -dialect — единственная нога прогона (остальные исключаются).
	if opts.dialect != "" {
		filtered := make([]leg, 0, 1)
		for _, l := range legs {
			if l.name == opts.dialect {
				filtered = append(filtered, l)
			}
		}
		legs = filtered
	}
	for _, l := range legs {
		if cancelled(ctx, r) {
			return
		}
		cliSuite(ctx, r, ctl, root, tmp, l)
		if cancelled(ctx, r) {
			return
		}
		restSuite(ctx, r, srv, tmp, l)
	}
}

// cancelled фиксирует отмену прогона сигналом: помечаются оставшиеся фазы —
// заметкой пропуска, шаги дальше не выполняются и не числятся FAIL.
func cancelled(ctx context.Context, r *report) bool {
	if ctx.Err() == nil {
		return false
	}
	r.note("прогон отменён сигналом — оставшиеся фазы пропущены")
	return true
}

// findRepoRoot — корень модуля (go.mod) от текущего каталога вверх.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod не найден от %s", dir)
		}
		dir = parent
	}
}

// envDialect — фильтр ноги прогона из CATALOG_QA_DIALECT ("" — все
// доступные; конвенция общей переменной с qa/load_test.go).
func envDialect() string { return os.Getenv("CATALOG_QA_DIALECT") }

// envScale — масштаб нагрузочной прикидки из CATALOG_QA_SCALE (0 — не задан).
func envScale() int {
	v := os.Getenv("CATALOG_QA_SCALE")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
