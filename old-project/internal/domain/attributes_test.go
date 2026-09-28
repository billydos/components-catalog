package domain

import (
	"reflect"
	"testing"
)

func TestAttributeFieldNames_PinExpectedFields(t *testing.T) {
	expected := []string{
		"Structure", "Technology", "Package", "PackageMaterial", "ColorMarking", "Pinout",
		"EsdSensitive", "MilitaryGrade", "RadiationHardened", "Tu", "Notes",
		"YearFrom", "YearTo", "MassMax", "DatasheetUrl",
	}
	if got := AttributeFieldNames(); !reflect.DeepEqual(got, expected) {
		t.Errorf("AttributeFieldNames() = %v, ожидалось %v", got, expected)
	}
}

func TestAttributesHaveAnyValue_AllFieldsEmpty_IsFalse(t *testing.T) {
	if AttributesHaveAnyValue(&TransistorAttributes{}) {
		t.Error("AttributesHaveAnyValue = true для пустых атрибутов")
	}
}

func TestAttributesHaveAnyValue_AnySingleFieldSet_IsTrue(t *testing.T) {
	attributes := []TransistorAttributes{
		{Structure: Ptr("npn")},
		{Technology: Ptr("планарная")},
		{Package: Ptr("TO-92")},
		{PackageMaterial: Ptr("металл")},
		{ColorMarking: Ptr("красная точка")},
		{Pinout: Ptr("КБЭ")},
		{EsdSensitive: Ptr(true)},
		{MilitaryGrade: Ptr(false)},
		{RadiationHardened: Ptr(true)},
		{Tu: Ptr("ТУ 11.365.001-71")},
		{Notes: Ptr("примечание")},
		{YearFrom: Ptr(1970)},
		{YearTo: Ptr(1985)},
		{MassMax: Ptr(1.2)},
		{DatasheetUrl: Ptr("https://example.com")},
	}
	for i := range attributes {
		if !AttributesHaveAnyValue(&attributes[i]) {
			t.Errorf("атрибуты №%d: AttributesHaveAnyValue = false", i+1)
		}
	}
}
