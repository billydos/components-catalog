// Команда embed — пример встраивания библиотеки components-catalog во
// внешний модуль (docs/plan/04-module-functionality.md §1). Зависимость
// подключена как у сайт-проекта: require на модуль + replace на локальный
// путь; пакеты internal/ отсюда не импортируются — только публичный фасад
// components. Прогон: открытие приложения, импорт файла наполнения
// (включая потоковый NDJSON с колбэком прогресса), поиск и карточка
// через сервисный слой, разбор обозначения, поток изменений, round-trip
// экспорт и REST /api/v1 через app.Handler().
//
// Сборка и запуск примера — фаза consumer QA-прогонщика: компилируемость
// у внешнего потребителя — часть контракта фасада.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/billydos/components-catalog/components"
)

// fill — минимальный файл наполнения на два транзистора (форма —
// docs/fill-format.md; коды параметров/условий — стартовый каталог сидов).
const fill = `{
	"transistors": [
		{
			"name": "КТ315Б",
			"attributes": {"structure": "npn", "yearFrom": 1967},
			"manufacturers": ["Восход"],
			"parameters": [
				{"parameter": "hfe", "min": 50, "max": 350, "Vce": 10, "Ic": 1}
			],
			"ratings": [
				{"parameter": "VceoMax", "value": 20}
			]
		},
		{
			"name": "BC547B",
			"attributes": {"structure": "npn"},
			"parameters": [
				{"parameter": "hfe", "min": 200, "max": 450, "Vce": 5, "Ic": 2}
			]
		}
	]
}`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "embed: %v\n", err)
		os.Exit(1)
	}
}

// run — тело main: отложенные закрытия выполняются до выхода с ненулевым
// кодом — os.Exit только в main.
func run() error {
	// База примера — одноразовый файл sqlite во временном каталоге.
	dir, err := os.MkdirTemp("", "embed-example-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) //nolint:errcheck — одноразовый каталог примера

	ctx := context.Background()

	// Открытие приложения: хранилище, сервисный слой, импортёр и REST.
	app, err := components.Open(ctx, components.Config{
		Dialect:       "sqlite",
		DSN:           filepath.Join(dir, "catalog.db"),
		EnsureCreated: true, // DDL + сиды каталога на пустую базу
	})
	if err != nil {
		return err
	}
	defer app.Close() //nolint:errcheck — закрытие при выходе

	// Импорт файла наполнения (jsonc/yaml/ndjson; формат — по расширению
	// имени). Повторный прогон того же файла — Skipped по всем записям.
	rep, err := app.Importer.Import(ctx, bytes.NewReader([]byte(fill)), "example.jsonc", false)
	if err != nil {
		return err
	}
	fmt.Printf("import: records=%d added=%d issues=%d\n",
		rep.Records, rep.Added, len(rep.Issues))

	// Потоковый импорт NDJSON с колбэком прогресса (CC-6): большие файлы
	// читаются построчно без загрузки целиком; колбэк наблюдает чтение
	// и итоговые исходы применения каждой записи.
	ndjson := "{\"transistors\": {\"name\": \"КТ361Б\", \"attributes\": {\"structure\": \"pnp\"}}}\n" +
		"{\"transistors\": {\"name\": \"BC557B\"}}\n"
	var last components.Progress
	rep, err = app.Importer.Import(ctx, strings.NewReader(ndjson), "example.ndjson", false,
		components.WithProgress(func(p components.Progress) { last = p }))
	if err != nil {
		return err
	}
	fmt.Printf("import ndjson: records=%d added=%d issues=%d (progress: read=%d applied=%d line=%d)\n",
		rep.Records, rep.Added, len(rep.Issues), last.Read, last.Applied, last.Line)

	// Поиск через сервисный слой: подстрока обозначения и класс.
	page, err := app.Services.Devices.Search(ctx, components.SearchQuery{
		Kind:  components.KindTransistor,
		Query: "КТ3",
	})
	if err != nil {
		return err
	}
	for _, item := range page.Items {
		fmt.Printf("search: %s (kind=%s system=%s id=%d)\n",
			item.Designation, item.Kind, item.System, item.ID)
	}

	// Карточка по стабильному id — для обозначений с «/» (MIL M39003/…).
	card, ok, err := app.Services.Devices.GetByID(ctx, page.Items[0].ID)
	if err != nil || !ok {
		return fmt.Errorf("get by id: ok=%v err=%v", ok, err)
	}
	fmt.Printf("card: %s, групп значений: %d\n", card.Designation, len(card.Groups))

	// Разбор обозначения с автодетектом класса и системы.
	parsed, err := app.Services.Designations.Parse(ctx, "КТ315Б")
	if err != nil {
		return err
	}
	fmt.Printf("parse: %s kind=%s system=%s\n",
		parsed.Designation, parsed.Kind, parsed.System)

	// Поток изменений для инкрементальной синхронизации производных
	// индексов (курсор — id последнего события).
	changes, err := app.Services.Devices.Changes(ctx, 0, components.ChangesDefaultLimit)
	if err != nil {
		return err
	}
	fmt.Printf("changes: events=%d data_revision=%d\n",
		len(changes.Events), changes.DataRevision)

	// REST /api/v1 — обычный http.Handler: монтируется в роутер сайта
	// (mux.Handle("/api/v1/", app.Handler()); здесь — прямой вызов).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/components?kind=transistor&q=КТ3", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return fmt.Errorf("GET /api/v1/components: статус %d: %s", rec.Code, rec.Body.String())
	}
	fmt.Printf("rest: GET /api/v1/components — %s\n", rec.Result().Status)

	// Round-trip экспорт записей (тот же формат наполнения).
	var out bytes.Buffer
	if err := app.Importer.Export(ctx, &out, components.FormatNDJSON, nil); err != nil {
		return err
	}
	fmt.Printf("export: %d байт ndjson\n", out.Len())

	fmt.Println("embed example: ok")
	return nil
}
