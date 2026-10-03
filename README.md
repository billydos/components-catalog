# components-catalog

Локальный/встраиваемый справочник электронных компонентов (советская и
мировая элементная база): Go-библиотека с сервисным слоем, встраиваемый
версионируемый REST API `/api/v1` и CLI `catalogctl`. Хранилища: SQLite и
PostgreSQL (равноправные, pure Go). Четыре класса приборов — транзисторы,
диоды, резисторы, конденсаторы; системы обозначений ГОСТ/ОСТ,
PRO ELECTRON, JEDEC, JIS, серии и `other`.

Документация: `docs/fill-format.md` — регламент наполнения (формат файлов,
типичные ошибки, процедура); `CHANGELOG.md` — история релизов;
`qa/README.md` — сквозные прогоны и нагрузочная прикидка;
`qa/architecture-review.md` — ревью архитектуры против плана. Архитектура
и контракты — `docs/plan/` (`README.md` — решения D1–D8, `01`–`08` — разделы).

## Сборка и проверка

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

CLI: `go build ./cmd/catalogctl`. Справка команд: `catalogctl help`.
Сквозные сценарии CLI+REST на SQLite и PostgreSQL: `./qa/scenarios.sh`
(PostgreSQL — `CATALOG_TEST_POSTGRES_DSN`).

## Быстрый старт (CLI)

```
catalogctl init --db catalog.db
catalogctl import data/transistors.jsonc --db catalog.db
for f in data/*.jsonc; do catalogctl import "$f" --db catalog.db; done
catalogctl list --kind transistor --q КТ3 --db catalog.db
catalogctl info КТ315Б --db catalog.db
catalogctl find 2Т315Б --db catalog.db        # подсказка: КТ315Б
catalogctl export --format ndjson --db catalog.db
```

Формат файлов наполнения (jsonc/yaml/ndjson), семантика секций, единицы
и типичные ошибки — `docs/fill-format.md`. Примеры записей по классам и
системам обозначений — `docs/plan/06-examples.md`; выверенное наполнение в
репозитории — `data/` (транзисторы, диоды, резисторы, конденсаторы;
матрица исполнений К50-35, ряд мощностей С2-33Н, направленные аналоги) —
источники по каждой записи: `docs/plan/08-data-verification.md`. NDJSON-поток
для больших объёмов воспроизводится экспортом:
`catalogctl export --format ndjson`.

## Встраивание (Go API)

Модуль не запускает серверов и не читает глобальных флагов/env: сайт
создаёт `App` при старте и закрывает при завершении.

```go
app, err := service.Open(ctx, service.Config{
    Dialect: "sqlite", // "sqlite" | "postgres"
    DSN:     "catalog.db",
    EnsureCreated: true, // DDL + сиды каталога на пустую базу (команды записи)
})
if err != nil { … }
defer app.Close()

app.Services() // Devices / Designations / Catalog
```

## Монтирование REST в роутер сайта

`httpapi.New(app, cfg).Handler()` возвращает обычный `http.Handler` —
монтируется в любой роутер (аутентификация, авторизация, rate limiting
и CORS — на стороне сайта; запись монтируется только в защищённые
маршруты). Пример со стандартным `http.ServeMux`:

```go
mux := http.NewServeMux()
mux.Handle("/api/v1/", httpapi.New(app, httpapi.Config{
    // Log:            slog.Default(),           // nil — slog.Default()
    // DisableSuggest: true,                     // не монтировать GET /suggest
    // Metrics:        myMetrics,                // хук счётчиков сайта
}).Handler())
```

Маршруты: `GET /kinds`, `GET /catalog` (ETag по `catalog_revision`),
`GET /components` — поиск с фильтрами `attr.<код>`, `par.<код>.min/.max/.exact`,
`sort`, `limit ≤ 200`, `offset` (ETag по `data_revision`),
`GET|PUT|DELETE /components/{kind}/{designation}`, `POST /components`,
`GET /components/id/{id}`, `GET /suggest`, `GET /stats`. Полный контракт —
`api/openapi.yaml` (OpenAPI 3.1; совпадение с реализацией закреплено
тестом). Ошибки — JSON `{"code", "message", "details"}` со статусами
400/404/409/422/500.

Локализация (решение D9): база и формат наполнения языконезависимы —
каталог хранит только коды (включая латинские коды единиц `V`, `ohm`,
`pF`…); отображаемые названия, символы и тексты сообщений — бандлы
модуля (en — канонический, ru — полный словарь). REST заполняет
отображаемые поля (`name`, `kind_name`, имена и символы единиц в
`/catalog`, текст `message` ошибок) по заголовку `Accept-Language`
(по умолчанию `en`), CLI — флагом `--lang en|ru` (включая заголовки,
справку и итоги прогона, значения форматируются с инженерными
приставками: мкФ, МОм, кГц). Кодовые поля (`kind`, `system`, `unit`…)
и машиночитаемые коды ошибок от локали не зависят.

Обозначения с символом `/` (MIL-имена `M39003/…`) адресуются через
`GET /components/id/{id}` или поиск — прямой путь для них не
гарантируется.

## Импорт/экспорт

```
catalogctl import data/transistors.jsonc --db catalog.db
catalogctl import bulk.ndjson --db catalog.db          # потоковый формат
catalogctl import data/transistors.jsonc --dry-run --db catalog.db
catalogctl export --format ndjson --db catalog.db
catalogctl catalog export|import|list --db catalog.db
```

Импорт идемпотентен: повторный прогон того же файла — «без изменений»
по всем записям, без инкремента ревизий. Каталог (параметры, атрибуты,
условия, семейства) расширяется секцией `catalog` файлов наполнения —
без правки кода и пересборки.

## Производительность

Ориентиры первого релиза: поиск с фильтрами ≤ 100 мс, карточка ≤ 20 мс
на 10⁴–10⁵ устройств. Подтверждено прикидкой на 10⁴ (SQLite, локально);
на 10⁵ карточки/подстрока/suggest в норме, фильтрованный поиск (EXISTS
по EAV) на SQLite превышает ориентир — детали и план Б (R2):
`qa/reports/2026-10-02-stage7.md`.

## Лицензия

MIT.
