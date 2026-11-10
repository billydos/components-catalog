package catalog_test

import (
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Синтетическое расширение: текстовый параметр и numberвой атрибут —
// типы значений, не задействованные стартовыми сидами.
func extendedEngine(t *testing.T) *catalog.Engine {
	t.Helper()
	snap := seedSnapshot(t)
	out, probs := catalog.ApplyCatalog(snap, catalog.Input{
		Parameters: []catalog.ParameterDef{
			{Code: "dielectric", Group: "electrical",
				ValueType: catalog.ValueText, SortOrder: 480, Active: true},
		},
		Attributes: []catalog.AttributeDef{
			{Code: "toleranceGrade", Type: catalog.AttrNumber, SortOrder: 180, Active: true},
		},
	})
	if len(probs) != 0 {
		t.Fatalf("расширение: %v", messages(probs))
	}
	return catalog.NewEngine(out)
}

func TestTextParameterAndNumberAttribute(t *testing.T) {
	e := extendedEngine(t)
	cases := []struct {
		name string
		dev  catalog.Device
		want string
	}{
		{"текст корректен", catalog.Device{
			Kind: domain.KindCapacitor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{{Parameter: "dielectric", Section: "parameters", Text: s("полипропилен")}},
		}, ""},
		{"текст отсутствует", catalog.Device{
			Kind: domain.KindCapacitor, System: domain.SystemGost,
			Values: []catalog.ParameterValue{{Parameter: "dielectric", Section: "parameters", Exact: f(1)}},
		}, "parameter «dielectric»: value type text does not allow key value"},
		{"number атрибута", catalog.Device{
			Kind:       domain.KindResistor,
			System:     domain.SystemOther,
			Attributes: []catalog.AttributeValue{{Attribute: "toleranceGrade", Num: f(2.5)}},
		}, ""},
		{"неположительное number атрибута", catalog.Device{
			Kind:       domain.KindResistor,
			System:     domain.SystemOther,
			Attributes: []catalog.AttributeValue{{Attribute: "toleranceGrade", Num: f(0)}},
		}, "attribute «toleranceGrade»: value must be positive"},
		{"number атрибута не задано", catalog.Device{
			Kind:       domain.KindResistor,
			System:     domain.SystemOther,
			Attributes: []catalog.AttributeValue{{Attribute: "toleranceGrade"}},
		}, "attribute «toleranceGrade»: a number is expected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probs := e.ValidateDevice(&tc.dev)
			if tc.want == "" {
				if len(probs) != 0 {
					t.Fatalf(" got: %v", messages(probs))
				}
				return
			}
			hasProblem(t, probs, tc.want)
		})
	}
}

// Структурные проверки метасхемы: обязательные поля строк каталога
// (названия/описания/символы — бандлы i18n, в метасхеме не проверяются — D9).
func TestMetaschemaRequiredFields(t *testing.T) {
	base := seedSnapshot(t)
	cases := []struct {
		name string
		in   catalog.Input
		want string
	}{
		{"пустой код раздела", catalog.Input{Units: []catalog.UnitDef{{}}},
			"catalog: section units: code is not set"},
		{"группа без секции", catalog.Input{Groups: []catalog.GroupDef{{Code: "g"}}},
			"catalog: group «g»: section name is not set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, probs := catalog.ApplyCatalog(base, tc.in)
			hasProblem(t, probs, tc.want)
		})
	}
}

// API снимка: доступ к правилам класса, группе по секции, семейству,
// nil-стойкость.
func TestSnapshotAPI(t *testing.T) {
	snap := seedSnapshot(t)
	if got := snap.RulesForKind(domain.KindCapacitor); len(got) != 1 || got[0] != "cap_variant_matrix" {
		t.Fatalf("правила конденсаторов: %v", got)
	}
	if got := snap.RulesForKind(domain.KindTransistor); len(got) != 0 {
		t.Fatalf("правила транзисторов: %v", got)
	}
	if g, ok := snap.GroupBySection("dimensions"); !ok || g.Code != "dimensional" {
		t.Fatalf("группа секции dimensions: %+v", g)
	}
	if _, ok := snap.GroupBySection("no_such_section"); ok {
		t.Error("несуществующая секция ошибочно найдена")
	}
	var nilSnap *catalog.Snapshot
	if _, ok := nilSnap.Parameter("Eta"); ok {
		t.Error("nil-снимок не должен находить параметры")
	}
	if nilSnap.SystemAppliesTo(domain.SystemGost, domain.KindDiode) {
		t.Error("nil-снимок не должен знать применимость систем")
	}
}

// Problem.Err — преобразование в ошибку домена (код и текст сохраняются).
func TestProblemErr(t *testing.T) {
	p := catalog.Problemf(domain.CodeUnknownParameter, domain.MsgEngineParamUnknown, "X")
	err := p.Err()
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeUnknownParameter || err.Error() != "unknown parameter «X»" {
		t.Fatalf("проблема не преобразована в ошибку домена: %v", err)
	}
}

// RowsEqual — равенство определений снимков без учёта порядка: чтение БД
// упорядочивает разделы канонически, применение каталога дописывает новые
// строки в конец — порядок не часть данных (идемпотентность повторного
// импорта каталога строится на этом равенстве).
func TestRowsEqual(t *testing.T) {
	fv := func(f float64) *float64 { return &f }
	base := func() *catalog.Snapshot {
		return &catalog.Snapshot{
			Kinds: []catalog.KindDef{{Code: domain.KindDiode}, {Code: domain.KindTransistor}},
			Units: []catalog.UnitDef{{Code: "V"}, {Code: "mV"}},
			Parameters: []catalog.ParameterDef{{
				Code: "hfe", Group: "electrical", ValueType: catalog.ValueEnum,
				Kinds:      []domain.Kind{domain.KindDiode, domain.KindTransistor},
				EnumValues: []string{"a", "b"},
				ConditionSets: []catalog.ConditionSet{{
					No: 1,
					Items: []catalog.ConditionSetItem{
						{Condition: "Vce", Mode: catalog.ModeRequired, FixedValue: fv(5)},
						{Condition: "Ic", Mode: catalog.ModeOptional},
					},
				}},
			}},
		}
	}
	reordered := func() *catalog.Snapshot {
		s := base()
		s.Kinds = []catalog.KindDef{{Code: domain.KindTransistor}, {Code: domain.KindDiode}}
		s.Parameters = []catalog.ParameterDef{{
			Code: "hfe", Group: "electrical", ValueType: catalog.ValueEnum,
			Kinds:      []domain.Kind{domain.KindTransistor, domain.KindDiode},
			EnumValues: []string{"b", "a"},
			ConditionSets: []catalog.ConditionSet{{
				No: 1,
				Items: []catalog.ConditionSetItem{
					{Condition: "Ic", Mode: catalog.ModeOptional},
					{Condition: "Vce", Mode: catalog.ModeRequired, FixedValue: fv(5)},
				},
			}},
		}}
		return s
	}

	if !catalog.RowsEqual(base(), reordered()) {
		t.Fatal("переупорядоченные разделы ошибочно неравны")
	}
	if catalog.RowsEqual(base(), nil) || catalog.RowsEqual(nil, base()) {
		t.Fatal("nil-снимок не равен непустому")
	}
	if !catalog.RowsEqual(nil, nil) {
		t.Fatal("nil-снимки неравны")
	}

	changed := reordered()
	changed.Units = append(changed.Units, catalog.UnitDef{Code: "kV"})
	if catalog.RowsEqual(base(), changed) {
		t.Fatal("новая строка раздела не замечена")
	}

	changed = reordered()
	changed.Parameters[0].Group = "env"
	if catalog.RowsEqual(base(), changed) {
		t.Fatal("изменённое поле строки не замечено")
	}

	// Необязательные числа: «не задано» (nil) ≠ явный ноль — и у
	// fixed_value элемента набора, и у потолка параметра.
	nilFixed := base()
	nilFixed.Parameters[0].ConditionSets[0].Items[1].FixedValue = nil
	zeroFixed := base()
	zeroFixed.Parameters[0].ConditionSets[0].Items[1].FixedValue = fv(0)
	if catalog.RowsEqual(nilFixed, zeroFixed) {
		t.Fatal("fixed_value «не задано» равно нулю")
	}
	nilCeiling := base()
	nilCeiling.Parameters[0].Ceiling = nil
	zeroCeiling := base()
	zeroCeiling.Parameters[0].Ceiling = fv(0)
	if catalog.RowsEqual(nilCeiling, zeroCeiling) {
		t.Fatal("потолок «не задано» равен нулю")
	}
}
