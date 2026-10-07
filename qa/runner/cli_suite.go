// Сценарии CLI: сквозной прогон catalogctl против ноги прогона (sqlite или
// postgres). Проверки утверждают канонический вывод en (D9 — en каноничен,
// локаль CLI по умолчанию), поэтому прогон не зависит от русских текстов.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// leg — нога прогона: СУБД и флаги подключения catalogctl/restsrv.
type leg struct {
	name   string
	dbArgs []string
}

// cliRunner — catalogctl с зафиксированными флагами подключения.
type cliRunner struct {
	bin  string
	root string
	db   []string
}

// run выполняет команду catalogctl (вывод stdout+stderr объединён).
func (c cliRunner) run(ctx context.Context, args ...string) cmdResult {
	return runCmd(ctx, c.root, cmdTimeout, c.bin, append(args, c.db...)...)
}

// runOut выполняет команду с перенаправлением stdout в файл.
func (c cliRunner) runOut(ctx context.Context, outPath string, args ...string) cmdResult {
	return runCmdOut(ctx, c.root, outPath, cmdTimeout, c.bin, append(args, c.db...)...)
}

// importCountRE — число записей в итоге импорта («records: N, …»).
var importCountRE = regexp.MustCompile(`records: (\d+)`)

// importCount — суммарное число записей файла из итога импорта.
func importCount(output string) (int, bool) {
	m := importCountRE.FindStringSubmatch(output)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// dataFiles — выверенная выборка наполнения (все классы).
var dataFiles = []string{
	"transistors", "diodes", "resistors", "capacitors",
}

// cliSuite прогоняет сценарии CLI на ноге; возвращает суммарное число
// проверок прогона не останавливая на отказах (итог — в отчёте).
func cliSuite(ctx context.Context, r *report, ctl, root, tmp string, l leg) {
	r.section("phase: cli " + l.name)
	c := cliRunner{bin: ctl, root: root, db: l.dbArgs}

	// init: база создаётся и инициализируется сидами каталога.
	res := c.run(ctx, "init")
	r.check("cli/init", "init: база инициализирована",
		res.code == 0 && strings.Contains(res.output, "database initialized"),
		envDetails(res, 10)...)

	// import: выверенная выборка целиком; счётчики по классам для
	// последующих проверок идемпотентности.
	total := 0
	perFile := map[string]int{}
	for _, name := range dataFiles {
		path := filepath.Join("data", name+".jsonc")
		res = c.run(ctx, "import", path)
		n, ok := importCount(res.output)
		if ok {
			perFile[name] = n
			total += n
		}
		r.check("cli/import/"+name, "import: "+path,
			res.code == 0 && ok && n > 0, envDetails(res, 10)...)
	}

	// Идемпотентность: повторный импорт — «без изменений» по всем записям,
	// без отвергнутых и без записи в базу.
	res = c.run(ctx, "import", filepath.Join("data", "transistors.jsonc"))
	r.check("cli/idempotent", "повторный импорт — без изменений и отказов",
		res.code == 0 &&
			strings.Contains(res.output, "unchanged: "+strconv.Itoa(perFile["transistors"])) &&
			!strings.Contains(res.output, "rejected"),
		envDetails(res, 10)...)

	// count: суммарное число записей выборки.
	res = c.run(ctx, "count")
	got, err := strconv.Atoi(strings.TrimSpace(res.output))
	r.check("cli/count", "count: записи выборки",
		res.code == 0 && err == nil && got == total,
		envDetails(res, 5)...)

	// list: подстрока обозначения и фильтр класса.
	res = c.run(ctx, "list", "--kind", "transistor", "--q", "КТ3", "--limit", "10")
	r.check("cli/list-substr", "list: подстрока обозначения и фильтр класса",
		res.code == 0 && strings.Contains(res.output, "КТ315Б"),
		envDetails(res, 10)...)

	// list: фильтр системы обозначений.
	res = c.run(ctx, "list", "--kind", "resistor", "--system", "gost")
	r.check("cli/list-system", "list: фильтр системы обозначений",
		res.code == 0 && strings.Contains(res.output, "С2-33Н"),
		envDetails(res, 10)...)

	// info: карточка транзистора (система gost) и матрица исполнений
	// конденсатора.
	res = c.run(ctx, "info", "КТ315Б")
	r.check("cli/info-gost", "info: карточка транзистора (система gost)",
		res.code == 0 && strings.Contains(res.output, "(gost)"),
		envDetails(res, 10)...)
	res = c.run(ctx, "info", "К50-35")
	r.check("cli/info-variants", "info: матрица исполнений конденсатора",
		res.code == 0 && strings.Contains(res.output, "Variant «"),
		envDetails(res, 10)...)

	// find: точное совпадение и подсказка равнозначного по материалу
	// (код выхода 1).
	res = c.run(ctx, "find", "КТ315Б")
	r.check("cli/find-exact", "find: точное совпадение",
		res.code == 0 && strings.Contains(res.output, "КТ315Б — "),
		envDetails(res, 10)...)
	res = c.run(ctx, "find", "2Т315Б")
	r.check("cli/find-suggestion", "find: подсказка равнозначного по материалу, код 1",
		res.code == 1 && strings.Contains(res.output, "equivalent by material: КТ315Б"),
		envDetails(res, 10)...)

	// parse: автодетект без базы, JEDEC и неверное обозначение
	// («Error: », код 1).
	res = c.run(ctx, "parse", "КТ315Б")
	r.check("cli/parse-autodetect", "parse: автодетект без базы",
		res.code == 0 && strings.Contains(res.output, "transistor") &&
			strings.Contains(res.output, "gost"),
		envDetails(res, 10)...)
	res = c.run(ctx, "parse", "2N2222")
	r.check("cli/parse-jedec", "parse: JEDEC",
		res.code == 0 && strings.Contains(res.output, "jedec"),
		envDetails(res, 10)...)
	res = c.run(ctx, "parse", "2222")
	r.check("cli/parse-invalid", "parse: неверное обозначение — «Error: », код 1",
		res.code == 1 && strings.Contains(res.output, "Error: "),
		envDetails(res, 10)...)

	// round-trip: экспорт ndjson по классу и yaml полностью → импорт
	// без изменений.
	capsPath := filepath.Join(tmp, "caps.ndjson")
	res = c.runOut(ctx, capsPath, "export", "--kind", "capacitor", "--format", "ndjson")
	importRes := c.run(ctx, "import", capsPath)
	r.check("cli/roundtrip-ndjson", "round-trip: экспорт ndjson → импорт без изменений",
		res.code == 0 && importRes.code == 0 &&
			strings.Contains(importRes.output, "unchanged: "+strconv.Itoa(perFile["capacitors"])) &&
			!strings.Contains(importRes.output, "rejected"),
		envDetails(importRes, 10)...)

	allPath := filepath.Join(tmp, "all.yaml")
	res = c.runOut(ctx, allPath, "export", "--format", "yaml")
	importRes = c.run(ctx, "import", allPath)
	r.check("cli/roundtrip-yaml", "round-trip: полный экспорт yaml → импорт без изменений",
		res.code == 0 && importRes.code == 0 &&
			strings.Contains(importRes.output, "unchanged: "+strconv.Itoa(total)) &&
			!strings.Contains(importRes.output, "rejected"),
		envDetails(importRes, 10)...)

	// dry-run: контрольный прогон без записи.
	res = c.run(ctx, "import", filepath.Join("data", "diodes.jsonc"), "--dry-run")
	r.check("cli/dry-run", "import --dry-run: контрольный прогон без записи",
		res.code == 0 &&
			strings.Contains(res.output, "dry run (no database write)") &&
			strings.Contains(res.output, "unchanged: "+strconv.Itoa(perFile["diodes"])),
		envDetails(res, 10)...)

	// delete: удаление с каскадом, запись отсутствует, count уменьшился.
	res = c.run(ctx, "delete", "КД522А")
	r.check("cli/delete", "delete: удаление с каскадом",
		res.code == 0 && strings.Contains(res.output, "deleted"),
		envDetails(res, 10)...)
	res = c.run(ctx, "info", "КД522А")
	r.check("cli/deleted-gone", "delete: запись отсутствует, код 1",
		res.code == 1 && strings.Contains(res.output, "not found"),
		envDetails(res, 10)...)
	res = c.run(ctx, "count")
	got, err = strconv.Atoi(strings.TrimSpace(res.output))
	r.check("cli/count-after-delete", fmt.Sprintf("count: %d после удаления", total-1),
		res.code == 0 && err == nil && got == total-1,
		envDetails(res, 5)...)

	// Битый файл импорта: «Error: », код 1.
	brokenPath := filepath.Join(tmp, "broken.jsonc")
	if err := os.WriteFile(brokenPath, []byte("{ broken json"), 0o644); err != nil {
		r.check("cli/broken-import", "import: битый файл", false, err.Error())
	} else {
		res = c.run(ctx, "import", brokenPath)
		r.check("cli/broken-import", "import: битый файл — «Error: », код 1",
			res.code == 1 && strings.Contains(res.output, "Error: "),
			envDetails(res, 10)...)
	}

	// catalog list: справка из каталога (параметры, код h21e).
	res = c.run(ctx, "catalog", "list")
	r.check("cli/catalog-list", "catalog list: справка из каталога",
		res.code == 0 && strings.Contains(res.output, "Parameters:") &&
			strings.Contains(res.output, "h21e"),
		envDetails(res, 15)...)
}
