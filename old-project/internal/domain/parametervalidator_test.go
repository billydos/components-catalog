package domain

import (
	"fmt"
	"testing"
)

var sampleConditionValues = map[ConditionKey]float64{
	CondUke:  10,
	CondUkb:  5,
	CondUeb:  5,
	CondIk:   1,
	CondIe:   10,
	CondIb:   50,
	CondFreq: 1.8,
	CondRg:   500,
	CondRbe:  100,
}

// forEachKindVariant обходит каталог × варианты (аналог KindVariantPairs).
func forEachKindVariant(t *testing.T, test func(t *testing.T, info ParameterInfo, variantIndex int)) {
	t.Helper()
	for _, info := range ParameterCatalog {
		for variantIndex := range info.Conditions.Variants {
			t.Run(fmt.Sprintf("%s/%d", info.Kind, variantIndex), func(t *testing.T) {
				test(t, info, variantIndex)
			})
		}
	}
}

// parameterFromVariant — параметр с границей по правилу вида и условиями
// из обязательных ключей варианта (без одного ключа).
func parameterFromVariant(kind ParameterKind, variant ConditionVariant, except ConditionKey) ElectricalParameter {
	parameter := withBounds(kind, ElectricalParameter{Kind: kind})
	for _, key := range variant.RequiredKeys() {
		if key != except {
			parameter = ConditionWithValue(parameter, key, sampleConditionValues[key])
		}
	}
	return parameter
}

func withBounds(kind ParameterKind, parameter ElectricalParameter) ElectricalParameter {
	switch ParameterInfoOf(kind).Direction {
	case AtLeast:
		parameter.ValueMin = Ptr(50.0)
	case AtMost:
		parameter.ValueMax = Ptr(1.0)
	case AtLeastOrRange:
		parameter.ValueMin = Ptr(50.0)
	}
	return parameter
}

func satisfiedByOtherVariant(spec *ConditionSpec, parameter ElectricalParameter, exceptIndex int) bool {
	for index, variant := range spec.Variants {
		if index != exceptIndex && variant.IsSatisfiedBy(parameter) {
			return true
		}
	}
	return false
}

func hasErrorsWithFragment(t *testing.T, errors []string, fragment string) {
	t.Helper()
	for _, e := range errors {
		if containsSubstring(e, fragment) {
			return
		}
	}
	t.Errorf("ни одно из сообщений %v не содержит %q", errors, fragment)
}

func TestRequiredKeysOnly_NoErrors(t *testing.T) {
	forEachKindVariant(t, func(t *testing.T, info ParameterInfo, variantIndex int) {
		variant := info.Conditions.Variants[variantIndex]
		if errors := ValidateElectricalParameter(parameterFromVariant(info.Kind, variant, "")); len(errors) > 0 {
			t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
		}
	})
}

func TestMissingRequiredKey_ProducesConditionError(t *testing.T) {
	forEachKindVariant(t, func(t *testing.T, info ParameterInfo, variantIndex int) {
		spec := info.Conditions
		variant := spec.Variants[variantIndex]
		for _, required := range variant.RequiredKeys() {
			parameter := parameterFromVariant(info.Kind, variant, required)
			errors := ValidateElectricalParameter(parameter)
			if !satisfiedByOtherVariant(spec, parameter, variantIndex) {
				hasErrorsWithFragment(t, errors, "условия —")
			}
		}
	})
}

func TestForbiddenKey_ProducesConditionError(t *testing.T) {
	forEachKindVariant(t, func(t *testing.T, info ParameterInfo, variantIndex int) {
		spec := info.Conditions
		variant := spec.Variants[variantIndex]
		for _, key := range AllConditionKeys {
			if variant.IsRequired(key) || variant.IsOptional(key) {
				continue
			}
			parameter := ConditionWithValue(parameterFromVariant(info.Kind, variant, ""), key, sampleConditionValues[key])
			errors := ValidateElectricalParameter(parameter)
			if !satisfiedByOtherVariant(spec, parameter, variantIndex) {
				hasErrorsWithFragment(t, errors, "условия —")
			}
		}
	})
}

func TestOptionalKey_AddedToRequiredSet_NoErrors(t *testing.T) {
	forEachKindVariant(t, func(t *testing.T, info ParameterInfo, variantIndex int) {
		variant := info.Conditions.Variants[variantIndex]
		for _, optional := range variant.OptionalKeys() {
			parameter := ConditionWithValue(parameterFromVariant(info.Kind, variant, ""), optional, sampleConditionValues[optional])
			if errors := ValidateElectricalParameter(parameter); len(errors) > 0 {
				t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
			}
		}
	})
}

func TestTemp_IsAllowedWithAnyConditions(t *testing.T) {
	forEachKindVariant(t, func(t *testing.T, info ParameterInfo, variantIndex int) {
		variant := info.Conditions.Variants[variantIndex]
		parameter := parameterFromVariant(info.Kind, variant, "")
		parameter.Temp = Ptr(25.0)
		if errors := ValidateElectricalParameter(parameter); len(errors) > 0 {
			t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
		}
	})
}

func TestConditionViolation_ProducesSingleMessage(t *testing.T) {
	parameter := ElectricalParameter{Kind: KindH21E, ValueMin: Ptr(50.0), Ib: Ptr(10.0)}
	errors := ValidateElectricalParameter(parameter)

	count := 0
	for _, e := range errors {
		if containsSubstring(e, "условия —") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("сообщений «условия —»: %d, ожидалось 1 (%v)", count, errors)
	}
	hasErrorsWithFragment(t, errors, "задано: Iб (Ib)")
}

func TestDescribe_JoinsVariantsWithEither(t *testing.T) {
	cases := []struct {
		spec     *ConditionSpec
		expected string
	}{
		{SpecPairUkeIkOrUkbIe, "Uкэ (Uke) + Iк (Ik) либо Uкб (Ukb) + Iэ (Ie)"},
		{SpecExactlyOneCurrent, "Iк (Ik) либо Iэ (Ie)"},
		{SpecOnlyUke, "Uкэ (Uke)"},
	}
	for _, tc := range cases {
		if got := tc.spec.Describe(); got != tc.expected {
			t.Errorf("Describe() = %q, ожидалось %q", got, tc.expected)
		}
	}
}

func TestAtLeastParameter_RequiresMinAndForbidsMax(t *testing.T) {
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{Kind: KindCutoffFrequency}),
		"обязательно значение «не менее» (min)")
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{
		Kind: KindCutoffFrequency, ValueMin: Ptr(5.0), ValueMax: Ptr(10.0), Uke: Ptr(10.0), Ik: Ptr(1.0),
	}), "верхняя граница (max) не допускается")
}

func TestAtMostParameter_RequiresMaxAndForbidsMin(t *testing.T) {
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{Kind: KindCollectorCutoffCurrent, Ukb: Ptr(10.0)}),
		"обязательно значение «не более» (max)")
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{
		Kind: KindCollectorCutoffCurrent, ValueMin: Ptr(0.5), ValueMax: Ptr(1.0), Ukb: Ptr(10.0),
	}), "нижняя граница (min) не допускается")
}

func TestAtLeastOrRangeParameter_RequiresMin_MaxOptional(t *testing.T) {
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{Kind: KindH21E, Uke: Ptr(10.0), Ik: Ptr(1.0)}),
		"обязательна нижняя граница (min)")
	if errors := ValidateElectricalParameter(ElectricalParameter{
		Kind: KindH21E, ValueMin: Ptr(50.0), ValueMax: Ptr(200.0), Uke: Ptr(10.0), Ik: Ptr(1.0),
	}); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestRangeWithMinAboveMax_ProducesError(t *testing.T) {
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{
		Kind: KindH21E, ValueMin: Ptr(200.0), ValueMax: Ptr(50.0), Uke: Ptr(10.0), Ik: Ptr(1.0),
	}), "нижняя граница больше верхней")
}

func TestNonPositiveValues_ProduceErrors(t *testing.T) {
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{
		Kind: KindH21E, ValueMin: Ptr(-50.0), Uke: Ptr(10.0), Ik: Ptr(1.0),
	}), "значение должно быть положительным")
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{
		Kind: KindH21E, ValueMin: Ptr(50.0), ValueMax: Ptr(0.0), Uke: Ptr(10.0), Ik: Ptr(1.0),
	}), "значение должно быть положительным")
}

func TestNonPositiveConditions_ProduceErrorsForEachKey(t *testing.T) {
	for _, key := range AllConditionKeys {
		for _, info := range ParameterCatalog {
			var variantWithValue *ConditionVariant
			for i := range info.Conditions.Variants {
				variant := info.Conditions.Variants[i]
				if variant.IsRequired(key) || variant.IsOptional(key) {
					variantWithValue = &info.Conditions.Variants[i]
					break
				}
			}
			if variantWithValue == nil {
				continue
			}
			parameter := ConditionWithValue(parameterFromVariant(info.Kind, *variantWithValue, ""), key, 0)
			hasErrorsWithFragment(t, ValidateElectricalParameter(parameter), ConditionNonPositiveMessage(key))
		}
	}
}

func TestEfficiencyAboveCeiling_ProducesError(t *testing.T) {
	hasErrorsWithFragment(t, ValidateElectricalParameter(ElectricalParameter{
		Kind: KindCollectorEfficiency, ValueMin: Ptr(101.0), Freq: Ptr(100.0),
	}), "не может превышать 100 %")
}

func TestEfficiencyWithinCeiling_NoErrors(t *testing.T) {
	if errors := ValidateElectricalParameter(ElectricalParameter{
		Kind: KindCollectorEfficiency, ValueMin: Ptr(100.0), Freq: Ptr(100.0),
	}); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestEveryParameter_HasConditionSpecWithVariant(t *testing.T) {
	for _, info := range ParameterCatalog {
		if info.Conditions == nil || len(info.Conditions.Variants) == 0 {
			t.Errorf("%s: спецификация условий пуста", info.Code)
		}
	}
}

func TestParameterCodes_AreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, info := range ParameterCatalog {
		if seen[info.Code] {
			t.Errorf("код %s дублируется", info.Code)
		}
		seen[info.Code] = true
	}
}

func TestTryParameterKindByCode_ResolvesCodes(t *testing.T) {
	if kind, ok := TryParameterKindByCode("h21e"); !ok || kind != KindH21E {
		t.Errorf("TryParameterKindByCode(\"h21e\") = (%v, %v)", kind, ok)
	}
	if _, ok := TryParameterKindByCode("h21"); ok {
		t.Error("TryParameterKindByCode(\"h21\") = true")
	}
}

func TestFrequencyKinds_RequireFrequencyInEveryVariant(t *testing.T) {
	frequencyKinds := []ParameterKind{KindNoiseFigure, KindOutputPower, KindPowerGain, KindCollectorEfficiency}
	for _, kind := range frequencyKinds {
		for _, variant := range ParameterInfoOf(kind).Conditions.Variants {
			if !variant.IsRequired(CondFreq) {
				t.Errorf("%s: вариант без обязательной частоты", kind)
			}
		}
	}
}

func TestOnlyUkeAndRbe_AreAllowedForIkep(t *testing.T) {
	spec := ParameterInfoOf(KindCollectorEmitterCutoffCurrentRbe).Conditions

	if !spec.Allows(CondUke) {
		t.Error("Uke не допускается для Ikep")
	}
	if !spec.Allows(CondRbe) {
		t.Error("Rbe не допускается для Ikep")
	}
	for _, variant := range spec.Variants {
		if variant.IsOptional(CondUke) {
			t.Error("Uke не должен быть необязательным у Ikep")
		}
	}
}
