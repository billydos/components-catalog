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
		scale = flag.Int("scale", envScale(), "масштаб нагрузочной прикидки, записей (0 — фаза опущена)")
		keep  = flag.Bool("keep", false, "не удалять временный каталог прогона (диагностика)")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "runner: %v\n", err)
		os.Exit(2)
	}

	path := *reportPath
	if path == "" {
		path = filepath.Join(root, "qa", "reports", fmt.Sprintf("qa-%s-%s-%s.log",
			time.Now().Format("20060102-150405"), runtime.GOOS, runtime.GOARCH))
	}
	r, err := newReport(path, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runner: отчёт %s: %v\n", path, err)
		os.Exit(2)
	}

	opts := runOptions{scale: *scale, keep: *keep}
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

	run(ctx, r, root, opts)
	verdict := r.finish()
	fmt.Printf("отчёт: %s (%s)\n", path, verdict)
	if verdict != "PASS" {
		os.Exit(1)
	}
}

// runOptions — параметры прогона.
type runOptions struct {
	postgresDSN   string
	resetPostgres bool
	scale         int
	keep          bool
}

// run выполняет фазы прогона; отказ фазы не прерывает последующие
// (кроме сбоя сборки бинарников — сценарии невозможны).
func run(ctx context.Context, r *report, root string, opts runOptions) {
	writeEnvSection(r, root)
	if opts.postgresDSN != "" {
		r.kv("postgres_server", pgVersion(ctx, opts.postgresDSN))
	} else {
		r.note("нога PostgreSQL опущена: CATALOG_TEST_POSTGRES_DSN / -postgres-dsn не заданы")
	}
	r.kv("sqlite_server", sqliteVersion())

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

	ctl, srv, ok := buildBinaries(ctx, r, root, tmp)
	if !ok {
		return
	}
	staticPhase(ctx, r, root)

	legs := []leg{{name: "sqlite", dbArgs: []string{"--db", filepath.Join(tmp, "qa.db")}}}
	if opts.postgresDSN != "" {
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
	}
	for _, l := range legs {
		cliSuite(ctx, r, ctl, root, tmp, l)
		restSuite(ctx, r, srv, tmp, l)
	}

	if opts.scale > 0 {
		loadPhase(ctx, r, root, opts.scale)
	}
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
