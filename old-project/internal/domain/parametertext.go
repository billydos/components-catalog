package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Fmt — число без лишних нулей: целое печатается без точки, дробное —
// кратчайшей записью (аналог инвариантной культуры C#).
func Fmt(value float64) string {
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// FormatParameterValue — значение параметра с единицей («не менее 50», «20...200»).
func FormatParameterValue(p ElectricalParameter) string {
	info := ParameterInfoOf(p.Kind)
	unit := ""
	if info.Unit != "" {
		unit = " " + info.Unit
	}
	switch {
	case p.ValueMin != nil && p.ValueMax != nil:
		return fmt.Sprintf("%s...%s%s", Fmt(*p.ValueMin), Fmt(*p.ValueMax), unit)
	case p.ValueMin != nil:
		return fmt.Sprintf("не менее %s%s", Fmt(*p.ValueMin), unit)
	case p.ValueMax != nil:
		return fmt.Sprintf("не более %s%s", Fmt(*p.ValueMax), unit)
	}
	return "—"
}

// FormatParameterConditions — условия измерения («при Uкэ = 5 В, ...»).
func FormatParameterConditions(p ElectricalParameter) string {
	var parts []string
	if p.Uke != nil {
		parts = append(parts, fmt.Sprintf("Uкэ = %s В", Fmt(*p.Uke)))
	}
	if p.Ukb != nil {
		parts = append(parts, fmt.Sprintf("Uкб = %s В", Fmt(*p.Ukb)))
	}
	if p.Ueb != nil {
		parts = append(parts, fmt.Sprintf("Uэб = %s В", Fmt(*p.Ueb)))
	}
	if p.Ik != nil {
		parts = append(parts, fmt.Sprintf("Iк = %s мА", Fmt(*p.Ik)))
	}
	if p.Ie != nil {
		parts = append(parts, fmt.Sprintf("Iэ = %s мА", Fmt(*p.Ie)))
	}
	if p.Ib != nil {
		parts = append(parts, fmt.Sprintf("Iб = %s мА", Fmt(*p.Ib)))
	}
	if p.Freq != nil {
		parts = append(parts, fmt.Sprintf("f = %s МГц", Fmt(*p.Freq)))
	}
	if p.Rg != nil {
		parts = append(parts, fmt.Sprintf("Rг = %s Ом", Fmt(*p.Rg)))
	}
	if p.Rbe != nil {
		parts = append(parts, fmt.Sprintf("Rбэ = %s Ом", Fmt(*p.Rbe)))
	}
	if p.Temp != nil {
		parts = append(parts, fmt.Sprintf("T = %s °C", Fmt(*p.Temp)))
	}
	if len(parts) == 0 {
		return "без условий"
	}
	return "при " + strings.Join(parts, ", ")
}
