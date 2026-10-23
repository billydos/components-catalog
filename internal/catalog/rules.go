package catalog

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Rule — именованный код-валидатор межполевого правила. Реестр реализаций
// мал по построению (риск R7): правило добавляется только тогда, когда оно
// не выражается данными каталога (условия/типы/enum). Привязка — данными:
// parameters.validation_rule, attributes.validation_rule,
// kind_validation_rules (docs/plan/03-data-model.md §11).
type Rule interface {
	Code() string
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

// Коды именованных правил — стабильные ключи каталога: сиды и данные
// наполнения ссылаются на правила только через эти константы;
// соответствие реестру ruleRegistry закреплено пин-тестами сидов.
const (
	RuleCapDimensionsForm    = "cap_dimensions_form"
	RuleCapVariantMatrix     = "cap_variant_matrix"
	RuleRelayVariantCoil     = "relay_variant_coil"
	RuleResistorVariantPower = "resistor_variant_power"
	RuleTempPair             = "temp_pair"
	RuleYearRange            = "year_range"
)

// Реестр правил: код → реализация. Сиды validation_rules обязаны
// соответствовать реестру (пин-тест seed); неизвестный код в данных
// каталога — ошибка импорта каталога (громко, не молча).
var ruleRegistry = map[string]Rule{
	"year_range":             yearRangeRule{},
	"cap_dimensions_form":    capDimensionsFormRule{},
	"temp_pair":              tempPairRule{},
	"cap_variant_matrix":     capVariantMatrixRule{},
	"relay_variant_coil":     relayVariantCoilRule{},
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
	return slices.SortedFunc(maps.Values(ruleRegistry), func(a, b Rule) int {
		return strings.Compare(a.Code(), b.Code())
	})
}

func ruleProblem(id domain.MsgID, a ...any) Problem {
	return Problemf(domain.CodeValidationFailed, id, a...)
}

// Границы годов выпуска — общие для правила year_range (атрибуты
// yearFrom/yearTo) и проверки годов синонимов сервисного слоя
// (03-data-model.md §9, §11): единая точка изменения границы.
const (
	YearRangeMin = 1949
	YearRangeMax = 2100
)

// yearRangeRule — годы выпуска: yearFrom < yearTo, диапазон 1949–2100
// (привязка — атрибуты yearFrom/yearTo).
type yearRangeRule struct{}

func (yearRangeRule) Code() string { return "year_range" }

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
		if y.ok && (y.value < YearRangeMin || y.value > YearRangeMax) {
			probs = append(probs, ruleProblem(domain.MsgRuleYearRange, y.code))
		}
	}
	if hasFrom && hasTo && from >= to {
		probs = append(probs, ruleProblem(domain.MsgRuleYearOrder))
	}
	return probs
}

// tempPairRule — согласованность температурных пар: TempMin < TempMax
// (предельные данные) и opTempMin < opTempMax (характеристики конденсаторов),
// если заданы оба значения (привязка — все четыре параметра).
type tempPairRule struct{}

func (tempPairRule) Code() string { return "temp_pair" }

func (tempPairRule) CheckValues(_ *Engine, _ domain.Kind, vals []ParameterValue) []Problem {
	var probs []Problem
	for _, pair := range [][2]string{{"TempMin", "TempMax"}, {"opTempMin", "opTempMax"}} {
		lo, hasLo := paramExact(vals, pair[0])
		hi, hasHi := paramExact(vals, pair[1])
		if hasLo && hasHi && lo >= hi {
			probs = append(probs, ruleProblem(domain.MsgRuleTempPair, pair[0], pair[1]))
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
// производителем не нормируется) — docs/plan/08-data-verification.md.
type capDimensionsFormRule struct{}

func (capDimensionsFormRule) Code() string { return "cap_dimensions_form" }

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
			return []Problem{ruleProblem(domain.MsgRuleCapCylIncomplete)}
		}
		if _, ok := paramAny(vals, "leadLength"); !ok {
			if _, ok := paramAny(vals, "height"); !ok {
				return []Problem{ruleProblem(domain.MsgRuleCapCylIncomplete)}
			}
		}
		if has("length", "width") {
			return []Problem{ruleProblem(domain.MsgRuleCapFormMix)}
		}
		return nil
	}
	if has("length", "width", "height") {
		for _, c := range []string{"length", "width", "height"} {
			if _, ok := paramAny(vals, c); !ok {
				return []Problem{ruleProblem(domain.MsgRuleCapRectIncomplete)}
			}
		}
	}
	return nil
}

// capVariantMatrixRule — вариант конденсатора обязан иметь Vnom и Cnom;
// при наличии габаритов — согласованный набор формы корпуса; уникальны
// метка и ячейка матрицы (caseVariant, Vnom, Cnom) в пределах записи
// (привязка — класс capacitor, kind_validation_rules). Матрица «ёмкость ×
// напряжение → габариты/масса»: у электролитических (К50-35) вариант —
// напряжение с диапазоном ряда Cnom на него, у плёночных (К73-17) — ячейка
// (Vnom × Cnom) со своим набором габаритов, вплоть до точечного номинала
// (min = max); повтор Vnom допустим — ячейки различаются ёмкостью либо
// конструктивным исполнением caseVariant (таблицы «исполнение 1»/«1а»
// одной записи, 03-data-model.md §7).
type capVariantMatrixRule struct{}

func (capVariantMatrixRule) Code() string { return "cap_variant_matrix" }

func (capVariantMatrixRule) AllowsVariants() bool { return true }

// capVariantCell — ячейка матрицы исполнений: конструктивное исполнение
// caseVariant (пустая строка — не задано) + точный Vnom + границы Cnom.
type capVariantCell struct {
	caseVariant string
	vnom        float64
	cnomMin     float64
	cnomMax     float64
}

func (capVariantMatrixRule) CheckDevice(e *Engine, d *Device) []Problem {
	var probs []Problem
	seenCell := make(map[capVariantCell]string, len(d.Variants))
	seenLabel := make(map[string]bool, len(d.Variants))
	for i := range d.Variants {
		v := &d.Variants[i]
		name := variantName(v, i)
		if _, ok := paramAny(v.Values, "Vnom"); !ok {
			probs = append(probs, ruleProblem(domain.MsgRuleVariantVnomMissing, name))
		}
		if _, ok := paramAny(v.Values, "Cnom"); !ok {
			probs = append(probs, ruleProblem(domain.MsgRuleVariantCnomMissing, name))
		}
		probs = append(probs, capDimensionsFormRule{}.CheckValues(e, d.Kind, v.Values)...)
		// Дубликат ячейки ищется только по полному ключу: варианты с
		// неполными Vnom/Cnom уже отмечены проверками значений выше.
		if vnom, ok := paramExact(v.Values, "Vnom"); ok {
			if cnom, ok := paramAny(v.Values, "Cnom"); ok && cnom.Min != nil && cnom.Max != nil {
				cell := capVariantCell{
					caseVariant: attrText(v.Attributes, "caseVariant"),
					vnom:        vnom,
					cnomMin:     *cnom.Min,
					cnomMax:     *cnom.Max,
				}
				if prev, dup := seenCell[cell]; dup {
					probs = append(probs, ruleProblem(domain.MsgRuleVariantCellDup, name, prev))
				} else {
					seenCell[cell] = name
				}
			}
		}
		if v.Label != "" {
			if seenLabel[v.Label] {
				probs = append(probs, ruleProblem(domain.MsgRuleVariantLabelDup, name))
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

func (resistorVariantPowerRule) AllowsVariants() bool { return true }

func (resistorVariantPowerRule) CheckDevice(_ *Engine, d *Device) []Problem {
	var probs []Problem
	seenPower := make(map[float64]string, len(d.Variants))
	seenLabel := make(map[string]bool, len(d.Variants))
	for i := range d.Variants {
		v := &d.Variants[i]
		name := variantName(v, i)
		if _, ok := paramAny(v.Values, "Pnom"); !ok {
			probs = append(probs, ruleProblem(domain.MsgRuleVariantPnomMissing, name))
		}
		if p, ok := paramExact(v.Values, "Pnom"); ok {
			if prev, dup := seenPower[p]; dup {
				probs = append(probs, ruleProblem(domain.MsgRuleVariantPnomDup, name, prev))
			} else {
				seenPower[p] = name
			}
		}
		if v.Label != "" {
			if seenLabel[v.Label] {
				probs = append(probs, ruleProblem(domain.MsgRuleVariantLabelDup, name))
			} else {
				seenLabel[v.Label] = true
			}
		}
	}
	return probs
}

// relayVariantCoilRule — исполнение реле (паспорт, 03 §7) обязано иметь
// сопротивление обмотки Rcoil (диапазон допуска) и обмоточный номинал:
// VcoilNom (точный) либо Ipickup (не более) — токовые паспорта без
// номинального напряжения; уникальность метки-паспорта в пределах записи
// (привязка — класс relay, kind_validation_rules).
type relayVariantCoilRule struct{}

func (relayVariantCoilRule) Code() string { return "relay_variant_coil" }

func (relayVariantCoilRule) AllowsVariants() bool { return true }

func (relayVariantCoilRule) CheckDevice(_ *Engine, d *Device) []Problem {
	var probs []Problem
	seenLabel := make(map[string]bool, len(d.Variants))
	for i := range d.Variants {
		v := &d.Variants[i]
		name := variantName(v, i)
		if _, ok := paramAny(v.Values, "Rcoil"); !ok {
			probs = append(probs, ruleProblem(domain.MsgRuleRelayVariantRcoilMissing, name))
		}
		_, hasVoltage := paramAny(v.Values, "VcoilNom")
		_, hasCurrent := paramAny(v.Values, "Ipickup")
		if !hasVoltage && !hasCurrent {
			probs = append(probs, ruleProblem(domain.MsgRuleRelayVariantNominalMissing, name))
		}
		if v.Label != "" {
			if seenLabel[v.Label] {
				probs = append(probs, ruleProblem(domain.MsgRuleVariantLabelDup, name))
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

// attrText ищет первое текстовое значение атрибута (пустая строка —
// не задано).
func attrText(vals []AttributeValue, code string) string {
	for _, av := range vals {
		if av.Attribute == code && av.Text != nil {
			return *av.Text
		}
	}
	return ""
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
