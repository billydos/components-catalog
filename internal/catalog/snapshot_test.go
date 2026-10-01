package catalog_test

import (
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Синтетическое расширение: текстовый параметр и числовой атрибут —
// типы значений, не задействованные стартовыми сидами.
func extendedEngine(t *testing.T) *catalog.Engine {
	t.Helper()
	snap := seedSnapshot(t)
	out, probs := catalog.ApplyCatalog(snap, catalog.Input{
		Parameters: []catalog.ParameterDef{
			{Code: "dielectric", Group: "electrical", DisplayName: "диэлектрик (материал)",
				ValueType: catalog.ValueText, SortOrder: 480, Active: true},
		},
		Attributes: []catalog.AttributeDef{
			{Code: "toleranceGrade", DisplayName: "класс точности", Type: catalog.AttrNumber, SortOrder: 180, Active: true},
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
		}, "параметр «dielectric»: тип значения text не допускает ключ value"},
		{"число атрибута", catalog.Device{
			Kind:       domain.KindResistor,
			System:     domain.SystemOther,
			Attributes: []catalog.AttributeValue{{Attribute: "toleranceGrade", Num: f(2.5)}},
		}, ""},
		{"неположительное число атрибута", catalog.Device{
			Kind:       domain.KindResistor,
			System:     domain.SystemOther,
			Attributes: []catalog.AttributeValue{{Attribute: "toleranceGrade", Num: f(0)}},
		}, "атрибут «toleranceGrade»: значение должно быть положительным"},
		{"число атрибута не задано", catalog.Device{
			Kind:       domain.KindResistor,
			System:     domain.SystemOther,
			Attributes: []catalog.AttributeValue{{Attribute: "toleranceGrade"}},
		}, "атрибут «toleranceGrade»: ожидается число"},
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

// Структурные проверки метасхемы: обязательные поля строк каталога.
func TestMetaschemaRequiredFields(t *testing.T) {
	base := seedSnapshot(t)
	cases := []struct {
		name string
		in   catalog.Input
		want string
	}{
		{"единица без символа", catalog.Input{Units: []catalog.UnitDef{{Code: "эВ", Name: "электронвольт"}}},
			"каталог: единица «эВ»: не заданы название и символ"},
		{"класс без названия", catalog.Input{Kinds: []catalog.KindDef{{Code: domain.Kind("optocoupler")}}},
			"каталог: класс «optocoupler»: не задано название"},
		{"система без названия", catalog.Input{Systems: []catalog.SystemDef{{Code: domain.System("din")}}},
			"каталог: система «din»: не задано название"},
		{"условие без названия", catalog.Input{Conditions: []catalog.ConditionDef{{Code: "newc"}}},
			"каталог: условие «newc»: не задано название"},
		{"группа без названия", catalog.Input{Groups: []catalog.GroupDef{{Code: "g", SectionName: "section"}}},
			"каталог: группа «g»: не заданы имя секции и отображаемое название"},
		{"пустой код раздела", catalog.Input{Units: []catalog.UnitDef{{Name: "x", Symbol: "x"}}},
			"каталог: раздел units: не задан код"},
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
	if f, ok := snap.Family("МЛТ", domain.KindResistor); !ok || f.TailSemantic != catalog.TailSemanticPower {
		t.Fatalf("семейство МЛТ: %+v", f)
	}
	var nilSnap *catalog.Snapshot
	if _, ok := nilSnap.Parameter("Kpd"); ok {
		t.Error("nil-снимок не должен находить параметры")
	}
	if nilSnap.SystemAppliesTo(domain.SystemGost, domain.KindDiode) {
		t.Error("nil-снимок не должен знать применимость систем")
	}
}

// Problem.Err — преобразование в ошибку домена (код и текст сохраняются).
func TestProblemErr(t *testing.T) {
	p := catalog.Problem{Code: domain.CodeUnknownParameter, Message: "неизвестный параметр «X»"}
	err := p.Err()
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeUnknownParameter || err.Error() != "неизвестный параметр «X»" {
		t.Fatalf("проблема не преобразована в ошибку домена: %v", err)
	}
}
