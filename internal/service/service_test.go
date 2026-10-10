package service

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/testutil"
)

// Интеграционный набор этапа 3 (план работ 3.2–3.8): один и тот же набор
// на SQLite (память/файл) и PostgreSQL (локально по CATALOG_TEST_POSTGRES_DSN).
// Тексты сообщений — дословный контракт.

// configFactory — фабрика конфигураций: каждый подтест получает свежую базу.
type configFactory func(t *testing.T) Config

func memoryConfig(t *testing.T) Config {
	t.Helper()
	return Config{Dialect: "sqlite", DSN: ":memory:", EnsureCreated: true}
}

func fileConfig(t *testing.T) Config {
	t.Helper()
	return Config{Dialect: "sqlite", DSN: filepath.Join(t.TempDir(), "catalog.db"), EnsureCreated: true}
}

func postgresConfig(t *testing.T) Config {
	t.Helper()
	dsn := testutil.PostgresDSN(t)
	testutil.DropAllTables(t, dsn)
	return Config{Dialect: "postgres", DSN: dsn, EnsureCreated: true}
}

func TestIntegrationSQLiteMemory(t *testing.T) { runSuite(t, memoryConfig) }
func TestIntegrationSQLiteFile(t *testing.T)   { runSuite(t, fileConfig) }
func TestIntegrationPostgres(t *testing.T)     { runSuite(t, postgresConfig) }

// Помощники построения данных.

func ptr[T any](v T) *T { return &v }

func attrText(code, v string) catalog.AttributeValue {
	return catalog.AttributeValue{Attribute: code, Text: &v}
}

func attrNum(code string, v float64) catalog.AttributeValue {
	return catalog.AttributeValue{Attribute: code, Num: &v}
}

func cond(code string, v float64) catalog.ConditionValue {
	return catalog.ConditionValue{Condition: code, Value: v}
}

func section(name string, vals ...catalog.ParameterValue) SectionInput {
	return SectionInput{Section: name, Values: vals}
}

func pvExact(code string, v float64, conds ...catalog.ConditionValue) catalog.ParameterValue {
	return catalog.ParameterValue{Parameter: code, Exact: &v, Conditions: conds}
}

func pvRange(code string, min, max float64, conds ...catalog.ConditionValue) catalog.ParameterValue {
	return catalog.ParameterValue{Parameter: code, Min: &min, Max: &max, Conditions: conds}
}

func pvAtMost(code string, max float64, conds ...catalog.ConditionValue) catalog.ParameterValue {
	return catalog.ParameterValue{Parameter: code, Max: &max, Conditions: conds}
}

func pvAtLeast(code string, min float64, conds ...catalog.ConditionValue) catalog.ParameterValue {
	return catalog.ParameterValue{Parameter: code, Min: &min, Conditions: conds}
}

func pvText(code, v string) catalog.ParameterValue {
	return catalog.ParameterValue{Parameter: code, Text: &v}
}

func mustUpsert(t *testing.T, svc *DeviceService, in DeviceInput) Outcome {
	t.Helper()
	out, err := svc.Upsert(context.Background(), in)
	if err != nil {
		t.Fatalf("upsert %q: %v", in.Name, err)
	}
	return out
}

func wantDomainError(t *testing.T, err error, code domain.Code, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка %s, получен nil", code)
	}
	de, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("ожидалась *domain.Error, получено %v", err)
	}
	if de.Code != code {
		t.Fatalf("код: %s, ожидался %s (%s)", de.Code, code, de.Message)
	}
	if de.Message != message {
		t.Fatalf("текст: %q, ожидался %q", de.Message, message)
	}
}

func openSuiteApp(t *testing.T, factory configFactory) *App {
	t.Helper()
	app, err := Open(context.Background(), factory(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

// seedTransistors — базовые записи для тестов поиска и аналогов.
func seedTransistors(t *testing.T, svc *DeviceService) {
	t.Helper()
	mustUpsert(t, svc, DeviceInput{
		Name: "КТ315Б",
		Attributes: []catalog.AttributeValue{
			attrText("structure", "npn"), attrNum("yearFrom", 1967), attrNum("yearTo", 1992),
		},
		Sections: []SectionInput{
			section("parameters",
				pvRange("h21e", 50, 350, cond("Uke", 10), cond("Ik", 1)),
				pvAtMost("Ikbo", 0.5, cond("Ukb", 10), cond("temp", 25))),
			section("ratings",
				pvExact("UkeoMax", 20), pvExact("TempMin", -60), pvExact("TempMax", 100)),
		},
		Manufacturers: &[]string{"Восход", "Терма"},
	})
	mustUpsert(t, svc, DeviceInput{
		Name: "BC547B",
		Attributes: []catalog.AttributeValue{
			attrText("structure", "npn"), attrText("package", "TO-92"),
		},
		Sections: []SectionInput{
			section("parameters",
				pvRange("h21e", 200, 450, cond("Uke", 5), cond("Ik", 2)),
				pvAtLeast("FGran", 300, cond("Uke", 5), cond("Ik", 2))),
			section("ratings", pvExact("UkeoMax", 45), pvExact("IkMax", 100)),
		},
	})
	mustUpsert(t, svc, DeviceInput{
		Name: "МП39",
		Attributes: []catalog.AttributeValue{
			attrText("structure", "pnp"), attrText("description", "низкочастотный"),
		},
		Sections: []SectionInput{
			section("parameters", pvRange("h21e", 20, 50, cond("Uke", 5), cond("Ik", 5))),
			section("ratings", pvExact("UkeoMax", 15), pvExact("PkMax", 150)),
		},
	})
}

func runSuite(t *testing.T, factory configFactory) {
	t.Run("upsert sections", func(t *testing.T) { suiteUpsertSections(t, factory) })
	t.Run("atomic on error", func(t *testing.T) { suiteAtomicOnError(t, factory) })
	t.Run("variants", func(t *testing.T) { suiteVariants(t, factory) })
	t.Run("variant filters", func(t *testing.T) { suiteVariantFilters(t, factory) })
	t.Run("analogs", func(t *testing.T) { suiteAnalogs(t, factory) })
	t.Run("find", func(t *testing.T) { suiteFind(t, factory) })
	t.Run("get by ids", func(t *testing.T) { suiteGetByIDs(t, factory) })
	t.Run("search", func(t *testing.T) { suiteSearch(t, factory) })
	t.Run("search validation", func(t *testing.T) { suiteSearchValidation(t, factory) })
	t.Run("delete cascade", func(t *testing.T) { suiteDeleteCascade(t, factory) })
	t.Run("create", func(t *testing.T) { suiteCreate(t, factory) })
	t.Run("fields rewrite on update", func(t *testing.T) { suiteFieldsRewriteOnUpdate(t, factory) })
	t.Run("revisions", func(t *testing.T) { suiteRevisions(t, factory) })
	t.Run("catalog import", func(t *testing.T) { suiteCatalogImport(t, factory) })
	t.Run("classification fields", func(t *testing.T) { suiteClassificationFields(t, factory) })
	t.Run("parse and suggest", func(t *testing.T) { suiteParseSuggest(t, factory) })
}

// suiteUpsertSections — семантика секций: замена целиком, очистка, Skipped
// по каноническому сравнению слитого состояния.
func suiteUpsertSections(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	in := DeviceInput{
		Name: "КТ315Б",
		Attributes: []catalog.AttributeValue{
			attrText("structure", "npn"), attrNum("yearFrom", 1967), attrNum("yearTo", 1992),
		},
		Sections: []SectionInput{
			section("parameters",
				pvRange("h21e", 50, 350, cond("Uke", 10), cond("Ik", 1)),
				pvAtMost("Ikbo", 0.5, cond("Ukb", 10), cond("temp", 25))),
			section("ratings", pvExact("UkeoMax", 20), pvExact("TempMin", -60), pvExact("TempMax", 100)),
		},
		Manufacturers: &[]string{"Восход", "Терма"},
	}
	if out := mustUpsert(t, svc, in); out != OutcomeAdded {
		t.Fatalf("исход: %s, ожидался Added", out)
	}

	card, ok, err := svc.Get(ctx, "transistor", "КТ315Б")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if card.System != domain.SystemGost || card.Kind != domain.KindTransistor {
		t.Fatalf("карточка: system=%s kind=%s", card.System, card.Kind)
	}
	field := func(name string) (domain.Field, bool) { return card.FieldByName(name) }
	if f, ok := field("material"); !ok || f.String() != "si" {
		t.Fatalf("поле material: %+v", f)
	}
	if f, ok := field("subclass"); !ok || f.String() != "bjt" {
		t.Fatalf("поле subclass: %+v", f)
	}
	if f, ok := field("dev_number"); !ok || f.String() != "315" {
		t.Fatalf("поле dev_number: %+v", f)
	}
	if len(card.Attributes) != 3 ||
		card.Attributes[0].Code != "structure" || *card.Attributes[0].Text != "npn" ||
		card.Attributes[2].Code != "yearTo" || *card.Attributes[2].Num != 1992 {
		t.Fatalf("атрибуты: %+v", card.Attributes)
	}
	if len(card.Groups) != 2 ||
		card.Groups[0].Section != "parameters" || len(card.Groups[0].Values) != 2 ||
		card.Groups[1].Section != "ratings" || len(card.Groups[1].Values) != 3 {
		t.Fatalf("группы: %+v", card.Groups)
	}
	h21e := card.Groups[0].Values[0]
	if *h21e.Min != 50 || *h21e.Max != 350 || len(h21e.Conditions) != 2 ||
		h21e.Conditions[0] != cond("Ik", 1) || h21e.Conditions[1] != cond("Uke", 10) {
		t.Fatalf("h21e: %+v", h21e)
	}
	if !slices.Equal(card.Manufacturers, []string{"Восход", "Терма"}) {
		t.Fatalf("производители: %v", card.Manufacturers)
	}

	// Повтор того же входа — Skipped, ревизии не меняются.
	_, dataBefore, _ := app.Revisions(ctx)
	if out := mustUpsert(t, svc, in); out != OutcomeSkipped {
		t.Fatalf("повтор: %s, ожидался Skipped", out)
	}
	if _, dataAfter, _ := app.Revisions(ctx); dataAfter != dataBefore {
		t.Fatalf("data_revision изменился при Skipped: %d → %d", dataBefore, dataAfter)
	}

	// Секция attributes задана — замена целиком; параметры не тронуты.
	if out := mustUpsert(t, svc, DeviceInput{
		Name:       "КТ315Б",
		Attributes: []catalog.AttributeValue{attrText("package", "КТ-13")},
	}); out != OutcomeUpdatedExisting {
		t.Fatalf("замена атрибутов: %s", out)
	}
	card, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if len(card.Attributes) != 1 || card.Attributes[0].Code != "package" {
		t.Fatalf("атрибуты после замены: %+v", card.Attributes)
	}
	if len(card.Groups[0].Values) != 2 || len(card.Groups[1].Values) != 3 {
		t.Fatalf("параметры затронуты заменой атрибутов: %+v", card.Groups)
	}

	// Пустая секция — очистить только свою группу.
	if out := mustUpsert(t, svc, DeviceInput{
		Name:     "КТ315Б",
		Sections: []SectionInput{{Section: "parameters"}},
	}); out != OutcomeUpdatedExisting {
		t.Fatalf("очистка секции: %s", out)
	}
	card, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if len(card.Groups) != 1 || card.Groups[0].Section != "ratings" {
		t.Fatalf("группы после очистки parameters: %+v", card.Groups)
	}

	// Полная очистка атрибутов пустым списком.
	if out := mustUpsert(t, svc, DeviceInput{
		Name:       "КТ315Б",
		Attributes: []catalog.AttributeValue{},
	}); out != OutcomeUpdatedExisting {
		t.Fatalf("очистка атрибутов: %s", out)
	}
	card, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if len(card.Attributes) != 0 {
		t.Fatalf("атрибуты после очистки: %+v", card.Attributes)
	}
	if !slices.Equal(card.Manufacturers, []string{"Восход", "Терма"}) {
		t.Fatalf("производители затронуты: %v", card.Manufacturers)
	}
}

// suiteAtomicOnError — ошибка в значении секции: запись не применяется
// вовсе (частичное применение запрещено).
func suiteAtomicOnError(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)

	// Неположительное точное значение и неверная комбинация условий.
	_, err := svc.Upsert(ctx, DeviceInput{
		Name: "КТ315Б",
		Sections: []SectionInput{
			section("ratings", pvExact("UkeoMax", 0), pvExact("TempMin", -60), pvExact("TempMax", 100)),
		},
	})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"parameter «UkeoMax»: value of key value must be positive")

	_, err = svc.Upsert(ctx, DeviceInput{
		Name:     "КТ315Б",
		Sections: []SectionInput{section("parameters", pvRange("h21e", 50, 350))},
	})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"parameter «h21e»: the combination of conditions does not match any condition set of the parameter")

	// Запись не изменилась.
	card, _, _ := svc.Get(ctx, "transistor", "КТ315Б")
	if len(card.Groups[0].Values) != 2 || len(card.Groups[1].Values) != 3 {
		t.Fatalf("запись изменилась при ошибке секции: %+v", card.Groups)
	}
	if len(card.Attributes) != 3 {
		t.Fatalf("атрибуты изменились при ошибке секции: %+v", card.Attributes)
	}

	// temp_pair: TempMin >= TempMax — ошибка слитого состояния.
	_, err = svc.Upsert(ctx, DeviceInput{
		Name:     "КТ315Б",
		Sections: []SectionInput{section("ratings", pvExact("TempMin", 100), pvExact("TempMax", 50), pvExact("UkeoMax", 25))},
	})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"parameter «TempMin» must be less than parameter «TempMax»")

	// Дубликаты и пустые значения входа.
	_, err = svc.Upsert(ctx, DeviceInput{
		Name:       "КТ315Б",
		Attributes: []catalog.AttributeValue{attrText("package", "X"), attrText("package", "Y")},
	})
	wantDomainError(t, err, domain.CodeValidationFailed, "attribute «package» is set more than once")

	names := []string{"Восход", " Восход "}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "КТ315Б", Manufacturers: &names})
	wantDomainError(t, err, domain.CodeValidationFailed, "manufacturer «Восход» is set more than once")

	empty := []string{" "}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "КТ315Б", Manufacturers: &empty})
	wantDomainError(t, err, domain.CodeValidationFailed, "empty manufacturer name")

	// Неизвестная секция.
	_, err = svc.Upsert(ctx, DeviceInput{
		Name:     "КТ315Б",
		Sections: []SectionInput{{Section: "nosuch"}},
	})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"section «nosuch» does not match any catalog group")

	// Несовпадение системы.
	_, err = svc.Upsert(ctx, DeviceInput{Name: "КТ315Б", System: domain.SystemPro})
	wantDomainError(t, err, domain.CodeDesignationMismatch,
		"designation «КТ315Б» does not match designation system pro")
}

// suiteVariants — исполнения: матрица в карточке, полная замена секции,
// ошибки правил вариантов.
func suiteVariants(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	resistor := DeviceInput{
		Name: "С2-33Н",
		Sections: []SectionInput{
			section("parameters", pvRange("Rnom", 1, 10000000), pvAtMost("Dop", 5)),
			section("ratings", pvExact("TempMin", -60), pvExact("TempMax", 125)),
		},
		Variants: &[]VariantInput{
			{
				Label: "0.125 Вт",
				Sections: []SectionInput{
					section("ratings", pvExact("Pnom", 0.125), pvExact("Umax", 250)),
					section("dimensions", pvExact("massMax", 0.15)),
				},
			},
			{
				Label: "1 Вт",
				Sections: []SectionInput{
					section("ratings", pvExact("Pnom", 1), pvExact("Umax", 500)),
					section("dimensions", pvExact("massMax", 0.8)),
				},
			},
		},
	}
	if out := mustUpsert(t, svc, resistor); out != OutcomeAdded {
		t.Fatalf("исход: %s", out)
	}
	card, _, _ := svc.Get(ctx, "resistor", "С2-33Н")
	if len(card.Variants) != 2 || card.Variants[0].Label != "0.125 Вт" {
		t.Fatalf("варианты: %+v", card.Variants)
	}
	// Матрица: у исполнения свои группы (ratings + dimensions).
	if len(card.Variants[0].Groups) != 2 ||
		card.Variants[0].Groups[0].Section != "ratings" ||
		card.Variants[0].Groups[1].Section != "dimensions" {
		t.Fatalf("группы варианта: %+v", card.Variants[0].Groups)
	}
	pnom := card.Variants[0].Groups[0].Values[0]
	if pnom.Parameter != "Pnom" || *pnom.Exact != 0.125 {
		t.Fatalf("Pnom: %+v", pnom)
	}

	// Полная замена набора исполнений (вместе со значениями).
	replaced := []VariantInput{{
		Label:    "2 Вт",
		Sections: []SectionInput{section("ratings", pvExact("Pnom", 2))},
	}}
	if out := mustUpsert(t, svc, DeviceInput{Name: "С2-33Н", Variants: &replaced}); out != OutcomeUpdatedExisting {
		t.Fatalf("замена вариантов: %s", out)
	}
	card, _, _ = svc.Get(ctx, "resistor", "С2-33Н")
	if len(card.Variants) != 1 || card.Variants[0].Label != "2 Вт" ||
		len(card.Variants[0].Groups[0].Values) != 1 {
		t.Fatalf("варианты после замены: %+v", card.Variants)
	}
	// Данные типа в целом не затронуты.
	if len(card.Groups[0].Values) != 2 {
		t.Fatalf("тип затронут заменой вариантов: %+v", card.Groups)
	}

	// Очистка исполнений.
	empty := []VariantInput{}
	if out := mustUpsert(t, svc, DeviceInput{Name: "С2-33Н", Variants: &empty}); out != OutcomeUpdatedExisting {
		t.Fatalf("очистка вариантов: %s", out)
	}
	card, _, _ = svc.Get(ctx, "resistor", "С2-33Н")
	if len(card.Variants) != 0 {
		t.Fatalf("варианты после очистки: %+v", card.Variants)
	}

	// Вариант без Pnom — ошибка правила.
	badVariants := []VariantInput{{Label: "0.5 Вт", Sections: []SectionInput{
		section("ratings", pvExact("Umax", 350)),
	}}}
	_, err := svc.Upsert(ctx, DeviceInput{Name: "С2-33Н", Variants: &badVariants})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"variant «0.5 Вт»: mandatory parameter Pnom is missing")

	// Транзисторы — без исполнений.
	tv := []VariantInput{{Label: "x", Sections: []SectionInput{
		section("ratings", pvExact("UkeoMax", 20)),
	}}}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "КТ315Б", Variants: &tv})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"kind transistor does not support variants")

	// Матрица конденсатора: вариант обязан иметь Unom и Cnom.
	cv := []VariantInput{{Label: "25 В", Sections: []SectionInput{
		section("parameters", pvExact("Unom", 25)),
	}}}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "К50-35", Variants: &cv})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"variant «25 В»: mandatory parameter Cnom is missing")
}

// suiteVariantFilters — вариантные параметрические фильтры применяются
// в пределах одного исполнения (EXISTS по device_variants).
func suiteVariantFilters(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	mustUpsert(t, svc, DeviceInput{
		Name: "К50-35",
		Sections: []SectionInput{
			section("parameters", pvAtMost("Dop", 20), pvAtMost("Tgd", 0.15, cond("temp", 20))),
		},
		Variants: &[]VariantInput{
			{
				Label: "25 В",
				Sections: []SectionInput{
					section("parameters", pvExact("Unom", 25), pvRange("Cnom", 47000000, 4700000000)),
					section("dimensions", pvExact("diameter", 10), pvExact("leadLength", 16), pvExact("massMax", 3)),
				},
			},
			{
				Label: "160 В",
				Sections: []SectionInput{
					section("parameters", pvExact("Unom", 160), pvRange("Cnom", 1000000, 10000000)),
					section("dimensions", pvExact("diameter", 8), pvExact("leadLength", 12), pvExact("massMax", 1.5)),
				},
			},
		},
	})

	search := func(filters ...ParameterFilter) []string {
		t.Helper()
		page, err := svc.Search(ctx, SearchQuery{Kind: domain.KindCapacitor, Parameters: filters})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		out := make([]string, 0, len(page.Items))
		for _, it := range page.Items {
			out = append(out, it.Designation)
		}
		return out
	}

	// Unom=25 и diameter ≤ 10 — один вариант (25 В) удовлетворяет обоим.
	got := search(
		ParameterFilter{Parameter: "Unom", Exact: ptr(25.0)},
		ParameterFilter{Parameter: "diameter", Max: ptr(10.0)},
	)
	if !slices.Equal(got, []string{"К50-35"}) {
		t.Fatalf("Unom=25, diameter≤10: %v", got)
	}
	// Unom=25 и diameter ≤ 6: у 25 В диаметр 10, у 160 В — 8, но другой Unom:
	// перекрёстные сочетания исполнений не сопоставляются.
	got = search(
		ParameterFilter{Parameter: "Unom", Exact: ptr(25.0)},
		ParameterFilter{Parameter: "diameter", Max: ptr(6.0)},
	)
	if len(got) != 0 {
		t.Fatalf("Unom=25, diameter≤6: %v (перекрёстное сочетание исполнений)", got)
	}
	// Данные типа в целом участвуют вместе с вариантом: Dop — тип, Unom — вариант.
	got = search(
		ParameterFilter{Parameter: "Unom", Exact: ptr(160.0)},
		ParameterFilter{Parameter: "Dop", Max: ptr(20.0)},
	)
	if !slices.Equal(got, []string{"К50-35"}) {
		t.Fatalf("Unom=160 + Dop≤20: %v", got)
	}
}

// suiteAnalogs — направленные ссылки (D8): замена исходящих не затрагивает
// встречные, структурная валидация, блоки карточки.
func suiteAnalogs(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)

	mustUpsert(t, svc, DeviceInput{
		Name:    "КТ315Б",
		Analogs: &[]AnalogInput{{Designation: "BC547B", Note: "полный аналог"}},
	})
	a, _, _ := svc.Get(ctx, "transistor", "КТ315Б")
	b, _, _ := svc.Get(ctx, "transistor", "BC547B")
	if len(a.Analogs) != 1 || a.Analogs[0].Designation != "BC547B" ||
		a.Analogs[0].System != domain.SystemPro || a.Analogs[0].Note != "полный аналог" {
		t.Fatalf("исходящие: %+v", a.Analogs)
	}
	if len(b.Backlinks) != 1 || b.Backlinks[0].Designation != "КТ315Б" {
		t.Fatalf("встречные: %+v", b.Backlinks)
	}

	// Встречная ссылка B→A — самостоятельная string.
	mustUpsert(t, svc, DeviceInput{
		Name:    "BC547B",
		Analogs: &[]AnalogInput{{Designation: "КТ315Б", Note: "с ограничениями"}},
	})

	// Замена исходящих A (A→МП39): ссылка A→B исчезает, встречная B→A остаётся.
	mustUpsert(t, svc, DeviceInput{
		Name:    "КТ315Б",
		Analogs: &[]AnalogInput{{Designation: "МП39"}},
	})
	a, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if len(a.Analogs) != 1 || a.Analogs[0].Designation != "МП39" {
		t.Fatalf("исходящие после замены: %+v", a.Analogs)
	}
	b, _, _ = svc.Get(ctx, "transistor", "BC547B")
	if len(b.Analogs) != 1 || b.Analogs[0].Designation != "КТ315Б" || b.Analogs[0].Note != "с ограничениями" {
		t.Fatalf("встречная ссылка B→A затронута: %+v", b.Analogs)
	}
	if len(b.Backlinks) != 0 {
		t.Fatalf("встречные B после замены A: %+v", b.Backlinks)
	}

	// Очистка исходящих.
	empty := []AnalogInput{}
	mustUpsert(t, svc, DeviceInput{Name: "КТ315Б", Analogs: &empty})
	a, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if len(a.Analogs) != 0 {
		t.Fatalf("исходящие после очистки: %+v", a.Analogs)
	}

	// Структурная валидация: неизвестное обозначение, самоссылка, дубликат.
	_, err := svc.Upsert(ctx, DeviceInput{
		Name:    "МП39",
		Analogs: &[]AnalogInput{{Designation: "НЕТ123"}},
	})
	wantDomainError(t, err, domain.CodeNotFound, "analog «НЕТ123» not found in kind transistor")

	_, err = svc.Upsert(ctx, DeviceInput{
		Name:    "МП39",
		Analogs: &[]AnalogInput{{Designation: "мп39"}},
	})
	wantDomainError(t, err, domain.CodeValidationFailed, "record «МП39» cannot be an analog of itself")

	_, err = svc.Upsert(ctx, DeviceInput{
		Name:    "МП39",
		Analogs: &[]AnalogInput{{Designation: "КТ315Б"}, {Designation: "кт315б"}},
	})
	wantDomainError(t, err, domain.CodeValidationFailed, "analog «КТ315Б» is set more than once")

	// Аналог из другого класса не разрешается (в пределах класса).
	_, err = svc.Upsert(ctx, DeviceInput{
		Name:    "МП39",
		Analogs: &[]AnalogInput{{Designation: "К10-17Б"}},
	})
	wantDomainError(t, err, domain.CodeNotFound, "analog «К10-17Б» not found in kind transistor")
}

// suiteFind — точное совпадение, раздельность равнозначных записей и
// подсказка по материалу.
func suiteFind(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	// Только «2Т312»: точного «КТ312» нет — подсказка равнозначной записи.
	mustUpsert(t, svc, DeviceInput{
		Name:       "2Т312",
		Attributes: []catalog.AttributeValue{attrText("structure", "npn")},
		Sections: []SectionInput{
			section("parameters", pvRange("h21e", 50, 280, cond("Uke", 5), cond("Ik", 10))),
		},
	})
	res, err := svc.Find(ctx, "", "КТ312")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if res.Found != nil {
		t.Fatalf("точное совпадение не ожидалось: %+v", res.Found.Designation)
	}
	if res.Suggestion == nil || res.Suggestion.Designation != "2Т312" || res.Suggestion.Kind != domain.KindTransistor {
		t.Fatalf("подсказка: %+v", res.Suggestion)
	}

	// «КТ312» появляется — раздельные записи, точное совпадение побеждает.
	mustUpsert(t, svc, DeviceInput{
		Name:       "КТ312",
		Attributes: []catalog.AttributeValue{attrText("structure", "npn")},
	})
	res, err = svc.Find(ctx, "", "КТ312")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if res.Found == nil || res.Found.Designation != "КТ312" || res.Suggestion != nil {
		t.Fatalf("find после добавления: found=%v suggestion=%v", res.Found, res.Suggestion)
	}
	if got, ok, _ := svc.Get(ctx, "transistor", "2Т312"); !ok || got.Designation != "2Т312" {
		t.Fatalf("раздельность записей: ok=%v", ok)
	}

	// Подсказки нет у систем без пар символов материала.
	mustUpsert(t, svc, DeviceInput{
		Name: "2N2222A",
		Sections: []SectionInput{
			section("parameters", pvRange("h21e", 100, 300, cond("Uke", 10), cond("Ik", 10))),
		},
	})
	res, err = svc.Find(ctx, "", "2N2222A")
	if err != nil || res.Found == nil {
		t.Fatalf("find jedec: err=%v found=%v", err, res.Found)
	}
}

// suiteGetByIDs — пакетная выборка карточек по списку id: порядок
// результата — по порядку входа, отсутствующие записи пропускаются,
// карточка эквивалентна одиночному Get.
func suiteGetByIDs(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)

	page, err := svc.Search(ctx, SearchQuery{Kind: domain.KindTransistor, Limit: 200})
	if err != nil {
		t.Fatalf("поиск: %v", err)
	}
	if len(page.Items) < 3 {
		t.Fatalf("записей в поиске: %d, ожидалось не меньше 3", len(page.Items))
	}
	ids := make([]int64, len(page.Items))
	for i, item := range page.Items {
		ids[i] = item.ID
	}

	// Вход — обратный порядок с несуществующим id в середине: результат
	// следует порядку входа и пропускает отсутствующие записи.
	input := make([]int64, 0, len(ids)+1)
	for i := len(ids) - 1; i >= 0; i-- {
		input = append(input, ids[i])
		if i == len(ids)/2 {
			input = append(input, 1<<40)
		}
	}
	cards, err := svc.GetByIDs(ctx, input)
	if err != nil {
		t.Fatalf("get by ids: %v", err)
	}
	if len(cards) != len(page.Items) {
		t.Fatalf("карточек: %d, ожидалось %d", len(cards), len(page.Items))
	}
	for i, card := range cards {
		want := page.Items[len(page.Items)-1-i]
		if card.ID != want.ID || card.Designation != want.Designation {
			t.Fatalf("карточка %d: %s (id %d), ожидалась %s (id %d)",
				i, card.Designation, card.ID, want.Designation, want.ID)
		}
	}

	// Карточка пакетной выборки эквивалентна одиночной (поля, группы,
	// атрибуты, производители).
	single, ok, err := svc.Get(ctx, domain.KindTransistor, page.Items[0].Designation)
	if err != nil || !ok {
		t.Fatalf("get %s: ok=%v err=%v", page.Items[0].Designation, ok, err)
	}
	if !reflect.DeepEqual(cards[len(cards)-1], single) {
		t.Fatalf("пакетная карточка %s отличается от одиночной:\n%+v\n%+v",
			single.Designation, cards[len(cards)-1], single)
	}

	if cards, err = svc.GetByIDs(ctx, nil); err != nil || len(cards) != 0 {
		t.Fatalf("пустой список: карточек %d (err %v), ожидалось 0", len(cards), err)
	}
	if cards, err = svc.GetByIDs(ctx, []int64{1 << 40}); err != nil || len(cards) != 0 {
		t.Fatalf("несуществующий id: карточек %d (err %v), ожидалось 0", len(cards), err)
	}
}

// suiteSearch — подstring, фильтры полей/атрибутов/параметров, сортировка,
// пагинация.
func suiteSearch(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)
	mustUpsert(t, svc, DeviceInput{
		Name: "К10-17Б",
		Sections: []SectionInput{
			section("parameters",
				pvRange("Cnom", 22, 1000000), pvText("TKE", "Н30"), pvExact("Unom", 25)),
			section("dimensions",
				pvExact("massMax", 1), pvExact("length", 6), pvExact("width", 4), pvExact("height", 5)),
		},
	})

	designations := func(page SearchPage) []string {
		out := make([]string, 0, len(page.Items))
		for _, it := range page.Items {
			out = append(out, it.Designation)
		}
		return out
	}

	// Подstring обозначения (канонический регистр ввода не важен).
	page, err := svc.Search(ctx, SearchQuery{Query: "кт3"})
	if err != nil || !slices.Equal(designations(page), []string{"КТ315Б"}) {
		t.Fatalf("подstring: %v err=%v", designations(page), err)
	}

	// Фильтр полей: материал — сквозной между gost и pro.
	page, err = svc.Search(ctx, SearchQuery{
		Kind:   domain.KindTransistor,
		Fields: []FieldFilter{{Field: "material", Text: "si"}},
	})
	if err != nil {
		t.Fatalf("material: %v", err)
	}
	if !slices.Equal(designations(page), []string{"BC547B", "КТ315Б"}) {
		t.Fatalf("material=si: %v", designations(page))
	}

	// Числовой фильтр поля.
	page, err = svc.Search(ctx, SearchQuery{
		Kind:   domain.KindTransistor,
		Fields: []FieldFilter{{Field: "dev_number", Num: 315, HasNum: true, Op: OpEq}},
	})
	if err != nil || !slices.Equal(designations(page), []string{"КТ315Б"}) {
		t.Fatalf("dev_number=315: %v err=%v", designations(page), err)
	}

	// Attributes: текстовый и numberвой.
	page, err = svc.Search(ctx, SearchQuery{
		Kind:       domain.KindTransistor,
		Attributes: []AttributeFilter{{Attribute: "structure", Text: "npn"}},
	})
	if err != nil || !slices.Equal(designations(page), []string{"BC547B", "КТ315Б"}) {
		t.Fatalf("structure=npn: %v err=%v", designations(page), err)
	}
	page, err = svc.Search(ctx, SearchQuery{
		Kind:       domain.KindTransistor,
		Attributes: []AttributeFilter{{Attribute: "yearFrom", Num: 1990, HasNum: true, Op: OpGte}},
	})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("yearFrom≥1990: %v err=%v", designations(page), err)
	}

	// Parameters: границы по гарантированному значению (нижняя граница
	// диапазона / точное значение) и точные значения.
	page, err = svc.Search(ctx, SearchQuery{
		Kind:       domain.KindTransistor,
		Parameters: []ParameterFilter{{Parameter: "h21e", Min: ptr(50.0)}},
	})
	if err != nil || !slices.Equal(designations(page), []string{"BC547B", "КТ315Б"}) {
		t.Fatalf("h21e≥50: %v err=%v", designations(page), err)
	}
	page, err = svc.Search(ctx, SearchQuery{
		Kind:       domain.KindTransistor,
		Parameters: []ParameterFilter{{Parameter: "h21e", Min: ptr(300.0)}},
	})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("h21e≥300 (гарантированная граница): %v err=%v", designations(page), err)
	}
	page, err = svc.Search(ctx, SearchQuery{
		Kind: domain.KindCapacitor,
		Parameters: []ParameterFilter{
			{Parameter: "Unom", Exact: ptr(25.0)},
			{Parameter: "TKE", Text: "Н30"},
		},
	})
	if err != nil || !slices.Equal(designations(page), []string{"К10-17Б"}) {
		t.Fatalf("Unom=25, TKE=Н30: %v err=%v", designations(page), err)
	}

	// Сортировка обозначения по убыванию + финальный (kind, designation).
	asc, err := svc.Search(ctx, SearchQuery{Sort: []SortField{{Key: "designation"}}})
	if err != nil {
		t.Fatalf("sort asc: %v", err)
	}
	desc, err := svc.Search(ctx, SearchQuery{Sort: []SortField{{Key: "designation", Desc: true}}})
	if err != nil {
		t.Fatalf("sort desc: %v", err)
	}
	if len(asc.Items) != len(desc.Items) || len(asc.Items) == 0 {
		t.Fatalf("сортировка: %d / %d", len(asc.Items), len(desc.Items))
	}
	for i := range asc.Items {
		if asc.Items[i].Designation != desc.Items[len(desc.Items)-1-i].Designation {
			t.Fatalf("сортировка не симметрична: %v vs %v", designations(asc), designations(desc))
		}
	}

	// Пагинация и total.
	all, err := svc.Search(ctx, SearchQuery{})
	if err != nil {
		t.Fatalf("все: %v", err)
	}
	if all.Total != 4 || len(all.Items) != 4 {
		t.Fatalf("total=%d items=%d", all.Total, len(all.Items))
	}
	page, err = svc.Search(ctx, SearchQuery{Limit: 2, Offset: 2})
	if err != nil || len(page.Items) != 2 || page.Total != 4 ||
		page.Items[0].Designation != all.Items[2].Designation {
		t.Fatalf("пагинация: %+v err=%v", designations(page), err)
	}
	// limit ≤ 200: ограничение сверху.
	page, err = svc.Search(ctx, SearchQuery{Limit: 100000})
	if err != nil || page.Limit != SearchLimitMax {
		t.Fatalf("clamp limit: %d err=%v", page.Limit, err)
	}

	// Система обозначений как фильтр.
	page, err = svc.Search(ctx, SearchQuery{System: domain.SystemSeries})
	if err != nil || !slices.Equal(designations(page), []string{"МП39"}) {
		t.Fatalf("system=series: %v err=%v", designations(page), err)
	}

	// Count.
	if n, _ := svc.Count(ctx, nil); n != 4 {
		t.Fatalf("count all: %d", n)
	}
	k := domain.KindTransistor
	if n, _ := svc.Count(ctx, &k); n != 3 {
		t.Fatalf("count transistors: %d", n)
	}
}

// suiteSearchValidation — ошибки валидации запроса (D7, неизвестные коды).
func suiteSearchValidation(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	_, err := svc.Search(ctx, SearchQuery{
		Parameters: []ParameterFilter{{Parameter: "nosuch", Min: ptr(1.0)}},
	})
	wantDomainError(t, err, domain.CodeUnknownParameter, "unknown parameter «nosuch»")

	_, err = svc.Search(ctx, SearchQuery{
		Kind:       domain.KindResistor,
		Parameters: []ParameterFilter{{Parameter: "h21e", Min: ptr(1.0)}},
	})
	wantDomainError(t, err, domain.CodeParameterNotApplicable,
		"parameter «h21e» is not applicable to kind resistor")

	_, err = svc.Search(ctx, SearchQuery{
		Attributes: []AttributeFilter{{Attribute: "nosuch", Text: "x"}},
	})
	wantDomainError(t, err, domain.CodeUnknownAttribute, "unknown attribute «nosuch»")

	_, err = svc.Search(ctx, SearchQuery{
		Kind:       domain.KindTransistor,
		Attributes: []AttributeFilter{{Attribute: "polarized", Num: 1, HasNum: true}},
	})
	wantDomainError(t, err, domain.CodeAttributeNotApplicable,
		"attribute «polarized» is not applicable to kind transistor")

	_, err = svc.Search(ctx, SearchQuery{Kind: domain.Kind("thyristor")})
	wantDomainError(t, err, domain.CodeValidationFailed, "unknown device kind «thyristor»")
}

// suiteDeleteCascade — каскадное удаление записи с данными и ссылками,
// чистка сирот производителей.
func suiteDeleteCascade(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)
	mustUpsert(t, svc, DeviceInput{
		Name:    "КТ315Б",
		Analogs: &[]AnalogInput{{Designation: "BC547B"}},
	})

	card, _, _ := svc.Get(ctx, "transistor", "КТ315Б")
	id := card.ID

	if _, dataBefore, _ := app.Revisions(ctx); true {
		deleted, err := svc.Delete(ctx, "transistor", "кт315б")
		if err != nil || !deleted {
			t.Fatalf("delete: %v %v", deleted, err)
		}
		if _, dataAfter, _ := app.Revisions(ctx); dataAfter != dataBefore+1 {
			t.Fatalf("data_revision после удаления: %d → %d", dataBefore, dataAfter)
		}
	}
	if _, ok, _ := svc.Get(ctx, "transistor", "КТ315Б"); ok {
		t.Fatal("запись не удалена")
	}
	if _, ok, _ := svc.GetByID(ctx, id); ok {
		t.Fatal("запись не удалена по id")
	}
	// Ссылка-аналог удалена с обеих сторон.
	b, _, _ := svc.Get(ctx, "transistor", "BC547B")
	if len(b.Backlinks) != 0 || len(b.Analogs) != 0 {
		t.Fatalf("ссылки после удаления: %+v %+v", b.Backlinks, b.Analogs)
	}
	// Сироты производителей вычищены.
	if n, err := app.db.CountManufacturers(ctx); err != nil || n != 0 {
		t.Fatalf("производители-сироты: %d (err %v)", n, err)
	}
	// Повторное удаление — false без инкремента ревизии
	// (атомарное удаление по ключу: пустого успеха не бывает).
	_, dataAfter, _ := app.Revisions(ctx)
	if deleted, _ := svc.Delete(ctx, "transistor", "КТ315Б"); deleted {
		t.Fatal("повторное удаление вернуло true")
	}
	if _, dataNext, _ := app.Revisions(ctx); dataNext != dataAfter {
		t.Fatalf("data_revision при удалении отсутствующей: %d → %d", dataAfter, dataNext)
	}
	// Удаление без класса — детерминированно первая запись по kind_code.
	if deleted, _ := svc.Delete(ctx, "", "МП39"); !deleted {
		t.Fatal("удаление без класса не нашло запись")
	}
	if _, ok, _ := svc.Get(ctx, "transistor", "МП39"); ok {
		t.Fatal("запись не удалена без класса")
	}
}

// suiteCreate — режим «только создание» (POST REST): существующая запись —
// отказ already_exists без применения секций и без инкремента ревизии;
// новая запись создаётся (Added), повтор — уже конфликт.
func suiteCreate(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)

	_, dataBefore, _ := app.Revisions(ctx)
	_, err := svc.Create(ctx, DeviceInput{
		Name:          "КТ315Б",
		Attributes:    []catalog.AttributeValue{attrText("package", "КТ-13")},
		Manufacturers: &[]string{"Захват"},
	})
	wantDomainError(t, err, domain.CodeAlreadyExists, "record «КТ315Б» already exists")

	// Секции проигравшего не применены, ревизия не двигалась.
	card, ok, err := svc.Get(ctx, "transistor", "КТ315Б")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if len(card.Attributes) != 3 || card.Attributes[0].Code != "structure" {
		t.Fatalf("атрибуты изменены конфликтом Create: %+v", card.Attributes)
	}
	if !slices.Equal(card.Manufacturers, []string{"Восход", "Терма"}) {
		t.Fatalf("производители изменены конфликтом Create: %v", card.Manufacturers)
	}
	if _, dataAfter, _ := app.Revisions(ctx); dataAfter != dataBefore {
		t.Fatalf("data_revision при конфликте Create: %d → %d", dataBefore, dataAfter)
	}

	// Новая запись — Added; повтор — конфликт.
	in := DeviceInput{
		Name:       "ГТ109Г",
		Attributes: []catalog.AttributeValue{attrText("structure", "pnp")},
	}
	if out, err := svc.Create(ctx, in); err != nil || out != OutcomeAdded {
		t.Fatalf("create: %v %v", out, err)
	}
	_, err = svc.Create(ctx, in)
	wantDomainError(t, err, domain.CodeAlreadyExists, "record «ГТ109Г» already exists")
}

// suiteFieldsRewriteOnUpdate — обновление перезаписывает и продукты
// разбора обозначения: после прямого изменения designation_fields в БД
// (имитация дрейфа грамматики) Upsert восстановления восстанавливает
// инвариант «хранимые поля = продукты разбора ∪ explicit».
func suiteFieldsRewriteOnUpdate(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	seedTransistors(t, svc)

	// Дрейф: продукт разбора потерян, добавлено устаревшее поле.
	card, _, _ := svc.Get(ctx, "transistor", "КТ315Б")
	tx, err := app.db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.DeleteDesignationFields(ctx, card.ID, []string{"dev_number"}); err != nil {
		t.Fatalf("удаление полей: %v", err)
	}
	if err := tx.InsertDesignationFields(ctx, card.ID, []domain.Field{
		domain.TextField("stale", "x"),
	}); err != nil {
		t.Fatalf("вставка полей: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Обновление с секцией fields: хранимые поля = продукты разбора ∪
	// explicit — продукт восстановлен, устаревшее поле удалено.
	fields := []domain.Field{domain.TextField("category", "switching")}
	if out := mustUpsert(t, svc, DeviceInput{
		Name:   "КТ315Б",
		Fields: &fields,
	}); out != OutcomeUpdatedExisting {
		t.Fatalf("обновление: %s", out)
	}
	card, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if _, ok := card.FieldByName("dev_number"); !ok {
		t.Fatalf("продукт разбора не восстановлен: %+v", card.Fields)
	}
	if f, ok := card.FieldByName("material"); !ok || f.String() != "si" {
		t.Fatalf("продукт material: %+v", f)
	}
	if f, ok := card.FieldByName("category"); !ok || f.String() != "switching" {
		t.Fatalf("явное поле category: %+v", f)
	}
	if _, ok := card.FieldByName("stale"); ok {
		t.Fatalf("устаревшее поле пережило обновление: %+v", card.Fields)
	}

	// Обновление без секции fields: явные поля сохраняются, продукты
	// перезаписываются (drift-поля классифицируются как явные).
	tx, err = app.db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.DeleteDesignationFields(ctx, card.ID, []string{"dev_number"}); err != nil {
		t.Fatalf("удаление полей: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if out := mustUpsert(t, svc, DeviceInput{
		Name:       "КТ315Б",
		Attributes: []catalog.AttributeValue{attrText("package", "КТ-13")},
	}); out != OutcomeUpdatedExisting {
		t.Fatalf("обновление без секции: %s", out)
	}
	card, _, _ = svc.Get(ctx, "transistor", "КТ315Б")
	if _, ok := card.FieldByName("dev_number"); !ok {
		t.Fatalf("продукт не восстановлен без секции fields: %+v", card.Fields)
	}
	if f, ok := card.FieldByName("category"); !ok || f.String() != "switching" {
		t.Fatalf("явное поле category изменено: %+v", f)
	}
}

// suiteRevisions — счётчики ревизий: инкремент только при фактическом
// изменении (Skipped не меняет).
func suiteRevisions(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	catBefore, dataBefore, _ := app.Revisions(ctx)
	mustUpsert(t, svc, DeviceInput{
		Name:       "КТ315Б",
		Attributes: []catalog.AttributeValue{attrText("structure", "npn")},
	})
	catAfter, dataAfter, _ := app.Revisions(ctx)
	if catAfter != catBefore || dataAfter != dataBefore+1 {
		t.Fatalf("ревизии после добавления: catalog %d→%d data %d→%d",
			catBefore, catAfter, dataBefore, dataAfter)
	}

	// Идемпотентный повтор — Skipped, ревизии на месте.
	mustUpsert(t, svc, DeviceInput{
		Name:       "КТ315Б",
		Attributes: []catalog.AttributeValue{attrText("structure", "npn")},
	})
	_, dataAgain, _ := app.Revisions(ctx)
	if dataAgain != dataAfter {
		t.Fatalf("data_revision при Skipped: %d → %d", dataAfter, dataAgain)
	}

	// Импорт каталога меняет только catalog_revision.
	err := app.Services().Catalog.Import(ctx, catalog.Input{
		Units: []catalog.UnitDef{{Code: "test_unit"}},
	})
	if err != nil {
		t.Fatalf("импорт каталога: %v", err)
	}
	catNext, dataNext, _ := app.Revisions(ctx)
	if catNext != catAfter+1 || dataNext != dataAgain {
		t.Fatalf("ревизии после каталога: catalog %d→%d data %d→%d",
			catAfter, catNext, dataAgain, dataNext)
	}
}

// suiteCatalogImport — расширение каталога данными: новое семейство series
// работает end-to-end без правки кода; неверный каталог отвергается целиком.
func suiteCatalogImport(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	cat := app.Services().Catalog

	// Новое семейство — данными каталога.
	err := cat.Import(ctx, catalog.Input{
		SeriesFamilies: []catalog.SeriesFamilyDef{
			{Series: "ФГТ", Kind: domain.KindTransistor},
		},
	})
	if err != nil {
		t.Fatalf("импорт семейства: %v", err)
	}
	snap, err := cat.Snapshot(ctx)
	if err != nil {
		t.Fatalf("снимок: %v", err)
	}
	if _, ok := snap.Family("ФГТ", domain.KindTransistor); !ok {
		t.Fatal("семейство не появилось в снимке")
	}

	// Запись нового семейства — end-to-end (автодетект по реестру каталога).
	if out := mustUpsert(t, svc, DeviceInput{
		Name:       "ФГТ-5",
		Attributes: []catalog.AttributeValue{attrText("structure", "npn")},
		Sections: []SectionInput{
			section("parameters", pvRange("h21e", 40, 200, cond("Uke", 5), cond("Ik", 5))),
		},
	}); out != OutcomeAdded {
		t.Fatalf("исход: %s", out)
	}
	card, ok, err := svc.Get(ctx, "transistor", "ФГТ-5")
	if err != nil || !ok || card.System != domain.SystemSeries {
		t.Fatalf("карточка ФГТ-5: ok=%v err=%v system=%s", ok, err, card.System)
	}
	if f, ok := card.FieldByName("series"); !ok || f.String() != "ФГТ" {
		t.Fatalf("поле series: %+v", f)
	}

	// Неверный каталог — проблемы метасхемы, применение целиком отменено.
	err = cat.Import(ctx, catalog.Input{
		Rules: []catalog.RuleDef{{Code: "nosuch_rule"}},
	})
	wantDomainError(t, err, domain.CodeInvalidImportFile,
		"catalog: section validation_rules: unknown rule «nosuch_rule»")

	snap, _ = cat.Snapshot(ctx)
	if _, ok := snap.Family("ФГТ", domain.KindTransistor); !ok {
		t.Fatal("семейство потеряно при отвергнутом импорте")
	}
}

// suiteParseSuggest — разбор обозначений и автодополнение.
func suiteParseSuggest(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices
	ds := app.Services().Designations
	seedTransistors(t, svc)

	p, err := ds.Parse(ctx, "млт-0.5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Kind != domain.KindResistor || p.System != domain.SystemSeries || p.Designation != "МЛТ-0.5" {
		t.Fatalf("parse МЛТ-0.5: %+v", p)
	}
	if f, ok := p.FieldByName("power"); !ok || f.Num != 0.5 {
		t.Fatalf("power: %+v", f)
	}

	if _, err := ds.Parse(ctx, "QXZ-1"); err == nil {
		t.Fatal("неизвестное семейство должно ошибкой")
	}

	sugg, err := ds.Suggest(ctx, "КТ3", "", 10)
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(sugg) != 1 || sugg[0].Designation != "КТ315Б" || sugg[0].System != domain.SystemGost {
		t.Fatalf("suggest: %+v", sugg)
	}
	sugg, err = ds.Suggest(ctx, "КТ3", domain.KindResistor, 10)
	if err != nil || len(sugg) != 0 {
		t.Fatalf("suggest по классу: %+v err=%v", sugg, err)
	}
}

// suiteClassificationFields — явные классификационные поля (секция fields):
// пер-полое правило (парсер системы сам не устанавливает поле),
// применимость к классу, коды словарей, семантика секции (nil — не менять,
// задано — заменить целиком) и Skipped по каноническому сравнению.
func suiteClassificationFields(t *testing.T, factory configFactory) {
	ctx := context.Background()
	app := openSuiteApp(t, factory)
	svc := app.Services().Devices

	// Создание с полями; повторное применение того же входа — Skipped.
	fields := []domain.Field{
		domain.TextField("material", "si"),
		domain.TextField("subclass", "bjt"),
		domain.TextField("category", "general_purpose"),
	}
	in := DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &fields}
	if out := mustUpsert(t, svc, in); out != OutcomeAdded {
		t.Fatalf("исход: %v", out)
	}
	if out := mustUpsert(t, svc, in); out != OutcomeSkipped {
		t.Fatalf("повтор: %v", out)
	}
	card, found, err := svc.Get(ctx, domain.KindTransistor, "MJE340")
	if err != nil || !found {
		t.Fatalf("карточка: %v %v", err, found)
	}
	if f, ok := card.FieldByName("category"); !ok || f.Text != "general_purpose" {
		t.Fatalf("category: %v", f)
	}

	// Замена набора целиком: category исчезает, material меняется.
	fields2 := []domain.Field{domain.TextField("material", "ge")}
	in.Fields = &fields2
	if out := mustUpsert(t, svc, in); out != OutcomeUpdatedExisting {
		t.Fatalf("замена: %v", out)
	}
	card, _, _ = svc.Get(ctx, domain.KindTransistor, "MJE340")
	if f, ok := card.FieldByName("material"); !ok || f.Text != "ge" {
		t.Fatalf("material после замены: %v", f)
	}
	if _, ok := card.FieldByName("category"); ok {
		t.Fatal("category не удалён заменой набора")
	}

	// nil — не менять: поля сохраняются.
	in.Fields = nil
	if out := mustUpsert(t, svc, in); out != OutcomeSkipped {
		t.Fatalf("nil-секция: %v", out)
	}

	// Пер-полое правило: поле парсера системы явно не задаётся.
	bad := []domain.Field{domain.TextField("material", "si")}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "КТ315Б", Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"field «material» is derived from the designation by the gost parser and cannot be set explicitly")
	// Грамматическое поле даже для other — не классификационное.
	bad = []domain.Field{domain.TextField("dev_number", "315")}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"unknown classification field «dev_number» (allowed: material, subclass, adjustment, category, assembly)")
	// Применимость к классу: adjustment — только резисторы/конденсаторы.
	bad = []domain.Field{domain.TextField("adjustment", "variable")}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"field «adjustment» is not applicable to kind transistor")
	// Коды словарей и диапазон assembly.
	bad = []domain.Field{domain.TextField("subclass", "thyristor2")}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"unknown subclass «thyristor2» (allowed: bjt, fet, ujt, avalanche, thyristor, triac, rectifier, zener, varicap, tunnel, gunn, generator, led, detector, signal, multiplier, magnetic, photo or a localized display name)")
	bad = []domain.Field{domain.TextField("category", "bogus")}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"unknown category «bogus» (allowed: audio, composite, fast, general_purpose, high_voltage, lownoise, power, precision, pulse, rf, switching)")
	bad = []domain.Field{domain.NumField("assembly", 2)}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"value of field assembly must be 0 or 1")
	// Дубликат поля.
	bad = []domain.Field{domain.TextField("material", "si"), domain.TextField("material", "ge")}
	_, err = svc.Upsert(ctx, DeviceInput{Name: "MJE340", System: domain.SystemOther, Kind: domain.KindTransistor, Fields: &bad})
	wantDomainError(t, err, domain.CodeValidationFailed,
		"classification field «material» is set more than once")

	// Отвергнутый вход не меняет запись (атомарность).
	card, _, _ = svc.Get(ctx, domain.KindTransistor, "MJE340")
	if f, ok := card.FieldByName("material"); !ok || f.Text != "ge" {
		t.Fatalf("material после отказа: %v", f)
	}
}
