package storage

import (
	"reflect"
	"testing"
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
		{"явный jit=off не дублируется", "host=h jit=off", "host=h jit=off"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withJitDisabled(c.dsn); got != c.want {
				t.Fatalf("DSN: %q, ожидался %q", got, c.want)
			}
		})
	}
}
