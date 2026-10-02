package catalog

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Rule — именованный код-валидатор межполевого правила. Реестр реализаций
// мал по построению (риск R7): правило добавляется только тогда, когда оно
// не выражается данными каталога (условия/типы/enum). Привязка — данными:
// parameters.validation_rule, attributes.validation_rule,
// kind_validation_rules (plan/03-data-model.md §10).
type Rule interface {
	Code() string
	Description() string
}

// ParameterRule — правило, привязываемое к параметрам: проверяет набор
// значений одного контекста (запись в целом либо одно исполнение).
type ParameterRule interface {
	Rule
	CheckValues(e *Engine, kind domain.Kind, vals []ParameterValue) []Problem
}

// AttributeRule — правило, привязываемое к атрибутам записи.
type AttributeRule interface {
	Rule
	CheckAttributes(e *Engine, kind domain.Kind, vals []AttributeValue) []Problem
}

// DeviceRule — правило записей класса в целом (kind_validation_rules):
// проверки, не привязанные к отдельному параметру (матрицы исполнений).
// AllowsVariants сообщает, придаёт ли правило классу семантику исполнений
// (D6): секция variants допустима только для классов хотя бы с одним
// таким правилом (транзисторы/диоды — без вариантов).
type DeviceRule interface {
	Rule
	CheckDevice(e *Engine, d *Device) []Problem
	AllowsVariants() bool
}

// Реестр правил: код → реализация. Сиды validation_rules обязаны
// соответствовать реестру (пин-тест seed); неизвестный код в данных
// каталога — ошибка импорта каталога (громко, не молча).
var ruleRegistry = map[string]Rule{
	"year_range":             yearRangeRule{},
	"cap_dimensions_form":    capDimensionsFormRule{},
	"temp_pair":              tempPairRule{},
	"cap_variant_matrix":     capVariantMatrixRule{},
	"resistor_variant_power": resistorVariantPowerRule{},
}

// RuleByCode ищет правило реестра по коду.
func RuleByCode(code string) (Rule, bool) {
	r, ok := ruleRegistry[code]
	return r, ok
}

// Rules возвращает все зарегистрированные правила, отсортированные по коду
// (порядок сидов validation_rules).
func Rules() []Rule {
	out := make([]Rule, 0, len(ruleRegistry))
	for _, r := range ruleRegistry {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b Rule) int {
		return strings.Compare(a.Code(), b.Code())
	})
	return out
}

func ruleProblem(format string, a ...any) Problem {
	return Problem{Code: domain.CodeValidationFailed, Message: fmt.Sprintf(format, a...)}
}

// yearRangeRule — годы выпуска: yearFrom < yearTo, диапазон 1949–2100
// (привязка — атрибуты yearFrom/yearTo).
type yearRangeRule struct{}

func (yearRangeRule) Code() string { return "year_range" }
func (yearRangeRule) Description() string {
	return "годы выпуска: yearFrom < yearTo; диапазон 1949–2100"
}

func (yearRangeRule) CheckAttributes(_ *Engine, _ domain.Kind, vals []AttributeValue) []Problem {
	var probs []Problem
	from, hasFrom := attrNum(vals, "yearFrom")
	to, hasTo := attrNum(vals, "yearTo")
	for _, y := range []struct {
		code  string
		value float64
		ok    bool
	}{
		{"yearFrom", from, hasFrom},
		{"yearTo", to, hasTo},
	} {
		if y.ok && (y.value < 1949 || y.value > 2100) {
			probs = append(probs, ruleProblem("атрибут «%s»: год вне диапазона 1949–2100", y.code))
		}
	}
	if hasFrom && hasTo && from >= to {
		probs = append(probs, ruleProblem(
			"атрибуты «yearFrom» и «yearTo»: год начала должен быть меньше года окончания"))
	}
	return probs
}

// tempPairRule — согласованность температурных пар: TempMin < TempMax
// (предельные данные) и opTempMin < opTempMax (характеристики конденсаторов),
// если заданы оба значения (привязка — все четыре параметра).
type tempPairRule struct{}

func (tempPairRule) Code() string { return "temp_pair" }
func (tempPairRule) Description() string {
	return "согласованность температурной пары: TempMin < TempMax и opTempMin < opTempMax (если заданы оба)"
}

func (tempPairRule) CheckValues(_ *Engine, _ domain.Kind, vals []ParameterValue) []Problem {
	var probs []Problem
	for _, pair := range [][2]string{{"TempMin", "TempMax"}, {"opTempMin", "opTempMax"}} {
		lo, hasLo := paramExact(vals, pair[0])
		hi, hasHi := paramExact(vals, pair[1])
		if hasLo && hasHi && lo >= hi {
			probs = append(probs, ruleProblem(
				"параметр «%s» должен быть меньше параметра «%s»", pair[0], pair[1]))
		}
	}
	return probs
}

// capDimensionsFormRule — согласованность формы корпуса в одном контексте
// (запись в целом либо исполнение): прямоугольная (length+width+height),
// осевая цилиндрическая (diameter+leadLength) либо радиальная
// цилиндрическая (diameter+height, высота корпуса); смешение форм и
// неполный набор — ошибка (привязка — параметры группы dimensional).
// Радиальная форма добавлена этапом 6 по реальным данным К50-35
// (ОЖ0.464.214 ТУ: D×H и шаг выводов; длина радиальных выводов
// производителем не нормируется) — plan/08-data-verification.md.
type capDimensionsFormRule struct{}

func (capDimensionsFormRule) Code() string { return "cap_dimensions_form" }
func (capDimensionsFormRule) Description() string {
	return "согласованность формы корпуса: прямоугольная (length+width+height) либо цилиндрическая — осевая (diameter+leadLength) или радиальная (diameter+height), смешение — ошибка"
}

func (capDimensionsFormRule) CheckValues(_ *Engine, _ domain.Kind, vals []ParameterValue) []Problem {
	has := func(codes ...string) bool {
		for _, c := range codes {
			if _, ok := paramAny(vals, c); ok {
				return true
			}
		}
		return false
	}
	if has("diameter", "leadLength") {
		if _, ok := paramAny(vals, "diameter"); !ok {
			return []Problem{ruleProblem(
				"габариты: неполный цилиндрический набор корпуса — требуются diameter и leadLength (осевые) либо diameter и height (радиальные)")}
		}
		if _, ok := paramAny(vals, "leadLength"); !ok {
			if _, ok := paramAny(vals, "height"); !ok {
				return []Problem{ruleProblem(
					"габариты: неполный цилиндрический набор корпуса — требуются diameter и leadLength (осевые) либо diameter и height (радиальные)")}
			}
		}
		if has("length", "width") {
			return []Problem{ruleProblem(
				"габариты: смешение форм корпуса — прямоугольная (length+width+height) и цилиндрическая (diameter+leadLength/height)")}
		}
		return nil
	}
	if has("length", "width", "height") {
		for _, c := range []string{"length", "width", "height"} {
			if _, ok := paramAny(vals, c); !ok {
				return []Problem{ruleProblem(
					"габариты: неполный прямоугольный набор корпуса — требуются length, width и height")}
			}
		}
	}
	return nil
}

// capVariantMatrixRule — вариант электролитического конденсатора обязан
// иметь Unom и Cnom; при наличии габаритов — согласованный набор формы
// корпуса; уникальность Unom и метки в пределах записи (привязка — класс
// capacitor, kind_validation_rules).
type capVariantMatrixRule struct{}

func (capVariantMatrixRule) Code() string { return "cap_variant_matrix" }
func (capVariantMatrixRule) Description() string {
	return "вариант электролитического конденсатора: обязательны Unom и Cnom, габариты — согласованным набором формы корпуса, уникальность Unom и метки"
}

func (capVariantMatrixRule) AllowsVariants() bool { return true }

func (capVariantMatrixRule) CheckDevice(e *Engine, d *Device) []Problem {
	var probs []Problem
	seenUnom := make(map[float64]string, len(d.Variants))
	seenLabel := make(map[string]bool, len(d.Variants))
	for i := range d.Variants {
		v := &d.Variants[i]
		name := variantName(v, i)
		if _, ok := paramAny(v.Values, "Unom"); !ok {
			probs = append(probs, ruleProblem("вариант %s: отсутствует обязательный параметр Unom", name))
		}
		if _, ok := paramAny(v.Values, "Cnom"); !ok {
			probs = append(probs, ruleProblem("вариант %s: отсутствует обязательный параметр Cnom", name))
		}
		probs = append(probs, capDimensionsFormRule{}.CheckValues(e, d.Kind, v.Values)...)
		if unom, ok := paramExact(v.Values, "Unom"); ok {
			if prev, dup := seenUnom[unom]; dup {
				probs = append(probs, ruleProblem("вариант %s: Unom повторяется (уже задан вариантом %s)", name, prev))
			} else {
				seenUnom[unom] = name
			}
		}
		if v.Label != "" {
			if seenLabel[v.Label] {
				probs = append(probs, ruleProblem("вариант %s: метка повторяется", name))
			} else {
				seenLabel[v.Label] = true
			}
		}
	}
	return probs
}

// resistorVariantPowerRule — вариант резистора обязан иметь Pnom;
// уникальность Pnom и метки в пределах записи (привязка — класс resistor).
type resistorVariantPowerRule struct{}

func (resistorVariantPowerRule) Code() string { return "resistor_variant_power" }
func (resistorVariantPowerRule) Description() string {
	return "вариант резистора: обязателен Pnom, уникальность Pnom и метки"
}

func (resistorVariantPowerRule) AllowsVariants() bool { return true }

func (resistorVariantPowerRule) CheckDevice(_ *Engine, d *Device) []Problem {
	var probs []Problem
	seenPower := make(map[float64]string, len(d.Variants))
	seenLabel := make(map[string]bool, len(d.Variants))
	for i := range d.Variants {
		v := &d.Variants[i]
		name := variantName(v, i)
		if _, ok := paramAny(v.Values, "Pnom"); !ok {
			probs = append(probs, ruleProblem("вариант %s: отсутствует обязательный параметр Pnom", name))
		}
		if p, ok := paramExact(v.Values, "Pnom"); ok {
			if prev, dup := seenPower[p]; dup {
				probs = append(probs, ruleProblem("вариант %s: Pnom повторяется (уже задан вариантом %s)", name, prev))
			} else {
				seenPower[p] = name
			}
		}
		if v.Label != "" {
			if seenLabel[v.Label] {
				probs = append(probs, ruleProblem("вариант %s: метка повторяется", name))
			} else {
				seenLabel[v.Label] = true
			}
		}
	}
	return probs
}

// variantName — имя исполнения для сообщений: метка либо порядковый номер.
func variantName(v *Variant, i int) string {
	if v.Label != "" {
		return "«" + v.Label + "»"
	}
	return fmt.Sprintf("№%d", i+1)
}

// attrNum ищет первое числовое значение атрибута.
func attrNum(vals []AttributeValue, code string) (float64, bool) {
	for _, av := range vals {
		if av.Attribute == code && av.Num != nil {
			return *av.Num, true
		}
	}
	return 0, false
}

// paramAny ищет первое значение параметра по коду.
func paramAny(vals []ParameterValue, code string) (*ParameterValue, bool) {
	for i := range vals {
		if vals[i].Parameter == code {
			return &vals[i], true
		}
	}
	return nil, false
}

// paramExact ищет первое точное значение (value) параметра.
func paramExact(vals []ParameterValue, code string) (float64, bool) {
	for i := range vals {
		v := &vals[i]
		if v.Parameter == code && v.Exact != nil {
			return *v.Exact, true
		}
	}
	return 0, false
}

// isWholeNumber сообщает, целое ли число (без дробной части).
func isWholeNumber(v float64) bool {
	return v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v)
}
