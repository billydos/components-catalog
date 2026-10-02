package catalog_test

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Реестр правил: состав, сортировка, неизвестный код.
func TestRuleRegistry(t *testing.T) {
	want := []string{
		"cap_dimensions_form",
		"cap_variant_matrix",
		"resistor_variant_power",
		"temp_pair",
		"year_range",
	}
	rules := catalog.Rules()
	got := make([]string, 0, len(rules))
	for _, r := range rules {
		got = append(got, r.Code())
		if r.Description() == "" {
			t.Errorf("правило %s без описания", r.Code())
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("реестр правил:\n got:  %v\n want: %v", got, want)
	}
	if _, ok := catalog.RuleByCode("no_such_rule"); ok {
		t.Error("неизвестное правило ошибочно найдено")
	}
	if _, ok := catalog.RuleByCode("year_range"); !ok {
		t.Error("year_range отсутствует в реестре")
	}
}

// year_range: границы 1949–2100 и порядок год < год (03 §10).
func TestYearRangeRule(t *testing.T) {
	e := seedEngine(t)
	cases := []struct {
		name string
		avs  []catalog.AttributeValue
		want string
	}{
		{"корректно", []catalog.AttributeValue{
			{Attribute: "yearFrom", Num: f(1967)}, {Attribute: "yearTo", Num: f(1992)}}, ""},
		{"только год начала", []catalog.AttributeValue{{Attribute: "yearFrom", Num: f(1967)}}, ""},
		{"раньше 1949", []catalog.AttributeValue{{Attribute: "yearFrom", Num: f(1930)}},
			"атрибут «yearFrom»: год вне диапазона 1949–2100"},
		{"позже 2100", []catalog.AttributeValue{{Attribute: "yearTo", Num: f(2200)}},
			"атрибут «yearTo»: год вне диапазона 1949–2100"},
		{"начало позже окончания", []catalog.AttributeValue{
			{Attribute: "yearFrom", Num: f(1992)}, {Attribute: "yearTo", Num: f(1967)}},
			"атрибуты «yearFrom» и «yearTo»: год начала должен быть меньше года окончания"},
		{"равные годы", []catalog.AttributeValue{
			{Attribute: "yearFrom", Num: f(1980)}, {Attribute: "yearTo", Num: f(1980)}},
			"атрибуты «yearFrom» и «yearTo»: год начала должен быть меньше года окончания"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := catalog.Device{
				Kind: domain.KindDiode, System: domain.SystemGost, Designation: "КД522Б",
				Attributes: tc.avs,
			}
			probs := e.ValidateDevice(&d)
			if tc.want == "" {
				if len(probs) != 0 {
					t.Fatalf(" got: %v", messages(probs))
				}
				return
			}
			if len(probs) != 1 || probs[0].Message != tc.want {
				t.Fatalf(" got: %v\n want: %s", messages(probs), tc.want)
			}
		})
	}
}

// temp_pair: обе пары (TempMin/TempMax и opTempMin/opTempMax), одностороннее
// задание корректно (03 §10).
func TestTempPairRule(t *testing.T) {
	e := seedEngine(t)
	t.Run("TempMin < TempMax", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{
				{Parameter: "TempMin", Section: "ratings", Exact: f(-60)},
				{Parameter: "TempMax", Section: "ratings", Exact: f(100)},
			},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("TempMin ≥ TempMax", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{
				{Parameter: "TempMin", Section: "ratings", Exact: f(100)},
				{Parameter: "TempMax", Section: "ratings", Exact: f(50)},
			},
		}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "параметр «TempMin» должен быть меньше параметра «TempMax»" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("только TempMin", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{{Parameter: "TempMin", Section: "ratings", Exact: f(-60)}},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("opTempMin ≥ opTempMax", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindCapacitor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{
				{Parameter: "opTempMin", Section: "parameters", Exact: f(70)},
				{Parameter: "opTempMax", Section: "parameters", Exact: f(25)},
			},
		}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "параметр «opTempMin» должен быть меньше параметра «opTempMax»" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
}

// cap_dimensions_form: прямоугольный, осевой и радиальный
// цилиндрические наборы, смешение и неполные наборы (03 §10).
func TestCapDimensionsFormRule(t *testing.T) {
	e := seedEngine(t)
	dims := func(vals ...catalog.ParameterValue) []catalog.ParameterValue {
		return vals
	}
	rect := dims(
		catalog.ParameterValue{Parameter: "length", Section: "dimensions", Exact: f(6)},
		catalog.ParameterValue{Parameter: "width", Section: "dimensions", Exact: f(4)},
		catalog.ParameterValue{Parameter: "height", Section: "dimensions", Exact: f(5)},
	)
	cyl := dims(
		catalog.ParameterValue{Parameter: "diameter", Section: "dimensions", Exact: f(8)},
		catalog.ParameterValue{Parameter: "leadLength", Section: "dimensions", Exact: f(12)},
	)
	radial := dims(
		catalog.ParameterValue{Parameter: "diameter", Section: "dimensions", Exact: f(18)},
		catalog.ParameterValue{Parameter: "height", Section: "dimensions", Exact: f(35)},
	)
	cases := []struct {
		name  string
		vals  []catalog.ParameterValue
		want  string
		extra bool // добавлять massMax (нейтрален для формы)
	}{
		{"прямоугольный", rect, "", true},
		{"осевой цилиндрический", cyl, "", true},
		{"радиальный цилиндрический", radial, "", true},
		{"смешение", append(slices.Clone(rect), cyl...),
			"габариты: смешение форм корпуса — прямоугольная (length+width+height) и цилиндрическая (diameter+leadLength/height)", false},
		{"радиальный с шириной", append(slices.Clone(radial),
			catalog.ParameterValue{Parameter: "width", Section: "dimensions", Exact: f(4)}),
			"габариты: смешение форм корпуса — прямоугольная (length+width+height) и цилиндрическая (diameter+leadLength/height)", false},
		{"неполный прямоугольный", rect[:2],
			"габариты: неполный прямоугольный набор корпуса — требуются length, width и height", false},
		{"неполный цилиндрический", cyl[:1],
			"габариты: неполный цилиндрический набор корпуса — требуются diameter и leadLength (осевые) либо diameter и height (радиальные)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vals := slices.Clone(tc.vals)
			if tc.extra {
				vals = append(vals, catalog.ParameterValue{Parameter: "massMax", Section: "dimensions", Exact: f(1)})
			}
			d := catalog.Device{
				Kind: domain.KindCapacitor, System: domain.SystemGost, Designation: "К10-17Б",
				Values: vals,
			}
			probs := e.ValidateDevice(&d)
			if tc.want == "" {
				if len(probs) != 0 {
					t.Fatalf(" got: %v", messages(probs))
				}
				return
			}
			if len(probs) != 1 || probs[0].Message != tc.want {
				t.Fatalf(" got: %v\n want: %s", messages(probs), tc.want)
			}
		})
	}
	t.Run("только масса", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindResistor, System: domain.SystemGost, Designation: "С2-33Н",
			Values: []catalog.ParameterValue{{Parameter: "massMax", Section: "dimensions", Exact: f(0.15)}},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("только высота без диаметра", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindCapacitor, System: domain.SystemGost, Designation: "К10-17Б",
			Values: []catalog.ParameterValue{{Parameter: "height", Section: "dimensions", Exact: f(5)}},
		}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "габариты: неполный прямоугольный набор корпуса — требуются length, width и height" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
}

// resistor_variant_power: ряд мощностей резистора из плана 06 §3 (С2-33Н).
func TestResistorVariants(t *testing.T) {
	e := seedEngine(t)
	base := catalog.Device{
		Kind: domain.KindResistor, System: domain.SystemGost, Designation: "С2-33Н",
		Values: []catalog.ParameterValue{
			{Parameter: "Rnom", Section: "parameters", Min: f(1), Max: f(10000000)},
			{Parameter: "Dop", Section: "parameters", Max: f(5)},
			{Parameter: "TempMin", Section: "ratings", Exact: f(-60)},
			{Parameter: "TempMax", Section: "ratings", Exact: f(125)},
		},
		Variants: []catalog.Variant{
			{Label: "0.125 Вт", Values: []catalog.ParameterValue{
				{Parameter: "Pnom", Section: "ratings", Exact: f(0.125)},
				{Parameter: "Umax", Section: "ratings", Exact: f(250)},
				{Parameter: "massMax", Section: "dimensions", Exact: f(0.15)},
			}},
			{Label: "1 Вт", Values: []catalog.ParameterValue{
				{Parameter: "Pnom", Section: "ratings", Exact: f(1)},
				{Parameter: "Umax", Section: "ratings", Exact: f(500)},
				{Parameter: "massMax", Section: "dimensions", Exact: f(0.8)},
			}},
		},
	}
	if probs := e.ValidateDevice(&base); len(probs) != 0 {
		t.Fatalf("ряд мощностей С2-33Н: %v", messages(probs))
	}

	t.Run("вариант без Pnom", func(t *testing.T) {
		d := base
		d.Variants = []catalog.Variant{{Label: "0.5 Вт", Values: []catalog.ParameterValue{
			{Parameter: "Umax", Section: "ratings", Exact: f(350)},
		}}}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "вариант «0.5 Вт»: отсутствует обязательный параметр Pnom" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("повтор Pnom", func(t *testing.T) {
		d := base
		d.Variants = append(slices.Clone(d.Variants), catalog.Variant{Label: "повтор", Values: []catalog.ParameterValue{
			{Parameter: "Pnom", Section: "ratings", Exact: f(1)},
		}})
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "вариант «повтор»: Pnom повторяется (уже задан вариантом «1 Вт»)" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("повтор метки", func(t *testing.T) {
		d := base
		d.Variants = append(slices.Clone(d.Variants), catalog.Variant{Label: "1 Вт", Values: []catalog.ParameterValue{
			{Parameter: "Pnom", Section: "ratings", Exact: f(2)},
		}})
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "вариант «1 Вт»: метка повторяется" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("вариант без метки в сообщении", func(t *testing.T) {
		d := base
		d.Variants = []catalog.Variant{{Values: nil}}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "вариант №1: отсутствует обязательный параметр Pnom" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("транзистор без вариантов", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost, Designation: "КТ315Б",
			Variants: []catalog.Variant{{Label: "бис", Values: []catalog.ParameterValue{
				{Parameter: "UkeoMax", Section: "ratings", Exact: f(20)},
			}}},
		}
		probs := e.ValidateDevice(&d)
		hasProblem(t, probs, "класс transistor не поддерживает исполнения (варианты)")
	})
}

// cap_variant_matrix: К50-35 из плана 06 §4 — матрица «ёмкость ×
// напряжение → габариты/масса».
func TestCapacitorVariants(t *testing.T) {
	e := seedEngine(t)
	base := catalog.Device{
		Kind: domain.KindCapacitor, System: domain.SystemGost, Designation: "К50-35",
		Attributes: []catalog.AttributeValue{
			{Attribute: "polarized", Bool: b(true)},
			{Attribute: "tu", Text: s("ОЖ0.464.036ТУ")},
			{Attribute: "yearFrom", Num: f(1980)},
		},
		Values: []catalog.ParameterValue{
			{Parameter: "Dop", Section: "parameters", Max: f(20)},
			{Parameter: "Tgd", Section: "parameters", Max: f(0.15), Conditions: []catalog.ConditionValue{{Condition: "temp", Value: 20}}},
		},
		Variants: []catalog.Variant{
			{Label: "160 В", Values: []catalog.ParameterValue{
				{Parameter: "Unom", Section: "parameters", Exact: f(160)},
				{Parameter: "Cnom", Section: "parameters", Min: f(1000000), Max: f(10000000)},
				{Parameter: "diameter", Section: "dimensions", Exact: f(8)},
				{Parameter: "leadLength", Section: "dimensions", Exact: f(12)},
				{Parameter: "massMax", Section: "dimensions", Exact: f(1.5)},
			}},
			{Label: "25 В", Values: []catalog.ParameterValue{
				{Parameter: "Unom", Section: "parameters", Exact: f(25)},
				{Parameter: "Cnom", Section: "parameters", Min: f(47000000), Max: f(4700000000)},
				{Parameter: "diameter", Section: "dimensions", Exact: f(10)},
				{Parameter: "leadLength", Section: "dimensions", Exact: f(16)},
				{Parameter: "massMax", Section: "dimensions", Exact: f(3)},
			}},
		},
	}
	if probs := e.ValidateDevice(&base); len(probs) != 0 {
		t.Fatalf("матрица К50-35: %v", messages(probs))
	}

	t.Run("вариант без Cnom", func(t *testing.T) {
		d := base
		d.Variants = []catalog.Variant{{Label: "160 В", Values: []catalog.ParameterValue{
			{Parameter: "Unom", Section: "parameters", Exact: f(160)},
		}}}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "вариант «160 В»: отсутствует обязательный параметр Cnom" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("повтор Unom", func(t *testing.T) {
		d := base
		d.Variants = append(slices.Clone(d.Variants), catalog.Variant{Label: "160 В ", Values: []catalog.ParameterValue{
			{Parameter: "Unom", Section: "parameters", Exact: f(160)},
			{Parameter: "Cnom", Section: "parameters", Min: f(1), Max: f(2)},
		}})
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "вариант «160 В »: Unom повторяется (уже задан вариантом «160 В»)" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("габариты варианта несогласованы", func(t *testing.T) {
		d := base
		d.Variants = []catalog.Variant{{Label: "63 В", Values: []catalog.ParameterValue{
			{Parameter: "Unom", Section: "parameters", Exact: f(63)},
			{Parameter: "Cnom", Section: "parameters", Min: f(1), Max: f(2)},
			{Parameter: "length", Section: "dimensions", Exact: f(10)},
		}}}
		probs := e.ValidateDevice(&d)
		hasProblem(t, probs, "габариты: неполный прямоугольный набор корпуса — требуются length, width и height")
	})
	t.Run("уровень типа без вариантов", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindCapacitor, System: domain.SystemGost, Designation: "К10-17Б",
			Values: []catalog.ParameterValue{
				{Parameter: "Cnom", Section: "parameters", Min: f(22), Max: f(1000000)},
				{Parameter: "TKE", Section: "parameters", Text: s("Н30")},
				{Parameter: "Unom", Section: "parameters", Exact: f(25)},
				{Parameter: "massMax", Section: "dimensions", Exact: f(1)},
				{Parameter: "length", Section: "dimensions", Exact: f(6)},
				{Parameter: "width", Section: "dimensions", Exact: f(4)},
				{Parameter: "height", Section: "dimensions", Exact: f(5)},
			},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
}
