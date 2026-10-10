// Package testutil — общий код интеграционных тестов: доступ к локальному
// PostgreSQL (CATALOG_TEST_POSTGRES_DSN, прогон опционален) и очистка
// одноразовой базы между тестами.
package testutil

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// dropTimeout — бюджет одного DROP очистки: висячий сервер не держит
// тест до внешнего таймаута прогона.
const dropTimeout = 30 * time.Second

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

// catalogTables — таблицы модуля для очистки одноразовой тестовой базы
// PostgreSQL между прогонами (схема создаётся заново EnsureCreated).
// Список — копия storage.TableNames() (выводится из DDL): импорт storage
// здесь запрещён (тесты storage импортируют testutil — цикл), синхронность
// с DDL закреплена пин-тестом tables_test.go.
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
	"categories",
	"series_families",
	"designation_system_kinds",
	"designation_systems",
	"kinds",
	"schema_meta",
}

// DropAllTables очищает одноразовую тестовую базу PostgreSQL между прогонами
// (схема создаётся заново EnsureCreated). DROP идёт с таймаутом на каждый
// оператор, текст ошибки маскирует DSN — сообщение драйвера может включать
// строку подключения целиком.
func DropAllTables(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("postgres cleanup: %v", maskDSNIn(err.Error(), dsn))
	}
	defer db.Close()
	for _, tbl := range catalogTables {
		ctx, cancel := context.WithTimeout(context.Background(), dropTimeout)
		_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
		cancel()
		if err != nil {
			t.Fatalf("postgres cleanup %s: %v", tbl, maskDSNIn(err.Error(), dsn))
		}
	}
}

// maskDSNIn заменяет вхождения DSN замаскированной формой.
func maskDSNIn(msg, dsn string) string {
	if dsn == "" {
		return msg
	}
	return strings.ReplaceAll(msg, dsn, maskDSN(dsn))
}
