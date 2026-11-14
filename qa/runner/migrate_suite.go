// Фаза migrate (план 09 §9): база-фикстура версии SchemaBaseline →
// catalogctl migrate → повторный запуск («база актуальна», код 0) →
// --dry-run (пустой план) → CLI-смоук против мигрированной базы.
// Фикстура sqlite — файл qa/fixtures/schema_v<N>.sqlite (создан релизной
// сборкой версии N: init + импорт выверенного файла data/), копируется
// во временный каталог — файл репозитория не мутируется. Фикстура
// postgres — схема замороженного среза DDL (storage.CreateFixture);
// данные догоняются штатным import уже над мигрированной базой.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/storage"
)

// migrateStepRE — строка шага плана dry-run («N: описание»).
var migrateStepRE = regexp.MustCompile(`(?m)^\s*\d+: `)

// fixtureSQLiteName — имя файла sqlite-фикстуры baseline.
func fixtureSQLiteName() string {
	return fmt.Sprintf("schema_v%d.sqlite", storage.SchemaBaseline)
}

// copyFile копирует файл (байт-в-байт).
func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck — чтение копии
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close() //nolint:errcheck — уже отказ
		return err
	}
	return out.Close()
}

// prepareMigrateFixture поднимает базу-фикстуру ноги: sqlite — копия
// файла qa/fixtures, postgres — схема фикстуры через storage. Сброс
// базы ноги уже выполнен подготовкой прогона (env-конвенция или
// -reset-postgres); при -postgres-dsn без -reset-postgres фаза работает
// по фактическому состоянию чужой базы — CreateFixture идемпотентен по
// CREATE IF NOT EXISTS, заметка в отчёте фиксирует это.
func prepareMigrateFixture(ctx context.Context, r *report, root, tmp string, l leg, reset bool) ([]string, bool) {
	switch l.name {
	case "sqlite":
		src := filepath.Join(root, "qa", "fixtures", fixtureSQLiteName())
		dst := filepath.Join(tmp, "migrate-fixture.db")
		if err := copyFile(dst, src); err != nil {
			r.check("migrate/fixture", "копия фикстуры "+fixtureSQLiteName(), false, err.Error())
			return nil, false
		}
		return []string{"--db", dst}, true
	case "postgres":
		if !reset {
			r.note("фикстура postgres поднимается без сброса базы (-postgres-dsn без -reset-postgres): " +
				"существующие таблицы модуля остаются, проверки фазы идут по ним")
		}
		db, err := storage.Open(ctx, storage.Config{
			Dialect: "postgres",
			DSN:     l.dbArgs[len(l.dbArgs)-1],
		})
		if err != nil {
			r.check("migrate/fixture", "открытие базы для фикстуры postgres", false, err.Error())
			return nil, false
		}
		defer db.Close() //nolint:errcheck — подключение только для фикстуры
		if err := db.CreateFixture(ctx, storage.SchemaBaseline); err != nil {
			r.check("migrate/fixture", "схема фикстуры postgres (CreateFixture)", false, err.Error())
			return nil, false
		}
		return l.dbArgs, true
	}
	r.check("migrate/fixture", "фикстура диалекта "+l.name, false, "неизвестный диалект ноги")
	return nil, false
}

// migrateSuite — сценарии фазы migrate на ноге (фикстура → применение →
// идемпотентность → dry-run → CLI/REST-смоук против мигрированной базы;
// план 09 §9).
func migrateSuite(ctx context.Context, r *report, ctl, srv, root, tmp string, l leg, reset bool) {
	r.section("phase: migrate " + l.name)
	dbArgs, ok := prepareMigrateFixture(ctx, r, root, tmp, l, reset)
	if !ok {
		return
	}
	c := cliRunner{bin: ctl, root: root, db: dbArgs}

	// Применение к базе фикстуры: итог зависит от реестра модуля. Пока
	// SchemaVersion == SchemaBaseline (реестр пуст), мигрировать нечего —
	// «база уже на версии»; с шагами обязана печататься «migrated: …».
	res := c.run(ctx, "migrate")
	wantApply := strings.Contains(res.output, "migrated: ")
	if storage.SchemaBaseline == storage.SchemaVersion {
		wantApply = strings.Contains(res.output, "database is already at version")
	}
	r.check("migrate/apply", "migrate: применение шагов к базе фикстуры",
		res.code == 0 && wantApply, envDetails(res, 10)...)

	// Повторный запуск — «база актуальна», код 0 (идемпотентность).
	res = c.run(ctx, "migrate")
	r.check("migrate/idempotent", "повторный migrate — база актуальна, код 0",
		res.code == 0 && strings.Contains(res.output, "database is already at version"),
		envDetails(res, 10)...)

	// dry-run: заголовок плана без строк шагов и без записи.
	res = c.run(ctx, "migrate", "--dry-run")
	r.check("migrate/dry-run", "migrate --dry-run: план без записи (пуст)",
		res.code == 0 && strings.Contains(res.output, "migration plan") &&
			!migrateStepRE.MatchString(res.output),
		envDetails(res, 10)...)

	// Смоук против мигрированной базы: данные фикстуры (sqlite) либо
	// импорт над мигрированной базой (postgres), карточка известной
	// записи. Импорт транзисторов идемпотентен для готовой базы фикстуры.
	res = c.run(ctx, "import", filepath.Join("data", "transistors.jsonc"))
	r.check("migrate/import", "import над мигрированной базой",
		res.code == 0, envDetails(res, 10)...)
	res = c.run(ctx, "count")
	n, err := strconv.Atoi(strings.TrimSpace(res.output))
	r.check("migrate/count", "count: записи мигрированной базы",
		res.code == 0 && err == nil && n > 0, envDetails(res, 5)...)
	res = c.run(ctx, "info", "КТ315Б")
	r.check("migrate/info", "info: карточка записи мигрированной базы",
		res.code == 0 && strings.Contains(res.output, "(gost)"),
		envDetails(res, 10)...)

	// REST-смоук против той же мигрированной базы (план 09 §9): хост
	// поднимается на dbArgs фазы, пара запросов контракта.
	if h, ok := startAPISrv(ctx, r, srv, tmp, "restsrv-migrate-"+l.name+".log", dbArgs); ok {
		cl := restClient{base: "http://" + h.addr + "/api/v1",
			client: &http.Client{Timeout: requestTimeout}}
		status, body, _ := cl.do(ctx, http.MethodGet, "/components?limit=1", "", nil)
		r.check("migrate/rest-components", "GET /components против мигрированной базы: 200",
			status == http.StatusOK && strings.Contains(body, "\"items\""),
			detail(status, body)...)
		status, body, _ = cl.do(ctx, http.MethodGet, "/stats", "", nil)
		r.check("migrate/rest-stats", "GET /stats против мигрированной базы: 200, ревизии",
			status == http.StatusOK && strings.Contains(body, "schema_version"),
			detail(status, body)...)
		h.shutdown()
	}
}
