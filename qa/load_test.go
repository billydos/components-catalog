package qa

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/importer"
	"github.com/billydos/components-catalog/internal/service"
	"github.com/billydos/components-catalog/internal/testutil"
)

// Ориентиры производительности первого релиза — plan/04-module-functionality.md §5:
// поиск с фильтрами по 10⁴–10⁵ устройств ≤ 100 мс, карточка ≤ 20 мс.
// Прикидка — измерение, а не порог: медианы серий сравниваются с ориентирами
// и попадают в отчёт прогона (qa/reports/); превышения отмечаются меткой
// «ПРЕВЫШЕНИЕ» и в обычном режиме не валят прогон (план Б по риску R2 —
// денормализация — отложен за пределы v0.1). CATALOG_QA_STRICT=1 — строгий
// режим: любое превышение цели на порядок — отказ.
const (
	targetSearch = 100 * time.Millisecond
	targetCard   = 20 * time.Millisecond
	failMargin   = 10 // строгий режим: провал — превышение цели в failMargin раз
	warmupRuns   = 3
	measuredRuns = 10
)

// TestLoad — нагрузочная прикидка (этап 7.1). Запуск явный:
//
//	CATALOG_QA_SCALE=10000 go test ./qa -run TestLoad -v -timeout 30m
//	CATALOG_QA_DIALECT=postgres CATALOG_TEST_POSTGRES_DSN=postgres://… \
//	  CATALOG_QA_SCALE=10000 go test ./qa -run TestLoad -v -timeout 30m
//
// Без CATALOG_QA_SCALE тест пропускается (обычный `go test ./...`
// нагрузочной прикидки не выполняет). Генерируется NDJSON-файл синтетических
// транзисторов JEDEC (2N#### с суффиксами) с параметрами, условиями и
// атрибутами; импорт идёт боевым путём (importer), измерения — сервисным
// слоем (REST добавляет лишь сериализацию).
func TestLoad(t *testing.T) {
	scaleStr := os.Getenv("CATALOG_QA_SCALE")
	if scaleStr == "" {
		t.Skip("CATALOG_QA_SCALE не задан — нагрузочная прикидка опущена (регламент: qa/README.md)")
	}
	scale, err := strconv.Atoi(scaleStr)
	if err != nil || scale <= 0 {
		t.Fatalf("CATALOG_QA_SCALE: ожидается положительное целое, получено %q", scaleStr)
	}
	dialect := os.Getenv("CATALOG_QA_DIALECT")
	if dialect == "" {
		dialect = "sqlite"
	}
	dsn := os.Getenv("CATALOG_QA_DB")
	if dsn == "" {
		dsn = filepath.Join(t.TempDir(), "load.db")
	}
	if dialect == "postgres" {
		dsn = testutil.PostgresDSN(t)
		testutil.DropAllTables(t, dsn)
	} else if os.Getenv("CATALOG_QA_DB") == "" {
		t.Cleanup(func() { os.Remove(dsn) }) //nolint:errcheck — временная база прогона
	}

	ctx := context.Background()
	app, err := service.Open(ctx, service.Config{
		Dialect: dialect, DSN: dsn, EnsureCreated: true,
	})
	if err != nil {
		t.Fatalf("открытие базы (%s): %v", dialect, err)
	}
	defer app.Close() //nolint:errcheck — закрытие при выходе

	// Генерация NDJSON-файла синтетики (JEDEC: номер без ведущего нуля,
	// до четырёх цифр + суффиксные буквы).
	path := filepath.Join(t.TempDir(), "load.ndjson")
	if err := writeLoadDataset(path, scale); err != nil {
		t.Fatalf("генерация датасета: %v", err)
	}

	// Импорт боевым путём: разбор формата + валидация движком + upsert
	// (транзакция на запись — реальный путь наполнения).
	importStart := time.Now()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открытие датасета: %v", err)
	}
	rep, err := importer.New(app).Import(ctx, f, path, false)
	closeErr := f.Close()
	if err != nil {
		t.Fatalf("импорт: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("закрытие датасета: %v", closeErr)
	}
	importTime := time.Since(importStart)
	if rep.Rejected > 0 || rep.HasIssues() {
		for _, issue := range rep.Issues {
			t.Errorf("проблема импорта: %s", issue.String())
		}
		t.Fatalf("импорт: отвергнуто записей: %d", rep.Rejected)
	}
	if rep.Added != scale {
		t.Fatalf("импорт: добавлено %d записей, ожидалось %d", rep.Added, scale)
	}
	t.Logf("диалект=%s записей=%d импорт=%v (%.0f записей/с)", dialect, scale, importTime,
		float64(scale)/importTime.Seconds())

	// Опорные записи для измерений карточки.
	page, err := app.Services().Devices.Search(ctx, service.SearchQuery{
		Kind: domain.KindTransistor, Query: "2N3", Limit: 1,
	})
	if err != nil {
		t.Fatalf("поиск: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("поиск не вернул записей")
	}
	probe := page.Items[0]

	// Измерения: медиана measuredRuns прогонов после warmupRuns разогревочных.
	strict := os.Getenv("CATALOG_QA_STRICT") == "1"
	h21eMin := 100.0
	bench := func(name string, target time.Duration, run func() error) {
		for i := 0; i < warmupRuns; i++ {
			if err := run(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		samples := make([]time.Duration, 0, measuredRuns)
		for i := 0; i < measuredRuns; i++ {
			start := time.Now()
			if err := run(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			samples = append(samples, time.Since(start))
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		median := samples[len(samples)/2]
		mark := "ok"
		if median > target {
			mark = "ПРЕВЫШЕНИЕ"
		}
		t.Logf("%-42s p50=%-14v цель≤%v  %s", name, median, target, mark)
		if strict && median > target*failMargin {
			t.Errorf("%s: медиана %v превышает цель %v более чем в %d раз (R2)",
				name, median, target, failMargin)
		}
	}

	devices := app.Services().Devices
	designations := app.Services().Designations

	bench("поиск: подстрока обозначения", targetSearch, func() error {
		p, err := devices.Search(ctx, service.SearchQuery{Kind: domain.KindTransistor, Query: "2N3", Limit: 50})
		if err != nil {
			return err
		}
		if len(p.Items) == 0 {
			return fmt.Errorf("пустой результат")
		}
		return nil
	})
	bench("поиск: фильтры полей", targetSearch, func() error {
		_, err := devices.Search(ctx, service.SearchQuery{
			Kind: domain.KindTransistor,
			Fields: []service.FieldFilter{
				{Field: "junctions", Num: 2, HasNum: true, Op: service.OpEq},
				{Field: "dev_number", Num: 1000, HasNum: true, Op: service.OpEq},
			},
			Limit: 50,
		})
		return err
	})
	bench("поиск: параметрические фильтры", targetSearch, func() error {
		ukeoMax := 200.0
		_, err := devices.Search(ctx, service.SearchQuery{
			Kind: domain.KindTransistor,
			Parameters: []service.ParameterFilter{
				{Parameter: "h21e", Min: &h21eMin},
				{Parameter: "UkeoMax", Max: &ukeoMax},
			},
			Limit: 50,
		})
		return err
	})
	bench("поиск: параметрический фильтр (селективный)", targetSearch, func() error {
		h21sel := 195.0
		_, err := devices.Search(ctx, service.SearchQuery{
			Kind:       domain.KindTransistor,
			Parameters: []service.ParameterFilter{{Parameter: "h21e", Min: &h21sel}},
			Limit:      50,
		})
		return err
	})
	bench("поиск: фильтр атрибута", targetSearch, func() error {
		_, err := devices.Search(ctx, service.SearchQuery{
			Kind:       domain.KindTransistor,
			Attributes: []service.AttributeFilter{{Attribute: "package", Text: "TO-92"}},
			Limit:      50,
		})
		return err
	})
	bench("карточка: по стабильному id", targetCard, func() error {
		_, found, err := devices.GetByID(ctx, probe.ID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("запись %d не найдена", probe.ID)
		}
		return nil
	})
	bench("карточка: по ключу (kind, designation)", targetCard, func() error {
		_, found, err := devices.Get(ctx, domain.KindTransistor, probe.Designation)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("запись %s не найдена", probe.Designation)
		}
		return nil
	})
	bench("suggest: префикс", targetSearch, func() error {
		s, err := designations.Suggest(ctx, "2N3", domain.KindTransistor, 10)
		if err != nil {
			return err
		}
		if len(s) == 0 {
			return fmt.Errorf("пустой результат")
		}
		return nil
	})
}

// writeLoadDataset пишет NDJSON-датасет синтетических транзисторов JEDEC:
// обозначения 2N<номер><буквы> (уникальность — комбинацией номера и суффикса),
// значения параметров с условиями измерения, атрибуты. Числа варьируются,
// чтобы параметрические фильтры были селективными.
func writeLoadDataset(path string, scale int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	letters := suffixAlphabet()
	for i := 0; i < scale; i++ {
		number := 1000 + i%9000
		suffix := letters[i/9000]
		h21min := 20 + i%180
		fgran := 5 + i%500
		ukeo := 15 + i%300
		pk := 50 + i%800
		mass := 0.2 + float64(i%50)/100
		fmt.Fprintf(w,
			`{"transistors":{"name":"2N%d%s","system":"jedec","attributes":{"package":"TO-92","structure":"n-p-n"},`+
				`"parameters":[{"parameter":"h21e","min":%d,"max":%d,"Ik":1,"Uke":5},{"parameter":"FGran","min":%d,"Ik":1,"Uke":5}],`+
				`"ratings":[{"parameter":"UkeoMax","value":%d},{"parameter":"PkMax","value":%d}],`+
				`"dimensions":[{"parameter":"massMax","value":%.2f}]}}`+"\n",
			number, suffix, h21min, h21min+20+i%60, fgran, ukeo, pk, mass)
	}
	if err := w.Flush(); err != nil {
		f.Close() //nolint:errcheck — ошибка создания уже возвращена/логируется
		return err
	}
	return f.Close()
}

// suffixAlphabet — суффиксы JEDEC для уникальности обозначений: A..Z, AA..ZZ
// (номер регистрации 1000–9999 × суффиксы покрывают 10⁴–10⁵ записей).
func suffixAlphabet() []string {
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		out = append(out, string(c))
	}
	for a := 'A'; a <= 'Z'; a++ {
		for b := 'A'; b <= 'Z'; b++ {
			out = append(out, string(a)+string(b))
		}
	}
	return out
}
