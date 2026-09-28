package domain

import "testing"

func TestFormatParameterValue_MinOnly_WithoutUnit(t *testing.T) {
	parameter := ElectricalParameter{Kind: KindH21E, ValueMin: Ptr(50.0), Uke: Ptr(10.0), Ik: Ptr(1.0)}
	if got := FormatParameterValue(parameter); got != "не менее 50" {
		t.Errorf("Value = %q, ожидалось %q", got, "не менее 50")
	}
}

func TestFormatParameterValue_Range_BothBounds(t *testing.T) {
	parameter := ElectricalParameter{Kind: KindH21E, ValueMin: Ptr(20.0), ValueMax: Ptr(200.0), Uke: Ptr(10.0), Ik: Ptr(1.0)}
	if got := FormatParameterValue(parameter); got != "20...200" {
		t.Errorf("Value = %q, ожидалось %q", got, "20...200")
	}
}

func TestFormatParameterValue_MaxOnly_WithUnit(t *testing.T) {
	parameter := ElectricalParameter{Kind: KindCollectorCutoffCurrent, ValueMax: Ptr(1.2), Ukb: Ptr(10.0)}
	if got := FormatParameterValue(parameter); got != "не более 1.2 мкА" {
		t.Errorf("Value = %q, ожидалось %q", got, "не более 1.2 мкА")
	}
}

func TestFormatParameterConditions_WithoutConditions(t *testing.T) {
	parameter := ElectricalParameter{Kind: KindH21E, ValueMin: Ptr(50.0)}
	if got := FormatParameterConditions(parameter); got != "без условий" {
		t.Errorf("Conditions = %q, ожидалось %q", got, "без условий")
	}
}

func TestFormatParameterConditions_AllSpecified_InCanonicalOrder(t *testing.T) {
	parameter := ElectricalParameter{
		Kind:     KindNoiseFigure,
		ValueMax: Ptr(4.0),
		Uke:      Ptr(5.0),
		Ik:       Ptr(1.0),
		Freq:     Ptr(1.8),
		Rg:       Ptr(500.0),
		Temp:     Ptr(25.0),
	}
	expected := "при Uкэ = 5 В, Iк = 1 мА, f = 1.8 МГц, Rг = 500 Ом, T = 25 °C"
	if got := FormatParameterConditions(parameter); got != expected {
		t.Errorf("Conditions = %q, ожидалось %q", got, expected)
	}
}
