package domain

import (
	"fmt"
	"strings"
)

func ValidateElectricalParameter(p ElectricalParameter) []string {
	var errors []string
	info := ParameterInfoOf(p.Kind)

	switch info.Direction {
	case AtLeast:
		if p.ValueMin == nil {
			errors = append(errors, fmt.Sprintf("«%s»: обязательно значение «не менее» (min)", info.Code))
		}
		if p.ValueMax != nil {
			errors = append(errors, fmt.Sprintf("«%s»: верхняя граница (max) не допускается — параметр вида «не менее»", info.Code))
		}
	case AtMost:
		if p.ValueMax == nil {
			errors = append(errors, fmt.Sprintf("«%s»: обязательно значение «не более» (max)", info.Code))
		}
		if p.ValueMin != nil {
			errors = append(errors, fmt.Sprintf("«%s»: нижняя граница (min) не допускается — параметр вида «не более»", info.Code))
		}
	case AtLeastOrRange:
		if p.ValueMin == nil {
			errors = append(errors, fmt.Sprintf("«%s»: обязательна нижняя граница (min)", info.Code))
		}
	}
	if p.ValueMin != nil && *p.ValueMin <= 0 {
		errors = append(errors, fmt.Sprintf("«%s»: значение должно быть положительным", info.Code))
	}
	if p.ValueMax != nil && *p.ValueMax <= 0 {
		errors = append(errors, fmt.Sprintf("«%s»: значение должно быть положительным", info.Code))
	}
	if p.ValueMin != nil && p.ValueMax != nil && *p.ValueMin > *p.ValueMax {
		errors = append(errors, fmt.Sprintf("«%s»: нижняя граница больше верхней", info.Code))
	}
	if info.ValueCeiling != nil {
		unit := ""
		if info.Unit != "" {
			unit = " " + info.Unit
		}
		limit := fmt.Sprintf("«%s»: %s не может превышать %s%s", info.Code, info.DisplayName, Fmt(*info.ValueCeiling), unit)
		if p.ValueMin != nil && *p.ValueMin > *info.ValueCeiling {
			errors = append(errors, limit)
		}
		if p.ValueMax != nil && *p.ValueMax > *info.ValueCeiling {
			errors = append(errors, limit)
		}
	}

	if !info.Conditions.IsSatisfiedBy(p) {
		errors = append(errors, fmt.Sprintf("«%s»: условия — %s; задано: %s", info.Code, info.Conditions.Describe(), describeConditionsOf(p)))
	}

	for _, key := range AllConditionKeys {
		if value := ConditionValueOf(p, key); value != nil && *value <= 0 {
			errors = append(errors, ConditionNonPositiveMessage(key))
		}
	}
	return errors
}

func describeConditionsOf(p ElectricalParameter) string {
	var parts []string
	for _, key := range AllConditionKeys {
		if ConditionValueOf(p, key) != nil {
			parts = append(parts, ConditionLabel(key))
		}
	}
	if len(parts) == 0 {
		return "ничего"
	}
	return strings.Join(parts, ", ")
}
