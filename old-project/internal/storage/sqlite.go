package storage

import (
	"database/sql"

	_ "modernc.org/sqlite" // драйвер регистрируется как "sqlite" (pure Go, без cgo)
)

// NewSqliteStore — реализация для SQLite: файл базы создаётся при первом
// обращении, хранение — UTF-8.
func NewSqliteStore(databasePath string) *RelationalStore {
	return NewRelationalStore(Dialect{
		Open: func() (*sql.DB, error) {
			db, err := sql.Open("sqlite", databasePath)
			if err != nil {
				return nil, err
			}
			// одно соединение на экземпляр (порт одно-соединенной модели ядра):
			// SQLite требует включения внешних ключей для каждого соединения
			// (каскадное удаление данных), а с единственным соединением
			// достаточно одного PRAGMA после открытия.
			db.SetMaxOpenConns(1)
			db.SetMaxIdleConns(1)
			if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
				db.Close()
				return nil, err
			}
			return db, nil
		},
		CreateTableSql:  sqliteCreateTableSql(),
		LastInsertIdSql: "SELECT last_insert_rowid()",
	})
}

// Для перехода на PostgreSQL/MariaDB: задайте другую Dialect для
// RelationalStore — OpenConnection (pgx/mysql), CreateTableSql (id: GENERATED
// BY DEFAULT AS IDENTITY / AUTO_INCREMENT) и LastInsertIdSql (SELECT lastval() /
// SELECT LAST_INSERT_ID()). Остальной DML менять не нужно.
// DDL electrical_parameters собирается из каталога параметров (ddl.go).
//
// AUTOINCREMENT у id-колонок (transistors, manufacturers) — осознанный запрет
// переиспользования id после удаления: id — стабильная внешняя ссылка на запись
// (FK-связи, вывод, экспорт), а IDENTITY/AUTO_INCREMENT целевых СУБД ведут себя
// так же. Не упрощать до INTEGER PRIMARY KEY.
func sqliteCreateTableSql() string {
	return `CREATE TABLE IF NOT EXISTS transistors (
    Id            INTEGER PRIMARY KEY AUTOINCREMENT, -- id не переиспользуются после delete — не убирать AUTOINCREMENT
    Material      TEXT    NOT NULL,     -- 1. материал: Г|1, К|2, А|3, И|4
    Subclass      TEXT    NOT NULL,     -- 2. подкласс: Т или П
    Assembly      INTEGER NOT NULL,     -- 2.1. сборка: 1 — буква «С» в обозначении, 0 — нет
    Feature       INTEGER NOT NULL,     -- 3. эксплуатационный признак: 1–9
    DevNumber     INTEGER NOT NULL,     -- 4. номер разработки: 01–999
    Letters       TEXT    NOT NULL,     -- 5. классификация: 1–2 заглавные русские буквы
    Modification  INTEGER     NULL,     -- 6. модификация: 1–9 (необязательно)
    ChipVariant   INTEGER     NULL,     -- 7. бескорпусное исполнение: 1–6 (необязательно)
    CONSTRAINT chk_material     CHECK (Material IN ('Г','1','К','2','А','3','И','4')),
    CONSTRAINT chk_subclass     CHECK (Subclass IN ('Т','П')),
    CONSTRAINT chk_assembly     CHECK (Assembly IN (0, 1)),
    CONSTRAINT chk_feature      CHECK (Feature BETWEEN 1 AND 9),
    CONSTRAINT chk_dev_number   CHECK (DevNumber BETWEEN 1 AND 999),
    -- chk_letters — сознательно слабее домена (ValidateTransistor/IsUpperLetters
    -- требуют 1–2 заглавных кириллических): переносимого предиката для CHECK у целевых СУБД нет
    -- (GLOB — только SQLite; LENGTH() в MariaDB считает октеты; MariaDB запрещает regex в CHECK).
    -- CHECK отсекает лишь пустую строку; полную проверку выполняет домен. Не «усиливать».
    CONSTRAINT chk_letters      CHECK (Letters <> ''),
    CONSTRAINT chk_modification CHECK (Modification IS NULL OR (Modification BETWEEN 1 AND 9)),
    CONSTRAINT chk_chip_variant CHECK (ChipVariant IS NULL OR (ChipVariant BETWEEN 1 AND 6)),
    CONSTRAINT uq_transistor    UNIQUE (Material, Subclass, Assembly, Feature, DevNumber, Letters, Modification, ChipVariant)
);

CREATE TABLE IF NOT EXISTS transistor_attributes (
    TransistorId INTEGER PRIMARY KEY REFERENCES transistors(Id) ON DELETE CASCADE,
    Structure        TEXT    NULL,   -- структура проводимости: npn, pnp, n-fet, p-fet...
    Technology       TEXT    NULL,   -- технология: сплавная, планарная, эпитаксиальная...
    Package          TEXT    NULL,   -- корпус: КТ-13, TO-92...
    PackageMaterial  TEXT    NULL,   -- материал корпуса: металл, металлокерамика, пластик
    ColorMarking     TEXT    NULL,   -- цветовая маркировка (если обозначения на корпусе нет)
    Pinout           TEXT    NULL,   -- цоколёвка: КБЭ, 1-Э 2-К 3-Б...
    EsdSensitive     INTEGER NULL,   -- повышенная чувствительность к статическому напряжению: 0/1
    MilitaryGrade     INTEGER NULL,   -- военное исполнение: 0/1
    RadiationHardened INTEGER NULL, -- радиационная стойкость: 0/1
    Tu               TEXT    NULL,   -- обозначение ТУ/ОТУ
    Notes            TEXT    NULL,   -- примечание
    YearFrom         INTEGER NULL,   -- год начала выпуска (1949–2100)
    YearTo           INTEGER NULL,   -- год окончания выпуска (NULL — выпускался на момент описания)
    MassMax          REAL    NULL,   -- масса «не более», г
    DatasheetUrl     TEXT    NULL,   -- ссылка на документацию
    CONSTRAINT chk_esd       CHECK (EsdSensitive IS NULL OR EsdSensitive IN (0, 1)),
    CONSTRAINT chk_military  CHECK (MilitaryGrade IS NULL OR MilitaryGrade IN (0, 1)),
    CONSTRAINT chk_radiation CHECK (RadiationHardened IS NULL OR RadiationHardened IN (0, 1)),
    CONSTRAINT chk_years CHECK (
        (YearFrom IS NULL OR (YearFrom >= 1949 AND YearFrom <= 2100))
        AND (YearTo IS NULL OR (YearTo >= 1949 AND YearTo <= 2100))
        AND (YearFrom IS NULL OR YearTo IS NULL OR YearFrom < YearTo)
    ),
    CONSTRAINT chk_mass CHECK (MassMax IS NULL OR MassMax > 0),
    CONSTRAINT chk_any_attribute CHECK (
        ` + anyAttributeCheckSql() + `
    )
);

CREATE TABLE IF NOT EXISTS manufacturers (
    Id   INTEGER PRIMARY KEY AUTOINCREMENT, -- id не переиспользуются после delete — не убирать AUTOINCREMENT
    Name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS transistor_manufacturers (
    TransistorId   INTEGER NOT NULL REFERENCES transistors(Id) ON DELETE CASCADE,
    ManufacturerId INTEGER NOT NULL REFERENCES manufacturers(Id) ON DELETE CASCADE,
    PRIMARY KEY (TransistorId, ManufacturerId)
);

` + electricalParametersCreateTableSql() + `

CREATE TABLE IF NOT EXISTS maximum_ratings (
    TransistorId INTEGER PRIMARY KEY REFERENCES transistors(Id) ON DELETE CASCADE,
    UkeMax      REAL NULL,      -- постоянное напряжение коллектор-эмиттер, В
    UkbMax      REAL NULL,      -- постоянное напряжение коллектор-база (к-б/б-к), В
    UbeMax      REAL NULL,      -- постоянное напряжение база-эмиттер (б-э/э-б), В
    UkeoMax     REAL NULL,      -- постоянное напряжение к-э при разомкнутой базе, В
    IkMax       REAL NULL,      -- постоянный ток коллектора, мА
    IbMax       REAL NULL,      -- постоянный ток базы, мА
    PkMax       REAL NULL,      -- постоянная рассеиваемая мощность коллектора, мВт
    IkPulseMax  REAL NULL,      -- импульсный ток коллектора, мА
    PkPulseMax  REAL NULL,      -- импульсная рассеиваемая мощность, мВт
    PulseDuration REAL NULL,    -- длительность импульса, мкс (обязательна при импульсных значениях)
    TempMin     REAL NULL,     -- минимальная температура среды, °C
    TempMax     REAL NULL,     -- максимальная температура среды, °C
    TempJunctionMax REAL NULL, -- максимальная температура перехода, °C
    Rth         REAL NULL,      -- тепловое сопротивление переход-корпус, °C/Вт
    CONSTRAINT chk_ratings CHECK (
        (UkeMax IS NULL OR UkeMax > 0)
        AND (UkbMax IS NULL OR UkbMax > 0)
        AND (UbeMax IS NULL OR UbeMax > 0)
        AND (UkeoMax IS NULL OR UkeoMax > 0)
        AND (IkMax IS NULL OR IkMax > 0)
        AND (IbMax IS NULL OR IbMax > 0)
        AND (PkMax IS NULL OR PkMax > 0)
        AND (IkPulseMax IS NULL OR IkPulseMax > 0)
        AND (PkPulseMax IS NULL OR PkPulseMax > 0)
        AND (PulseDuration IS NULL OR PulseDuration > 0)
        AND (TempJunctionMax IS NULL OR TempJunctionMax > 0)
        AND (Rth IS NULL OR Rth > 0)
        AND (TempMin IS NULL OR TempMax IS NULL OR TempMin < TempMax)
        AND ((IkPulseMax IS NOT NULL OR PkPulseMax IS NOT NULL) = (PulseDuration IS NOT NULL))
    )
);`
}
