package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/testutil"
)

// openSnapshotDB открывает тестовую базу с одним классом и базовой порцией
// каталога. SQLite — файл (разделённые пулы чтения и записи — возможна
// гонка читателя с писателем); PostgreSQL — по CATALOG_TEST_POSTGRES_DSN
// (без переменной нога пропускается).
func openSnapshotDB(t *testing.T, dialect string) *DB {
	t.Helper()
	ctx := context.Background()
	var dsn string
	switch dialect {
	case "sqlite":
		dsn = filepath.Join(t.TempDir(), "snap.db")
	case "postgres":
		dsn = testutil.PostgresDSN(t)
		testutil.DropAllTables(t, dsn)
	default:
		t.Fatalf("неизвестный диалект %q", dialect)
	}
	db, err := Open(ctx, Config{Dialect: dialect, DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.exec(ctx, `INSERT INTO kinds(code) VALUES ('resistor')`, nil); err != nil {
		t.Fatalf("kinds: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := writeCatalogChunk(ctx, db, 0); err != nil {
		t.Fatalf("базовая порция: %v", err)
	}
	return db
}

// insertCatalogChunk вставляет порцию расширения каталога и инкрементирует
// catalog_revision внутри переданной транзакции. Порция добавляет условие,
// группу и параметр, который на них ссылается: снимок, смешавший состояния
// до и после порции, нарушает ссылки или равенство количеств.
func insertCatalogChunk(ctx context.Context, tx *Tx, n int) error {
	cond := fmt.Sprintf("cond%d", n)
	group := fmt.Sprintf("group%d", n)
	param := fmt.Sprintf("param%d", n)
	for _, ins := range []struct {
		q    string
		args map[string]any
	}{
		{`INSERT INTO conditions(code, unit_code, allow_negative) VALUES (@code, NULL, 0)`,
			map[string]any{"code": cond}},
		{`INSERT INTO parameter_groups(code, section_name, sort_order) VALUES (@code, @section, @sort)`,
			map[string]any{"code": group, "section": fmt.Sprintf("section%d", n), "sort": n}},
		{`INSERT INTO parameters(code, group_code, unit_code, value_type, sort_order, is_active)
VALUES (@code, @grp, NULL, 'exact', @sort, 1)`,
			map[string]any{"code": param, "grp": group, "sort": n}},
		{`INSERT INTO parameter_kinds(parameter_code, kind_code) VALUES (@param, 'resistor')`,
			map[string]any{"param": param}},
		{`INSERT INTO parameter_condition_sets(parameter_code, set_no) VALUES (@param, 1)`,
			map[string]any{"param": param}},
		{`INSERT INTO parameter_condition_set_items(parameter_code, set_no, condition_code, mode, fixed_value)
VALUES (@param, 1, @cond, 'required', NULL)`,
			map[string]any{"param": param, "cond": cond}},
	} {
		if _, err := tx.exec(ctx, ins.q, ins.args); err != nil {
			return err
		}
	}
	return tx.BumpCatalogRevision(ctx)
}

// writeCatalogChunk применяет порцию каталога атомарно — по образцу
// CatalogService.Import: одна транзакция записи с инкрементом ревизии.
func writeCatalogChunk(ctx context.Context, db *DB, n int) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	if err := insertCatalogChunk(ctx, tx, n); err != nil {
		return err
	}
	return tx.Commit()
}

func snapshotConditionCodes(snap *catalog.Snapshot) map[string]bool {
	out := make(map[string]bool, len(snap.Conditions))
	for _, c := range snap.Conditions {
		out[c.Code] = true
	}
	return out
}

func snapshotGroupCodes(snap *catalog.Snapshot) map[string]bool {
	out := make(map[string]bool, len(snap.Groups))
	for _, g := range snap.Groups {
		out[g.Code] = true
	}
	return out
}

func snapshotParameterCodes(snap *catalog.Snapshot) map[string]bool {
	out := make(map[string]bool, len(snap.Parameters))
	for _, p := range snap.Parameters {
		out[p.Code] = true
	}
	return out
}

// assertSnapshotSelfConsistent проверяет внутреннюю согласованность снимка:
// ссылки параметров существуют в этом же снимке, количества строк порций
// согласованы. Порции каталога атомарны, поэтому смешанный снимок нарушает
// хотя бы одно из этих условий.
func assertSnapshotSelfConsistent(t *testing.T, snap *catalog.Snapshot) {
	t.Helper()
	conds := snapshotConditionCodes(snap)
	groups := snapshotGroupCodes(snap)
	for _, p := range snap.Parameters {
		if !groups[p.Group] {
			t.Fatalf("снимок смешал состояния: параметр %s ссылается на группу %s, отсутствующую в этом же снимке",
				p.Code, p.Group)
		}
		for _, cs := range p.ConditionSets {
			for _, it := range cs.Items {
				if !conds[it.Condition] {
					t.Fatalf("снимок смешал состояния: параметр %s ссылается на условие %s, отсутствующее в этом же снимке",
						p.Code, it.Condition)
				}
			}
		}
	}
	if len(snap.Parameters) != len(snap.Conditions) || len(snap.Parameters) != len(snap.Groups) {
		t.Fatalf("снимок смешал состояния: параметров %d, условий %d, групп %d — порции атомарны, числа должны совпадать",
			len(snap.Parameters), len(snap.Conditions), len(snap.Groups))
	}
}

// LoadSnapshot читает определения и catalog_revision одним согласованным
// чтением: незафиксированная транзакция записи невидима, а ревизия снимка
// соответствует загруженным строкам.
func TestLoadSnapshotExcludesUncommittedWrite(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openSnapshotDB(t, dialect)
			ctx := context.Background()

			rev0, err := db.CatalogRevision(ctx)
			if err != nil {
				t.Fatalf("CatalogRevision: %v", err)
			}

			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
			if err := insertCatalogChunk(ctx, tx, 1); err != nil {
				t.Fatalf("порция без фиксации: %v", err)
			}

			snap, err := db.LoadSnapshot(ctx)
			if err != nil {
				t.Fatalf("LoadSnapshot при открытой транзакции записи: %v", err)
			}
			for code, present := range map[string]bool{
				"cond1":  snapshotConditionCodes(snap)["cond1"],
				"group1": snapshotGroupCodes(snap)["group1"],
				"param1": snapshotParameterCodes(snap)["param1"],
			} {
				if present {
					t.Fatalf("незафиксированное определение %s видно в снимке", code)
				}
			}
			if snap.Revision != rev0 {
				t.Fatalf("ревизия снимка при открытой транзакции записи: %d, ожидалась %d",
					snap.Revision, rev0)
			}

			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
			snap, err = db.LoadSnapshot(ctx)
			if err != nil {
				t.Fatalf("LoadSnapshot после фиксации: %v", err)
			}
			for code, present := range map[string]bool{
				"cond1":  snapshotConditionCodes(snap)["cond1"],
				"group1": snapshotGroupCodes(snap)["group1"],
				"param1": snapshotParameterCodes(snap)["param1"],
			} {
				if !present {
					t.Fatalf("зафиксированное определение %s отсутствует в снимке", code)
				}
			}
			if snap.Revision != rev0+1 {
				t.Fatalf("ревизия снимка после фиксации: %d, ожидалась %d", snap.Revision, rev0+1)
			}
			for _, p := range snap.Parameters {
				if p.Code == "param1" {
					if len(p.ConditionSets) != 1 || len(p.ConditionSets[0].Items) != 1 ||
						p.ConditionSets[0].Items[0].Condition != "cond1" {
						t.Fatalf("набор условий param1: %+v", p.ConditionSets)
					}
				}
			}
		})
	}
}

// Гонка «импорт каталога параллельно загрузке снимка»: порции атомарны,
// поэтому любой снимок целиком до или после каждой порции — ссылки целы,
// количества согласованы, ревизия монотонна; итоговый снимок содержит все
// порции, и его ревизия в точности равна числу применённых транзакций.
func TestLoadSnapshotConsistentDuringConcurrentImports(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openSnapshotDB(t, dialect)
			ctx := context.Background()

			const chunks = 100
			written := make(chan error, 1)
			go func() {
				for i := 1; i <= chunks; i++ {
					if err := writeCatalogChunk(ctx, db, i); err != nil {
						written <- fmt.Errorf("порция %d: %w", i, err)
						return
					}
				}
				written <- nil
			}()

			var prev int64
			deadline := time.After(30 * time.Second)
			for {
				select {
				case err := <-written:
					if err != nil {
						t.Fatal(err)
					}
					snap, err := db.LoadSnapshot(ctx)
					if err != nil {
						t.Fatalf("LoadSnapshot: %v", err)
					}
					assertSnapshotSelfConsistent(t, snap)
					if want := chunks + 1; len(snap.Parameters) != want {
						t.Fatalf("итоговых параметров %d, ожидалось %d", len(snap.Parameters), want)
					}
					// 1 (создание базы) + 1 (базовая порция) + chunks порций.
					if want := int64(chunks + 2); snap.Revision != want {
						t.Fatalf("итоговая ревизия %d, ожидалась %d", snap.Revision, want)
					}
					return
				case <-deadline:
					t.Fatal("таймаут: писатель не завершился")
				default:
				}
				snap, err := db.LoadSnapshot(ctx)
				if err != nil {
					t.Fatalf("LoadSnapshot: %v", err)
				}
				assertSnapshotSelfConsistent(t, snap)
				if snap.Revision < prev {
					t.Fatalf("ревизия снимка убывает: %d после %d", snap.Revision, prev)
				}
				prev = snap.Revision
			}
		})
	}
}
