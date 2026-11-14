package storage

import (
	"context"
	"fmt"
	"strconv"

	"github.com/billydos/components-catalog/internal/domain"
)

// ddlFixtures — замороженные срезы DDL прошлых версий схемы (план 09
// §9): источник построения баз-фикстур для проверок эквивалентности
// «свежая ≡ мигрированная» (тесты storage) и фазы migrate qa-прогонщика
// (нога postgres). Срез — дословная копия ddlStatements соответствующей
// версии на момент её релиза: правка замороженного среза — правка
// истории; новая версия схемы добавляется новым ключом, существующие
// не меняются. В производственный путь не входит.
var ddlFixtures = map[int][]string{
	SchemaBaseline: ddlStatementsV15,
}

// ddlStatementsV15 — DDL версии 15 (инвариант D11 на уровне БД: kind_code
// и UNIQUE (kind_code, designation) в device_synonyms). Заморожен срезом
// ddlStatements этой версии; живой ddlStatements дальше эволюционирует
// независимо.
var ddlStatementsV15 = []string{
	`CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS kinds (
    code TEXT PRIMARY KEY
)`,

	`CREATE TABLE IF NOT EXISTS designation_systems (
    code TEXT PRIMARY KEY
)`,

	`CREATE TABLE IF NOT EXISTS designation_system_kinds (
    system_code TEXT NOT NULL REFERENCES designation_systems(code) ON DELETE CASCADE,
    kind_code   TEXT NOT NULL REFERENCES kinds(code) ON DELETE CASCADE,
    PRIMARY KEY (system_code, kind_code)
)`,

	`CREATE TABLE IF NOT EXISTS units (
    code TEXT PRIMARY KEY
)`,

	`CREATE TABLE IF NOT EXISTS categories (
    code TEXT PRIMARY KEY
)`,

	// Словарь типов источников данных записи (план 03 §13): данные
	// каталога, отображаемые названия — бандлы sourcetype.<код> (D9).
	`CREATE TABLE IF NOT EXISTS source_types (
    code TEXT PRIMARY KEY
)`,

	`CREATE TABLE IF NOT EXISTS conditions (
    code           TEXT PRIMARY KEY,
    unit_code      TEXT NULL REFERENCES units(code),
    allow_negative INTEGER NOT NULL DEFAULT 0
)`,

	`CREATE TABLE IF NOT EXISTS parameter_groups (
    code         TEXT PRIMARY KEY,
    section_name TEXT NOT NULL,
    sort_order   INTEGER NOT NULL
)`,

	`CREATE TABLE IF NOT EXISTS validation_rules (
    code TEXT PRIMARY KEY
)`,

	`CREATE TABLE IF NOT EXISTS parameters (
    code            TEXT PRIMARY KEY,
    group_code      TEXT NOT NULL REFERENCES parameter_groups(code),
    unit_code       TEXT NULL REFERENCES units(code),
    value_type      TEXT NOT NULL,
    value_ceiling   REAL NULL,
    allow_negative  INTEGER NOT NULL DEFAULT 0,
    validation_rule TEXT NULL REFERENCES validation_rules(code),
    sort_order      INTEGER NOT NULL,
    is_active       INTEGER NOT NULL DEFAULT 1
)`,

	`CREATE TABLE IF NOT EXISTS parameter_kinds (
    parameter_code TEXT NOT NULL REFERENCES parameters(code) ON DELETE CASCADE,
    kind_code      TEXT NOT NULL REFERENCES kinds(code) ON DELETE CASCADE,
    PRIMARY KEY (parameter_code, kind_code)
)`,

	`CREATE TABLE IF NOT EXISTS parameter_enum_values (
    parameter_code TEXT NOT NULL REFERENCES parameters(code) ON DELETE CASCADE,
    value          TEXT NOT NULL,
    PRIMARY KEY (parameter_code, value)
)`,

	`CREATE TABLE IF NOT EXISTS parameter_condition_sets (
    parameter_code TEXT NOT NULL REFERENCES parameters(code) ON DELETE CASCADE,
    set_no         INTEGER NOT NULL,
    PRIMARY KEY (parameter_code, set_no)
)`,

	`CREATE TABLE IF NOT EXISTS parameter_condition_set_items (
    parameter_code TEXT NOT NULL,
    set_no         INTEGER NOT NULL,
    condition_code TEXT NOT NULL REFERENCES conditions(code),
    mode           TEXT NOT NULL,
    fixed_value    REAL NULL,
    PRIMARY KEY (parameter_code, set_no, condition_code),
    FOREIGN KEY (parameter_code, set_no)
        REFERENCES parameter_condition_sets(parameter_code, set_no)
        ON DELETE CASCADE
)`,

	`CREATE TABLE IF NOT EXISTS attributes (
    code            TEXT PRIMARY KEY,
    "group"         TEXT NULL,
    value_type      TEXT NOT NULL,
    unit_code       TEXT NULL REFERENCES units(code),
    validation_rule TEXT NULL REFERENCES validation_rules(code),
    sort_order      INTEGER NOT NULL,
    is_active       INTEGER NOT NULL DEFAULT 1
)`,

	`CREATE TABLE IF NOT EXISTS attribute_kinds (
    attribute_code TEXT NOT NULL REFERENCES attributes(code) ON DELETE CASCADE,
    kind_code      TEXT NOT NULL REFERENCES kinds(code) ON DELETE CASCADE,
    PRIMARY KEY (attribute_code, kind_code)
)`,

	`CREATE TABLE IF NOT EXISTS attribute_enum_values (
    attribute_code TEXT NOT NULL REFERENCES attributes(code) ON DELETE CASCADE,
    value          TEXT NOT NULL,
    PRIMARY KEY (attribute_code, value)
)`,

	`CREATE TABLE IF NOT EXISTS kind_validation_rules (
    kind_code       TEXT NOT NULL REFERENCES kinds(code) ON DELETE CASCADE,
    validation_rule TEXT NOT NULL REFERENCES validation_rules(code) ON DELETE CASCADE,
    PRIMARY KEY (kind_code, validation_rule)
)`,

	`CREATE TABLE IF NOT EXISTS devices (
    id           INTEGER ` + autoIncToken + `,
    kind_code    TEXT NOT NULL REFERENCES kinds(code),
    system_code  TEXT NOT NULL REFERENCES designation_systems(code),
    designation  TEXT NOT NULL,
    UNIQUE (kind_code, designation)
)`,

	`CREATE TABLE IF NOT EXISTS device_designation_fields (
    device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    field      TEXT NOT NULL,
    text_value TEXT NULL,
    num_value  REAL NULL,
    PRIMARY KEY (device_id, field)
)`,

	`CREATE TABLE IF NOT EXISTS device_attribute_values (
    device_id      INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    attribute_code TEXT NOT NULL REFERENCES attributes(code),
    text_value     TEXT NULL,
    num_value      REAL NULL,
    bool_value     INTEGER NULL,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, attribute_code, sort_order),
    CHECK (bool_value IS NULL OR bool_value IN (0, 1))
)`,

	`CREATE TABLE IF NOT EXISTS device_variants (
    id         INTEGER ` + autoIncToken + `,
    device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    label      TEXT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
)`,

	// Атрибуты исполнений (план 03 §7): форма строк — как у атрибутов
	// записи (device_attribute_values), владелец — вариант; замена набора
	// исполнений удаляет строки каскадом FK.
	`CREATE TABLE IF NOT EXISTS device_variant_attribute_values (
    variant_id     INTEGER NOT NULL REFERENCES device_variants(id) ON DELETE CASCADE,
    attribute_code TEXT NOT NULL REFERENCES attributes(code),
    text_value     TEXT NULL,
    num_value      REAL NULL,
    bool_value     INTEGER NULL,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (variant_id, attribute_code, sort_order),
    CHECK (bool_value IS NULL OR bool_value IN (0, 1))
)`,

	`CREATE TABLE IF NOT EXISTS parameter_values (
    id             INTEGER ` + autoIncToken + `,
    device_id      INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    variant_id     INTEGER NULL REFERENCES device_variants(id) ON DELETE CASCADE,
    parameter_code TEXT NOT NULL REFERENCES parameters(code),
    value_exact    REAL NULL,
    value_min      REAL NULL,
    value_max      REAL NULL,
    value_text     TEXT NULL,
    sort_order     INTEGER NOT NULL DEFAULT 0
)`,

	`CREATE TABLE IF NOT EXISTS parameter_value_conditions (
    parameter_value_id INTEGER NOT NULL REFERENCES parameter_values(id) ON DELETE CASCADE,
    condition_code     TEXT NOT NULL REFERENCES conditions(code),
    value              REAL NOT NULL,
    PRIMARY KEY (parameter_value_id, condition_code)
)`,

	`CREATE TABLE IF NOT EXISTS manufacturers (
    id   INTEGER ` + autoIncToken + `,
    name TEXT NOT NULL UNIQUE
)`,

	// Реестр источников данных записи (план 03 §13, D13): библиографическое
	// описание — данные записи (как manufacturers/notes — не локализуемые
	// строки, не каталожные отображаемые); code — стабильный ключ формата
	// наполнения, назначаемый редактором.
	`CREATE TABLE IF NOT EXISTS sources (
    id        INTEGER ` + autoIncToken + `,
    code      TEXT NOT NULL UNIQUE,
    title     TEXT NOT NULL,
    type_code TEXT NOT NULL REFERENCES source_types(code),
    year      INTEGER NULL,
    publisher TEXT NULL,
    edition   TEXT NULL,
    isbn      TEXT NULL
)`,

	// Лица источника (план 03 §13): авторы и редакторы — упорядоченные
	// списки строк, по строке на элемент (образец text_list); role —
	// author | editor (семантика — сервисный слой, CHECK не каталожный).
	`CREATE TABLE IF NOT EXISTS source_persons (
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    pos       INTEGER NOT NULL,
    role      TEXT NOT NULL,
    name      TEXT NOT NULL,
    PRIMARY KEY (source_id, pos)
)`,

	// Ссылки на документацию источника (план 03 §13): упорядоченный
	// список (оригинал, зеркала, архивные копии — порядок значим,
	// первый элемент предпочитаемый).
	`CREATE TABLE IF NOT EXISTS source_urls (
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    pos       INTEGER NOT NULL,
    url       TEXT NOT NULL,
    PRIMARY KEY (source_id, pos)
)`,

	`CREATE TABLE IF NOT EXISTS device_manufacturers (
    device_id       INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    manufacturer_id INTEGER NOT NULL REFERENCES manufacturers(id) ON DELETE CASCADE,
    PRIMARY KEY (device_id, manufacturer_id)
)`,

	`CREATE TABLE IF NOT EXISTS device_analogs (
    device_id       INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    analog_device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    note            TEXT NULL,
    PRIMARY KEY (device_id, analog_device_id),
    CHECK (device_id <> analog_device_id)
)`,

	// Исторические названия записей (синонимы — план 03 §9): строка
	// принадлежит владельцу device_id, у синонима нет своей записи;
	// note — происхождение имени, year_from/year_to — годы его
	// употребления (элементы заказа/даты — не локализуемые строки).
	// kind_code — класс владельца (денормализация): UNIQUE с designation
	// подкрепляет инвариант D11 «синоним уникален в пределах класса»
	// на уровне БД — параллельные писатели не коммитят двух владельцев
	// одного синонима.
	`CREATE TABLE IF NOT EXISTS device_synonyms (
    device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    kind_code   TEXT NOT NULL REFERENCES kinds(code),
    designation TEXT NOT NULL,
    note        TEXT NULL,
    year_from   INTEGER NULL,
    year_to     INTEGER NULL,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, designation),
    UNIQUE (kind_code, designation)
)`,

	// Ссылки записей на источники данных (план 03 §13): note — место
	// в источнике («таблица 5.3», «с. 214»); замена секции — целиком,
	// чистка сирот sources — по образцу производителей.
	`CREATE TABLE IF NOT EXISTS device_sources (
    device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    source_id  INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    note       TEXT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, source_id)
)`,

	// Поток изменений записей (change feed — docs/plan/02-database.md §2.3):
	// события применённых операций над записями устройств; FK намеренно
	// нет — события deleted переживают удаляемую строку.
	`CREATE TABLE IF NOT EXISTS change_events (
    id          INTEGER ` + autoIncToken + `,
    revision    INTEGER NOT NULL,
    op          TEXT NOT NULL,
    kind_code   TEXT NOT NULL,
    designation TEXT NOT NULL,
    device_id   INTEGER NOT NULL
)`,

	// Индексы — docs/plan/02-database.md §3.
	`CREATE INDEX IF NOT EXISTS idx_devices_system ON devices(system_code)`,

	`CREATE INDEX IF NOT EXISTS idx_ddf_field_text ON device_designation_fields(field, text_value)`,
	`CREATE INDEX IF NOT EXISTS idx_ddf_field_num ON device_designation_fields(field, num_value)`,

	`CREATE INDEX IF NOT EXISTS idx_dav_attr_text ON device_attribute_values(attribute_code, text_value)`,
	`CREATE INDEX IF NOT EXISTS idx_dav_attr_num ON device_attribute_values(attribute_code, num_value)`,

	`CREATE INDEX IF NOT EXISTS idx_dvav_attr_text ON device_variant_attribute_values(attribute_code, text_value)`,
	`CREATE INDEX IF NOT EXISTS idx_dvav_attr_num ON device_variant_attribute_values(attribute_code, num_value)`,

	`CREATE INDEX IF NOT EXISTS idx_pv_code_min ON parameter_values(parameter_code, value_min)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_code_max ON parameter_values(parameter_code, value_max)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_code_exact ON parameter_values(parameter_code, value_exact)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_code_text ON parameter_values(parameter_code, value_text)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_device ON parameter_values(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_variant ON parameter_values(variant_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_device_code ON parameter_values(device_id, parameter_code, value_min, value_max, value_exact)`,
	`CREATE INDEX IF NOT EXISTS idx_pv_variant_code ON parameter_values(variant_id, parameter_code, value_min, value_max, value_exact)`,

	`CREATE INDEX IF NOT EXISTS idx_dv_device ON device_variants(device_id)`,

	`CREATE INDEX IF NOT EXISTS idx_dm_manufacturer ON device_manufacturers(manufacturer_id)`,

	`CREATE INDEX IF NOT EXISTS idx_analogs_analog ON device_analogs(analog_device_id)`,

	`CREATE INDEX IF NOT EXISTS idx_synonym_designation ON device_synonyms(designation)`,

	`CREATE INDEX IF NOT EXISTS idx_device_sources_source ON device_sources(source_id)`,
}

// CreateFixture создаёт схему фикстуры версии version в открытой базе:
// полный DDL версии и schema_meta с этой версией схемы и ревизиями 1 —
// отправная точка проверок миграции (тесты эквивалентности, фаза migrate
// qa-прогонщика). Рассчитана на пустую базу (CREATE … IF NOT EXISTS и
// вставка меты подавляют конфликты); в производственный путь не входит.
func (d *DB) CreateFixture(ctx context.Context, version int) error {
	stmts, ok := ddlFixtures[version]
	if !ok {
		return fmt.Errorf("schema fixture version %d is not registered", version)
	}
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	for _, stmt := range stmts {
		for attempt := 0; ; attempt++ {
			_, err := tx.exec(ctx, d.dialect.RewriteDDL(stmt), nil)
			if err == nil {
				break
			}
			if attempt < 3 && isAlreadyExistsError(err) {
				continue
			}
			return domain.NewErrorf(domain.CodeStorageDDLFailed,
				domain.MsgStorageDDL, err.Error())
		}
	}
	for key, value := range map[string]string{
		metaSchemaVersion:   strconv.Itoa(version),
		metaCatalogRevision: "1",
		metaDataRevision:    "1",
	} {
		if _, err := tx.exec(ctx, `
INSERT INTO schema_meta(key, value)
SELECT @key, @value
WHERE NOT EXISTS (SELECT 1 FROM schema_meta WHERE key = @key)
ON CONFLICT DO NOTHING`,
			map[string]any{"key": key, "value": value}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
