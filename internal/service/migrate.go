package service

import (
	"context"

	"github.com/billydos/components-catalog/internal/storage"
)

// Миграции схемы (docs/plan/09-schema-migrations.md): явное действие —
// App.Migrate поднимает существующую базу шагами реестра storage до
// версии модуля, App.MigrationPlan строит план без записи (dry-run).
// Config.Migrate открывает прежнюю версию базы для явной миграции:
// обычное открытие (CheckSchema) её отвергает.

// MigrateReport — итог миграции: версия схемы до, после и число
// применённых шагов (From == To — база уже была актуальна).
type MigrateReport = storage.MigrateReport

// MigrationInfo — шаг миграции для планов и отчётов.
type MigrationInfo = storage.MigrationInfo

// MigrationPlan — план миграции: текущая версия схемы базы и неприменённые
// шаги (для dry-run и отчётов; без открытия транзакций записи).
type MigrationPlan struct {
	Current int
	Steps   []MigrationInfo
}

// Migrate доводит схему базы до версии модуля (план 09 §4): шаги
// применяются каждая одной транзакцией, падение оставляет базу на
// последней зафиксированной версии. Идемпотентна: актуальная база —
// отчёт From == To без единой транзакции записи. Журнал — по строке на
// фактически применённый шаг (slog info: версия, описание), в том числе
// при последующем сбое цепочки.
func (a *App) Migrate(ctx context.Context) (MigrateReport, error) {
	rep, err := a.db.Migrate(ctx)
	for _, s := range rep.Applied {
		a.log.Info("schema migration applied",
			"to", s.To, "desc", s.Desc)
	}
	return rep, err
}

// MigrationPlan возвращает план миграции: текущая версия схемы базы и
// шаги с To выше неё. Состояния меты вне диапазона поддержки — те же
// отказы, что и у Migrate (база новее модуля, старее базовой, без меты).
func (a *App) MigrationPlan(ctx context.Context) (MigrationPlan, error) {
	v, err := a.db.SchemaVersion(ctx)
	if err != nil {
		return MigrationPlan{}, err
	}
	if err := storage.ClassifySchemaVersion(v); err != nil {
		return MigrationPlan{}, err
	}
	steps := make([]MigrationInfo, 0)
	for _, s := range a.db.MigrationSteps() {
		if s.To > v {
			steps = append(steps, s)
		}
	}
	return MigrationPlan{Current: v, Steps: steps}, nil
}
