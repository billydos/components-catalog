package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// Config — конфигурация открытия хранилища (docs/plan/01-architecture.md §2.3):
// диалект, DSN/путь и параметры пулов соединений.
type Config struct {
	Dialect string // "sqlite" | "postgres"
	DSN     string // путь файла sqlite / DSN postgres
	Pool    PoolConfig
}

// PoolConfig — параметры пулов соединений. Для postgres — единый пул
// (MaxOpen/MaxIdle/ConnMaxLifetime); для sqlite — размер пула чтения
// (запись всегда идёт через одно выделенное соединение) и busy_timeout.
type PoolConfig struct {
	MaxOpen         int
	MaxIdle         int
	ConnMaxLifetime int64 // наносекунды; 0 — без ограничения
	BusyTimeout     int64 // миллисекунды (sqlite); 0 — 5000
}

const defaultBusyTimeoutMs = 5000

// Dialect — диалектозависимая часть хранилища (docs/plan/02-database.md §4):
// открытие и пулы, определение авто-PK в DDL, получение id после вставки.
// Всё прочее — переносимый DML на database/sql + sql.Named (@имя).
type Dialect interface {
	// Name возвращает код диалекта ("sqlite" | "postgres").
	Name() string

	// Open открывает пулы соединений по конфигурации.
	Open(ctx context.Context, cfg Config) (*pools, error)

	// AutoIncSQL возвращает определение авто-PK колонки id
	// (sqlite: PRIMARY KEY AUTOINCREMENT; postgres: IDENTITY).
	AutoIncSQL() string

	// InsertReturningID выполняет INSERT внутри транзакции и возвращает
	// сгенерированный id (sqlite: last_insert_rowid() того же соединения;
	// postgres: INSERT … RETURNING id).
	InsertReturningID(ctx context.Context, tx *sql.Tx, query string, args []any) (int64, error)

	// InsertIfAbsentReturningID выполняет INSERT внутри транзакции,
	// атомарно подавляя конфликт уникальности (ON CONFLICT DO NOTHING),
	// и возвращает сгенерированный id. inserted=false — строка уже есть:
	// либо существовала, либо вставлена конкурирующей транзакцией
	// (postgres: писатели параллельны — общий пул соединений); id в этом
	// случае возвращает SELECT вызывающего кода. Ошибки уникальности
	// не возникает — транзакция остаётся рабочей в обоих диалектах.
	InsertIfAbsentReturningID(ctx context.Context, tx *sql.Tx, query string, args []any) (id int64, inserted bool, err error)
}

// pools — соединения хранилища: пул чтения и соединение записи.
// У sqlite это два независимых пула над одним файлом (чтение — несколько
// соединений, запись — ровно одно); у postgres — единый пул в обеих ролях.
type pools struct {
	reads  *sql.DB
	writes *sql.DB
	same   bool // reads и writes — один пул (postgres): закрывать один раз
}

// DB — открытое хранилище. Правило (docs/plan/01-architecture.md §2.3): весь DML —
// только внутри явных транзакций Begin (соединение записи); вне транзакций —
// только SELECT из пула чтения. Бизнес-правил не содержит.
type DB struct {
	dialect Dialect
	reads   *sql.DB
	writes  *sql.DB
	same    bool
}

// Open открывает хранилище по конфигурации; схему не проверяет и не создаёт
// (это делают CheckSchema/EnsureCreated).
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("хранилище: не задан DSN (диалект %q)", cfg.Dialect)
	}
	var d Dialect
	switch cfg.Dialect {
	case "sqlite":
		d = sqliteDialect{}
	case "postgres":
		d = postgresDialect{}
	default:
		return nil, fmt.Errorf("хранилище: неизвестный диалект %q", cfg.Dialect)
	}
	p, err := d.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &DB{dialect: d, reads: p.reads, writes: p.writes, same: p.same}, nil
}

// Close закрывает пулы соединений.
func (d *DB) Close() error {
	if d.same {
		return d.reads.Close()
	}
	werr := d.writes.Close()
	rerr := d.reads.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

// DialectName возвращает код диалекта хранилища.
func (d *DB) DialectName() string { return d.dialect.Name() }

// Begin открывает явную транзакцию записи (единственный путь DML).
func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	tx, err := d.writes.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx, dialect: d.dialect}, nil
}

// Tx — транзакция записи: Commit фиксирует, откат — Rollback (присутствует
// в defer вызывающего). Все DML-методы хранилища принимают *Tx.
type Tx struct {
	tx      *sql.Tx
	dialect Dialect
}

// Commit фиксирует транзакцию.
func (t *Tx) Commit() error { return t.tx.Commit() }

// Rollback откатывает транзакцию.
func (t *Tx) Rollback() error { return t.tx.Rollback() }

// queryer — общий интерфейс пула чтения и транзакции для переносимых SELECT.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// named — собирает именованные аргументы (@имя) из отображения.
func named(args map[string]any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, 0, len(args))
	for k, v := range args {
		out = append(out, sql.Named(k, v))
	}
	return out
}

// Чтение вне транзакций — только из пула чтения.

func (d *DB) query(ctx context.Context, q string, args map[string]any) (*sql.Rows, error) {
	return d.reads.QueryContext(ctx, q, named(args)...)
}

func (d *DB) queryRow(ctx context.Context, q string, args map[string]any) *sql.Row {
	return d.reads.QueryRowContext(ctx, q, named(args)...)
}

// Чтение внутри транзакции записи — то же соединение.

func (t *Tx) query(ctx context.Context, q string, args map[string]any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, q, named(args)...)
}

func (t *Tx) queryRow(ctx context.Context, q string, args map[string]any) *sql.Row {
	return t.tx.QueryRowContext(ctx, q, named(args)...)
}

// exec выполняет DML внутри транзакции.
func (t *Tx) exec(ctx context.Context, q string, args map[string]any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, q, named(args)...)
}

// InsertReturningID — вставка с возвратом сгенерированного id (метод
// диалекта; сигнатура зафиксирована планом — docs/plan/02-database.md §4).
func (t *Tx) InsertReturningID(ctx context.Context, query string, args map[string]any) (int64, error) {
	return t.dialect.InsertReturningID(ctx, t.tx, query, named(args))
}

// InsertIfAbsentReturningID — вставка с атомарным подавлением конфликта
// уникальности и возвратом сгенерированного id (метод диалекта —
// docs/plan/02-database.md §4).
func (t *Tx) InsertIfAbsentReturningID(ctx context.Context, query string, args map[string]any) (int64, bool, error) {
	return t.dialect.InsertIfAbsentReturningID(ctx, t.tx, query, named(args))
}
