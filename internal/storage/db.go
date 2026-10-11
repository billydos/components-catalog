package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
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
// Всё прочее — переносимый DML на database/sql: текст с @имя-плейсхолдерами
// переписывается в позиционные $n (positional).
type Dialect interface {
	// Name возвращает код диалекта ("sqlite" | "postgres").
	Name() string

	// Open открывает пулы соединений по конфигурации.
	Open(ctx context.Context, cfg Config) (*pools, error)

	// RewriteDDL подставляет в переносимый оператор DDL определения
	// диалекта: авто-PK колонок id (sqlite: INTEGER PRIMARY KEY
	// AUTOINCREMENT; postgres: IDENTITY) и точный целочисленный тип
	// (postgres: INTEGER → BIGINT — паритет диапазона int64 со sqlite).
	RewriteDDL(stmt string) string

	// InsertReturningID выполняет INSERT внутри транзакции и возвращает
	// сгенерированный id (sqlite: INSERT … RETURNING id; postgres:
	// INSERT … RETURNING id).
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
//
// Ловушка единственного соединения: у базы sqlite :memory: (и при
// MaxOpenConns(1) у postgres) чтение и запись делят одно соединение —
// DB-чтение (методы DB) внутри открытой Tx самозаблокируется до отмены
// контекста: соединение занято транзакцией. Внутри транзакции читайте
// только методами Tx; DB-чтения — вне транзакций.
type DB struct {
	dialect Dialect
	reads   *sql.DB
	writes  *sql.DB
	same    bool
}

// Open открывает хранилище по конфигурации; схему не проверяет и не создаёт
// (это делают CheckSchema/EnsureCreated). Ошибки конфигурации и подключения —
// доменные (storage_dsn_missing / storage_dialect_unknown /
// storage_open_failed) — деталь драйвера передаётся аргументом сообщения.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		return nil, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgStorageDsnMissing, cfg.Dialect)
	}
	var d Dialect
	switch cfg.Dialect {
	case "sqlite":
		d = sqliteDialect{}
	case "postgres":
		d = postgresDialect{}
	default:
		return nil, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgStorageDialectUnknown, cfg.Dialect)
	}
	p, err := d.Open(ctx, cfg)
	if err != nil {
		return nil, domain.NewErrorf(domain.CodeStorageOpenFailed,
			domain.MsgStorageOpen, err.Error())
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

// BeginReadOnly открывает транзакцию чтения из пула чтения: проверки без
// DML (dry-run, каноническое сравнение Skipped, пакетные чтения карточки)
// не занимают выделенное соединение записи (SQLite) и не блокируют
// писателей. Транзакция только читается — Commit отвергается, откат —
// Rollback вызывающего (обычно defer).
func (d *DB) BeginReadOnly(ctx context.Context) (*Tx, error) {
	tx, err := d.reads.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx, dialect: d.dialect, readOnly: true}, nil
}

// Tx — транзакция записи: Commit фиксирует, откат — Rollback (присутствует
// в defer вызывающего). Все DML-методы хранилища принимают *Tx.
type Tx struct {
	tx       *sql.Tx
	dialect  Dialect
	readOnly bool // транзакция пула чтения: Commit запрещён
}

// Commit фиксирует транзакцию.
func (t *Tx) Commit() error {
	if t.readOnly {
		return domain.NewErrorf(domain.CodeInternal, domain.MsgStorageCommitReadOnly)
	}
	return t.tx.Commit()
}

// Rollback откатывает транзакцию.
func (t *Tx) Rollback() error { return t.tx.Rollback() }

// queryer — общий интерфейс пула чтения и транзакции для переносимых SELECT.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// positional переписывает @имя-плейсхолдеры запроса в позиционные $n
// (нумерация — по первому появлению имени) и собирает аргументы в порядке
// нумерации. $n понимают оба драйвера: pgx — нативно (его путь database/sql
// имена @имя-аргументов отбрасывает, текст запроса не переписывает),
// modernc/sqlite связывает $NNN с аргументом NNN. Повторное имя — один
// параметр. Строковые литералы '...' (кавычка внутри удваивается),
// закавыченные идентификаторы "..." (кавычка внутри удваивается) и
// комментарии -- и /* */ пропускаются; имя без значения не переписывается —
// ошибка связывания укажет его (баг кода, не пользовательский ввод); ключи
// без плейсхолдера игнорируются.
func positional(query string, args map[string]any) (string, []any) {
	if !strings.ContainsRune(query, '@') {
		return query, nil
	}
	var sb strings.Builder
	nums := make(map[string]int, len(args))
	vals := make([]any, 0, len(args))
	for i := 0; i < len(query); {
		switch c := query[i]; {
		case c == '\'':
			j := skipSQLString(query, i)
			sb.WriteString(query[i:j])
			i = j
		case c == '"':
			j := skipSQLQuotedIdent(query, i)
			sb.WriteString(query[i:j])
			i = j
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			j := skipSQLLineComment(query, i)
			sb.WriteString(query[i:j])
			i = j
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			j := skipSQLBlockComment(query, i)
			sb.WriteString(query[i:j])
			i = j
		case c == '@' && i+1 < len(query) && isIdentStart(query[i+1]):
			j := i + 1
			for j < len(query) && isIdentChar(query[j]) {
				j++
			}
			name := query[i+1 : j]
			if v, ok := args[name]; ok {
				n, seen := nums[name]
				if !seen {
					n = len(vals) + 1
					nums[name] = n
					vals = append(vals, v)
				}
				fmt.Fprintf(&sb, "$%d", n)
			} else {
				sb.WriteString(query[i:j])
			}
			i = j
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String(), vals
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}

// queryContext — общий путь SELECT пула чтения и транзакции.
func queryContext(ctx context.Context, q queryer, query string, args map[string]any) (*sql.Rows, error) {
	s, vals := positional(query, args)
	return q.QueryContext(ctx, s, vals...)
}

// queryRowContext — общий путь SELECT … LIMIT 1 пула чтения и транзакции.
func queryRowContext(ctx context.Context, q queryer, query string, args map[string]any) *sql.Row {
	s, vals := positional(query, args)
	return q.QueryRowContext(ctx, s, vals...)
}

// execContext — общий путь DML внутри транзакции записи.
func execContext(ctx context.Context, tx *sql.Tx, query string, args map[string]any) (sql.Result, error) {
	s, vals := positional(query, args)
	return tx.ExecContext(ctx, s, vals...)
}

// maxInList — предел списка IN одного оператора батч-чтений: у драйвера
// sqlite потолок 32766 связанных аргументов, у протокола postgres — 65535;
// чанкование со слиянием результатов страхует контракт для любого
// вызывающего без сырой ошибки драйвера.
const maxInList = 1000

// inList — список «(@p0, @p1, …)» для условия IN по идентификаторам:
// именованные аргументы дописываются в args (пакетное чтение дочерних
// таблиц по списку записей); перезапись в позиционные аргументы — общая
// точка positional.
func inList(prefix string, ids []int64, args map[string]any) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		name := fmt.Sprintf("%s%d", prefix, i)
		parts[i] = "@" + name
		args[name] = id
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// eachIDChunk обходит список id чанками не длиннее maxInList: батч-чтения
// выполняют запрос по чанку и сливают результаты (карта или срез
// вызывающего), лимит аргументов одного оператора не превышается.
func eachIDChunk(ids []int64, fn func(chunk []int64) error) error {
	for start := 0; start < len(ids); start += maxInList {
		end := min(start+maxInList, len(ids))
		if err := fn(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// Чтение вне транзакций — только из пула чтения.

func (d *DB) query(ctx context.Context, q string, args map[string]any) (*sql.Rows, error) {
	return queryContext(ctx, d.reads, q, args)
}

func (d *DB) queryRow(ctx context.Context, q string, args map[string]any) *sql.Row {
	return queryRowContext(ctx, d.reads, q, args)
}

// Чтение внутри транзакции записи — то же соединение.

func (t *Tx) query(ctx context.Context, q string, args map[string]any) (*sql.Rows, error) {
	return queryContext(ctx, t.tx, q, args)
}

func (t *Tx) queryRow(ctx context.Context, q string, args map[string]any) *sql.Row {
	return queryRowContext(ctx, t.tx, q, args)
}

// exec выполняет DML внутри транзакции.
func (t *Tx) exec(ctx context.Context, q string, args map[string]any) (sql.Result, error) {
	return execContext(ctx, t.tx, q, args)
}

// InsertReturningID — вставка с возвратом сгенерированного id (метод
// диалекта; сигнатура зафиксирована планом — docs/plan/02-database.md §4).
func (t *Tx) InsertReturningID(ctx context.Context, query string, args map[string]any) (int64, error) {
	q, vals := positional(query, args)
	return t.dialect.InsertReturningID(ctx, t.tx, q, vals)
}

// InsertIfAbsentReturningID — вставка с атомарным подавлением конфликта
// уникальности и возвратом сгенерированного id (метод диалекта —
// docs/plan/02-database.md §4); для таблиц с суррогатным id.
func (t *Tx) InsertIfAbsentReturningID(ctx context.Context, query string, args map[string]any) (int64, bool, error) {
	q, vals := positional(query, args)
	return t.dialect.InsertIfAbsentReturningID(ctx, t.tx, q, vals)
}

// withOnConflict дописывает подавление конфликта уникальности к оператору
// INSERT: единая точка суффикса « ON CONFLICT DO NOTHING» (хвостовой
// «;»/пробелы оператора не допускаются — снимаются до конкатенации).
func withOnConflict(query string) string {
	return strings.TrimRight(query, "; \t\n\r") + " ON CONFLICT DO NOTHING"
}

// InsertIfAbsent — вставка с атомарным подавлением конфликта уникальности
// для таблиц без суррогатного id (составной PK: связи производителей,
// атрибуты, аналоги): inserted=false — строка уже есть (вставлена
// конкурирующей транзакцией). ON CONFLICT DO NOTHING и RowsAffected
// переносимы, RETURNING не используется.
func (t *Tx) InsertIfAbsent(ctx context.Context, query string, args map[string]any) (bool, error) {
	res, err := execContext(ctx, t.tx, withOnConflict(query), args)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
