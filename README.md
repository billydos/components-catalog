# components-catalog

Локальный/встраиваемый справочник электронных компонентов (советская и
мировая элементная база): Go-библиотека с сервисным слоем, встраиваемый
версионируемый REST API `/api/v1` и CLI `catalogctl`. Хранилища: SQLite и
PostgreSQL (равноправные, pure Go). Архитектура и контракты — `plan/`
(`README.md` — решения, `01`–`06` — разделы плана).

## Сборка и проверка

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

CLI: `go build ./cmd/catalogctl`. Справка команд: `catalogctl help`.

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

Обозначения с символом `/` (MIL-имена `M39003/…`) адресуются через
`GET /components/id/{id}` или поиск — прямой путь для них не
гарантируется.

## Импорт/экспорт

Файлы наполнения jsonc/yaml/ndjson (примеры — `sample-data/`):

```
catalogctl import sample-data/transistors.jsonc --db catalog.db
catalogctl export --format ndjson --db catalog.db
catalogctl catalog export|import|list --db catalog.db
```

## Лицензия

MIT.
