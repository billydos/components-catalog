package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/testutil"
)

// Пин-тест реестра миграций (план 09 §9): цепочка непрерывна от
// SchemaBaseline+1 до SchemaVersion, последний шаг ведёт к версии модуля;
// пустой реестр валиден только пока версия не выросла над базовой.
// RB-1/RB-2: инкремент SchemaVersion выше SchemaBaseline без шага и
// дыры нумерации — видимый отказ, а не молчаливый пропуск базы.
func TestMigrationsRegistry(t *testing.T) {
	if len(migrations) == 0 {
		if SchemaVersion != SchemaBaseline {
			t.Fatalf("реестр миграций пуст при SchemaVersion %d > SchemaBaseline %d: "+
				"каждый инкремент версии выше базовой обязан сопровождаться шагом", SchemaVersion, SchemaBaseline)
		}
		return
	}
	if SchemaVersion <= SchemaBaseline {
		t.Fatalf("реестр не пуст (%d шагов) при SchemaVersion %d <= SchemaBaseline %d",
			len(migrations), SchemaVersion, SchemaBaseline)
	}
	want := SchemaBaseline + 1
	for _, m := range migrations {
		if m.To != want {
			t.Fatalf("шаг To=%d (Desc %q): ожидалась непрерывная цепочка (To=%d)", m.To, m.Desc, want)
		}
		if m.Desc == "" {
			t.Errorf("шаг To=%d без описания (Desc)", m.To)
		}
		want++
	}
	if last := want - 1; last != SchemaVersion {
		t.Fatalf("последний шаг To=%d != SchemaVersion %d", last, SchemaVersion)
	}
}

// Отказы Migrate до любых записей (план 09 §1): без меты —
// database_not_initialized, база новее модуля — schema_version_mismatch,
// старее SchemaBaseline — schema_version_unsupported.
func TestMigrateRefusalsSQLite(t *testing.T) {
	ctx := context.Background()

	t.Run("без меты", func(t *testing.T) {
		_, db := openSQLiteFile(t)
		_, err := db.Migrate(ctx)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeDatabaseNotInitialized {
			t.Fatalf("ожидался database_not_initialized, получено %v", err)
		}
	})

	prepare := func(t *testing.T, version string) *DB {
		t.Helper()
		_, db := openSQLiteFile(t)
		if _, err := db.EnsureCreated(ctx); err != nil {
			t.Fatalf("EnsureCreated: %v", err)
		}
		setSchemaVersion(t, db, version)
		return db
	}

	t.Run("новее модуля", func(t *testing.T) {
		db := prepare(t, "99")
		_, err := db.Migrate(ctx)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeSchemaVersionMismatch {
			t.Fatalf("ожидался schema_version_mismatch, получено %v", err)
		}
	})

	t.Run("старее baseline", func(t *testing.T) {
		db := prepare(t, "14")
		_, err := db.Migrate(ctx)
		de, ok := domain.AsError(err)
		if !ok || de.Code != domain.CodeSchemaVersionUnsupported {
			t.Fatalf("ожидался schema_version_unsupported, получено %v", err)
		}
		if de.Message != domain.SchemaVersionUnsupported(14, SchemaBaseline).Message {
			t.Fatalf("текст: %q", de.Message)
		}
	})
}

// CheckSchema и EnsureCreated классифицируют прежнюю версию так же:
// ниже baseline — schema_version_unsupported (пересоздание импортом),
// в диапазоне поддержки — schema_version_mismatch (совет migrate).
func TestCheckSchemaClassificationSQLite(t *testing.T) {
	ctx := context.Background()
	_, db := openSQLiteFile(t)
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	for v, want := range map[string]domain.Code{
		"14": domain.CodeSchemaVersionUnsupported,
		"99": domain.CodeSchemaVersionMismatch,
	} {
		setSchemaVersion(t, db, v)
		err := db.CheckSchema(ctx)
		de, ok := domain.AsError(err)
		if !ok || de.Code != want {
			t.Fatalf("версия %s: ожидался %s, получено %v", v, want, err)
		}
		_, err = db.EnsureCreated(ctx)
		de, ok = domain.AsError(err)
		if !ok || de.Code != want {
			t.Fatalf("EnsureCreated версии %s: ожидался %s, получено %v", v, want, err)
		}
	}
}

// Migrate на актуальной базе — отчёт «без изменений» и ни одной
// транзакции записи (идемпотентность механизма).
func TestMigrateUpToDateSQLite(t *testing.T) {
	ctx := context.Background()
	_, db := openSQLiteFile(t)
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	rep, err := db.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if rep.From != SchemaVersion || rep.To != SchemaVersion || rep.Steps != 0 {
		t.Fatalf("отчёт актуальной базы: %+v", rep)
	}
	cat, data, err := db.Revisions(ctx)
	if err != nil || cat != 1 || data != 1 {
		t.Fatalf("ревизии после холостого Migrate: catalog=%d data=%d err=%v", cat, data, err)
	}
}

// Шаг миграции атомарен: сбой Data-хука после DDL откатывает всю
// транзакцию шага — версия и схема не меняются (schema_migration_failed).
func TestMigrationStepFailureRollsBackSQLite(t *testing.T) {
	ctx := context.Background()
	_, db := openSQLiteFile(t)
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	step := Migration{
		To:   SchemaVersion + 1,
		Desc: "test: шаг с ошибкой Data-хука",
		DDL:  []string{`CREATE TABLE migration_probe (id INTEGER PRIMARY KEY)`},
		Data: func(ctx context.Context, tx *Tx) error {
			return errors.New("probe failure")
		},
	}
	err := db.applyStep(ctx, step, SchemaVersion)
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeSchemaMigrationFailed {
		t.Fatalf("ожидался schema_migration_failed, получено %v", err)
	}
	if de.Message != domain.SchemaMigrationFailed(SchemaVersion, step.To, "probe failure").Message {
		t.Fatalf("текст: %q", de.Message)
	}
	v, err := db.SchemaVersion(ctx)
	if err != nil || v != SchemaVersion {
		t.Fatalf("версия после сбоя шага: %d (err %v)", v, err)
	}
	if n := countTables(t, db, "migration_probe"); n != 0 {
		t.Fatal("DDL шага зафиксирован несмотря на сбой — транзакция не откатилась")
	}
}

// Успешный шаг: DDL → Data (правило ревизий — план 09 §6) → CAS-фиксация
// версии; CAS с чужим from не находит строку — сигнал гонки.
func TestMigrationStepApplyAndRaceSQLite(t *testing.T) {
	ctx := context.Background()
	_, db := openSQLiteFile(t)
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}

	t.Run("CAS не находит чужую версию", func(t *testing.T) {
		// Версия уже ушла вперёд: applyStep с устаревшим from обязан
		// вернуть сигнал гонки, а не применять повторно.
		setSchemaVersion(t, db, "16")
		step := Migration{To: 16, Desc: "test: гонка", DDL: []string{
			`CREATE TABLE migration_probe (id INTEGER PRIMARY KEY)`,
		}}
		err := db.applyStep(ctx, step, 15)
		if !errors.Is(err, errStepRace) {
			t.Fatalf("ожидался сигнал гонки, получено %v", err)
		}
		if n := countTables(t, db, "migration_probe"); n != 0 {
			t.Fatal("транзакция проигравшего гонку шага не откатилась")
		}
		setSchemaVersion(t, db, "15")
	})

	t.Run("применение с инкрементом ревизии", func(t *testing.T) {
		step := Migration{
			To:   SchemaVersion + 1,
			Desc: "test: успешный шаг",
			DDL:  []string{`CREATE TABLE migration_probe (id INTEGER PRIMARY KEY)`},
			Data: func(ctx context.Context, tx *Tx) error {
				return tx.BumpDataRevision(ctx)
			},
		}
		if err := db.applyStep(ctx, step, SchemaVersion); err != nil {
			t.Fatalf("applyStep: %v", err)
		}
		v, err := db.SchemaVersion(ctx)
		if err != nil || v != step.To {
			t.Fatalf("версия после шага: %d (err %v)", v, err)
		}
		if n := countTables(t, db, "migration_probe"); n != 1 {
			t.Fatal("DDL успешного шага не зафиксирован")
		}
		_, data, err := db.Revisions(ctx)
		if err != nil || data != 2 {
			t.Fatalf("data_revision после Data-хука: %d (err %v), ожидалась 2", data, err)
		}
		// Восстановление версии: тестовая база одноразовая, но версия выше
		// модуля ломает Close-независимые проверки других тестов пакета
		// только через этот файл — возвращаем каноническое значение.
		setSchemaVersion(t, db, "15")
	})
}

// Хуки sqlite: PreMigrate отключает внешние ключи выделенного соединения
// записи, RestoreMigrate возвращает (отдельно от PostMigrate — путь
// отказа цепочки), PostMigrate возвращает и обслуживает статистику
// (план 09 §4).
func TestSQLiteMigrateHooks(t *testing.T) {
	ctx := context.Background()
	_, db := openSQLiteFile(t)
	if _, err := db.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	fk := func() int {
		var v int
		if err := db.writes.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&v); err != nil {
			t.Fatalf("PRAGMA foreign_keys: %v", err)
		}
		return v
	}
	if err := db.dialect.PreMigrate(ctx, db.writes); err != nil {
		t.Fatalf("PreMigrate: %v", err)
	}
	if got := fk(); got != 0 {
		t.Fatalf("foreign_keys после PreMigrate: %d, ожидался 0", got)
	}
	if err := db.dialect.RestoreMigrate(ctx, db.writes); err != nil {
		t.Fatalf("RestoreMigrate: %v", err)
	}
	if got := fk(); got != 1 {
		t.Fatalf("foreign_keys после RestoreMigrate: %d, ожидался 1", got)
	}
	if err := db.dialect.PreMigrate(ctx, db.writes); err != nil {
		t.Fatalf("PreMigrate: %v", err)
	}
	if err := db.dialect.PostMigrate(ctx, db.writes); err != nil {
		t.Fatalf("PostMigrate: %v", err)
	}
	if got := fk(); got != 1 {
		t.Fatalf("foreign_keys после PostMigrate: %d, ожидался 1", got)
	}
}

// Сквозной путь сбоя цепочки (PreMigrate → шаг → восстановление режима
// соединения) достижим только при SchemaVersion > SchemaBaseline: пока
// реестр пуст, цикл шагов Migrate неработает (база всегда актуальна либо
// вне диапазона) — восстановление FK покрыто хуками выше и прямым
// applyStep ниже; первым реальным шагом (v16) путь закрепляется тестом.

// fixtureSQLitePath — путь файла базы-фикстуры версии version
// (qa/fixtures): создан релизной сборкой этой версии (init + импорт
// выверенного файла data/); план 09 §9.
func fixtureSQLitePath(t *testing.T, version int) string {
	t.Helper()
	p := filepath.Join("..", "..", "qa", "fixtures",
		"schema_v"+strconv.Itoa(version)+".sqlite")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("файл фикстуры %s: %v", p, err)
	}
	return p
}

// fixtureSQLiteCopy — копия файла фикстуры во временный каталог теста:
// файл репозитория не открывается на запись и не мутируется (режим WAL
// требует записи в каталог при открытии — копия снимает и это).
func fixtureSQLiteCopy(t *testing.T, version int) string {
	t.Helper()
	src, err := os.ReadFile(fixtureSQLitePath(t, version))
	if err != nil {
		t.Fatalf("чтение фикстуры: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "fixture-copy.db")
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		t.Fatalf("копия фикстуры: %v", err)
	}
	return dst
}

// introspectSchema — нормализованный снимок схемы открытой базы
// (методы диалекта — только тесты).
func introspectSchema(t *testing.T, db *DB) []string {
	t.Helper()
	ctx := context.Background()
	var (
		lines []string
		err   error
	)
	switch dd := db.dialect.(type) {
	case sqliteDialect:
		lines, err = dd.introspect(ctx, db.reads)
	case postgresDialect:
		lines, err = dd.introspect(ctx, db.reads)
	default:
		t.Fatalf("интроспекция: неизвестный диалект %T", db.dialect)
	}
	if err != nil {
		t.Fatalf("интроспекция: %v", err)
	}
	return lines
}

// Пин-тест двух частей фикстур по всем поддерживаемым версиям (правило
// сопровождения плана 09 §9: релиз, инкрементирующий SchemaVersion,
// добавляет фикстуру предыдущей версии): каждой версии из ожидаемого
// набора — замороженный срез ddlFixtures и файл qa/fixtures/schema_vN.sqlite,
// их интроспекции совпадают; посторонние срезы и файлы — дрейф, а не
// молчаливое накопление. Набор: baseline всегда, плюс все версии
// [SchemaBaseline, SchemaVersion) (при SchemaVersion == SchemaBaseline
// вырождается в один baseline). Интроспекция файла — по копии: файл
// репозитория не открывается на запись.
func TestFixtureFilesMatchDDLSnapshots(t *testing.T) {
	required := map[int]bool{SchemaBaseline: true}
	for n := SchemaBaseline; n < SchemaVersion; n++ {
		required[n] = true
	}
	for n := range ddlFixtures {
		if !required[n] {
			t.Errorf("ddlFixtures: срез версии %d вне ожидаемого набора %v", n, required)
		}
	}
	for n := range required {
		if _, ok := ddlFixtures[n]; !ok {
			t.Errorf("версия %d: нет замороженного среза ddlFixtures", n)
		}
	}

	entries, err := os.ReadDir(filepath.Join("..", "..", "qa", "fixtures"))
	if err != nil {
		t.Fatalf("qa/fixtures: %v", err)
	}
	files := map[int]bool{}
	for _, e := range entries {
		m := fixtureFileRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("имя фикстуры %s: %v", e.Name(), err)
		}
		if !required[n] {
			t.Errorf("файл фикстуры %s вне ожидаемого набора %v", e.Name(), required)
			continue
		}
		files[n] = true
	}
	for n := range required {
		if !files[n] {
			t.Errorf("версия %d: нет файла qa/fixtures/schema_v%d.sqlite", n, n)
		}
	}

	for n := range required {
		n := n
		t.Run("версия "+strconv.Itoa(n), func(t *testing.T) {
			db, err := Open(context.Background(), Config{
				Dialect: "sqlite",
				DSN:     fixtureSQLiteCopy(t, n),
			})
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer db.Close() //nolint:errcheck — тестовое соединение
			want := introspectSchema(t, db)

			fresh, err := Open(context.Background(), Config{
				Dialect: "sqlite",
				DSN:     filepath.Join(t.TempDir(), "snapshot.db"),
			})
			if err != nil {
				t.Fatalf("open snapshot: %v", err)
			}
			defer fresh.Close() //nolint:errcheck — тестовое соединение
			if err := fresh.CreateFixture(context.Background(), n); err != nil {
				t.Fatalf("CreateFixture(%d): %v", n, err)
			}
			got := introspectSchema(t, fresh)
			if !slices.Equal(got, want) {
				for _, d := range diffLines(got, want) {
					t.Error(d)
				}
			}
		})
	}
}

// fixtureFileRE — имя файла sqlite-фикстуры версии N.
var fixtureFileRE = regexp.MustCompile(`^schema_v(\d+)\.sqlite$`)

// Инвариант эквивалентности (план 09 §9): база, созданная свежим DDL
// (EnsureCreated), схемно идентична базе, доведённой миграциями с
// фикстуры baseline. sqlite-путь.
func TestMigrationEquivalenceSQLite(t *testing.T) {
	ctx := context.Background()

	pathA := filepath.Join(t.TempDir(), "fresh.db")
	dbA, err := Open(ctx, Config{Dialect: "sqlite", DSN: pathA})
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	defer dbA.Close() //nolint:errcheck — тестовое соединение
	if _, err := dbA.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}

	dbB, err := Open(ctx, Config{Dialect: "sqlite", DSN: fixtureSQLiteCopy(t, SchemaBaseline)})
	if err != nil {
		t.Fatalf("open fixture copy: %v", err)
	}
	defer dbB.Close() //nolint:errcheck — тестовое соединение
	rep, err := dbB.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate фикстуры: %v", err)
	}
	if rep.To != SchemaVersion {
		t.Fatalf("фикстура после Migrate на версии %d, ожидалась %d", rep.To, SchemaVersion)
	}

	a, b := introspectSchema(t, dbA), introspectSchema(t, dbB)
	if !slices.Equal(a, b) {
		for _, d := range diffLines(b, a) {
			t.Error(d)
		}
	}
}

// PostgreSQL: та же эквивалентность «свежая ≡ мигрированная» через
// схему фикстуры ddlFixtures (внешние утилиты не привлекаются) и те же
// отказы/атомарность шага. Локальный прогон опционален
// (CATALOG_TEST_POSTGRES_DSN, база одноразовая).
func TestMigrationEquivalencePostgres(t *testing.T) {
	dsn := testutil.PostgresDSN(t)
	ctx := context.Background()
	testutil.DropAllTables(t, dsn)

	dbA, err := Open(ctx, Config{Dialect: "postgres", DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := dbA.EnsureCreated(ctx); err != nil {
		t.Fatalf("EnsureCreated: %v", err)
	}
	a := introspectSchema(t, dbA)
	if err := dbA.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// База-Б: фикстура baseline, поднятая миграцией.
	testutil.DropAllTables(t, dsn)
	dbB, err := Open(ctx, Config{Dialect: "postgres", DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer dbB.Close() //nolint:errcheck — тестовое соединение
	if err := dbB.CreateFixture(ctx, SchemaBaseline); err != nil {
		t.Fatalf("CreateFixture: %v", err)
	}
	rep, err := dbB.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if rep.To != SchemaVersion {
		t.Fatalf("после Migrate версия %d, ожидалась %d", rep.To, SchemaVersion)
	}

	// Отказы и атомарность шага — на той же базе-Б.
	setSchemaVersion(t, dbB, "14")
	if _, err := dbB.Migrate(ctx); !isCode(err, domain.CodeSchemaVersionUnsupported) {
		t.Fatalf("старее baseline: %v", err)
	}
	setSchemaVersion(t, dbB, "99")
	if _, err := dbB.Migrate(ctx); !isCode(err, domain.CodeSchemaVersionMismatch) {
		t.Fatalf("новее модуля: %v", err)
	}
	setSchemaVersion(t, dbB, strconv.Itoa(SchemaVersion))
	step := Migration{
		To:   SchemaVersion + 1,
		Desc: "test: шаг с ошибкой Data-хука (postgres)",
		DDL:  []string{`CREATE TABLE migration_probe (id INTEGER PRIMARY KEY)`},
		Data: func(ctx context.Context, tx *Tx) error {
			return errors.New("probe failure")
		},
	}
	if err := dbB.applyStep(ctx, step, SchemaVersion); !isCode(err, domain.CodeSchemaMigrationFailed) {
		t.Fatalf("сбой шага: %v", err)
	}
	if n := countTables(t, dbB, "migration_probe"); n != 0 {
		t.Fatal("DDL сбойного шага зафиксирован — транзакция не откатилась")
	}
	v, err := dbB.SchemaVersion(ctx)
	if err != nil || v != SchemaVersion {
		t.Fatalf("версия после сбоя: %d (err %v)", v, err)
	}

	b := introspectSchema(t, dbB)
	if !slices.Equal(b, a) {
		for _, d := range diffLines(b, a) {
			t.Error(d)
		}
	}
}

// setSchemaVersion — прямая порча версии в тестовой базе (одноразовой).
func setSchemaVersion(t *testing.T, db *DB, version string) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck — порча версии теста
	if _, err := tx.exec(context.Background(),
		`UPDATE schema_meta SET value = @v WHERE key = 'schema_version'`,
		map[string]any{"v": version}); err != nil {
		t.Fatalf("порча версии: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// countTables — число таблиц с данным именем (0/1), оба диалекта.
func countTables(t *testing.T, db *DB, table string) int {
	t.Helper()
	var n int
	var err error
	if db.DialectName() == "postgres" {
		err = db.queryRow(context.Background(),
			`SELECT count(*) FROM information_schema.tables
			 WHERE table_schema = current_schema() AND table_name = @n`,
			map[string]any{"n": table}).Scan(&n)
	} else {
		err = db.queryRow(context.Background(),
			`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = @n`,
			map[string]any{"n": table}).Scan(&n)
	}
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// isCode — ожидаемая ли это ошибка с кодом домена.
func isCode(err error, code domain.Code) bool {
	de, ok := domain.AsError(err)
	return ok && de.Code == code
}

// diffLines — построчная разница снимков для читаемого отказа.
func diffLines(got, want []string) []string {
	var out []string
	for _, l := range got {
		if !slices.Contains(want, l) {
			out = append(out, "лишняя строка мигрированной схемы: "+l)
		}
	}
	for _, l := range want {
		if !slices.Contains(got, l) {
			out = append(out, "недостающая строка мигрированной схемы: "+l)
		}
	}
	if len(out) == 0 {
		out = append(out, "снимки схем различаются целиком")
	}
	return out
}
