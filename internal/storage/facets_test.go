package storage

import (
	"context"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// seedFacetCatalog — минимальный каталог для FK parameter_values:
// группа, численный (range) и enum параметры.
func seedFacetCatalog(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO kinds(code) VALUES ('capacitor')`,
		`INSERT INTO parameter_groups(code, section_name, sort_order) VALUES ('el', 'parameters', 0)`,
		`INSERT INTO parameters(code, group_code, value_type, sort_order) VALUES ('hfe', 'el', 'range', 0)`,
		`INSERT INTO parameters(code, group_code, value_type, sort_order) VALUES ('case', 'el', 'enum', 1)`,
	} {
		if _, err := tx.exec(ctx, q, nil); err != nil {
			t.Fatalf("сид %q: %v", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// addFacetDevice вставляет запись резистора со значениями (без валидации
// движка — тест хранилища проверяет агрегаты, а не семантику каталога).
func addFacetDevice(t *testing.T, db *DB, designation string, values []ParamValue) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	id, _, err := tx.InsertDevice(ctx, "resistor", "gost", designation)
	if err != nil {
		t.Fatalf("запись %q: %v", designation, err)
	}
	if err := tx.InsertValues(ctx, id, nil, values); err != nil {
		t.Fatalf("значения %q: %v", designation, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func facetBounds(t *testing.T, db *DB, kind domain.Kind) map[string]ParamBounds {
	t.Helper()
	tx, err := db.BeginReadOnly(context.Background())
	if err != nil {
		t.Fatalf("begin read only: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	out, err := tx.ParameterBounds(context.Background(), kind)
	if err != nil {
		t.Fatalf("ParameterBounds: %v", err)
	}
	return out
}

func facetFreqs(t *testing.T, db *DB, kind domain.Kind) map[string][]ParamValueFreq {
	t.Helper()
	tx, err := db.BeginReadOnly(context.Background())
	if err != nil {
		t.Fatalf("begin read only: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	out, err := tx.ParameterValueFreqs(context.Background(), kind)
	if err != nil {
		t.Fatalf("ParameterValueFreqs: %v", err)
	}
	return out
}

func eqFloat(p *float64, want float64) bool { return p != nil && *p == want }

func f(v float64) *float64 { return &v }

func s(v string) *string { return &v }

// Границы и частоты: смешанные формы значений (range/exact/at_least/
// at_most/text) агрегируются по семантике фильтров; класс фильтрует;
// enum-частоты считают записи, а не строки значений.
func TestParameterFacets(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			seedFacetCatalog(t, db)

			pv := func(code string, min, max, exact *float64, text *string) ParamValue {
				return ParamValue{Parameter: code, Min: min, Max: max, Exact: exact, Text: text}
			}
			addFacetDevice(t, db, "R1", []ParamValue{
				pv("hfe", f(10), f(20), nil, nil),
				pv("case", nil, nil, nil, s("A")),
			})
			addFacetDevice(t, db, "R2", []ParamValue{
				pv("hfe", nil, nil, f(15), nil),
				pv("case", nil, nil, nil, s("A")),
			})
			// at_least: только нижняя граница — верхнюю не поднимает.
			addFacetDevice(t, db, "R3", []ParamValue{
				pv("hfe", f(100), nil, nil, nil),
				pv("case", nil, nil, nil, s("B")),
			})
			// Запись другого класса — в агрегаты резисторов не входит.
			addCap := func(designation string, min, max float64) {
				t.Helper()
				ctx := context.Background()
				tx, err := db.Begin(ctx)
				if err != nil {
					t.Fatalf("begin: %v", err)
				}
				id, _, err := tx.InsertDevice(ctx, "capacitor", "gost", designation)
				if err != nil {
					t.Fatalf("запись %q: %v", designation, err)
				}
				if err := tx.InsertValues(ctx, id, nil, []ParamValue{
					{Parameter: "hfe", Min: &min, Max: &max},
				}); err != nil {
					t.Fatalf("значения %q: %v", designation, err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit: %v", err)
				}
			}
			addCap("C1", 1, 2)

			bounds := facetBounds(t, db, "resistor")
			// Текстовый параметр тоже даёт группу агрегата — с nil-границами
			// (отбор по типу значения — сервисный слой, знающий каталог).
			if len(bounds) != 2 {
				t.Fatalf("параметров у резисторов: %d, ожидалось 2: %+v", len(bounds), bounds)
			}
			if c := bounds["case"]; c.Min != nil || c.Max != nil {
				t.Fatalf("границы текстового параметра case: %+v", c)
			}
			b := bounds["hfe"]
			// MIN по нижним границам (10, 15, 100) и MAX по верхним (20, 15):
			// точное значение 15 входит в обе стороны.
			if !eqFloat(b.Min, 10) || !eqFloat(b.Max, 20) {
				t.Fatalf("границы hfe: min=%v max=%v, ожидались 10 и 20", b.Min, b.Max)
			}

			capBounds := facetBounds(t, db, "capacitor")
			if !eqFloat(capBounds["hfe"].Min, 1) || !eqFloat(capBounds["hfe"].Max, 2) {
				t.Fatalf("границы конденсаторов: %+v", capBounds["hfe"])
			}

			// Класс без записей — пустые агрегаты.
			if bounds := facetBounds(t, db, "diode"); len(bounds) != 0 {
				t.Fatalf("границы пустого класса: %+v", bounds)
			}

			freqs := facetFreqs(t, db, "resistor")
			if len(freqs) != 1 {
				t.Fatalf("параметров с текстом: %d, ожидался 1: %+v", len(freqs), freqs)
			}
			cases := freqs["case"]
			if len(cases) != 2 {
				t.Fatalf("значений case: %+v", cases)
			}
			// Порядок — по убыванию частоты, затем по тексту: A (2 записи)
			// раньше B (1); каждая запись считается один раз.
			if cases[0].Text != "A" || cases[0].Devices != 2 ||
				cases[1].Text != "B" || cases[1].Devices != 1 {
				t.Fatalf("частоты case: %+v", cases)
			}
			if freqs := facetFreqs(t, db, "capacitor"); len(freqs) != 0 {
				t.Fatalf("частоты конденсаторов: %+v", freqs)
			}
		})
	}
}
