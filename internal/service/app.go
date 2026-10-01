package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/storage"
	"github.com/billydos/components-catalog/seed"
)

// Config — конфигурация открытия приложения (план 04 §1: диалект, DSN,
// пул, логгер). EnsureCreated выполняет DDL и применяет сиды каталога на
// пустую базу — только у команд записи (init/import/add); по умолчанию
// выключено: чтение требует инициализированную базу
// (database_not_initialized иначе).
type Config struct {
	Dialect       string // "sqlite" | "postgres"
	DSN           string // путь файла sqlite / DSN postgres
	Pool          PoolConfig
	Log           *slog.Logger
	EnsureCreated bool
}

// PoolConfig — параметры пулов соединений.
type PoolConfig struct {
	MaxOpen         int
	MaxIdle         int
	ConnMaxLifetime int64 // наносекунды; 0 — без ограничения
	BusyTimeout     int64 // миллисекунды (sqlite); 0 — по умолчанию
}

func (c Config) toStorage() storage.Config {
	return storage.Config{
		Dialect: c.Dialect,
		DSN:     c.DSN,
		Pool: storage.PoolConfig{
			MaxOpen:         c.Pool.MaxOpen,
			MaxIdle:         c.Pool.MaxIdle,
			ConnMaxLifetime: c.Pool.ConnMaxLifetime,
			BusyTimeout:     c.Pool.BusyTimeout,
		},
	}
}

// App — открытое приложение: хранилище, кэш каталога и сервисы.
// Безопасно для одновременного использования несколькими горутинами
// (stateless поверх пулов соединений; снимок каталога иммутабелен).
type App struct {
	db    *storage.DB
	cache *catalog.Cache
	log   *slog.Logger

	devices      *DeviceService
	designations *DesignationService
	catalogSvc   *CatalogService
}

// Services — доступ к сервисам приложения.
type Services struct {
	Devices      *DeviceService
	Designations *DesignationService
	Catalog      *CatalogService
}

// Open открывает приложение по конфигурации: открывает хранилище,
// при EnsureCreated выполняет DDL и применяет сиды каталога на пустую базу
// (plan/02-database.md §5.3), проверяет версию схемы (несовпадение —
// громкий отказ, продолжение работы запрещено).
func Open(ctx context.Context, cfg Config) (*App, error) {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	db, err := storage.Open(ctx, cfg.toStorage())
	if err != nil {
		return nil, err
	}
	if cfg.EnsureCreated {
		if _, err := db.EnsureCreated(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := db.CheckSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	app := &App{db: db, cache: catalog.NewCache(db), log: cfg.Log}
	app.devices = &DeviceService{app: app}
	app.designations = &DesignationService{app: app}
	app.catalogSvc = &CatalogService{app: app}
	if cfg.EnsureCreated {
		if err := app.seedIfEmpty(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	return app, nil
}

// Close закрывает приложение (пулы соединений).
func (a *App) Close() error { return a.db.Close() }

// Services возвращает набор сервисов приложения.
func (a *App) Services() Services {
	return Services{Devices: a.devices, Designations: a.designations, Catalog: a.catalogSvc}
}

// Snapshot возвращает актуальный снимок каталога (кэш с проверкой
// catalog_revision на каждом обращении).
func (a *App) Snapshot(ctx context.Context) (*catalog.Snapshot, error) {
	return a.cache.Snapshot(ctx)
}

// Revisions возвращает текущие счётчики catalog_revision и data_revision
// (ETag REST — plan/02-database.md §5.5).
func (a *App) Revisions(ctx context.Context) (catalogRev, dataRev int64, err error) {
	return a.db.Revisions(ctx)
}

// seedIfEmpty применяет сиды каталога на пустую базу (kinds пуста) —
// повторные запуски идемпотентны (upsert по коду).
func (a *App) seedIfEmpty(ctx context.Context) error {
	n, err := a.db.CountKinds(ctx)
	if err != nil {
		return fmt.Errorf("сидирование: %w", err)
	}
	if n > 0 {
		return nil
	}
	if err := a.catalogSvc.Import(ctx, seed.Catalog()); err != nil {
		return fmt.Errorf("сидирование: %w", err)
	}
	return nil
}
