package domain

import (
	"fmt"
	"strings"
)

// ConditionKey — условие измерения параметра; имя совпадает с колонкой БД.
// Температура среды (Temp) в модель ключей не входит: допустима у любого параметра.
type ConditionKey string

const (
	CondUke  ConditionKey = "Uke"
	CondUkb  ConditionKey = "Ukb"
	CondUeb  ConditionKey = "Ueb"
	CondIk   ConditionKey = "Ik"
	CondIe   ConditionKey = "Ie"
	CondIb   ConditionKey = "Ib"
	CondFreq ConditionKey = "Freq"
	CondRg   ConditionKey = "Rg"
	CondRbe  ConditionKey = "Rbe"
)

// AllConditionKeys — канонический порядок ключей (сообщения валидатора, справка, DDL).
var AllConditionKeys = []ConditionKey{
	CondUke, CondUkb, CondUeb, CondIk, CondIe, CondIb, CondFreq, CondRg, CondRbe,
}

// ConditionJsoncKey — имя ключа в jsonc: freq — единственное слово-исключение.
func ConditionJsoncKey(key ConditionKey) string {
	if key == CondFreq {
		return "freq"
	}
	return string(key)
}

// ConditionLabel — обозначение условия для сообщений и справки (ключ jsonc и единица).
func ConditionLabel(key ConditionKey) string {
	switch key {
	case CondUke:
		return "Uкэ (Uke)"
	case CondUkb:
		return "Uкб (Ukb)"
	case CondUeb:
		return "Uэб (Ueb)"
	case CondIk:
		return "Iк (Ik)"
	case CondIe:
		return "Iэ (Ie)"
	case CondIb:
		return "Iб (Ib)"
	case CondFreq:
		return "частота (freq, МГц)"
	case CondRg:
		return "Rг (Rg, Ом)"
	case CondRbe:
		return "Rбэ (Rbe, Ом)"
	}
	panic("неизвестный ключ условия: " + string(key))
}

// ConditionDescription — название условия с единицей для комментариев DDL.
func ConditionDescription(key ConditionKey) string {
	switch key {
	case CondUke:
		return "напряжение коллектор-эмиттер, В"
	case CondUkb:
		return "напряжение коллектор-база, В"
	case CondUeb:
		return "напряжение эмиттер-база, В"
	case CondIk:
		return "ток коллектора, мА"
	case CondIe:
		return "ток эмиттера, мА"
	case CondIb:
		return "ток базы, мА"
	case CondFreq:
		return "частота, МГц"
	case CondRg:
		return "сопротивление генератора, Ом"
	case CondRbe:
		return "сопротивление в цепи база-эмиттер, Ом"
	}
	panic("неизвестный ключ условия: " + string(key))
}

// ConditionUnit — каноническая единица условия: В, мА, МГц или Ом.
func ConditionUnit(key ConditionKey) string {
	switch key {
	case CondUke, CondUkb, CondUeb:
		return "В"
	case CondIk, CondIe, CondIb:
		return "мА"
	case CondFreq:
		return "МГц"
	case CondRg, CondRbe:
		return "Ом"
	}
	panic("неизвестный ключ условия: " + string(key))
}

// ConditionValueOf — значение условия в параметре (nil — не задано).
func ConditionValueOf(p ElectricalParameter, key ConditionKey) *float64 {
	switch key {
	case CondUke:
		return p.Uke
	case CondUkb:
		return p.Ukb
	case CondUeb:
		return p.Ueb
	case CondIk:
		return p.Ik
	case CondIe:
		return p.Ie
	case CondIb:
		return p.Ib
	case CondFreq:
		return p.Freq
	case CondRg:
		return p.Rg
	case CondRbe:
		return p.Rbe
	}
	panic("неизвестный ключ условия: " + string(key))
}

// ConditionWithValue — копия параметра с заданным значением условия; помимо
// доменных тестов используется importer'ом при чтении условий из файла импорта.
func ConditionWithValue(p ElectricalParameter, key ConditionKey, value float64) ElectricalParameter {
	switch key {
	case CondUke:
		p.Uke = Ptr(value)
	case CondUkb:
		p.Ukb = Ptr(value)
	case CondUeb:
		p.Ueb = Ptr(value)
	case CondIk:
		p.Ik = Ptr(value)
	case CondIe:
		p.Ie = Ptr(value)
	case CondIb:
		p.Ib = Ptr(value)
	case CondFreq:
		p.Freq = Ptr(value)
	case CondRg:
		p.Rg = Ptr(value)
	case CondRbe:
		p.Rbe = Ptr(value)
	default:
		panic("неизвестный ключ условия: " + string(key))
	}
	return p
}

// ConditionNonPositiveMessage — сообщение о неположительном значении условия.
func ConditionNonPositiveMessage(key ConditionKey) string {
	switch key {
	case CondUke:
		return "условие Uкэ должно быть положительным"
	case CondUkb:
		return "условие Uкб должно быть положительным"
	case CondUeb:
		return "условие Uэб должно быть положительным"
	case CondIk:
		return "условие Iк должно быть положительным"
	case CondIe:
		return "условие Iэ должно быть положительным"
	case CondIb:
		return "условие Iб должно быть положительным"
	case CondFreq:
		return "частота должна быть положительной"
	case CondRg:
		return "сопротивление генератора должно быть положительным"
	case CondRbe:
		return "сопротивление Rбэ должно быть положительным"
	}
	panic("неизвестный ключ условия: " + string(key))
}

// ConditionVariant — один допустимый набор условий: обязательные и необязательные
// ключи, все остальные — запрещены.
type ConditionVariant struct {
	required map[ConditionKey]bool
	optional map[ConditionKey]bool
}

// RequireConditionKeys начинает вариант с обязательных ключей; продолжается через Optionally.
func RequireConditionKeys(keys ...ConditionKey) ConditionVariant {
	required := make(map[ConditionKey]bool, len(keys))
	for _, key := range keys {
		required[key] = true
	}
	return ConditionVariant{required: required, optional: map[ConditionKey]bool{}}
}

// Optionally возвращает вариант с теми же обязательными ключами и дополнительными необязательными.
func (v ConditionVariant) Optionally(keys ...ConditionKey) ConditionVariant {
	optional := make(map[ConditionKey]bool, len(keys))
	for _, key := range keys {
		optional[key] = true
	}
	return ConditionVariant{required: v.required, optional: optional}
}

func (v ConditionVariant) IsRequired(key ConditionKey) bool { return v.required[key] }
func (v ConditionVariant) IsOptional(key ConditionKey) bool { return v.optional[key] }

// RequiredKeys — обязательные ключи в каноническом порядке.
func (v ConditionVariant) RequiredKeys() []ConditionKey {
	return orderedConditionKeys(v.required)
}

// OptionalKeys — необязательные ключи в каноническом порядке.
func (v ConditionVariant) OptionalKeys() []ConditionKey {
	return orderedConditionKeys(v.optional)
}

// IsSatisfiedBy: вариант подходит, если все обязательные ключи заданы,
// а из остальных — только необязательные.
func (v ConditionVariant) IsSatisfiedBy(p ElectricalParameter) bool {
	for _, key := range AllConditionKeys {
		set := ConditionValueOf(p, key) != nil
		if v.IsRequired(key) != set && !v.IsOptional(key) {
			return false
		}
	}
	return true
}

// ConditionSpec — набор взаимоисключающих вариантов; параметр корректен,
// если подходит хотя бы один.
type ConditionSpec struct {
	Variants []ConditionVariant
}

func NewConditionSpec(variants ...ConditionVariant) *ConditionSpec {
	if len(variants) == 0 {
		panic("Спецификация условий должна содержать хотя бы один вариант.")
	}
	return &ConditionSpec{Variants: variants}
}

func (s *ConditionSpec) IsSatisfiedBy(p ElectricalParameter) bool {
	for _, variant := range s.Variants {
		if variant.IsSatisfiedBy(p) {
			return true
		}
	}
	return false
}

// Allows — ключ допускается хотя бы одним вариантом: как обязательный или необязательный.
func (s *ConditionSpec) Allows(key ConditionKey) bool {
	for _, variant := range s.Variants {
		if variant.IsRequired(key) || variant.IsOptional(key) {
			return true
		}
	}
	return false
}

// Describe — человекочитаемое описание вариантов, разделённых «либо».
func (s *ConditionSpec) Describe() string {
	parts := make([]string, len(s.Variants))
	for i, variant := range s.Variants {
		parts[i] = describeConditionVariant(variant)
	}
	return strings.Join(parts, " либо ")
}

func describeConditionVariant(variant ConditionVariant) string {
	text := strings.Join(conditionLabels(variant.RequiredKeys()), " + ")
	if optional := variant.OptionalKeys(); len(optional) > 0 {
		text += fmt.Sprintf(" (опционально %s)", strings.Join(conditionLabels(optional), " + "))
	}
	return text
}

func conditionLabels(keys []ConditionKey) []string {
	labels := make([]string, len(keys))
	for i, key := range keys {
		labels[i] = ConditionLabel(key)
	}
	return labels
}

func orderedConditionKeys(set map[ConditionKey]bool) []ConditionKey {
	var keys []ConditionKey
	for _, key := range AllConditionKeys {
		if set[key] {
			keys = append(keys, key)
		}
	}
	return keys
}
