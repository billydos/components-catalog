package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/domain"
)

// TestPositional — перезапись @имя → $n: нумерация по первому появлению
// имени, повторное имя — один параметр, литералы и комментарии не
// затрагиваются, имя без значения остаётся как есть.
func TestPositional(t *testing.T) {
	cases := []struct {
		name string
		q    string
		args map[string]any
		want string
		vals []any
	}{
		{
			name: "без плейсхолдеров — текст не меняется",
			q:    `SELECT code FROM kinds ORDER BY code`,
			args: map[string]any{},
			want: `SELECT code FROM kinds ORDER BY code`,
			vals: nil,
		},
		{
			name: "порядок аргументов — по первым появлениям",
			q:    `SELECT id FROM devices WHERE kind_code = @kind AND designation = @designation`,
			args: map[string]any{"kind": "resistor", "designation": "МЛТ-0.5"},
			want: `SELECT id FROM devices WHERE kind_code = $1 AND designation = $2`,
			vals: []any{"resistor", "МЛТ-0.5"},
		},
		{
			name: "повторное имя — один параметр",
			q:    `SELECT 1 FROM schema_meta WHERE key = @key AND value <> @key`,
			args: map[string]any{"key": "schema_version"},
			want: `SELECT 1 FROM schema_meta WHERE key = $1 AND value <> $1`,
			vals: []any{"schema_version"},
		},
		{
			name: "литерал с @ и экранированной кавычкой не затрагивается",
			q:    `SELECT @a WHERE x = 'a@b '' @c' AND y = @b`,
			args: map[string]any{"a": 1, "b": 2},
			want: `SELECT $1 WHERE x = 'a@b '' @c' AND y = $2`,
			vals: []any{1, 2},
		},
		{
			name: "escape-литерал LIKE не затрагивается",
			q:    `SELECT d.id FROM devices d WHERE d.designation LIKE @p ESCAPE '\'`,
			args: map[string]any{"p": "%КТ%"},
			want: `SELECT d.id FROM devices d WHERE d.designation LIKE $1 ESCAPE '\'`,
			vals: []any{"%КТ%"},
		},
		{
			name: "комментарий с @ не затрагивается",
			q:    "SELECT @kind -- @comment\nFROM devices",
			args: map[string]any{"kind": "diode"},
			want: "SELECT $1 -- @comment\nFROM devices",
			vals: []any{"diode"},
		},
		{
			name: "имя без значения остаётся @имя",
			q:    `SELECT id FROM devices WHERE kind_code = @kind AND designation = @designation`,
			args: map[string]any{"kind": "resistor"},
			want: `SELECT id FROM devices WHERE kind_code = $1 AND designation = @designation`,
			vals: []any{"resistor"},
		},
		{
			name: "ключ без плейсхолдера игнорируется",
			q:    `SELECT id FROM devices WHERE kind_code = @kind`,
			args: map[string]any{"kind": "resistor", "extra": 1},
			want: `SELECT id FROM devices WHERE kind_code = $1`,
			vals: []any{"resistor"},
		},
		{
			name: "имена с цифрами и знаком подчёркивания",
			q:    `VALUES (@id, @f0, @t0, @n0)`,
			args: map[string]any{"id": int64(7), "f0": "material", "t0": "si", "n0": nil},
			want: `VALUES ($1, $2, $3, $4)`,
			vals: []any{int64(7), "material", "si", nil},
		},
		{
			name: "@ перед цифрой — не имя, остаётся как есть",
			q:    `SELECT @1, @kind FROM devices`,
			args: map[string]any{"kind": "diode", "1": "x"},
			want: `SELECT @1, $1 FROM devices`,
			vals: []any{"diode"},
		},
		{
			name: "незакрытый литерал — копируется без поиска имён",
			q:    `SELECT @kind, 'незакрытый @literal`,
			args: map[string]any{"kind": "diode"},
			want: `SELECT $1, 'незакрытый @literal`,
			vals: []any{"diode"},
		},
		{
			name: "@ последним символом — не плейсхолдер",
			q:    `SELECT @kind, @`,
			args: map[string]any{"kind": "diode"},
			want: `SELECT $1, @`,
			vals: []any{"diode"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, vals := positional(c.q, c.args)
			if got != c.want {
				t.Fatalf("запрос: %q, ожидался %q", got, c.want)
			}
			if len(vals) != len(c.vals) {
				t.Fatalf("аргументы: %v (%d), ожидалось %v (%d)", vals, len(vals), c.vals, len(c.vals))
			}
			for i := range vals {
				if vals[i] != c.vals[i] {
					t.Fatalf("аргумент[%d]: %v (%T), ожидался %v (%T)", i, vals[i], vals[i], c.vals[i], c.vals[i])
				}
			}
		})
	}
}

// TestPositionalRepeatedNameQuery — повторное имя связывается одним
// аргументом на обоих диалектах ($1 в двух местах текста).
func TestPositionalRepeatedNameQuery(t *testing.T) {
	_, db := openSQLiteFile(t)
	ctx := t.Context()
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck — тестовая транзакция
	q := `SELECT COUNT(*) FROM schema_meta WHERE key = @key OR value = @key`
	var n int
	if err := tx.queryRow(ctx, q, map[string]any{"key": "schema_version"}).Scan(&n); err != nil {
		t.Fatalf("запрос с повторным именем: %v", err)
	}
	if n != 1 {
		t.Fatalf("счёт: %d, ожидался 1", n)
	}
}

// TestPositionalIDsRoundtrip — нумерация устойчива к порядку ключей map:
// текст с последовательностью @a/@b/@a даёт согласованные $1/$2/$1.
func TestPositionalIDsRoundtrip(t *testing.T) {
	q := `SELECT @a, @b, @a`
	got, vals := positional(q, map[string]any{"a": "x", "b": "y"})
	if want := `SELECT $1, $2, $1`; got != want {
		t.Fatalf("запрос: %q, ожидался %q", got, want)
	}
	if !reflect.DeepEqual(vals, []any{"x", "y"}) {
		t.Fatalf("аргументы: %v", vals)
	}
}

// TestWithJitDisabled — jit=off дописывается к DSN в его форме (URL,
// ключевая), явный jit= в DSN не перекрывается.
func TestWithJitDisabled(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{"URL без запроса", "postgres://h/db", "postgres://h/db?jit=off"},
		{"URL с запросом", "postgres://h/db?sslmode=disable", "postgres://h/db?sslmode=disable&jit=off"},
		{"ключевая форма", "host=h port=5432", "host=h port=5432 jit=off"},
		{"явный jit=on сохраняется", "postgres://h/db?jit=on", "postgres://h/db?jit=on"},
		{"явный jit=off в URL не дублируется", "postgres://h/db?jit=off", "postgres://h/db?jit=off"},
		{"jit= в пароле URL — не параметр", "postgres://u:jit=xx@h/db", "postgres://u:jit=xx@h/db?jit=off"},
		{"ключ с jit в имени URL — не параметр", "postgres://h/db?ssljit=1", "postgres://h/db?ssljit=1&jit=off"},
		{"ключ с jit в имени ключевой формы — не параметр", "host=h ajit=1", "host=h ajit=1 jit=off"},
		{"jit= в значении ключевой формы — не параметр", "host=h password=jit=xx", "host=h password=jit=xx jit=off"},
		{"jit= в закавыченном значении — не параметр", `host=h password='foo jit=bar'`, `host=h password='foo jit=bar' jit=off`},
		{"jit= после экранированного пробела — не параметр", `host=h password=foo\ jit=bar`, `host=h password=foo\ jit=bar jit=off`},
		{"пробелы вокруг = — явный jit", "host=h jit = on", "host=h jit = on"},
		{"явный jit=off в ключевой форме не дублируется", "host=h jit=off", "host=h jit=off"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withJitDisabled(c.dsn); got != c.want {
				t.Fatalf("DSN: %q, ожидался %q", got, c.want)
			}
		})
	}
}

// TestPositionalBlockCommentAndQuotedIdent — блочные комментарии и
// закавыченные идентификаторы пропускаются лексером: @имя внутри них не
// переписывается.
func TestPositionalBlockCommentAndQuotedIdent(t *testing.T) {
	cases := []struct {
		name string
		q    string
		args map[string]any
		want string
		vals []any
	}{
		{
			name: "блочный комментарий с @",
			q:    "SELECT @kind /* @comment @x */ FROM devices",
			args: map[string]any{"kind": "diode"},
			want: "SELECT $1 /* @comment @x */ FROM devices",
			vals: []any{"diode"},
		},
		{
			name: "незакрытый блочный комментарий — до конца текста",
			q:    "SELECT @kind /* @comment",
			args: map[string]any{"kind": "diode"},
			want: "SELECT $1 /* @comment",
			vals: []any{"diode"},
		},
		{
			name: "закавыченный идентификатор с @",
			q:    `SELECT t."group@name" FROM t WHERE x = @group`,
			args: map[string]any{"group": "g"},
			want: `SELECT t."group@name" FROM t WHERE x = $1`,
			vals: []any{"g"},
		},
		{
			name: "кавычка внутри идентификатора удваивается",
			q:    `SELECT "a""@b" , @v FROM t`,
			args: map[string]any{"v": 1},
			want: `SELECT "a""@b" , $1 FROM t`,
			vals: []any{1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, vals := positional(c.q, c.args)
			if got != c.want {
				t.Fatalf("запрос: %q, ожидался %q", got, c.want)
			}
			if !reflect.DeepEqual(vals, c.vals) {
				t.Fatalf("аргументы: %v, ожидались %v", vals, c.vals)
			}
		})
	}
}

// TestMemoryDBReadInsideTxTimesOut — пин-тест ловушки единственного
// соединения: в режиме :memory: DB-чтение внутри открытой Tx ждёт
// соединение до отмены контекста (внутри транзакции — только методы Tx).
func TestMemoryDBReadInsideTxTimesOut(t *testing.T) {
	ctx := t.Context()
	db, err := Open(ctx, Config{Dialect: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close() //nolint:errcheck — тестовая база
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck — тестовая транзакция
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	var n int
	if err := db.queryRow(cctx, `SELECT COUNT(*) FROM kinds`, nil).Scan(&n); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ожидалась блокировка до отмены контекста, получено %v", err)
	}
}

// TestBumpRevisionMissingRow — отсутствие строки schema_meta при
// инкременте ревизии — доменная ошибка, а не молчаливый ноль.
func TestBumpRevisionMissingRow(t *testing.T) {
	_, db := openSQLiteFile(t)
	ctx := t.Context()
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck — тестовая транзакция
	if _, err := tx.exec(ctx, `DELETE FROM schema_meta WHERE key = 'data_revision'`, nil); err != nil {
		t.Fatalf("удаление строки ревизии: %v", err)
	}
	err = tx.BumpDataRevision(ctx)
	de, ok := domain.AsError(err)
	if !ok || de.MsgID != domain.MsgStorageRevisionRowMissing {
		t.Fatalf("ожидалась storage_revision_row_missing, получено %v", err)
	}
}

// pgFakeErr — ошибка с SQLState (поверхность драйвера postgres).
type pgFakeErr struct{ state string }

func (e pgFakeErr) Error() string    { return "pg error " + e.state }
func (e pgFakeErr) SQLState() string { return e.state }

// TestIsNoTableError — postgres распознаётся только по SQLState 42P01:
// «column does not exist» (42703) таблицей не является; sqlite — по тексту.
func TestIsNoTableError(t *testing.T) {
	if !isNoTableError(pgFakeErr{state: "42P01"}) {
		t.Fatal("42P01 должна распознаваться как отсутствие таблицы")
	}
	if isNoTableError(pgFakeErr{state: "42703"}) {
		t.Fatal("42703 (undefined_column) — не отсутствие таблицы")
	}
	if isNoTableError(errors.New(`pq: column "x" does not exist`)) {
		t.Fatal("текст «does not exist» без SQLState — не postgres-таблица")
	}
	if !isNoTableError(errors.New("SQL logic error: no such table: schema_meta (1)")) {
		t.Fatal("текст sqlite «no such table» должен распознаваться")
	}
	if isNoTableError(nil) || isNoTableError(errors.New("другая ошибка")) {
		t.Fatal("прочие ошибки не распознаются")
	}
}

// TestRewriteDDLWordBoundaries — замена INTEGER/REAL только целыми словами
// вне литералов, закавыченных идентификаторов и комментариев (включая
// точное совпадение слова внутри кавычек — «"INTEGER"» не трогается).
func TestRewriteDDLWordBoundaries(t *testing.T) {
	in := `CREATE TABLE t (
    id INTEGER {{AUTOINC}},
    "INTEGER_FIELD" TEXT,
    "INTEGER" TEXT,
    note TEXT DEFAULT 'INTEGER REAL',
    val REAL NULL -- INTEGER stays
)`
	want := `CREATE TABLE t (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    "INTEGER_FIELD" TEXT,
    "INTEGER" TEXT,
    note TEXT DEFAULT 'INTEGER REAL',
    val DOUBLE PRECISION NULL -- INTEGER stays
)`
	if got := (postgresDialect{}).RewriteDDL(in); got != want {
		t.Fatalf("RewriteDDL:\n%s\nожидалось:\n%s", got, want)
	}
}

// TestBatchReadsOverChunkLimit — списки длиннее предела maxInList
// читаются чанками со слиянием результатов, без сырой ошибки драйвера.
func TestBatchReadsOverChunkLimit(t *testing.T) {
	db := openDevicesDB(t, "sqlite")
	ctx := t.Context()
	const extra = 5
	total := maxInList + extra
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	ids := make([]int64, 0, total)
	for i := 0; i < total; i++ {
		id, _, err := tx.InsertDevice(ctx, "resistor", "gost", fmt.Sprintf("D%06d", i))
		if err != nil {
			t.Fatalf("вставка %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	rows, err := db.FindDevicesByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("FindDevicesByIDs: %v", err)
	}
	if len(rows) != total {
		t.Fatalf("записей: %d, ожидалось %d", len(rows), total)
	}
	for i, r := range rows {
		if r.ID != ids[i] {
			t.Fatalf("порядок: строка %d — id %d, ожидался %d", i, r.ID, ids[i])
		}
	}
	variants, err := db.LoadVariantsByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("LoadVariantsByIDs: %v", err)
	}
	if len(variants) != 0 {
		t.Fatalf("исполнений не было, получено %d", len(variants))
	}
}

// TestSearchPageValidation — отрицательные limit/offset отвергаются
// доменной ошибкой до запроса.
func TestSearchPageValidation(t *testing.T) {
	db := openDevicesDB(t, "sqlite")
	for _, r := range []SearchRequest{
		{Limit: -1, Offset: 0},
		{Limit: 0, Offset: -1},
	} {
		_, _, err := db.Search(t.Context(), r)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeValidationFailed || de.MsgID != domain.MsgStoragePageInvalid {
			t.Fatalf("limit=%d offset=%d: ожидалась storage_page_invalid, получено %v",
				r.Limit, r.Offset, err)
		}
	}
}

// TestSearchEmptyPageBeyondEndKeepsTotal — страница за концом выдачи
// пуста, total при этом корректен (добирается отдельным COUNT).
func TestSearchEmptyPageBeyondEndKeepsTotal(t *testing.T) {
	db := openDevicesDB(t, "sqlite")
	ctx := t.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, _, err := tx.InsertDevice(ctx, "resistor", "gost", fmt.Sprintf("D%d", i)); err != nil {
			t.Fatalf("вставка: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	items, total, err := db.Search(ctx, SearchRequest{Limit: 2, Offset: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(items) != 0 || total != 3 {
		t.Fatalf("страница %d записей, total %d — ожидались 0 и 3", len(items), total)
	}
}

// TestSearchSortByFieldKeys — сортировка по кодам полей разбора (LEFT JOIN
// device_designation_fields) работоспособна и детерминирована: записи без
// значения поля — в конце независимо от направления, финальный порядок —
// (kind, designation).
func TestSearchSortByFieldKeys(t *testing.T) {
	db := openDevicesDB(t, "sqlite")
	ctx := t.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// dev_number: 301, 315, 305; у D2 поля нет.
	for _, d := range []struct {
		desig string
		num   float64
	}{
		{"D1", 301}, {"D2", -1}, {"D3", 315}, {"D4", 305},
	} {
		id, _, err := tx.InsertDevice(ctx, "resistor", "gost", d.desig)
		if err != nil {
			t.Fatalf("вставка: %v", err)
		}
		if d.num >= 0 {
			if err := tx.InsertDesignationFields(ctx, id, []domain.Field{
				domain.NumField("dev_number", d.num),
			}); err != nil {
				t.Fatalf("поля: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	asc := []string{"D1", "D4", "D3", "D2"}
	desc := []string{"D1", "D4", "D3", "D2"} // NULL-последние сохраняются
	_ = desc
	wantDesc := []string{"D3", "D4", "D1", "D2"}
	for _, tc := range []struct {
		name  string
		sorts []SortKey
		want  []string
	}{
		{"возрастание", []SortKey{{Key: "dev_number", Numeric: true}}, asc},
		{"убывание", []SortKey{{Key: "dev_number", Numeric: true, Desc: true}}, wantDesc},
		{"два ключа", []SortKey{{Key: "dev_number", Numeric: true}, {Key: "designation"}}, asc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, total, err := db.Search(ctx, SearchRequest{Sorts: tc.sorts, Limit: 10})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if total != 4 {
				t.Fatalf("total: %d", total)
			}
			got := make([]string, len(items))
			for i, it := range items {
				got[i] = it.Designation
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("порядок: %v, ожидался %v", got, tc.want)
			}
		})
	}
}

// TestSearchSortByParamKeys — сортировка по кодам числовых параметров:
// производная таблица по parameter_values с GROUP BY device_id
// не размножает строки (повторные и вариантные значения одного кода),
// правило значения — по направлению (MIN нижних / MAX верхних границ,
// COALESCE падает на точное значение и единственную границу односторонних
// форм), записи без значения — в конце независимо от направления.
func TestSearchSortByParamKeys(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			seedFacetCatalog(t, db)
			ctx := context.Background()
			seedSortDevice := func(designation string, typeVals, variantVals []ParamValue) {
				t.Helper()
				tx, err := db.Begin(ctx)
				if err != nil {
					t.Fatalf("begin: %v", err)
				}
				id, _, err := tx.InsertDevice(ctx, "resistor", "gost", designation)
				if err != nil {
					t.Fatalf("запись %q: %v", designation, err)
				}
				if len(typeVals) > 0 {
					if err := tx.InsertValues(ctx, id, nil, typeVals); err != nil {
						t.Fatalf("значения %q: %v", designation, err)
					}
				}
				if len(variantVals) > 0 {
					vid, err := tx.InsertVariant(ctx, id, "", 0)
					if err != nil {
						t.Fatalf("исполнение %q: %v", designation, err)
					}
					if err := tx.InsertValues(ctx, id, &vid, variantVals); err != nil {
						t.Fatalf("вариантные значения %q: %v", designation, err)
					}
				}
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit: %v", err)
				}
			}
			// R1 — размножение строк: две строки типа (10..20 и 5..30)
			// и вариантное значение 25 одного кода: ключ ASC — 5
			// (минимум нижних), DESC — 30 (максимум верхних), total — 1.
			seedSortDevice("R1",
				[]ParamValue{
					{Parameter: "hfe", Min: f(10), Max: f(20)},
					{Parameter: "hfe", Min: f(5), Max: f(30)},
				},
				[]ParamValue{{Parameter: "hfe", Exact: f(25)}})
			// R2 — точное значение: участвует в обоих направлениях.
			seedSortDevice("R2", []ParamValue{{Parameter: "hfe", Exact: f(15)}}, nil)
			// R3 — односторонняя форма (at_most): единственная граница
			// становится ключом в обоих направлениях.
			seedSortDevice("R3", []ParamValue{{Parameter: "hfe", Max: f(100)}}, nil)
			// R4 — значения параметра нет (текстовое значение другого
			// кода не считается): в конце независимо от направления.
			seedSortDevice("R4", []ParamValue{{Parameter: "case", Text: s("A")}}, nil)

			par := SortKey{Key: "hfe", Param: true}
			for _, tc := range []struct {
				name  string
				sorts []SortKey
				want  []string
			}{
				{"возрастание (минимум нижних границ)", []SortKey{par}, []string{"R1", "R2", "R3", "R4"}},
				{"убывание (максимум верхних границ)", []SortKey{{Key: "hfe", Param: true, Desc: true}}, []string{"R3", "R1", "R2", "R4"}},
				{"вместе с ключом-полем", []SortKey{par, {Key: "dev_number", Numeric: true}}, []string{"R1", "R2", "R3", "R4"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					items, total, err := db.Search(ctx, SearchRequest{Sorts: tc.sorts, Limit: 10})
					if err != nil {
						t.Fatalf("search: %v", err)
					}
					if total != 4 {
						t.Fatalf("total: %d — строки размножились", total)
					}
					got := make([]string, len(items))
					for i, it := range items {
						got[i] = it.Designation
					}
					if !slices.Equal(got, tc.want) {
						t.Fatalf("порядок: %v, ожидался %v", got, tc.want)
					}
				})
			}
		})
	}
}
