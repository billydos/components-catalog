package domain

import (
	"reflect"
	"testing"
)

func TestEmptyRatings_NoErrors(t *testing.T) {
	if errors := ValidateMaximumRatings(&MaximumRatings{}); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestNegativeTemperatures_Allowed(t *testing.T) {
	ratings := MaximumRatings{TempMin: Ptr(-60.0), TempMax: Ptr(25.0)}
	if errors := ValidateMaximumRatings(&ratings); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestNonPositiveLimit_ProducesError(t *testing.T) {
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{UkeMax: Ptr(0.0)}), "должно быть положительным")
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{IkMax: Ptr(-100.0)}), "должно быть положительным")
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{Rth: Ptr(0.0)}), "должно быть положительным")
}

func TestTempMinNotBelowTempMax_ProducesError(t *testing.T) {
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{TempMin: Ptr(25.0), TempMax: Ptr(25.0)}), "должна быть меньше")
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{TempMin: Ptr(70.0), TempMax: Ptr(25.0)}), "должна быть меньше")
}

func TestPulseValues_RequirePulseDuration(t *testing.T) {
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{IkPulseMax: Ptr(500.0)}),
		"длительность импульса (pulseDuration, мкс) обязательна")
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{PkPulseMax: Ptr(300.0)}),
		"длительность импульса (pulseDuration, мкс) обязательна")
	if errors := ValidateMaximumRatings(&MaximumRatings{
		IkPulseMax: Ptr(500.0), PkPulseMax: Ptr(300.0), PulseDuration: Ptr(100.0),
	}); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestPulseDurationWithoutPulseValues_ProducesError(t *testing.T) {
	hasErrorsWithFragment(t, ValidateMaximumRatings(&MaximumRatings{PulseDuration: Ptr(100.0)}),
		"импульсные ток/мощность не заданы")
}

func TestRatingFieldNames_PinExpectedFields(t *testing.T) {
	expected := []string{
		"UkeMax", "UkbMax", "UbeMax", "UkeoMax", "IkMax", "IbMax", "PkMax",
		"IkPulseMax", "PkPulseMax", "PulseDuration", "TempMin", "TempMax",
		"TempJunctionMax", "Rth",
	}
	if got := RatingFieldNames(); !reflect.DeepEqual(got, expected) {
		t.Errorf("RatingFieldNames() = %v, ожидалось %v", got, expected)
	}
}
