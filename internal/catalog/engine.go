package catalog

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Engine — движок валидации значений по правилам каталога из снимка
// (план работ 2.2). Все предметные проверки читаются из данных снимка:
// применимость параметров/атрибутов к классу (D7), типы значений,
// позитивность, потолок, комбинации условий, enum; кодом — только
// механика проверок и именованные межполевые правила. Движок не имеет
// собственного состояния, снимок иммутабелен — безопасен для
// одновременного использования несколькими горутинами.
type Engine struct {
	snap *Snapshot
}

// NewEngine создаёт движок валидации над снимком каталога.
func NewEngine(snap *Snapshot) *Engine {
	return &Engine{snap: snap}
}

// Snapshot возвращает снимок, поверх которого работает движок.
func (e *Engine) Snapshot() *Snapshot {
	return e.snap
}

// strictSystems — строгие системы обозначений (для инварианта реестра
// series, plan/03-data-model.md §2.4).
var strictSystems = []domain.System{
	domain.SystemGost, domain.SystemOst, domain.SystemPro, domain.SystemJedec, domain.SystemJis,
}

// ValidateDevice проверяет запись наполнения целиком: заголовок (класс,
// система обозначений), атрибуты, значения параметров типа в целом,
// исполнения и именованные правила. Пустой результат — запись корректна;
// проблемы накапливаются все за один прогон (частичное применение записей
// запрещено — plan/01-architecture.md §2.4).
func (e *Engine) ValidateDevice(d *Device) []Problem {
	var probs []Problem
	probs = append(probs, e.validateHeader(d)...)
	probs = append(probs, e.validateAttributes(d.Kind, d.Attributes)...)
	probs = append(probs, e.validateValues(d.Kind, d.Values)...)
	if len(d.Variants) > 0 {
		if !e.kindAllowsVariants(d.Kind) {
			probs = append(probs, ruleProblem("класс %s не поддерживает исполнения (варианты)", string(d.Kind)))
		}
		for i := range d.Variants {
			probs = append(probs, e.validateValues(d.Kind, d.Variants[i].Values)...)
		}
	}
	probs = append(probs, e.runKindRules(d)...)
	return probs
}

// validateHeader — класс существует, система существует и применима
// к классу; записи системы series — семейство известно реестру для класса
// и обозначение не разбирается строгими системами (инвариант §2.4).
func (e *Engine) validateHeader(d *Device) []Problem {
	var probs []Problem
	if d.Kind == "" {
		return append(probs, Problem{domain.CodeValidationFailed, "класс прибора не задан"})
	}
	if _, ok := e.snap.Kind(d.Kind); !ok {
		probs = append(probs, Problem{
			Code:    domain.CodeValidationFailed,
			Message: fmt.Sprintf("неизвестный класс приборов «%s»", string(d.Kind)),
		})
	}
	if d.System == "" {
		return probs
	}
	if _, ok := e.snap.System(d.System); !ok {
		return append(probs, Problem{
			Code:    domain.CodeValidationFailed,
			Message: fmt.Sprintf("неизвестная система обозначений «%s»", string(d.System)),
		})
	}
	if !e.snap.SystemAppliesTo(d.System, d.Kind) {
		probs = append(probs, Problem{
			Code:    domain.CodeValidationFailed,
			Message: fmt.Sprintf("система обозначений «%s» неприменима к классу %s", string(d.System), string(d.Kind)),
		})
	}
	if d.System == domain.SystemSeries {
		probs = append(probs, e.validateSeriesRecord(d)...)
	}
	return probs
}

func (e *Engine) validateSeriesRecord(d *Device) []Problem {
	var probs []Problem
	if _, ok := e.snap.MatchSeriesFamily(d.Designation, d.Kind); !ok {
		probs = append(probs, Problem{
			Code: domain.CodeValidationFailed,
			Message: fmt.Sprintf("обозначение «%s»: неизвестное семейство (система series, класс %s)",
				d.Designation, string(d.Kind)),
		})
	}
	for _, sys := range strictSystems {
		if _, err := domain.ParseDesignationForSystem(d.Designation, sys, d.Kind); err == nil {
			probs = append(probs, Problem{
				Code: domain.CodeValidationFailed,
				Message: fmt.Sprintf(
					"обозначение «%s» разбирается строгой системой «%s» и не может принадлежать системе series",
					d.Designation, string(sys)),
			})
		}
	}
	return probs
}

// validateAttributes — существование, применимость к классу (D7), тип
// значения и именованные правила атрибутов.
func (e *Engine) validateAttributes(kind domain.Kind, vals []AttributeValue) []Problem {
	var probs []Problem
	var ruleCodes []string
	for _, av := range vals {
		a, ok := e.snap.Attribute(av.Attribute)
		if !ok {
			probs = append(probs, Problem{
				Code:    domain.CodeUnknownAttribute,
				Message: fmt.Sprintf("неизвестный атрибут «%s»", av.Attribute),
			})
			continue
		}
		if !a.Active {
			probs = append(probs, Problem{
				Code:    domain.CodeValidationFailed,
				Message: fmt.Sprintf("атрибут «%s» деактивирован", a.Code),
			})
			continue
		}
		if !a.AppliesTo(kind) {
			probs = append(probs, Problem{
				Code:    domain.CodeAttributeNotApplicable,
				Message: fmt.Sprintf("атрибут «%s» неприменим к классу %s", a.Code, string(kind)),
			})
			continue
		}
		probs = append(probs, checkAttrValue(a, av)...)
		if a.ValidationRule != "" && !slices.Contains(ruleCodes, a.ValidationRule) {
			ruleCodes = append(ruleCodes, a.ValidationRule)
		}
	}
	for _, code := range ruleCodes {
		if r, ok := RuleByCode(code); ok {
			if ar, ok := r.(AttributeRule); ok {
				probs = append(probs, ar.CheckAttributes(e, kind, vals)...)
			}
		}
	}
	return probs
}

// checkAttrValue — форма и семантика значения атрибута по типу
// (plan/03-data-model.md §9: текст непустой после trim, bool 0/1,
// enum из списка, число положительно).
func checkAttrValue(a *AttributeDef, av AttributeValue) []Problem {
	fail := func(msg string) []Problem {
		return []Problem{{Code: domain.CodeValidationFailed, Message: msg}}
	}
	switch a.Type {
	case AttrText:
		if av.Text == nil {
			return fail(fmt.Sprintf("атрибут «%s»: ожидается текстовое значение", a.Code))
		}
		if strings.TrimSpace(*av.Text) == "" {
			return fail(fmt.Sprintf("атрибут «%s»: текст не может быть пустым", a.Code))
		}
	case AttrEnum:
		if av.Text == nil {
			return fail(fmt.Sprintf("атрибут «%s»: ожидается значение из списка (%s)",
				a.Code, strings.Join(a.EnumValues, ", ")))
		}
		if !slices.Contains(a.EnumValues, *av.Text) {
			return fail(fmt.Sprintf("атрибут «%s»: значение «%s» не входит в допустимые (%s)",
				a.Code, *av.Text, strings.Join(a.EnumValues, ", ")))
		}
	case AttrBool:
		if av.Bool == nil {
			return fail(fmt.Sprintf("атрибут «%s»: ожидается логическое значение", a.Code))
		}
	case AttrInt:
		if av.Num == nil {
			return fail(fmt.Sprintf("атрибут «%s»: ожидается целое число", a.Code))
		}
		if !isWholeNumber(*av.Num) {
			return fail(fmt.Sprintf("атрибут «%s»: значение должно быть целым числом", a.Code))
		}
		if *av.Num <= 0 {
			return fail(fmt.Sprintf("атрибут «%s»: значение должно быть положительным", a.Code))
		}
	case AttrNumber:
		if av.Num == nil {
			return fail(fmt.Sprintf("атрибут «%s»: ожидается число", a.Code))
		}
		if *av.Num <= 0 {
			return fail(fmt.Sprintf("атрибут «%s»: значение должно быть положительным", a.Code))
		}
	}
	return nil
}

// validateValues — существование, применимость (D7), секция группы,
// форма значения по типу, условия, дубликаты и именованные правила
// параметров одного контекста (запись в целом либо исполнение).
func (e *Engine) validateValues(kind domain.Kind, vals []ParameterValue) []Problem {
	var probs []Problem
	seen := make(map[string]bool, len(vals))
	var ruleCodes []string
	for i := range vals {
		v := &vals[i]
		p, ok := e.snap.Parameter(v.Parameter)
		if !ok {
			probs = append(probs, Problem{
				Code:    domain.CodeUnknownParameter,
				Message: fmt.Sprintf("неизвестный параметр «%s»", v.Parameter),
			})
			continue
		}
		if !p.Active {
			probs = append(probs, Problem{
				Code:    domain.CodeValidationFailed,
				Message: fmt.Sprintf("параметр «%s» деактивирован", p.Code),
			})
			continue
		}
		if !p.AppliesTo(kind) {
			probs = append(probs, Problem{
				Code:    domain.CodeParameterNotApplicable,
				Message: fmt.Sprintf("параметр «%s» неприменим к классу %s", p.Code, string(kind)),
			})
			continue
		}
		if v.Section != "" {
			if g, ok := e.snap.Group(p.Group); ok && g.SectionName != v.Section {
				probs = append(probs, Problem{
					Code: domain.CodeValidationFailed,
					Message: fmt.Sprintf("параметр «%s» задан в секции «%s», относится к секции «%s»",
						p.Code, v.Section, g.SectionName),
				})
			}
		}
		probs = append(probs, checkValueShape(p, v)...)
		probs = append(probs, e.checkValueConditions(p, v)...)
		key := valueKey(v)
		if seen[key] {
			probs = append(probs, Problem{
				Code:    domain.CodeValidationFailed,
				Message: fmt.Sprintf("параметр «%s»: дубликат значения с теми же условиями", p.Code),
			})
		} else {
			seen[key] = true
		}
		if p.ValidationRule != "" && !slices.Contains(ruleCodes, p.ValidationRule) {
			ruleCodes = append(ruleCodes, p.ValidationRule)
		}
	}
	for _, code := range ruleCodes {
		if r, ok := RuleByCode(code); ok {
			if pr, ok := r.(ParameterRule); ok {
				probs = append(probs, pr.CheckValues(e, kind, vals)...)
			}
		}
	}
	return probs
}

// checkValueShape — форма значения по value_type параметра
// (plan/03-data-model.md §6): exact → value; at_least → min
// (необязательная верхняя граница max); at_most → max; range → min и max;
// text/enum → text. Позитивность (если не allow_negative) и потолок
// проверяются для каждой числовой части.
func checkValueShape(p *ParameterDef, v *ParameterValue) []Problem {
	var probs []Problem
	add := func(format string, a ...any) {
		probs = append(probs, Problem{Code: domain.CodeValidationFailed, Message: fmt.Sprintf(format, a...)})
	}
	numKey := func(key string, val *float64) {
		if val == nil {
			return
		}
		if *val <= 0 && !p.AllowNegative {
			add("параметр «%s»: значение ключа %s должно быть положительным", p.Code, key)
		}
		if p.Ceiling != nil && *val > *p.Ceiling {
			add("параметр «%s»: значение ключа %s превышает потолок %s", p.Code, key, formatNum(*p.Ceiling))
		}
	}
	switch p.ValueType {
	case ValueExact:
		if v.Exact == nil {
			add("параметр «%s»: тип значения exact — обязателен ключ value", p.Code)
		}
		if v.Min != nil {
			add("параметр «%s»: тип значения exact не допускает ключ min", p.Code)
		}
		if v.Max != nil {
			add("параметр «%s»: тип значения exact не допускает ключ max", p.Code)
		}
		if v.Text != nil {
			add("параметр «%s»: тип значения exact не допускает ключ text", p.Code)
		}
		numKey("value", v.Exact)
	case ValueAtLeast:
		if v.Min == nil {
			add("параметр «%s»: тип значения at_least — обязателен ключ min", p.Code)
		}
		if v.Exact != nil {
			add("параметр «%s»: тип значения at_least не допускает ключ value", p.Code)
		}
		if v.Text != nil {
			add("параметр «%s»: тип значения at_least не допускает ключ text", p.Code)
		}
		numKey("min", v.Min)
		numKey("max", v.Max)
		if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
			add("параметр «%s»: min превышает max", p.Code)
		}
	case ValueAtMost:
		if v.Max == nil {
			add("параметр «%s»: тип значения at_most — обязателен ключ max", p.Code)
		}
		if v.Min != nil {
			add("параметр «%s»: тип значения at_most не допускает ключ min", p.Code)
		}
		if v.Exact != nil {
			add("параметр «%s»: тип значения at_most не допускает ключ value", p.Code)
		}
		if v.Text != nil {
			add("параметр «%s»: тип значения at_most не допускает ключ text", p.Code)
		}
		numKey("max", v.Max)
	case ValueRange:
		if v.Min == nil || v.Max == nil {
			add("параметр «%s»: тип значения range — обязательны ключи min и max", p.Code)
		}
		if v.Exact != nil {
			add("параметр «%s»: тип значения range не допускает ключ value", p.Code)
		}
		if v.Text != nil {
			add("параметр «%s»: тип значения range не допускает ключ text", p.Code)
		}
		numKey("min", v.Min)
		numKey("max", v.Max)
		if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
			add("параметр «%s»: min превышает max", p.Code)
		}
	case ValueText:
		if v.Text == nil {
			add("параметр «%s»: тип значения text — обязателен ключ text", p.Code)
		} else if strings.TrimSpace(*v.Text) == "" {
			add("параметр «%s»: текст не может быть пустым", p.Code)
		}
		if v.Exact != nil {
			add("параметр «%s»: тип значения text не допускает ключ value", p.Code)
		}
		if v.Min != nil {
			add("параметр «%s»: тип значения text не допускает ключ min", p.Code)
		}
		if v.Max != nil {
			add("параметр «%s»: тип значения text не допускает ключ max", p.Code)
		}
	case ValueEnum:
		if v.Text == nil {
			add("параметр «%s»: тип значения enum — обязателен ключ text", p.Code)
		} else if !slices.Contains(p.EnumValues, *v.Text) {
			add("параметр «%s»: значение «%s» не входит в допустимые (%s)",
				p.Code, *v.Text, strings.Join(p.EnumValues, ", "))
		}
		if v.Exact != nil {
			add("параметр «%s»: тип значения enum не допускает ключ value", p.Code)
		}
		if v.Min != nil {
			add("параметр «%s»: тип значения enum не допускает ключ min", p.Code)
		}
		if v.Max != nil {
			add("параметр «%s»: тип значения enum не допускает ключ max", p.Code)
		}
	}
	return probs
}

// checkValueConditions — условия значения: известность, позитивность
// (allow_negative у условия), соответствие одному из наборов условий
// параметра (required обязательны, optional допустимы, остальные
// запрещены; fixed_value — константа, которую можно опустить либо задать
// равной — plan/02-database.md §2.2). У безусловного параметра условий
// быть не должно.
func (e *Engine) checkValueConditions(p *ParameterDef, v *ParameterValue) []Problem {
	var probs []Problem
	add := func(format string, a ...any) {
		probs = append(probs, Problem{Code: domain.CodeValidationFailed, Message: fmt.Sprintf(format, a...)})
	}
	conds := make(map[string]float64, len(v.Conditions))
	unknown := false
	for _, cv := range v.Conditions {
		if _, dup := conds[cv.Condition]; dup {
			add("параметр «%s»: условие «%s» задано повторно", p.Code, cv.Condition)
			continue
		}
		c, ok := e.snap.Condition(cv.Condition)
		if !ok {
			probs = append(probs, Problem{
				Code:    domain.CodeUnknownCondition,
				Message: fmt.Sprintf("параметр «%s»: неизвестное условие «%s»", p.Code, cv.Condition),
			})
			unknown = true
			continue
		}
		if cv.Value <= 0 && !c.AllowNegative {
			add("параметр «%s»: условие «%s» — значение должно быть положительным", p.Code, cv.Condition)
		}
		conds[cv.Condition] = cv.Value
	}
	if unknown {
		return probs
	}
	if len(p.ConditionSets) == 0 {
		if len(conds) > 0 {
			add("параметр «%s»: безусловный параметр — условия недопустимы", p.Code)
		}
		return probs
	}
	for _, set := range p.ConditionSets {
		if conditionSetMatches(set, conds) {
			return probs
		}
	}
	fixedReported := false
	for _, set := range p.ConditionSets {
		for _, it := range set.Items {
			if it.FixedValue == nil {
				continue
			}
			if val, has := conds[it.Condition]; has && val != *it.FixedValue {
				add("параметр «%s»: условие «%s» зафиксировано значением %s",
					p.Code, it.Condition, formatNum(*it.FixedValue))
				fixedReported = true
			}
		}
	}
	if !fixedReported {
		add("параметр «%s»: комбинация условий не соответствует ни одному набору условий параметра", p.Code)
	}
	return probs
}

// conditionSetMatches — соответствие заданных условий набору: required
// присутствуют (fixed_value — равными константе либо отсутствуют),
// посторонних условий нет.
func conditionSetMatches(set ConditionSet, conds map[string]float64) bool {
	for _, it := range set.Items {
		val, has := conds[it.Condition]
		if it.FixedValue != nil {
			if has && val != *it.FixedValue {
				return false
			}
			continue
		}
		if it.Mode == ModeRequired && !has {
			return false
		}
	}
	for code := range conds {
		found := false
		for _, it := range set.Items {
			if it.Condition == code {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// runKindRules — правила записей класса в целом (kind_validation_rules).
func (e *Engine) runKindRules(d *Device) []Problem {
	var probs []Problem
	for _, code := range e.snap.RulesForKind(d.Kind) {
		if r, ok := RuleByCode(code); ok {
			if dr, ok := r.(DeviceRule); ok {
				probs = append(probs, dr.CheckDevice(e, d)...)
			}
		}
	}
	return probs
}

// kindAllowsVariants — класс поддерживает исполнения, только если к нему
// привязано правило с семантикой исполнений (D6: конденсаторы — матрица
// номиналов, резисторы — ряд мощностей; транзисторы/диоды — без вариантов).
func (e *Engine) kindAllowsVariants(kind domain.Kind) bool {
	for _, code := range e.snap.RulesForKind(kind) {
		if r, ok := RuleByCode(code); ok {
			if dr, ok := r.(DeviceRule); ok && dr.AllowsVariants() {
				return true
			}
		}
	}
	return false
}

// valueKey — канонический ключ значения для контроля дубликатов
// «параметр + одинаковые условия» (plan/02-database.md §2.3).
func valueKey(v *ParameterValue) string {
	parts := make([]string, 0, len(v.Conditions))
	for _, c := range v.Conditions {
		parts = append(parts, c.Condition+"="+strconv.FormatFloat(c.Value, 'g', -1, 64))
	}
	slices.Sort(parts)
	return v.Parameter + "|" + strings.Join(parts, ";")
}

// formatNum — компактная запись числа для сообщений (целое — без точки).
func formatNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
