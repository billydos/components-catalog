package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite" // драйвер регистрируется как "sqlite" (pure Go, без cgo)
)

// sqliteDialect — диалект SQLite (modernc.org/sqlite): файл или память;
// пул чтения + одно выделенное соединение записи; на каждом соединении
// PRAGMA foreign_keys = ON и busy_timeout, для файла — WAL — одновременная
// работа читателей и одного писателя (docs/plan/02-database.md §4).
type sqliteDialect struct{}

func (sqliteDialect) Name() string { return "sqlite" }

func (sqliteDialect) AutoIncSQL() string {
	// AUTOINCREMENT — осознанный запрет переиспользования id после удаления:
	// id — стабильная внешняя ссылка (FK, REST). Не «оптимизировать».
	return "PRIMARY KEY AUTOINCREMENT"
}

func (d sqliteDialect) InsertReturningID(ctx context.Context, tx *sql.Tx, query string, args []any) (int64, error) {
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return 0, err
	}
	// Транзакция привязана к одному соединению — last_insert_rowid() корректен.
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT last_insert_rowid()").Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (d sqliteDialect) InsertIfAbsentReturningID(ctx context.Context, tx *sql.Tx, query string, args []any) (int64, bool, error) {
	res, err := tx.ExecContext(ctx, query+" ON CONFLICT DO NOTHING", args...)
	if err != nil {
		return 0, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, false, nil
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT last_insert_rowid()").Scan(&id); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// isMemoryDSN сообщает, адресует ли DSN базу в памяти.
func isMemoryDSN(dsn string) bool {
	if dsn == ":memory:" {
		return true
	}
	if strings.HasPrefix(dsn, "file:") {
		if u, err := url.Parse(dsn); err == nil {
			return u.Opaque == ":memory:" || strings.Contains(u.Path, ":memory:") ||
				u.Query().Get("mode") == "memory"
		}
	}
	return false
}

// sqlitePragmas дописывает к DSN параметры соединения: внешние ключи и
// busy_timeout — на каждом соединении; journal_mode=WAL — только для файла
// (у базы в памяти журнала нет).
func sqlitePragmas(dsn string, busyTimeoutMs int, wal bool) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMs))
	if wal {
		q.Add("_pragma", "journal_mode(WAL)")
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + q.Encode()
}

func (sqliteDialect) Open(ctx context.Context, cfg Config) (*pools, error) {
	busy := defaultBusyTimeoutMs
	if cfg.Pool.BusyTimeout > 0 {
		busy = int(cfg.Pool.BusyTimeout)
	}
	memory := isMemoryDSN(cfg.DSN)
	if memory {
		// База в памяти живёт, пока открыто хотя бы одно соединение:
		// чтение и запись делят один пул с единственным соединением —
		// вся база видна обеим ролям, дисциплина «DML только в транзакциях»
		// не меняется.
		dsn := sqlitePragmas(cfg.DSN, busy, false)
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, err
		}
		return &pools{reads: db, writes: db, same: true}, nil
	}
	// Файл: пул чтения (несколько соединений) + одно выделенное соединение
	// записи; WAL позволяет читателям не блокироваться писателем.
	writeDB, err := sql.Open("sqlite", sqlitePragmas(cfg.DSN, busy, true))
	if err != nil {
		return nil, err
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	if err := writeDB.PingContext(ctx); err != nil {
		writeDB.Close()
		return nil, err
	}
	readDB, err := sql.Open("sqlite", sqlitePragmas(cfg.DSN, busy, true))
	if err != nil {
		writeDB.Close()
		return nil, err
	}
	if cfg.Pool.MaxOpen > 0 {
		readDB.SetMaxOpenConns(cfg.Pool.MaxOpen)
	}
	if cfg.Pool.MaxIdle > 0 {
		readDB.SetMaxIdleConns(cfg.Pool.MaxIdle)
	}
	if cfg.Pool.ConnMaxLifetime > 0 {
		readDB.SetConnMaxLifetime(durationOf(cfg.Pool.ConnMaxLifetime))
	}
	if err := readDB.PingContext(ctx); err != nil {
		writeDB.Close()
		readDB.Close()
		return nil, err
	}
	return &pools{reads: readDB, writes: writeDB}, nil
}
