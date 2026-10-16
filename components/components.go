// Package components — публичный фасад библиотеки для встраивания
// (docs/plan/01-architecture.md §2.4, docs/plan/04-module-functionality.md
// §1): тонкий реэкспорт внутреннего сервисного слоя, импортёра файлов
// наполнения и REST /api/v1 — вместе с типами domain и catalog из их
// сигнатур — без перестройки слоёв. Внешний модуль импортирует только
// этот пакет: пакеты internal/ за пределами модуля недоступны, алиасы
// открывают типы внутреннего API под публичными именами (это те же
// типы, не копии).
//
//	app, err := components.Open(ctx, components.Config{
//		Dialect: "sqlite", DSN: "catalog.db", EnsureCreated: true,
//	})
//	if err != nil { … }
//	defer app.Close()
//
//	mux.Handle("/api/v1/", app.Handler())
//	page, err := app.Services.Devices.Search(ctx, components.SearchQuery{
//		Kind: components.KindTransistor, Query: "КТ3",
//	})
//
// Готовый пример встраивания — examples/embed: внешний модуль с replace
// на локальный путь, сборка и запуск — фаза consumer QA-прогонщика.
package components

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/billydos/components-catalog/internal/httpapi"
	"github.com/billydos/components-catalog/internal/importer"
	"github.com/billydos/components-catalog/internal/service"
)

// Config — конфигурация открытия приложения (план 04 §1): хранилище
// (Dialect/DSN/Pool), журнал, EnsureCreated (DDL и сиды каталога на пустую
// базу) и опции REST-транспорта Handler(): DisableSuggest (не монтировать
// GET /suggest) и Metrics (хук счётчиков сайта).
type Config struct {
	Dialect        string // "sqlite" | "postgres"
	DSN            string // путь файла sqlite / DSN postgres
	Pool           PoolConfig
	Log            *slog.Logger
	EnsureCreated  bool
	DisableSuggest bool
	Metrics        Metrics
}

// Metrics — хук метрик REST (docs/plan/04-module-functionality.md §5):
// счётчики и длительности вызовов по операциям; реализация — на стороне
// сайта, модуль свой экспорт метрик не заводит. errCode — код ошибки
// домена ("" — успешный запрос).
type Metrics = httpapi.Metrics

// App — открытое приложение фасада: сервисный слой (Services), импортёр
// файлов наполнения (Importer) и REST-обработчик (Handler). Безопасно для
// одновременного использования несколькими горутинами; Close — обязанность
// владельца (сайт создаёт App при старте и закрывает при завершении).
type App struct {
	svc *service.App
	api *httpapi.API

	Services Services
	Importer *Importer
}

// Open открывает приложение по конфигурации (план 01 §2.4): сервисный слой
// поверх хранилища (при EnsureCreated — DDL и сиды каталога на пустую базу,
// docs/plan/02-database.md §5.3) и REST-транспорт /api/v1 над ним.
func Open(ctx context.Context, cfg Config) (*App, error) {
	svc, err := service.Open(ctx, service.Config{
		Dialect:       cfg.Dialect,
		DSN:           cfg.DSN,
		Pool:          cfg.Pool,
		Log:           cfg.Log,
		EnsureCreated: cfg.EnsureCreated,
	})
	if err != nil {
		return nil, err
	}
	return &App{
		svc:      svc,
		api:      httpapi.New(svc, httpapi.Config{Log: cfg.Log, DisableSuggest: cfg.DisableSuggest, Metrics: cfg.Metrics}),
		Services: svc.Services(),
		Importer: importer.New(svc),
	}, nil
}

// Close закрывает приложение (пулы соединений).
func (a *App) Close() error { return a.svc.Close() }

// Handler возвращает http.Handler REST /api/v1 для монтирования в роутер
// сайта (http.ServeMux: mux.Handle("/api/v1/", app.Handler())).
func (a *App) Handler() http.Handler { return a.api.Handler() }

// Snapshot возвращает актуальный снимок каталога (кэш с проверкой
// catalog_revision на каждом обращении).
func (a *App) Snapshot(ctx context.Context) (*Snapshot, error) {
	return a.svc.Snapshot(ctx)
}

// Revisions возвращает текущие счётчики catalog_revision и data_revision
// (ETag REST — docs/plan/02-database.md §5.5).
func (a *App) Revisions(ctx context.Context) (catalogRev, dataRev int64, err error) {
	return a.svc.Revisions(ctx)
}

// Stats собирает агрегаты приложения: количества записей по классам,
// версия схемы модуля и счётчики ревизий.
func (a *App) Stats(ctx context.Context) (Stats, error) { return a.svc.Stats(ctx) }
