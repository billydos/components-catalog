package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Механизм миграций схемы — только вперёд от базовой версии
// (docs/plan/09-schema-migrations.md): свежая база создаётся целиком
// EnsureCreated (полный DDL, сразу SchemaVersion), миграции — для
// существующих; автомиграций при чтении нет — метод вызывают только
// явные точки входа (CLI migrate, App.Migrate). schema_meta.schema_version
// — единственный источник истины: таблица-журнал применённых миграций
// не заводится, строго последовательные шаги и единый счётчик версии
// делают её избыточной.

// Migration — правила перехода с версии To-1 на To: одна запись реестра,
// одна транзакция применения. DDL — переносимый текст (переписывается
// RewriteDDL диалекта, как ddlStatements); Data — опциональная доработка
// данных и диалектозависимые операции (ветвление по диалекту — только
// здесь, внутри storage). Шаг не пишет события change_events; ревизии —
// по правилу плана 09 §6 (BumpCatalogRevision/BumpDataRevision в хуке).
type Migration struct {
	To   int
	Desc string // краткое описание шага: dry-run, отчёты, журнал
	DDL  []string
	Data func(ctx context.Context, tx *Tx) error
}

// migrations — реестр шагов (план 09 §3). Пуст, пока
// SchemaVersion == SchemaBaseline; каждый инкремент версии выше базовой
// обязан сопровождаться шагом с To == SchemaVersion — непрерывность
// цепочки SchemaBaseline+1 … SchemaVersion закреплена пин-тестом
// migrations_test.go. Содержимое шага неизменяемо после релиза, его
// включившего: правка шага — правка истории (RB-5), исправление
// ошибочного шага — новым шагом.
var migrations = []Migration{}

// MigrationInfo — шаг реестра для планов и отчётов: тела шага (DDL,
// Data-хук) не покидают слой применения.
type MigrationInfo struct {
	To   int
	Desc string
}

// MigrateReport — итог миграции: версия схемы до, после, число
// применённых шагов (From == To — база уже была актуальна) и перечень
// фактически применённых этой сессией шагов (Applied — журнал slog,
// отчёты; при проигранной гонке competitor-шаги в него не попадают,
// хотя To их учитывает).
type MigrateReport struct {
	From    int
	To      int
	Steps   int
	Applied []MigrationInfo
}

// MigrationSteps — перечень шагов реестра для планов и отчётов без
// открытия транзакций (dry-run, справка).
func (d *DB) MigrationSteps() []MigrationInfo {
	out := make([]MigrationInfo, 0, len(migrations))
	for _, m := range migrations {
		out = append(out, MigrationInfo{To: m.To, Desc: m.Desc})
	}
	return out
}

// SchemaVersion читает версию схемы базы (schema_meta.schema_version —
// единственный источник истины). Отсутствие таблицы/строки и нечисловое
// значение — повреждённая или чужая база (database_not_initialized,
// как CheckSchema); диапазон не проверяется — классификация вызывающего.
func (d *DB) SchemaVersion(ctx context.Context) (int, error) {
	var value string
	err := d.queryRow(ctx, `SELECT value FROM schema_meta WHERE key = @key`,
		map[string]any{"key": metaSchemaVersion}).Scan(&value)
	if err != nil {
		if isNoTableError(err) || errors.Is(err, sql.ErrNoRows) {
			return 0, domain.DatabaseNotInitialized()
		}
		return 0, err
	}
	v, convErr := strconv.Atoi(strings.TrimSpace(value))
	if convErr != nil {
		return 0, domain.DatabaseNotInitialized()
	}
	return v, nil
}

// errStepRace — конкурентный писатель зафиксировал шаг раньше (CAS
// версии не нашёл ожидаемой строки): откат транзакции шага и
// продолжение цепочки с актуальной версии, а не отказ.
var errStepRace = errors.New("schema migration step claimed by a concurrent writer")

// Migrate доводит схему существующей базы до версии модуля (план 09 §4):
// применяет шаги v+1 … SchemaVersion, один шаг — одна транзакция
// (DDL → Data → CAS-фиксация версии → Commit); падение на середине
// оставляет базу на последней зафиксированной версии — повторный запуск
// продолжает цепочку. Отказы до любых записей: база новее модуля —
// schema_version_mismatch, старее SchemaBaseline —
// schema_version_unsupported, без меты — database_not_initialized.
// Идемпотентна: актуальная база — отчёт «изменений нет», ни одной
// транзакции записи. На любом выходе после PreMigrate режим соединения
// записи восстанавливается (sqlite: FK) — сбой цепочки не оставляет пул
// записи без внешних ключей.
func (d *DB) Migrate(ctx context.Context) (MigrateReport, error) {
	v, err := d.SchemaVersion(ctx)
	if err != nil {
		return MigrateReport{}, err
	}
	if err := ClassifySchemaVersion(v); err != nil {
		return MigrateReport{}, err
	}
	if v == SchemaVersion {
		return MigrateReport{From: v, To: v}, nil
	}
	from := v
	// Хуки диалекта — на выделенном соединении записи вне транзакций:
	// PRAGMA foreign_keys — no-op внутри транзакции sqlite, а шагам
	// пересоздания таблиц нужен отключённый FK на время миграции.
	if err := d.dialect.PreMigrate(ctx, d.writes); err != nil {
		return MigrateReport{From: from, To: v}, domain.SchemaMigrationFailed(
			v, v+1, fmt.Sprintf("pre migrate hook: %v", err))
	}
	// Восстановление режима соединения — на любом выходе после PreMigrate:
	// причина отказа важнее ошибки восстановления, отмена контекста его
	// не останавливает (статистику обслужит успешный повторный запуск).
	restore := func() {
		_ = d.dialect.RestoreMigrate(context.WithoutCancel(ctx), d.writes) //nolint:errcheck — best-effort
	}
	rep, err := d.applyMigrations(ctx, from)
	if err != nil {
		restore()
		return rep, err
	}
	if err := d.dialect.PostMigrate(ctx, d.writes); err != nil {
		// Схема уже актуальна: отказ обслуживания статистики — не отказ
		// миграции; повторный запуск сообщит «база актуальна».
		restore()
		return rep, fmt.Errorf("post migrate hook: %w", err)
	}
	return rep, nil
}

// applyMigrations — цикл шагов после успешного PreMigrate: от версии from
// до SchemaVersion. Отчёт несёт фактически применённые сессией шаги
// (Applied): проигранные конкурентам шаги в To входят, в Applied — нет.
func (d *DB) applyMigrations(ctx context.Context, from int) (MigrateReport, error) {
	rep := MigrateReport{From: from, To: from, Applied: []MigrationInfo{}}
	v := from
	for v < SchemaVersion {
		step, ok := migrationByTo(v + 1)
		if !ok {
			// Непрерывность цепочки закреплена пин-тестом реестра;
			// рассинхронизация — громкий отказ, а не молчаливая дыра версии.
			return rep, domain.SchemaMigrationFailed(
				v, v+1, "migration step is not registered")
		}
		err := d.applyStep(ctx, step, v)
		if errors.Is(err, errStepRace) {
			// Конкурент применил шаг: перечитываем версию и продолжаем
			// с актуального места (шаг конкурента уже зафиксирован).
			cur, rerr := d.SchemaVersion(ctx)
			if rerr != nil {
				return rep, rerr
			}
			if cur > SchemaVersion {
				// База ушла вперёд модуля (параллельная работа более новой
				// версии) — симметрично входной классификации: отказ.
				return rep, domain.SchemaVersionMismatch(cur, SchemaVersion)
			}
			if cur <= v {
				return rep, domain.SchemaMigrationFailed(
					v, step.To, "schema version did not advance after a lost race")
			}
			rep.To = cur
			v = cur
			continue
		}
		if err != nil {
			return rep, err
		}
		v = step.To
		rep.To = v
		rep.Steps++
		rep.Applied = append(rep.Applied, MigrationInfo{To: step.To, Desc: step.Desc})
	}
	return rep, nil
}

// migrationByTo — шаг реестра по целевой версии; ok=false — шаг не
// зарегистрирован.
func migrationByTo(to int) (Migration, bool) {
	for _, m := range migrations {
		if m.To == to {
			return m, true
		}
	}
	return Migration{}, false
}

// applyStep применяет один шаг миграции одной транзакцией: операторы DDL
// (через RewriteDDL, с ретраем гонки «already exists» параллельного
// писателя — как EnsureCreated), затем Data-хук, затем CAS-фиксация
// версии. RowsAffected = 0 у CAS — конкурент зафиксировал этот шаг раньше:
// откат всей транзакции шага (включая бэкфилл) и сигнал errStepRace.
func (d *DB) applyStep(ctx context.Context, m Migration, from int) error {
	fail := func(err error) error {
		return domain.SchemaMigrationFailed(from, m.To, err.Error())
	}
	tx, err := d.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	for _, stmt := range m.DDL {
		s := d.dialect.RewriteDDL(stmt)
		for attempt := 0; ; attempt++ {
			_, err := tx.exec(ctx, s, nil)
			if err == nil {
				break
			}
			if attempt < 3 && isAlreadyExistsError(err) {
				continue
			}
			return fail(err)
		}
	}
	if m.Data != nil {
		if err := m.Data(ctx, tx); err != nil {
			return fail(err)
		}
	}
	// CAS-фиксация версии — защита от параллельного migrate без
	// глобальных блокировок (план 09 §4): RowsAffected = 0 — конкурент
	// зафиксировал этот шаг раньше. Значения меты пишутся кодом модуля
	// чистыми числовыми строками, текстовое сравнение достаточно.
	res, err := tx.exec(ctx, `
UPDATE schema_meta SET value = @to
WHERE key = 'schema_version' AND value = @from`,
		map[string]any{"to": strconv.Itoa(m.To), "from": strconv.Itoa(from)})
	if err != nil {
		return fail(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fail(err)
	}
	if n == 0 {
		return errStepRace
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return nil
}
