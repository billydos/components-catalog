package qa

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/importer"
	"github.com/billydos/components-catalog/internal/service"
	"github.com/billydos/components-catalog/internal/testutil"
)

// Ориентиры производительности первого релиза — docs/plan/04-module-functionality.md §5:
// поиск с фильтрами по 10⁴–10⁵ устройств ≤ 100 мс, карточка ≤ 20 мс.
// Прикидка — измерение, а не порог: медиана (p50) и 95-й перцентиль (p95)
// серий попадают в отчёт прогона (qa/reports/); с ориентирами сравнивается
// медиана — превышение отмечается меткой «ПРЕВЫШЕНИЕ» и в обычном режиме
// не валят прогон (план Б по риску R2 — денормализация — отложен за
// пределы v0.1). CATALOG_QA_STRICT=1 — строгий режим: любое превышение
// цели на порядок — отказ.
const (
	targetSearch = 100 * time.Millisecond
	targetCard   = 20 * time.Millisecond
	failMargin   = 10 // строгий режим: провал — превышение цели в failMargin раз
	warmupRuns   = 3
	measuredRuns = 10
)

// TestLoad — нагрузочная прикидка (этап 7.1). Запуск явный:
//
//	CATALOG_QA_SCALE=10000 go test ./qa -run TestLoad -count=1 -v -timeout 30m
//	CATALOG_QA_DIALECT=postgres CATALOG_TEST_POSTGRES_DSN=postgres://… \
//	  CATALOG_QA_SCALE=10000 go test ./qa -run TestLoad -count=1 -v -timeout 30m
//
// Без CATALOG_QA_SCALE тест пропускается (обычный `go test ./...`
// нагрузочной прикидки не выполняет). Генерируется NDJSON-файл синтетических
// транзисторов JEDEC (2N#### с суффиксами) с параметрами, условиями и
// атрибутами; импорт идёт боевым путём (importer), измерения — сервисным
// слоем (REST добавляет лишь сериализацию).
//
// База sqlite — одноразовый каталог qa/tmp внутри репозитория: на реальном
// диске, а не в каталоге ОС (часть систем держит /tmp в tmpfs — памяти,
// и прикидка импорта перестала бы касаться диска вовсе). Явный CATALOG_QA_DB
// задаёт путь файла и сохраняет его после прогона.
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
	if dialect == "postgres" {
		dsn = testutil.PostgresDSN(t)
		testutil.DropAllTables(t, dsn)
	} else if dsn == "" {
		dsn = filepath.Join(loadDBDir(t), "load.db")
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
		t.Fatalf("импорт: added %d записей, ожидалось %d", rep.Added, scale)
	}
	dbLabel := dsn
	if dialect == "postgres" {
		dbLabel = "postgres" // DSN может содержать учётные данные — не печатается
	}
	t.Logf("диалект=%s база=%s записей=%d импорт=%v (%.0f записей/с)",
		dialect, dbLabel, scale, importTime.Round(time.Millisecond),
		float64(scale)/importTime.Seconds())

	// Опорные записи для измерений карточки.
	page, err := app.Services().Devices.Search(ctx, service.SearchQuery{
		Kind: domain.KindTransistor, Query: "2N3", Limit: 1,
	})
	if err != nil {
		t.Fatalf("поиск: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatalf("поиск опорной записи не вернул записей: сценарии ищут подстроку «2N3» — масштаб %d мал, прикидка рассчитана на 10⁴–10⁵", scale)
	}
	probe := page.Items[0]

	// Измерения: медиана и p95 measuredRuns прогонов после warmupRuns
	// разогревочных (p95 при 10 прогонах — худший прогон серии).
	strict := os.Getenv("CATALOG_QA_STRICT") == "1"
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
		slices.Sort(samples)
		median := percentile(samples, 50)
		p95 := percentile(samples, 95)
		mark := "ok"
		if median > target {
			mark = "ПРЕВЫШЕНИЕ"
		}
		// 44 — самое длинное имя сценария (43 руны) + зазор; длительности —
		// фиксированный millis, чтобы колонки строки были ровными.
		t.Logf("%-44s p50=%-10s p95=%-10s цель≤%-6v %s",
			name, millis(median), millis(p95), target, mark)
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
		h21eMin := 100.0
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

// percentile — перцентиль возрастающей выборки методом ближайшего ранга:
// pct в целых процентах, index = ceil(pct·n/100) − 1 — без плавающей
// арифметики (при measuredRuns=10 p95 — худший прогон серии).
func percentile(sorted []time.Duration, pct int) time.Duration {
	idx := (pct*len(sorted)+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

// millis — фиксированный формат миллисекунд для строк измерения:
// Duration.String даёт переменные точность и единицы (µs/ms) — колонки
// отчёта получились бы неровными.
func millis(d time.Duration) string {
	return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
}

// loadDBDir — одноразовый каталог базы нагрузки: qa/tmp внутри корня
// модуля. База прикидки обязана лежать на реальном диске: каталог ОС
// по умолчанию (TMPDIR, /tmp) на части систем — tmpfs в памяти, и
// измерение импорта перестаёт касаться диска. Каталог удаляется по
// завершении теста вместе с файлами WAL/SHM; уцелевший после сбоя
// остаток можно снять вручную — git каталог игнорирует.
func loadDBDir(t *testing.T) string {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatalf("каталог базы нагрузки: %v", err)
	}
	base := filepath.Join(root, "qa", "tmp")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("каталог базы нагрузки: %v", err)
	}
	dir, err := os.MkdirTemp(base, "load-")
	if err != nil {
		t.Fatalf("каталог базы нагрузки: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) }) //nolint:errcheck — одноразовый каталог прогона
	return dir
}

// moduleRoot — корень модуля (каталог с go.mod) от текущего каталога вверх.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod не найден от %s", dir)
		}
		dir = parent
	}
}

// numberSpan — диапазон номеров регистрации JEDEC 1000–9999: до 9000 записей
// на один суффикс алфавита suffixAlphabet.
const numberSpan = 9000

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
	if max := len(letters) * numberSpan; scale > max {
		f.Close() //nolint:errcheck — ошибка создания уже возвращена/логируется
		return fmt.Errorf("масштаб %d превышает ёмкость генератора %d (номер × суффикс)", scale, max)
	}
	for i := 0; i < scale; i++ {
		number := 1000 + i%numberSpan
		suffix := letters[i/numberSpan]
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
// (номера регистрации 1000–9999 × суффиксы покрывают 10⁴–10⁵ записей).
func suffixAlphabet() []string {
	out := make([]string, 0, 26+26*26)
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
