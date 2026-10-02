// Package testutil — общий код интеграционных тестов: доступ к локальному
// PostgreSQL (CATALOG_TEST_POSTGRES_DSN, прогон опционален) и очистка
// одноразовой базы между тестами.
package testutil

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresDSN возвращает DSN локального тестового PostgreSQL либо пропускает
// тест (без переменной окружения прогон PostgreSQL опционален — docs/plan/05
// задача 3.5).
func PostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CATALOG_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("CATALOG_TEST_POSTGRES_DSN не задан — прогон PostgreSQL опущен")
	}
	return dsn
}

// catalogTables — таблицы модуля в порядке, обратном зависимостям
// (docs/plan/02-database.md §2).
var catalogTables = []string{
	"device_analogs",
	"device_manufacturers",
	"manufacturers",
	"parameter_value_conditions",
	"parameter_values",
	"device_variants",
	"device_attribute_values",
	"device_designation_fields",
	"devices",
	"kind_validation_rules",
	"attribute_enum_values",
	"attribute_kinds",
	"attributes",
	"parameter_condition_set_items",
	"parameter_condition_sets",
	"parameter_enum_values",
	"parameter_kinds",
	"parameters",
	"validation_rules",
	"parameter_groups",
	"conditions",
	"units",
	"series_families",
	"designation_system_kinds",
	"designation_systems",
	"kinds",
	"schema_meta",
}

// DropAllTables очищает одноразовую тестовую базу PostgreSQL между прогонами
// (схема создаётся заново EnsureCreated).
func DropAllTables(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("postgres cleanup: %v", err)
	}
	defer db.Close()
	for _, tbl := range catalogTables {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + tbl + " CASCADE"); err != nil {
			t.Fatalf("postgres cleanup %s: %v", tbl, err)
		}
	}
}
