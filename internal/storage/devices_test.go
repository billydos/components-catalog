package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/testutil"
)

// openDevicesDB открывает тестовую базу с сидами классов и систем
// обозначений (FK устройств). PostgreSQL — по CATALOG_TEST_POSTGRES_DSN
// (без переменной нога пропускается).
func openDevicesDB(t *testing.T, dialect string) *DB {
	t.Helper()
	ctx := context.Background()
	var dsn string
	switch dialect {
	case "sqlite":
		dsn = filepath.Join(t.TempDir(), "devices.db")
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
	for _, q := range []string{
		`INSERT INTO kinds(code) VALUES ('resistor')`,
		`INSERT INTO designation_systems(code) VALUES ('gost')`,
	} {
		if _, err := tx.exec(ctx, q, nil); err != nil {
			t.Fatalf("сид %q: %v", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return db
}

// Вставка существующего обозначения подавляется: тот же id, created=false.
func TestInsertDeviceIfAbsent(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			ctx := context.Background()

			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			id1, created, err := tx.InsertDevice(ctx, "resistor", "gost", "КТ315")
			if err != nil {
				t.Fatalf("первая вставка: %v", err)
			}
			if !created {
				t.Fatal("первая вставка должна создавать запись")
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}

			tx, err = db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			id2, created, err := tx.InsertDevice(ctx, "resistor", "gost", "КТ315")
			if err != nil {
				t.Fatalf("повторная вставка: %v", err)
			}
			if created {
				t.Fatal("повторная вставка должна быть подавлена")
			}
			if id2 != id1 {
				t.Fatalf("id повторной вставки %d, ожидался %d", id2, id1)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
		})
	}
}

// Повторная замена производителей тем же списком: id перечитываются,
// дубликатов строк и сирот нет.
func TestReplaceManufacturersIdempotent(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			ctx := context.Background()

			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			id, _, err := tx.InsertDevice(ctx, "resistor", "gost", "КТ315")
			if err != nil {
				t.Fatalf("вставка записи: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}

			for pass := 1; pass <= 2; pass++ {
				tx, err = db.Begin(ctx)
				if err != nil {
					t.Fatalf("begin: %v", err)
				}
				if err := tx.ReplaceManufacturers(ctx, id, []string{"МЗПП", "Ореол"}); err != nil {
					t.Fatalf("замена %d: %v", pass, err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit: %v", err)
				}
				if n, err := db.CountManufacturers(ctx); err != nil || n != 2 {
					t.Fatalf("производителей после замены %d (err %v), ожидалось 2", n, err)
				}
			}

			tx, err = db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			if err := tx.ReplaceManufacturers(ctx, id, []string{"МЗПП"}); err != nil {
				t.Fatalf("усечение списка: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
			if n, err := db.CountManufacturers(ctx); err != nil || n != 1 {
				t.Fatalf("производителей после усечения %d (err %v), ожидался 1 (сирота удалён)", n, err)
			}
			names, err := db.LoadManufacturers(ctx, id)
			if err != nil {
				t.Fatalf("чтение производителей: %v", err)
			}
			if len(names) != 1 || names[0] != "МЗПП" {
				t.Fatalf("производители записи: %v, ожидался [МЗПП]", names)
			}
		})
	}
}

// TestDeviceWriteRacePostgres воспроизводит гонку параллельных писателей:
// конкурирующая транзакция держит незафиксированную строку, пока вставка
// этой заблокирована на уникальном индексе; после фиксации конкурента
// конфликт подавляется, id и строки перечитываются — ошибки уникальности
// наружу не выходят. Требует CATALOG_TEST_POSTGRES_DSN; у sqlite писатель
// один (одно выделенное соединение записи) — гонка там невозможна.
func TestDeviceWriteRacePostgres(t *testing.T) {
	db := openDevicesDB(t, "postgres")
	ctx := context.Background()
	fields := []domain.Field{domain.TextField("group", "1")}

	// Запись: A вставляет устройство и поля, держит транзакцию; B вставляет
	// то же обозначение — блокируется на UNIQUE(kind_code, designation);
	// A фиксирует — вставка B подавлена, id перечитан, поля уже есть.
	txA, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin A: %v", err)
	}
	defer txA.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	idA, created, err := txA.InsertDevice(ctx, "resistor", "gost", "КТ315")
	if err != nil {
		t.Fatalf("вставка A: %v", err)
	}
	if !created {
		t.Fatal("вставка A должна создавать запись")
	}
	if err := txA.InsertDesignationFields(ctx, idA, fields); err != nil {
		t.Fatalf("поля A: %v", err)
	}

	type insertResult struct {
		id      int64
		created bool
		err     error
	}
	ch := make(chan insertResult, 1)
	go func() {
		txB, err := db.Begin(ctx)
		if err != nil {
			ch <- insertResult{err: err}
			return
		}
		defer txB.Rollback() //nolint:errcheck — откат нефиксированной транзакции
		id, created, err := txB.InsertDevice(ctx, "resistor", "gost", "КТ315")
		if err != nil {
			ch <- insertResult{err: err}
			return
		}
		if err := txB.InsertDesignationFields(ctx, id, fields); err != nil {
			ch <- insertResult{err: err}
			return
		}
		ch <- insertResult{id: id, created: created, err: txB.Commit()}
	}()
	time.Sleep(200 * time.Millisecond) // B успевает заблокироваться на индексе
	if err := txA.Commit(); err != nil {
		t.Fatalf("commit A: %v", err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatalf("проигравший гонку писатель: %v", r.err)
	}
	if r.created {
		t.Fatal("вставка проигравшего должна быть подавлена")
	}
	if r.id != idA {
		t.Fatalf("id проигравшего %d, ожидался id победителя %d", r.id, idA)
	}
	got, err := db.LoadFields(ctx, idA)
	if err != nil || len(got) != 1 || got[0].Name != "group" {
		t.Fatalf("поля записи после гонки: %v (err %v), ожидалось одно поле group", got, err)
	}

	// Производитель: A пересоздаёт список с новым именем, держит транзакцию;
	// B заменяет тот же список той же записи — создание строки производителя
	// подавлено, id перечитан, связь и чистка сирот корректны.
	txA2, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin A2: %v", err)
	}
	defer txA2.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	if err := txA2.ReplaceManufacturers(ctx, idA, []string{"МЗПП"}); err != nil {
		t.Fatalf("производители A2: %v", err)
	}
	ch2 := make(chan error, 1)
	go func() {
		txB2, err := db.Begin(ctx)
		if err != nil {
			ch2 <- err
			return
		}
		defer txB2.Rollback() //nolint:errcheck — откат нефиксированной транзакции
		if err := txB2.ReplaceManufacturers(ctx, idA, []string{"МЗПП"}); err != nil {
			ch2 <- err
			return
		}
		ch2 <- txB2.Commit()
	}()
	time.Sleep(200 * time.Millisecond)
	if err := txA2.Commit(); err != nil {
		t.Fatalf("commit A2: %v", err)
	}
	if err := <-ch2; err != nil {
		t.Fatalf("проигравший гонку производителей: %v", err)
	}
	if n, err := db.CountManufacturers(ctx); err != nil || n != 1 {
		t.Fatalf("производителей после гонки %d (err %v), ожидался 1", n, err)
	}
	names, err := db.LoadManufacturers(ctx, idA)
	if err != nil || len(names) != 1 || names[0] != "МЗПП" {
		t.Fatalf("производители записи после гонки: %v (err %v)", names, err)
	}
}

// TestConcurrentDeviceWriters — параллельные писатели создают одну и ту же
// запись с одним производителем и теми же полями: конфликты подавляются,
// база сходится к одной записи. SQLite — писатель один (прогон тривиально
// сходится), PostgreSQL — реальная конкуренция соединений пула.
func TestConcurrentDeviceWriters(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			ctx := context.Background()
			const writers = 8
			fields := []domain.Field{domain.TextField("group", "1")}

			type result struct {
				id      int64
				created bool
				err     error
			}
			results := make([]result, writers)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range writers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					tx, err := db.Begin(ctx)
					if err != nil {
						results[i].err = err
						return
					}
					defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
					id, created, err := tx.InsertDevice(ctx, "resistor", "gost", "КТ315")
					if err != nil {
						results[i].err = err
						return
					}
					if err := tx.InsertDesignationFields(ctx, id, fields); err != nil {
						results[i].err = err
						return
					}
					if err := tx.ReplaceManufacturers(ctx, id, []string{"МЗПП"}); err != nil {
						results[i].err = err
						return
					}
					results[i] = result{id: id, created: created, err: tx.Commit()}
				}()
			}
			close(start)
			wg.Wait()

			created := 0
			for i, r := range results {
				if r.err != nil {
					t.Fatalf("писатель %d: %v", i, r.err)
				}
				if r.id != results[0].id {
					t.Fatalf("писатель %d: id %d, ожидался %d", i, r.id, results[0].id)
				}
				if r.created {
					created++
				}
			}
			if created != 1 {
				t.Fatalf("создавших запись писателей %d, ожидался 1", created)
			}
			if n, err := db.CountDevices(ctx, "resistor"); err != nil || n != 1 {
				t.Fatalf("записей после гонки %d (err %v), ожидалась 1", n, err)
			}
			if n, err := db.CountManufacturers(ctx); err != nil || n != 1 {
				t.Fatalf("производителей после гонки %d (err %v), ожидался 1", n, err)
			}
			names, err := db.LoadManufacturers(ctx, results[0].id)
			if err != nil || len(names) != 1 || names[0] != "МЗПП" {
				t.Fatalf("производители записи: %v (err %v)", names, err)
			}
			got, err := db.LoadFields(ctx, results[0].id)
			if err != nil || len(got) != 1 || got[0].Name != "group" {
				t.Fatalf("поля записи: %v (err %v), ожидалось одно поле group", got, err)
			}
		})
	}
}

// TestBatchDeviceReads — пакетные читатели карточек возвращают те же
// данные, что и одиночные загрузчики, и пропускают отсутствующие id;
// многострочная InsertValues сохраняет порядок и условия значений (в том
// числе за границей чанка). Обе СУБД.
func TestBatchDeviceReads(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			ctx := context.Background()

			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			for _, q := range []string{
				`INSERT INTO parameter_groups(code, section_name, sort_order) VALUES ('grp', 'params', 0)`,
				`INSERT INTO parameters(code, group_code, value_type, sort_order) VALUES ('vp', 'grp', 'exact', 0), ('vt', 'grp', 'text', 1)`,
				`INSERT INTO conditions(code) VALUES ('temp')`,
				`INSERT INTO attributes(code, value_type, sort_order) VALUES ('a1', 'number', 0)`,
			} {
				if _, err := tx.exec(ctx, q, nil); err != nil {
					t.Fatalf("сид %q: %v", q, err)
				}
			}
			idA, _, err := tx.InsertDevice(ctx, "resistor", "gost", "DEV-A")
			if err != nil {
				t.Fatalf("вставка A: %v", err)
			}
			idB, _, err := tx.InsertDevice(ctx, "resistor", "gost", "DEV-B")
			if err != nil {
				t.Fatalf("вставка B: %v", err)
			}
			if err := tx.InsertDesignationFields(ctx, idA, []domain.Field{
				domain.TextField("group", "1"), domain.NumField("dev_number", 42),
			}); err != nil {
				t.Fatalf("поля A: %v", err)
			}
			a1 := 3.5
			if err := tx.ReplaceAttributes(ctx, idA, []AttrRow{{Attribute: "a1", Num: &a1}}); err != nil {
				t.Fatalf("атрибуты A: %v", err)
			}
			exact1, exact2, exact3 := 1.5, 2.5, 3.5
			textVal := "abc"
			if err := tx.InsertValues(ctx, idA, nil, []ParamValue{
				{Parameter: "vp", Exact: &exact1, Conditions: []Cond{{Code: "temp", Value: 25}}},
				{Parameter: "vt", Text: &textVal},
			}); err != nil {
				t.Fatalf("значения типа A: %v", err)
			}
			vid1, err := tx.InsertVariant(ctx, idA, "5%", 0)
			if err != nil {
				t.Fatalf("исполнение 1: %v", err)
			}
			if err := tx.InsertValues(ctx, idA, &vid1, []ParamValue{{Parameter: "vp", Exact: &exact2}}); err != nil {
				t.Fatalf("значения исполнения 1: %v", err)
			}
			vid2, err := tx.InsertVariant(ctx, idA, "10%", 1)
			if err != nil {
				t.Fatalf("исполнение 2: %v", err)
			}
			if err := tx.InsertValues(ctx, idA, &vid2, []ParamValue{
				{Parameter: "vp", Exact: &exact3, Conditions: []Cond{{Code: "temp", Value: 125}}},
			}); err != nil {
				t.Fatalf("значения исполнения 2: %v", err)
			}
			if err := tx.ReplaceManufacturers(ctx, idA, []string{"Ореол", "МЗПП"}); err != nil {
				t.Fatalf("производители A: %v", err)
			}
			if err := tx.ReplaceAnalogs(ctx, idA, []AnalogRow{
				{TargetID: idB, Designation: "DEV-B", System: "gost", Note: "замена"},
			}); err != nil {
				t.Fatalf("аналоги A: %v", err)
			}
			if err := tx.ReplaceAnalogs(ctx, idB, []AnalogRow{
				{TargetID: idA, Designation: "DEV-A", System: "gost"},
			}); err != nil {
				t.Fatalf("аналоги B: %v", err)
			}
			// 120 значений без условий — два чанка многострочной вставки;
			// порядок и sort_order обязаны сохраниться.
			idC, _, err := tx.InsertDevice(ctx, "resistor", "gost", "DEV-C")
			if err != nil {
				t.Fatalf("вставка C: %v", err)
			}
			bulk := make([]ParamValue, 120)
			for i := range bulk {
				v := float64(i)
				bulk[i] = ParamValue{Parameter: "vp", Exact: &v}
			}
			if err := tx.InsertValues(ctx, idC, nil, bulk); err != nil {
				t.Fatalf("значения C: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}

			const missing = 1 << 40
			devs, err := db.FindDevicesByIDs(ctx, []int64{idB, missing, idA})
			if err != nil {
				t.Fatalf("записи по id: %v", err)
			}
			if len(devs) != 2 || devs[0].ID != idA || devs[1].ID != idB {
				t.Fatalf("записи по id: %+v, ожидались A и B по порядку id", devs)
			}

			fieldsBy, err := db.LoadFieldsByIDs(ctx, []int64{idA, idB, missing})
			if err != nil {
				t.Fatalf("поля по id: %v", err)
			}
			attrsBy, err := db.LoadAttributesByIDs(ctx, []int64{idA, idB})
			if err != nil {
				t.Fatalf("атрибуты по id: %v", err)
			}
			valuesBy, err := db.LoadValuesByIDs(ctx, []int64{idA, idC})
			if err != nil {
				t.Fatalf("значения по id: %v", err)
			}
			variantsBy, err := db.LoadVariantsByIDs(ctx, []int64{idA, idB})
			if err != nil {
				t.Fatalf("исполнения по id: %v", err)
			}
			manufsBy, err := db.LoadManufacturersByIDs(ctx, []int64{idA, idB})
			if err != nil {
				t.Fatalf("производители по id: %v", err)
			}
			analogsBy, err := db.LoadOutgoingAnalogsByIDs(ctx, []int64{idA, idB})
			if err != nil {
				t.Fatalf("аналоги по id: %v", err)
			}
			backBy, err := db.LoadBacklinksByIDs(ctx, []int64{idA, idB})
			if err != nil {
				t.Fatalf("встречные ссылки по id: %v", err)
			}

			// Одиночные загрузчики — эталон содержимого (чтение в транзакции).
			rtx, err := db.Begin(ctx)
			if err != nil {
				t.Fatalf("begin чтения: %v", err)
			}
			defer rtx.Rollback() //nolint:errcheck — чтение без побочных эффектов
			for _, id := range []int64{idA, idB} {
				wantFields, err := rtx.LoadFields(ctx, id)
				if err != nil {
					t.Fatalf("поля записи %d: %v", id, err)
				}
				if got := fieldsBy[id]; !slices.Equal(got, wantFields) {
					t.Fatalf("поля записи %d: %+v, ожидалось %+v", id, got, wantFields)
				}
				wantAttrs, err := rtx.LoadAttributes(ctx, id)
				if err != nil {
					t.Fatalf("атрибуты записи %d: %v", id, err)
				}
				if got := attrsBy[id]; !reflect.DeepEqual(got, wantAttrs) {
					t.Fatalf("атрибуты записи %d: %+v, ожидалось %+v", id, got, wantAttrs)
				}
				wantValues, err := rtx.LoadValues(ctx, id)
				if err != nil {
					t.Fatalf("значения записи %d: %v", id, err)
				}
				if got := valuesBy[id]; !reflect.DeepEqual(got, wantValues) {
					t.Fatalf("значения записи %d: %+v, ожидалось %+v", id, got, wantValues)
				}
				wantVariants, err := rtx.LoadVariants(ctx, id)
				if err != nil {
					t.Fatalf("исполнения записи %d: %v", id, err)
				}
				if got := variantsBy[id]; !slices.Equal(got, wantVariants) {
					t.Fatalf("исполнения записи %d: %+v, ожидалось %+v", id, got, wantVariants)
				}
				wantManufs, err := rtx.LoadManufacturers(ctx, id)
				if err != nil {
					t.Fatalf("производители записи %d: %v", id, err)
				}
				if got := manufsBy[id]; !slices.Equal(got, wantManufs) {
					t.Fatalf("производители записи %d: %+v, ожидалось %+v", id, got, wantManufs)
				}
				wantAnalogs, err := rtx.LoadOutgoingAnalogs(ctx, id)
				if err != nil {
					t.Fatalf("аналоги записи %d: %v", id, err)
				}
				if got := analogsBy[id]; !slices.Equal(got, wantAnalogs) {
					t.Fatalf("аналоги записи %d: %+v, ожидалось %+v", id, got, wantAnalogs)
				}
			}
			wantBackA := []BacklinkRow{{SourceID: idB, Kind: "resistor", Designation: "DEV-B", System: "gost"}}
			if got := backBy[idA]; !slices.Equal(got, wantBackA) {
				t.Fatalf("встречные ссылки A: %+v, ожидалось %+v", got, wantBackA)
			}
			wantBackB := []BacklinkRow{{SourceID: idA, Kind: "resistor", Designation: "DEV-A", System: "gost", Note: "замена"}}
			if got := backBy[idB]; !slices.Equal(got, wantBackB) {
				t.Fatalf("встречные ссылки B: %+v, ожидалось %+v", got, wantBackB)
			}
			if got := valuesBy[idC]; len(got) != len(bulk) {
				t.Fatalf("значения записи C: %d строк, ожидалось %d", len(got), len(bulk))
			}
			for i, vr := range valuesBy[idC] {
				if vr.VariantID != nil || vr.Value.Parameter != "vp" || *vr.Value.Exact != float64(i) {
					t.Fatalf("значение %d записи C: %+v — порядок либо sort_order нарушены", i, vr.Value)
				}
			}
		})
	}
}
