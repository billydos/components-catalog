package catalog_test

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/seed"
)

func f(v float64) *float64 { return &v }

func s(v string) *string { return &v }

func b(v bool) *bool { return &v }

func seedSnapshot(t *testing.T) *catalog.Snapshot {
	t.Helper()
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		for _, p := range probs {
			t.Errorf("сид: %s", p.Message)
		}
		t.Fatal("стартовый каталог не проходит метасхему")
	}
	return snap
}

func seedEngine(t *testing.T) *catalog.Engine {
	t.Helper()
	return catalog.NewEngine(seedSnapshot(t))
}

// hasProblem проверяет вхождение дословного сообщения в накопленные
// проблемы (движок накапливает все нарушения за один прогон).
func hasProblem(t *testing.T, probs []catalog.Problem, want string) {
	t.Helper()
	for _, m := range messages(probs) {
		if m == want {
			return
		}
	}
	t.Fatalf("сообщение отсутствует:\n want: %s\n got:  %v", want, messages(probs))
}
func messages(probs []catalog.Problem) []string {
	out := make([]string, 0, len(probs))
	for _, p := range probs {
		out = append(out, p.Message)
	}
	return out
}

func codes(probs []catalog.Problem) []domain.Code {
	out := make([]domain.Code, 0, len(probs))
	for _, p := range probs {
		out = append(out, p.Code)
	}
	return out
}

// Запись из плана 06-examples.md §1 (КТ315Б) обязана проходить валидацию
// целиком: атрибуты, значения с условиями, предельные данные.
func TestValidateDeviceTransistorExample(t *testing.T) {
	e := seedEngine(t)
	d := catalog.Device{
		Kind: domain.KindTransistor, System: domain.SystemGost, Designation: "КТ315Б",
		Attributes: []catalog.AttributeValue{
			{Attribute: "structure", Text: s("npn")},
			{Attribute: "package", Text: s("КТ-13")},
			{Attribute: "spec", Texts: []string{"ЖК3.365.200ТУ"}},
			{Attribute: "yearFrom", Num: f(1967)},
			{Attribute: "yearTo", Num: f(1992)},
		},
		Values: []catalog.ParameterValue{
			{Parameter: "hfe", Section: "parameters", Min: f(50), Max: f(350),
				Conditions: []catalog.ConditionValue{{Condition: "Vce", Value: 10}, {Condition: "Ic", Value: 1}}},
			{Parameter: "Icbo", Section: "parameters", Max: f(0.5),
				Conditions: []catalog.ConditionValue{{Condition: "Vcb", Value: 10}, {Condition: "temp", Value: 25}}},
			{Parameter: "VceoMax", Section: "ratings", Exact: f(20)},
			{Parameter: "TempMin", Section: "ratings", Exact: f(-60)},
			{Parameter: "TempMax", Section: "ratings", Exact: f(100)},
		},
	}
	if probs := e.ValidateDevice(&d); len(probs) != 0 {
		t.Fatalf("КТ315Б: %v", messages(probs))
	}
}

func TestValidateHeader(t *testing.T) {
	e := seedEngine(t)
	cases := []struct {
		name string
		dev  catalog.Device
		want string
	}{
		{"класс не задан", catalog.Device{}, "device kind is not set"},
		{"неизвестный класс", catalog.Device{Kind: "thyristor"},
			"unknown device kind «thyristor»"},
		{"неизвестная system", catalog.Device{Kind: domain.KindDiode, System: "din"},
			"unknown designation system «din»"},
		{"system неприменима", catalog.Device{Kind: domain.KindCapacitor, System: domain.SystemOst},
			"designation system «ost» is not applicable to kind capacitor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probs := e.ValidateDevice(&tc.dev)
			if len(probs) == 0 || probs[0].Message != tc.want {
				t.Fatalf(" got: %v\n want: %s", messages(probs), tc.want)
			}
		})
	}
}

func TestUnknownAndNotApplicable(t *testing.T) {
	e := seedEngine(t)
	d := catalog.Device{
		Kind: domain.KindTransistor, System: domain.SystemGost, Designation: "КТ315Б",
		Attributes: []catalog.AttributeValue{
			{Attribute: "nosuch", Text: s("x")},
			{Attribute: "polarized", Bool: b(true)},
		},
		Values: []catalog.ParameterValue{
			{Parameter: "nosuch", Section: "parameters", Exact: f(1)},
			{Parameter: "Tolerance", Section: "parameters", Max: f(5)},
		},
	}
	probs := e.ValidateDevice(&d)
	want := []domain.Code{
		domain.CodeUnknownAttribute,
		domain.CodeAttributeNotApplicable,
		domain.CodeUnknownParameter,
		domain.CodeParameterNotApplicable,
	}
	if !slices.Equal(codes(probs), want) {
		t.Fatalf("коды проблем:\n got:  %v\n want: %v", codes(probs), want)
	}
	wantMsgs := []string{
		"unknown attribute «nosuch»",
		"attribute «polarized» is not applicable to kind transistor",
		"unknown parameter «nosuch»",
		"parameter «Tolerance» is not applicable to kind transistor",
	}
	if !slices.Equal(messages(probs), wantMsgs) {
		t.Fatalf("тексты проблем:\n got:  %v\n want: %v", messages(probs), wantMsgs)
	}
}

// Деактивированный каталогом параметр отвергается (удаление строк
// файлом не поддерживается — только деактивация, 02 §5.4).
func TestDeactivatedParameter(t *testing.T) {
	snap := seedSnapshot(t)
	row, ok := snap.Parameter("Eta")
	if !ok {
		t.Fatal("Eta отсутствует в каталоге")
	}
	inactive := *row
	inactive.Active = false
	snap2, probs := catalog.ApplyCatalog(snap, catalog.Input{Parameters: []catalog.ParameterDef{inactive}})
	if len(probs) != 0 {
		t.Fatalf("деактивация: %v", messages(probs))
	}
	d := catalog.Device{
		Kind: domain.KindTransistor, System: domain.SystemGost, Designation: "КТ315Б",
		Values: []catalog.ParameterValue{{Parameter: "Eta", Min: f(60), Conditions: []catalog.ConditionValue{{Condition: "freq", Value: 400}}}},
	}
	got := catalog.NewEngine(snap2).ValidateDevice(&d)
	if len(got) != 1 || got[0].Message != "parameter «Eta» is deactivated" {
		t.Fatalf(" got: %v", messages(got))
	}
}

// Формы значений по типам (каждый тип × корректная и неверная форма).
func TestValueShapes(t *testing.T) {
	e := seedEngine(t)
	cases := []struct {
		name string
		kind domain.Kind
		val  catalog.ParameterValue
		want string // пустая — значения корректны
	}{
		{"exact без value", domain.KindCapacitor,
			catalog.ParameterValue{Parameter: "Vnom", Section: "parameters", Min: f(25)},
			"parameter «Vnom»: value type exact requires key value"},
		{"exact с min", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "VceoMax", Section: "ratings", Exact: f(20), Min: f(1)},
			"parameter «VceoMax»: value type exact does not allow key min"},
		{"at_least без min", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "fT", Section: "parameters", Max: f(100)},
			"parameter «fT»: value type at_least requires key min"},
		{"at_least с text", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "fT", Section: "parameters", Min: f(100), Text: s("много")},
			"parameter «fT»: value type at_least does not allow key text"},
		{"at_most без max", domain.KindDiode,
			catalog.ParameterValue{Parameter: "Vf", Section: "parameters", Min: f(1)},
			"parameter «Vf»: value type at_most requires key max"},
		{"at_most с min", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "NF", Section: "parameters", Min: f(1), Max: f(2), Conditions: []catalog.ConditionValue{{Condition: "freq", Value: 10}}},
			"parameter «NF»: value type at_most does not allow key min"},
		{"range неполный", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "hfe", Section: "parameters", Min: f(50), Conditions: []catalog.ConditionValue{{Condition: "Vce", Value: 10}, {Condition: "Ic", Value: 1}}},
			"parameter «hfe»: value type range requires keys min and max"},
		{"range min>max", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "hfe", Section: "parameters", Min: f(350), Max: f(50), Conditions: []catalog.ConditionValue{{Condition: "Vce", Value: 10}, {Condition: "Ic", Value: 1}}},
			"parameter «hfe»: min exceeds max"},
		{"range равные границы", domain.KindCapacitor,
			catalog.ParameterValue{Parameter: "Cnom", Section: "parameters", Min: f(100000), Max: f(100000)}, ""},
		{"enum без text", domain.KindCapacitor,
			catalog.ParameterValue{Parameter: "TCC", Section: "parameters", Exact: f(1)},
			"parameter «TCC»: value type enum does not allow key value"},
		{"enum вне списка", domain.KindCapacitor,
			catalog.ParameterValue{Parameter: "TCC", Section: "parameters", Text: s("Н80")},
			"parameter «TCC»: value «Н80» is not among the allowed ones (П100, П120, П60, П33, МП0, М33, М47, М75, М150, М220, М330, М470, М750, М700, М1500, М1300, М2200, Н10, Н20, Н30, Н50, Н70, Н90, C0G, NP0, U2J, X5R, X6S, X7R, X7S, X8R, Y5V, Z5U)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := catalog.Device{Kind: tc.kind, System: domain.SystemOther, Values: []catalog.ParameterValue{tc.val}}
			probs := e.ValidateDevice(&d)
			if tc.want == "" {
				if len(probs) != 0 {
					t.Fatalf("ожидалась корректность, получено: %v", messages(probs))
				}
				return
			}
			hasProblem(t, probs, tc.want)
		})
	}
}

// Позитивность (allow_negative) и потолок значения.
func TestPositivityAndCeiling(t *testing.T) {
	e := seedEngine(t)
	cases := []struct {
		name string
		kind domain.Kind
		val  catalog.ParameterValue
		want string
	}{
		{"отрицательное значение", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "VceoMax", Section: "ratings", Exact: f(-20)},
			"parameter «VceoMax»: value of key value must be positive"},
		{"отрицательный min", domain.KindResistor,
			catalog.ParameterValue{Parameter: "Rnom", Section: "parameters", Min: f(-1), Max: f(10)},
			"parameter «Rnom»: value of key min must be positive"},
		{"нулевое значение", domain.KindDiode,
			catalog.ParameterValue{Parameter: "Pmax", Section: "ratings", Exact: f(0)},
			"parameter «Pmax»: value of key value must be positive"},
		{"отрицательная температура разрешена", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "TempMin", Section: "ratings", Exact: f(-60)}, ""},
		{"потолок превышен", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "Eta", Section: "parameters", Min: f(101), Conditions: []catalog.ConditionValue{{Condition: "freq", Value: 400}}},
			"parameter «Eta»: value of key min exceeds the ceiling 100"},
		{"потолок достигнут", domain.KindTransistor,
			catalog.ParameterValue{Parameter: "Eta", Section: "parameters", Min: f(100), Conditions: []catalog.ConditionValue{{Condition: "freq", Value: 400}}},
			""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := catalog.Device{Kind: tc.kind, System: domain.SystemGost, Values: []catalog.ParameterValue{tc.val}}
			probs := e.ValidateDevice(&d)
			if tc.want == "" {
				if len(probs) != 0 {
					t.Fatalf("ожидалась корректность, получено: %v", messages(probs))
				}
				return
			}
			hasProblem(t, probs, tc.want)
		})
	}
}

// Наборы условий: соответствие, фиксированные значения, запрещённые
// условия, безусловные параметры, позитивность условий, дубликаты.
func TestConditionSets(t *testing.T) {
	e := seedEngine(t)
	valP := func(parameter string, conds ...catalog.ConditionValue) *catalog.Device {
		return &catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{{Parameter: parameter, Section: "parameters", Max: f(10), Conditions: conds}},
		}
	}
	t.Run("оба набора hfe", func(t *testing.T) {
		for _, conds := range [][]catalog.ConditionValue{
			{{Condition: "Vce", Value: 10}, {Condition: "Ic", Value: 1}},
			{{Condition: "Vcb", Value: 1}, {Condition: "Ie", Value: 10}},
		} {
			d := valP("hfe", conds...)
			d.Values[0].Min, d.Values[0].Max = f(50), f(350)
			if probs := e.ValidateDevice(d); len(probs) != 0 {
				t.Fatalf("набор %v: %v", conds, messages(probs))
			}
		}
	})
	t.Run("смешение наборов", func(t *testing.T) {
		d := valP("hfe",
			catalog.ConditionValue{Condition: "Vce", Value: 10}, catalog.ConditionValue{Condition: "Ie", Value: 1})
		d.Values[0].Min, d.Values[0].Max = f(50), f(350)
		hasProblem(t, e.ValidateDevice(d),
			"parameter «hfe»: the combination of conditions does not match any condition set of the parameter")
	})
	t.Run("постороннее условие", func(t *testing.T) {
		probs := e.ValidateDevice(valP("Icbo",
			catalog.ConditionValue{Condition: "Vcb", Value: 10}, catalog.ConditionValue{Condition: "freq", Value: 1}))
		if len(probs) != 1 || probs[0].Message !=
			"parameter «Icbo»: the combination of conditions does not match any condition set of the parameter" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("неизвестное условие", func(t *testing.T) {
		probs := e.ValidateDevice(valP("Icbo",
			catalog.ConditionValue{Condition: "Vcb", Value: 10}, catalog.ConditionValue{Condition: "no_such", Value: 1}))
		if !slices.Contains(codes(probs), domain.CodeUnknownCondition) {
			t.Fatalf(" got: %v", messages(probs))
		}
		hasProblem(t, probs, "parameter «Icbo»: unknown condition «no_such»")
	})
	t.Run("fixed_value опущено", func(t *testing.T) {
		d := valP("Esr")
		d.Kind = domain.KindCapacitor
		if probs := e.ValidateDevice(d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("fixed_value задано равным", func(t *testing.T) {
		d := valP("Esr", catalog.ConditionValue{Condition: "freq", Value: 0.1})
		d.Kind = domain.KindCapacitor
		if probs := e.ValidateDevice(d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("fixed_value нарушено", func(t *testing.T) {
		d := valP("Esr", catalog.ConditionValue{Condition: "freq", Value: 1})
		d.Kind = domain.KindCapacitor
		probs := e.ValidateDevice(d)
		if len(probs) != 1 || probs[0].Message != "parameter «Esr»: condition «freq» is fixed to value 0.1" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("обязательное pulse_duration", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{{Parameter: "IcPulseMax", Section: "ratings", Exact: f(100)}},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 1 {
			t.Fatalf(" got: %v", messages(probs))
		}
		d.Values[0].Conditions = []catalog.ConditionValue{{Condition: "pulse_duration", Value: 100}}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("набор из одних optional", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindCapacitor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{{Parameter: "DF", Section: "parameters", Max: f(0.15)}},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
		d.Values[0].Conditions = []catalog.ConditionValue{{Condition: "temp", Value: 20}}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("безусловный параметр", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindDiode, System: domain.SystemGost,
			Values: []catalog.ParameterValue{
				{Parameter: "Lambda", Section: "parameters", Min: f(650), Max: f(675),
					Conditions: []catalog.ConditionValue{{Condition: "temp", Value: 25}}},
			},
		}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "parameter «Lambda»: unconditional parameter — conditions are not allowed" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("позитивность условия", func(t *testing.T) {
		probs := e.ValidateDevice(valP("Icbo",
			catalog.ConditionValue{Condition: "Vcb", Value: -10}))
		if len(probs) != 1 || probs[0].Message != "parameter «Icbo»: condition «Vcb» — value must be positive" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("отрицательная температура условия", func(t *testing.T) {
		if probs := e.ValidateDevice(valP("Icbo",
			catalog.ConditionValue{Condition: "Vcb", Value: 10}, catalog.ConditionValue{Condition: "temp", Value: -60})); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("дубликат значения", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{
				{Parameter: "Icbo", Section: "parameters", Max: f(1), Conditions: []catalog.ConditionValue{{Condition: "Vcb", Value: 10}}},
				{Parameter: "Icbo", Section: "parameters", Max: f(2), Conditions: []catalog.ConditionValue{{Condition: "Vcb", Value: 10}}},
			},
		}
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "parameter «Icbo»: duplicate value with the same conditions" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("многозначный enum", func(t *testing.T) {
		txt := func(s string) *string { return &s }
		d := catalog.Device{
			Kind: domain.KindResistor, System: domain.SystemGost, Designation: "С2-33Н",
			Values: []catalog.ParameterValue{
				{Parameter: "nominalRow", Section: "parameters", Text: txt("E24")},
				{Parameter: "nominalRow", Section: "parameters", Text: txt("E96")},
			},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
		d.Values[1].Text = txt("E24")
		probs := e.ValidateDevice(&d)
		if len(probs) != 1 || probs[0].Message != "parameter «nominalRow»: duplicate value with the same conditions" {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
	t.Run("Icbo при двух температурах", func(t *testing.T) {
		d := catalog.Device{
			Kind: domain.KindTransistor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{
				{Parameter: "Icbo", Section: "parameters", Max: f(0.5), Conditions: []catalog.ConditionValue{{Condition: "Vcb", Value: 10}, {Condition: "temp", Value: 25}}},
				{Parameter: "Icbo", Section: "parameters", Max: f(15), Conditions: []catalog.ConditionValue{{Condition: "Vcb", Value: 10}, {Condition: "temp", Value: 85}}},
			},
		}
		if probs := e.ValidateDevice(&d); len(probs) != 0 {
			t.Fatalf(" got: %v", messages(probs))
		}
	})
}

// Параметр задан в чужой секции группы; пустая секция — такое же
// несоответствие (значения исполнений строятся с секцией, как и значения
// типа в целом).
func TestSectionMismatch(t *testing.T) {
	e := seedEngine(t)
	d := catalog.Device{
		Kind: domain.KindTransistor, System: domain.SystemGost,
		Values: []catalog.ParameterValue{
			{Parameter: "hfe", Section: "ratings", Min: f(50), Max: f(350),
				Conditions: []catalog.ConditionValue{{Condition: "Vce", Value: 10}, {Condition: "Ic", Value: 1}}},
			{Parameter: "VceoMax", Section: "ratings", Exact: f(20)},
		},
	}
	probs := e.ValidateDevice(&d)
	if len(probs) != 1 || probs[0].Message !=
		"parameter «hfe» is set in section «ratings» but belongs to section «parameters»" {
		t.Fatalf(" got: %v", messages(probs))
	}

	// Пустая секция — такое же несоответствие.
	d.Values[1].Section = ""
	probs = e.ValidateDevice(&d)
	if len(probs) != 2 || probs[1].Message !=
		"parameter «VceoMax» is set in section «» but belongs to section «ratings»" {
		t.Fatalf(" got: %v", messages(probs))
	}
}

// Значение исполнения в чужой секции отвергается так же, как значение
// типа в целом: секция принадлежит группе параметра.
func TestVariantSectionMismatch(t *testing.T) {
	e := seedEngine(t)
	d := catalog.Device{
		Kind: domain.KindCapacitor, System: domain.SystemGost,
		Variants: []catalog.Variant{{
			Label: "16V",
			Values: []catalog.ParameterValue{
				{Parameter: "Cnom", Section: "limits", Exact: f(100)},
			},
		}},
	}
	probs := e.ValidateDevice(&d)
	found := false
	for _, p := range probs {
		if p.MsgID == domain.MsgEngineParamWrongSection {
			found = true
		}
	}
	if !found {
		t.Fatalf("несоответствие секции значения исполнения не найдено: %v", messages(probs))
	}
}

// Атрибуты исполнения валидируются движком как атрибуты записи:
// существование, применимость к классу, тип значения (план 03 §7/§10).
func TestVariantAttributeValidation(t *testing.T) {
	e := seedEngine(t)
	d := catalog.Device{
		Kind: domain.KindResistor, System: domain.SystemGost,
		Variants: []catalog.Variant{{
			Label: "24",
			Values: []catalog.ParameterValue{
				{Parameter: "Pnom", Section: "ratings", Exact: f(0.125)},
			},
			Attributes: []catalog.AttributeValue{
				{Attribute: "functionalChar", Text: s("А")},
			},
		}},
	}
	if probs := e.ValidateDevice(&d); len(probs) != 0 {
		t.Fatalf("корректный атрибут исполнения: %v", messages(probs))
	}
	d.Variants[0].Attributes = []catalog.AttributeValue{
		{Attribute: "functionalChar", Text: s("Г")},
	}
	probs := e.ValidateDevice(&d)
	if len(probs) != 1 || probs[0].MsgID != domain.MsgEngineAttrEnumInvalid {
		t.Fatalf("enum вне списка: %v", messages(probs))
	}
	d.Variants[0].Attributes = []catalog.AttributeValue{
		{Attribute: "noSuch", Text: s("x")},
	}
	probs = e.ValidateDevice(&d)
	if len(probs) != 1 || probs[0].MsgID != domain.MsgEngineAttrUnknown {
		t.Fatalf("неизвестный атрибут: %v", messages(probs))
	}
	// Атрибут чужого класса — полярность конденсаторов.
	d.Variants[0].Attributes = []catalog.AttributeValue{
		{Attribute: "polarized", Bool: b(true)},
	}
	probs = e.ValidateDevice(&d)
	if len(probs) != 1 || probs[0].MsgID != domain.MsgEngineAttrNotApplicable {
		t.Fatalf("неприменимый атрибут: %v", messages(probs))
	}
}

// Значения атрибутов по типам (§9): текст непустой, bool, enum, int.
func TestAttributeValueTypes(t *testing.T) {
	e := seedEngine(t)
	cases := []struct {
		name string
		av   catalog.AttributeValue
		want string
	}{
		{"текст", catalog.AttributeValue{Attribute: "description", Text: s("выпрямительный")}, ""},
		{"пустой текст", catalog.AttributeValue{Attribute: "description", Text: s("  ")},
			"attribute «description»: text cannot be empty"},
		{"не текст", catalog.AttributeValue{Attribute: "description", Num: f(1)},
			"attribute «description»: a text value is expected"},
		{"bool", catalog.AttributeValue{Attribute: "esdSensitive", Bool: b(true)}, ""},
		{"не bool", catalog.AttributeValue{Attribute: "esdSensitive", Text: s("да")},
			"attribute «esdSensitive»: a boolean value is expected"},
		{"int", catalog.AttributeValue{Attribute: "yearFrom", Num: f(1967)}, ""},
		{"не int", catalog.AttributeValue{Attribute: "yearFrom", Num: f(1967.5)},
			"attribute «yearFrom»: value must be an integer"},
		{"int без значения", catalog.AttributeValue{Attribute: "yearFrom"},
			"attribute «yearFrom»: an integer is expected"},
		{"enum вне списка", catalog.AttributeValue{Attribute: "functionalChar", Text: s("Г")},
			"attribute «functionalChar»: value «Г» is not among the allowed ones (А, Б, В)"},
		{"список", catalog.AttributeValue{Attribute: "notes", Texts: []string{"заметка одна", "заметка две"}}, ""},
		{"список одним элементом", catalog.AttributeValue{Attribute: "notes", Texts: []string{"единственная"}}, ""},
		{"не список", catalog.AttributeValue{Attribute: "notes", Text: s("текстом")},
			"attribute «notes»: a list of text items is expected"},
		{"пустой список", catalog.AttributeValue{Attribute: "notes", Texts: []string{}},
			"attribute «notes»: the list of items cannot be empty"},
		{"пустой элемент списка", catalog.AttributeValue{Attribute: "notes", Texts: []string{"заметка", " "}},
			"attribute «notes»: list items cannot be empty"},
		{"повтор элемента списка", catalog.AttributeValue{Attribute: "notes", Texts: []string{"заметка", "заметка"}},
			"attribute «notes»: duplicate list item «заметка»"},
		{"повтор элемента списка по trim-ключу", catalog.AttributeValue{Attribute: "notes", Texts: []string{"заметка", " заметка "}},
			"attribute «notes»: duplicate list item « заметка »"},
		{"массив не для списка", catalog.AttributeValue{Attribute: "description", Texts: []string{"текстом"}},
			"attribute «description»: a text value is expected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := catalog.Device{
				Kind:       domain.KindResistor,
				System:     domain.SystemOther,
				Attributes: []catalog.AttributeValue{tc.av},
			}
			probs := e.ValidateDevice(&d)
			if tc.want == "" {
				if len(probs) != 0 {
					t.Fatalf("ожидалась корректность, получено: %v", messages(probs))
				}
				return
			}
			hasProblem(t, probs, tc.want)
		})
	}
}
