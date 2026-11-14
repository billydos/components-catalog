package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// Миграции схемы через сервисный слой (план 09): Config.Migrate открывает
// прежнюю версию базы для явного App.Migrate, MigrationPlan строит план
// без записи; состояния меты вне диапазона поддержки не открываются ни
// в каком режиме (база новее модуля — mismatch, старее baseline —
// unsupported). Пока SchemaVersion == SchemaBaseline, «прежней версии»
// в диапазоне нет: путь [Baseline, SchemaVersion) откроется первым же
// шагом реестра (покрыт тестами storage синтетическим шагом).

// downgradeSchemaVersion — порча версии тестовой базы (файл sqlite):
// эмулирует базу другого релиза. Прямой SQL поверх драйвера — сервисный
// слой не открывает чужих DML-путей.
func downgradeSchemaVersion(t *testing.T, dsn, version string) {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close() //nolint:errcheck — тестовое соединение
	if _, err := db.Exec(
		`UPDATE schema_meta SET value = ? WHERE key = 'schema_version'`, version); err != nil {
		t.Fatalf("порча версии: %v", err)
	}
}

func TestMigrateServiceSQLite(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "catalog.db")

	// Штатное открытие создаёт базу текущей версии.
	app, err := Open(ctx, Config{Dialect: "sqlite", DSN: dsn, EnsureCreated: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cases := []struct {
		name string
		code domain.Code
	}{
		{"99", domain.CodeSchemaVersionMismatch},
		{"14", domain.CodeSchemaVersionUnsupported},
	}
	for _, c := range cases {
		t.Run("версия "+c.name, func(t *testing.T) {
			downgradeSchemaVersion(t, dsn, c.name)
			_, err := Open(ctx, Config{Dialect: "sqlite", DSN: dsn})
			de, ok := domain.AsError(err)
			if !ok || de.Code != c.code {
				t.Fatalf("обычное открытие: %v", err)
			}
			_, err = Open(ctx, Config{Dialect: "sqlite", DSN: dsn, Migrate: true})
			de, ok = domain.AsError(err)
			if !ok || de.Code != c.code {
				t.Fatalf("открытие для миграции: %v", err)
			}
		})
	}
	if de := domain.SchemaVersionMismatch(99, storage.SchemaVersion); de.Message !=
		"database schema version 99 does not match module version "+strconv.Itoa(storage.SchemaVersion)+
			"; if the database is older, run migrate; if newer, update the module" {
		t.Fatalf("текст mismatch: %q", de.Message)
	}

	// Актуальная база: Migrate — отчёт «без изменений», MigrationPlan —
	// пустой план; открытие в режиме миграции допустимо.
	downgradeSchemaVersion(t, dsn, strconv.Itoa(storage.SchemaVersion))
	app, err = Open(ctx, Config{Dialect: "sqlite", DSN: dsn, Migrate: true})
	if err != nil {
		t.Fatalf("open migrate: %v", err)
	}
	plan, err := app.MigrationPlan(ctx)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Current != storage.SchemaVersion || len(plan.Steps) != 0 {
		t.Fatalf("план: %+v", plan)
	}
	rep, err := app.Migrate(ctx)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if rep.From != storage.SchemaVersion || rep.To != storage.SchemaVersion || rep.Steps != 0 {
		t.Fatalf("отчёт: %+v", rep)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
