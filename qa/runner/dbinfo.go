// Подготовка одноразовой базы PostgreSQL для ног прогона: очистка таблиц
// модуля перед сценариями, чтобы абсолютные проверки количества (count)
// не зависели от остатков предыдущих прогонов. Список таблиц —
// storage.TableNames() (выводится из DDL хранилища, единый источник).
// Сброс деструктивен, поэтому ограждён: автоматически чистится только базa
// из env-конвенции CATALOG_TEST_POSTGRES_DSN (одноразовая по регламенту —
// internal/testutil чистит её каждым `go test`); DSN из флага -postgres-dsn
// чистится исключительно с явным -reset-postgres (main.go).
package main

import (
	"context"
	"database/sql"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/billydos/components-catalog/internal/storage"
)

// pgDetail — деталь проверки с классом ошибки и маскированным DSN:
// драйвер включает строку подключения в текст ошибки «best effort»,
// пароль не должен попадать в закоммичаемый отчёт.
func pgDetail(where string, err error, dsn string) string {
	msg := err.Error()
	if dsn != "" {
		msg = strings.ReplaceAll(msg, dsn, storage.MaskDSN(dsn))
	}
	return where + ": " + msg
}

// resetPostgres очищает одноразовую базу; ok=false — база недоступна
// (нога PostgreSQL не запускается, детали — в отчёте).
func resetPostgres(ctx context.Context, r *report, dsn string) bool {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		r.check("postgres/reset", "очистка одноразовой базы PostgreSQL", false,
			pgDetail("open", err, dsn))
		return false
	}
	defer db.Close() //nolint:errcheck — подключение только для очистки
	pingCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		r.check("postgres/reset", "очистка одноразовой базы PostgreSQL", false,
			pgDetail("ping", err, dsn))
		return false
	}
	for _, tbl := range storage.TableNames() {
		dropCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
		_, err := db.ExecContext(dropCtx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
		cancel()
		if err != nil {
			r.check("postgres/reset", "очистка одноразовой базы PostgreSQL", false,
				pgDetail("drop "+tbl, err, dsn))
			return false
		}
	}
	return r.check("postgres/reset", "очистка одноразовой базы PostgreSQL", true)
}

// pgVersion — строка версии сервера PostgreSQL для секции environment.
func pgVersion(ctx context.Context, dsn string) string {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return ""
	}
	defer db.Close() //nolint:errcheck — подключение только для чтения версии
	verCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var version string
	if err := db.QueryRowContext(verCtx, "SELECT version()").Scan(&version); err != nil {
		return ""
	}
	return version
}

// sqliteVersion — строка версии библиотеки SQLite (modernc.org/sqlite)
// через временную базу в памяти, для секции environment.
func sqliteVersion() string {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return ""
	}
	defer db.Close() //nolint:errcheck — подключение только для чтения версии
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	var version string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return ""
	}
	return "SQLite " + version
}
