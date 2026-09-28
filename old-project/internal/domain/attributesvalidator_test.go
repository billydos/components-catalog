package domain

import "testing"

func TestEmptyAttributes_NoErrors(t *testing.T) {
	if errors := ValidateTransistorAttributes(&TransistorAttributes{}); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestWhitespaceTextAttributes_ProduceErrors(t *testing.T) {
	attributes := []TransistorAttributes{
		{Structure: Ptr(" ")},
		{Technology: Ptr(" ")},
		{Package: Ptr(" ")},
		{PackageMaterial: Ptr(" ")},
		{ColorMarking: Ptr(" ")},
		{Pinout: Ptr(" ")},
		{Tu: Ptr(" ")},
		{Notes: Ptr(" ")},
		{DatasheetUrl: Ptr(" ")},
	}
	for i := range attributes {
		errors := ValidateTransistorAttributes(&attributes[i])
		hasErrorsWithFragment(t, errors, "пустое значение")
	}
}

func TestAttributeYears_InAllowedRange_NoErrors(t *testing.T) {
	cases := []TransistorAttributes{
		{YearFrom: Ptr(1949)},
		{YearFrom: Ptr(1970), YearTo: Ptr(1985)},
		{YearTo: Ptr(2100)},
	}
	for i := range cases {
		if errors := ValidateTransistorAttributes(&cases[i]); len(errors) > 0 {
			t.Errorf("случай №%d: ожидалось отсутствие ошибок, получено: %v", i+1, errors)
		}
	}
}

func TestAttributeYears_OutOfRange_ProduceErrors(t *testing.T) {
	hasErrorsWithFragment(t, ValidateTransistorAttributes(&TransistorAttributes{YearFrom: Ptr(1948)}), "год начала выпуска")
	hasErrorsWithFragment(t, ValidateTransistorAttributes(&TransistorAttributes{YearTo: Ptr(2101)}), "год окончания выпуска")
}

func TestAttributeYearFrom_MustBeBeforeYearTo(t *testing.T) {
	hasErrorsWithFragment(t, ValidateTransistorAttributes(&TransistorAttributes{YearFrom: Ptr(1970), YearTo: Ptr(1970)}),
		"должен быть меньше года окончания")
	hasErrorsWithFragment(t, ValidateTransistorAttributes(&TransistorAttributes{YearFrom: Ptr(1980), YearTo: Ptr(1970)}),
		"должен быть меньше года окончания")
}

func TestAttributeMassMustBePositive(t *testing.T) {
	hasErrorsWithFragment(t, ValidateTransistorAttributes(&TransistorAttributes{MassMax: Ptr(0.0)}), "масса")
	if errors := ValidateTransistorAttributes(&TransistorAttributes{MassMax: Ptr(1.2)}); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}
