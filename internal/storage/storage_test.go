package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/testutil"
)

func openSQLiteFile(t *testing.T) (string, *DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(context.Background(), Config{Dialect: "sqlite", DSN: path})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return path, db
}

// Не инициализированная база (файл без таблиц) — database_not_initialized
// с дословным текстом контракта.
func TestCheckSchemaNotInitialized(t *testing.T) {
	_, db := openSQLiteFile(t)
	err := db.CheckSchema(context.Background())
	de, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("ожидалась *domain.Error, получено %v", err)
	}
	if de.Code != domain.CodeDatabaseNotInitialized {
		t.Fatalf("код: %s", de.Code)
	}
	want := "база данных не инициализирована или не является базой модуля; выполните init (CLI) или EnsureCreated"
	if de.Message != want {
		t.Fatalf("текст: %q, ожидался %q", de.Message, want)
	}
}

func TestEnsureCreatedSQLiteFile(t *testing.T) {
	_, db := openSQLiteFile(t)
	ctx := context.Background()

	created, err := db.EnsureCreated(ctx)
	if err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	if !created {
		t.Fatal("первый вызов должен создавать базу")
	}
	if err := db.CheckSchema(ctx); err != nil {
		t.Fatalf("CheckSchema: %v", err)
	}
	// Повторный вызов — без создания (идемпотентность, гонка создания).
	created, err = db.EnsureCreated(ctx)
	if err != nil {
		t.Fatalf("EnsureCreated повторно: %v", err)
	}
	if created {
		t.Fatal("повторный вызов не должен создавать базу")
	}

	cat, data, err := db.Revisions(ctx)
	if err != nil {
		t.Fatalf("Revisions: %v", err)
	}
	if cat != 1 || data != 1 {
		t.Fatalf("ревизии после создания: catalog=%d data=%d, ожидались 1 и 1", cat, data)
	}

	// Инкремент ревизий — только внутри транзакции записи.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.BumpDataRevision(ctx); err != nil {
		t.Fatalf("bump: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, data, err = db.Revisions(ctx); err != nil || data != 2 {
		t.Fatalf("data_revision после инкремента: %d (err %v), ожидалась 2", data, err)
	}
}

// Несовпадение версии схемы — schema_version_mismatch с дословным текстом
// (тихая порча данных исключена — D5).
func TestSchemaVersionMismatch(t *testing.T) {
	path, db := openSQLiteFile(t)
	ctx := context.Background()
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.exec(ctx,
		`UPDATE schema_meta SET value = '99' WHERE key = 'schema_version'`, nil); err != nil {
		t.Fatalf("порча версии: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	err = db.CheckSchema(ctx)
	de, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("ожидалась *domain.Error, получено %v", err)
	}
	if de.Code != domain.CodeSchemaVersionMismatch {
		t.Fatalf("код: %s", de.Code)
	}
	want := domain.SchemaVersionMismatch(99, SchemaVersion).Message
	if de.Message != want {
		t.Fatalf("текст: %q, ожидался %q", de.Message, want)
	}

	// Переоткрытие той же базы — тот же отказ.
	db.Close()
	db2, err := Open(ctx, Config{Dialect: "sqlite", DSN: path})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	err = db2.CheckSchema(ctx)
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeSchemaVersionMismatch {
		t.Fatalf("после переоткрытия: %v", err)
	}
}

func TestEnsureCreatedSQLiteMemory(t *testing.T) {
	db, err := Open(context.Background(), Config{Dialect: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	defer db.Close()
	created, err := db.EnsureCreated(context.Background())
	if err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	if !created {
		t.Fatal("ожидано создание базы в памяти")
	}
	if err := db.CheckSchema(context.Background()); err != nil {
		t.Fatalf("CheckSchema: %v", err)
	}
}

// PostgreSQL — тот же набор проверок; локальный прогон опционален
// (CATALOG_TEST_POSTGRES_DSN, база должна быть одноразовой).
func TestPostgres(t *testing.T) {
	dsn := testutil.PostgresDSN(t)
	ctx := context.Background()
	testutil.DropAllTables(t, dsn)

	db, err := Open(ctx, Config{Dialect: "postgres", DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	err = db.CheckSchema(ctx)
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDatabaseNotInitialized {
		t.Fatalf("CheckSchema пустой базы: %v", err)
	}
	created, err := db.EnsureCreated(ctx)
	if err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	if !created {
		t.Fatal("ожидано создание схемы")
	}
	if err := db.CheckSchema(ctx); err != nil {
		t.Fatalf("CheckSchema: %v", err)
	}
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated повторно: %v", err)
	}
	cat, data, err := db.Revisions(ctx)
	if err != nil || cat != 1 || data != 1 {
		t.Fatalf("ревизии: catalog=%d data=%d err=%v", cat, data, err)
	}
}
