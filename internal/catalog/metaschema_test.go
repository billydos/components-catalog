package catalog_test

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Состав стартового каталога (03 §3–§9): количества строк по разделам.
func TestSeedSnapshotCounts(t *testing.T) {
	snap := seedSnapshot(t)
	want := struct {
		units, conditions, groups, params, attrs, rules int
	}{23, 17, 3, 76, 17, 5}
	if got := len(snap.Units); got != want.units {
		t.Errorf("единиц: %d, ожидалось %d", got, want.units)
	}
	if got := len(snap.Conditions); got != want.conditions {
		t.Errorf("условий: %d, ожидалось %d", got, want.conditions)
	}
	if got := len(snap.Groups); got != want.groups {
		t.Errorf("групп: %d, ожидалось %d", got, want.groups)
	}
	if got := len(snap.Parameters); got != want.params {
		t.Errorf("параметров: %d, ожидалось %d", got, want.params)
	}
	if got := len(snap.Attributes); got != want.attrs {
		t.Errorf("атрибутов: %d, ожидалось %d", got, want.attrs)
	}
	if got := len(snap.Rules); got != want.rules {
		t.Errorf("правил: %d, ожидалось %d", got, want.rules)
	}
}

// Нарушения метасхемы (03 §11) — каждая проверка даёт дословное сообщение.
func TestMetaschemaViolations(t *testing.T) {
	p := func(mutate func(*catalog.ParameterDef)) catalog.ParameterDef {
		base, ok := seedSnapshot(t).Parameter("Kpd")
		if !ok {
			t.Fatal("Kpd отсутствует")
		}
		row := *base
		mutate(&row)
		return row
	}
	cases := []struct {
		name string
		in   catalog.Input
		want string
	}{
		{"несуществующая единица", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.Unit = "кОм" }),
		}}, "каталог: параметр «Kpd»: единица «кОм» не существует"},
		{"несуществующая группа", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.Group = "no_such_group" }),
		}}, "каталог: параметр «Kpd»: группа «no_such_group» не существует"},
		{"enum без значений", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.ValueType = catalog.ValueEnum; r.Unit = ""; r.EnumValues = nil }),
		}}, "каталог: параметр «Kpd»: тип enum требует непустой список значений"},
		{"единица у enum", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) {
				r.ValueType = catalog.ValueEnum
				r.EnumValues = []string{"x"}
			}),
		}}, "каталог: параметр «Kpd» типа enum не должен иметь единицу измерения"},
		{"enum-значения у численного типа", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.EnumValues = []string{"x"} }),
		}}, "каталог: параметр «Kpd»: enum-значения допустимы только для типа enum"},
		{"неположительный потолок", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.Ceiling = f(0) }),
		}}, "каталог: параметр «Kpd»: потолок должен быть положительным"},
		{"отрицательный порядок", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.SortOrder = -1 }),
		}}, "каталог: параметр «Kpd»: порядок сортировки должен быть неотрицательным"},
		{"неизвестный класс применимости", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.Kinds = []domain.Kind{"thyristor"} }),
		}}, "каталог: параметр «Kpd»: класс применимости «thyristor» не существует"},
		{"правило атрибутов у параметра", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.ValidationRule = "year_range" }),
		}}, "каталог: параметр «Kpd»: правило «year_range» не применяется к параметрам"},
		{"пустой набор условий", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) { r.ConditionSets = []catalog.ConditionSet{{No: 1}} }),
		}}, "каталог: параметр «Kpd»: набор условий 1 пуст"},
		{"несуществующее условие набора", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) {
				r.ConditionSets = []catalog.ConditionSet{{No: 1, Items: []catalog.ConditionSetItem{{Condition: "no_such", Mode: catalog.ModeRequired}}}}
			}),
		}}, "каталог: параметр «Kpd»: набор условий 1: условие «no_such» не существует"},
		{"fixed_value у optional", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(r *catalog.ParameterDef) {
				r.ConditionSets = []catalog.ConditionSet{{No: 1, Items: []catalog.ConditionSetItem{{Condition: "freq", Mode: catalog.ModeOptional, FixedValue: f(1)}}}}
			}),
		}}, "каталог: параметр «Kpd»: набор условий 1, условие «freq»: fixed_value допустим только у обязательного условия"},
		{"дубликат кода параметра", catalog.Input{Parameters: []catalog.ParameterDef{
			p(func(*catalog.ParameterDef) {}), p(func(*catalog.ParameterDef) {}),
		}}, "каталог: раздел parameters: дубликат кода «Kpd»"},
		{"неизвестное правило входа", catalog.Input{Rules: []catalog.RuleDef{
			{Code: "no_such_rule", Description: "x"},
		}}, "каталог: раздел validation_rules: неизвестное правило «no_such_rule»"},
		{"описание правила разошлось", catalog.Input{Rules: []catalog.RuleDef{
			{Code: "temp_pair", Description: "другая семантика"},
		}}, "каталог: правило «temp_pair»: описание расходится с реализацией (реестр: «согласованность температурной пары: TempMin < TempMax и opTempMin < opTempMax (если заданы оба)»)"},
		{"несуществующая единица условия", catalog.Input{Conditions: []catalog.ConditionDef{
			{Code: "Unew", Name: "новое", Unit: "мкВ"},
		}}, "каталог: условие «Unew»: единица «мкВ» не существует"},
		{"конфликт имени секции", catalog.Input{Groups: []catalog.GroupDef{
			{Code: "extra", SectionName: "ratings", DisplayName: "Ещё одна", SortOrder: 40},
		}}, "каталог: группа «extra»: имя секции «ratings» уже используется группой «limiting»"},
		{"неизвестный класс семейства", catalog.Input{SeriesFamilies: []catalog.SeriesFamilyDef{
			{Series: "XX", Kind: "thyristor", Name: "тест"},
		}}, "каталог: семейство «XX»: класс «thyristor» не существует"},
		{"неизвестная семантика хвоста", catalog.Input{SeriesFamilies: []catalog.SeriesFamilyDef{
			{Series: "XX", Kind: domain.KindResistor, Name: "тест", TailSemantic: "voltage"},
		}}, "каталог: семейство «XX» (класс resistor): неизвестная семантика хвоста «voltage»"},
		{"семейство разбирается строгой системой", catalog.Input{SeriesFamilies: []catalog.SeriesFamilyDef{
			{Series: "ГТ308", Kind: domain.KindTransistor, Name: "коллизия"},
		}}, "каталог: семейство «ГТ308»: код разбирается строгой системой «gost» — нарушен инвариант реестра series"},
		{"несуществующая система применимости", catalog.Input{SystemKinds: []catalog.SystemKindRef{
			{System: "din", Kind: domain.KindResistor},
		}}, "каталог: применимость систем: система «din» не существует"},
		{"несуществующий класс привязки правила", catalog.Input{KindRules: []catalog.KindRuleRef{
			{Kind: "thyristor", Rule: "cap_variant_matrix"},
		}}, "каталог: правило класса записей: класс «thyristor» не существует"},
		{"правило параметров у класса", catalog.Input{KindRules: []catalog.KindRuleRef{
			{Kind: domain.KindDiode, Rule: "temp_pair"},
		}}, "каталог: правило класса записей (diode): правило «temp_pair» не применяется к записям класса"},
	}
	base := seedSnapshot(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, probs := catalog.ApplyCatalog(base, tc.in)
			hasProblem(t, probs, tc.want)
		})
	}
}

// Привязка правила класса обязана ссылаться на строку validation_rules:
// мини-каталог с классом, но без строк правил.
func TestMetaschemaKindRuleWithoutRow(t *testing.T) {
	in := catalog.Input{
		Kinds:     []catalog.KindDef{{Code: domain.KindDiode, Name: "диоды"}},
		KindRules: []catalog.KindRuleRef{{Kind: domain.KindDiode, Rule: "cap_variant_matrix"}},
	}
	_, probs := catalog.ApplyCatalog(nil, in)
	hasProblem(t, probs,
		"каталог: правило класса записей (diode): строка правила «cap_variant_matrix» отсутствует в разделе validation_rules")
}

// Атрибуты: нарушения метасхемы по типам значений и привязкам правил.
func TestMetaschemaAttributeViolations(t *testing.T) {
	base, ok := seedSnapshot(t).Attribute("category")
	if !ok {
		t.Fatal("category отсутствует")
	}
	cases := []struct {
		name string
		in   catalog.Input
		want string
	}{
		{"неизвестный тип атрибута", catalog.Input{Attributes: []catalog.AttributeDef{
			withAttrType(base, "string"),
		}}, "каталог: атрибут «category»: неизвестный тип значения «string»"},
		{"несуществующая единица атрибута", catalog.Input{Attributes: []catalog.AttributeDef{
			withAttrUnit(yearFrom(t), "мкВ"),
		}}, "каталог: атрибут «yearFrom»: единица «мкВ» не существует"},
		{"enum атрибута без значений", catalog.Input{Attributes: []catalog.AttributeDef{
			withAttrType(base, catalog.AttrEnum),
		}}, "каталог: атрибут «category»: тип enum требует непустой список значений"},
		{"правило параметров у атрибута", catalog.Input{Attributes: []catalog.AttributeDef{
			withAttrRule(base, "temp_pair"),
		}}, "каталог: атрибут «category»: правило «temp_pair» не применяется к атрибутам"},
		{"неизвестный класс применимости атрибута", catalog.Input{Attributes: []catalog.AttributeDef{
			withAttrKinds(base, domain.Kind("thyristor")),
		}}, "каталог: атрибут «category»: класс применимости «thyristor» не существует"},
	}
	snap := seedSnapshot(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, probs := catalog.ApplyCatalog(snap, tc.in)
			hasProblem(t, probs, tc.want)
		})
	}
}

func yearFrom(t *testing.T) *catalog.AttributeDef {
	t.Helper()
	base, ok := seedSnapshot(t).Attribute("yearFrom")
	if !ok {
		t.Fatal("yearFrom отсутствует")
	}
	return base
}

func withAttrType(base *catalog.AttributeDef, t catalog.AttrType) catalog.AttributeDef {
	row := *base
	row.Type = t
	return row
}

func withAttrUnit(base *catalog.AttributeDef, unit string) catalog.AttributeDef {
	row := *base
	row.Unit = unit
	return row
}

func withAttrRule(base *catalog.AttributeDef, rule string) catalog.AttributeDef {
	row := *base
	row.ValidationRule = rule
	return row
}

func withAttrKinds(base *catalog.AttributeDef, kinds ...domain.Kind) catalog.AttributeDef {
	row := *base
	row.Kinds = kinds
	return row
}

// Применение каталога — upsert по коду (02 §5.4): обновление полей,
// замещение enum-значений/наборов условий/применимости целиком,
// добавление новых строк, деактивация вместо удаления.
func TestApplyCatalogUpsert(t *testing.T) {
	snap := seedSnapshot(t)

	t.Run("обновление потолка", func(t *testing.T) {
		row, _ := snap.Parameter("Kpd")
		updated := *row
		updated.Ceiling = f(90)
		out, probs := catalog.ApplyCatalog(snap, catalog.Input{Parameters: []catalog.ParameterDef{updated}})
		if len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
		got, _ := out.Parameter("Kpd")
		if got.Ceiling == nil || *got.Ceiling != 90 {
			t.Fatalf("потолок не обновлён: %v", got.Ceiling)
		}
	})

	t.Run("применимость замещается целиком", func(t *testing.T) {
		row, _ := snap.Parameter("Dop")
		updated := *row
		updated.Kinds = []domain.Kind{domain.KindResistor}
		out, probs := catalog.ApplyCatalog(snap, catalog.Input{Parameters: []catalog.ParameterDef{updated}})
		if len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
		got, _ := out.Parameter("Dop")
		if got.AppliesTo(domain.KindCapacitor) {
			t.Fatal("применимость Dop к конденсаторам должна быть снята замещением")
		}
		if !got.AppliesTo(domain.KindResistor) {
			t.Fatal("применимость Dop к резисторам потеряна")
		}
	})

	t.Run("enum замещается целиком", func(t *testing.T) {
		row, _ := snap.Parameter("TKE")
		updated := *row
		updated.EnumValues = []string{"МП0", "Н90"}
		out, probs := catalog.ApplyCatalog(snap, catalog.Input{Parameters: []catalog.ParameterDef{updated}})
		if len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
		got, _ := out.Parameter("TKE")
		if !slices.Equal(got.EnumValues, []string{"МП0", "Н90"}) {
			t.Fatalf("enum не замещён: %v", got.EnumValues)
		}
	})

	t.Run("применимость систем — только добавление", func(t *testing.T) {
		out, probs := catalog.ApplyCatalog(snap, catalog.Input{SystemKinds: []catalog.SystemKindRef{
			{System: domain.SystemGost, Kind: domain.KindDiode},
		}})
		if len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
		if len(out.SystemKinds) != len(snap.SystemKinds) {
			t.Fatal("повторная привязка не должна дублировать строку")
		}
	})

	t.Run("исходный снимок не мутирует", func(t *testing.T) {
		row, _ := snap.Parameter("TKE")
		updated := *row
		updated.EnumValues = []string{"МП0"}
		catalog.ApplyCatalog(snap, catalog.Input{Parameters: []catalog.ParameterDef{updated}})
		got, _ := snap.Parameter("TKE")
		if len(got.EnumValues) != 23 {
			t.Fatalf("исходный снимок изменён: %d значений", len(got.EnumValues))
		}
	})
}

// Критерий этапа 2: расширение каталога только вставкой строк — новый
// параметр с новым условием и новой единицей — проходит end-to-end
// (применение → метасхема → валидация значений) без правки кода.
func TestCatalogExtensionEndToEnd(t *testing.T) {
	snap := seedSnapshot(t)
	extension := catalog.Input{
		Units: []catalog.UnitDef{{Code: "В/мкс", Name: "вольт на микросекунду", Symbol: "В/мкс"}},
		Conditions: []catalog.ConditionDef{
			{Code: "Ugs", Name: "напряжение затвор-исток", Unit: "В"},
			{Code: "Rg_ext", Name: "сопротивление в цепи затвора", Unit: "Ом"},
		},
		Parameters: []catalog.ParameterDef{
			{Code: "UgsThr", Group: "electrical", DisplayName: "пороговое напряжение затвор-исток",
				Unit: "В", ValueType: catalog.ValueRange, SortOrder: 460, Active: true,
				Kinds: []domain.Kind{domain.KindTransistor},
				ConditionSets: []catalog.ConditionSet{
					{No: 1, Items: []catalog.ConditionSetItem{
						{Condition: "Uke", Mode: catalog.ModeRequired},
						{Condition: "Rg_ext", Mode: catalog.ModeOptional},
					}},
				}},
			{Code: "dUdt", Group: "electrical", DisplayName: "скорость нарастания", Unit: "В/мкс",
				ValueType: catalog.ValueAtMost, SortOrder: 470, Active: true},
		},
	}
	out, probs := catalog.ApplyCatalog(snap, extension)
	if len(probs) != 0 {
		t.Fatalf("расширение каталога: %v", messages(probs))
	}
	e := catalog.NewEngine(out)
	d := catalog.Device{
		Kind: domain.KindTransistor, System: domain.SystemGost, Designation: "2П798Г",
		Values: []catalog.ParameterValue{
			{Parameter: "UgsThr", Section: "parameters", Min: f(2), Max: f(4),
				Conditions: []catalog.ConditionValue{{Condition: "Uke", Value: 10}, {Condition: "Rg_ext", Value: 100}}},
			{Parameter: "dUdt", Section: "parameters", Max: f(500)},
		},
	}
	if probs := e.ValidateDevice(&d); len(probs) != 0 {
		t.Fatalf("значения нового каталога: %v", messages(probs))
	}
	// Старые определения не пострадали.
	old := catalog.Device{
		Kind: domain.KindDiode, System: domain.SystemGost, Designation: "КД522Б",
		Values: []catalog.ParameterValue{
			{Parameter: "Upr", Section: "parameters", Max: f(1),
				Conditions: []catalog.ConditionValue{{Condition: "Ipr", Value: 10}}},
		},
	}
	if probs := e.ValidateDevice(&old); len(probs) != 0 {
		t.Fatalf("старые значения: %v", messages(probs))
	}
	// Условия нового параметра работают: постороннее условие запрещено.
	d.Values[0].Conditions = append(d.Values[0].Conditions, catalog.ConditionValue{Condition: "Ik", Value: 1})
	if probs := e.ValidateDevice(&d); len(probs) == 0 {
		t.Fatal("постороннее условие не отвергнуто")
	}
}
