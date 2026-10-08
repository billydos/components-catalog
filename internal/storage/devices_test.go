package storage

import (
	"context"
	"path/filepath"
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
