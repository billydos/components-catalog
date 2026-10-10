package catalog

import (
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

// ValidateDevice проверяет запись наполнения целиком: заголовок (класс,
// система обозначений), атрибуты, значения параметров типа в целом,
// исполнения и именованные правила. Пустой результат — запись корректна;
// проблемы накапливаются все за один прогон (частичное применение записей
// запрещено — docs/plan/01-architecture.md §2.4).
func (e *Engine) ValidateDevice(d *Device) []Problem {
	var probs []Problem
	probs = append(probs, e.validateHeader(d)...)
	probs = append(probs, e.validateAttributes(d.Kind, d.Attributes)...)
	probs = append(probs, e.validateValues(d.Kind, d.Values)...)
	if len(d.Variants) > 0 {
		if !e.kindAllowsVariants(d.Kind) {
			probs = append(probs, ruleProblem(domain.MsgEngineKindNoVariants, string(d.Kind)))
		}
		for i := range d.Variants {
			probs = append(probs, e.validateValues(d.Kind, d.Variants[i].Values)...)
		}
	}
	probs = append(probs, e.runKindRules(d)...)
	return probs
}

// validateHeader — класс существует, система существует и применима
// к классу (03-data-model.md §1.1).
func (e *Engine) validateHeader(d *Device) []Problem {
	var probs []Problem
	if d.Kind == "" {
		return append(probs, Problemf(domain.CodeValidationFailed, domain.MsgEngineKindMissing))
	}
	if _, ok := e.snap.Kind(d.Kind); !ok {
		probs = append(probs, Problemf(domain.CodeValidationFailed, domain.MsgKindUnknown, string(d.Kind)))
	}
	if d.System == "" {
		return probs
	}
	if _, ok := e.snap.System(d.System); !ok {
		return append(probs, Problemf(domain.CodeValidationFailed, domain.MsgSystemUnknown, string(d.System)))
	}
	if !e.snap.SystemAppliesTo(d.System, d.Kind) {
		probs = append(probs, Problemf(domain.CodeValidationFailed, domain.MsgEngineSystemNotApplicable,
			string(d.System), string(d.Kind)))
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
			probs = append(probs, Problemf(domain.CodeUnknownAttribute, domain.MsgEngineAttrUnknown, av.Attribute))
			continue
		}
		if !a.Active {
			probs = append(probs, Problemf(domain.CodeValidationFailed, domain.MsgEngineAttrInactive, a.Code))
			continue
		}
		if !a.AppliesTo(kind) {
			probs = append(probs, Problemf(domain.CodeAttributeNotApplicable, domain.MsgEngineAttrNotApplicable,
				a.Code, string(kind)))
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
// (docs/plan/03-data-model.md §9: текст непустой после trim, bool 0/1,
// enum из списка, число положительно).
func checkAttrValue(a *AttributeDef, av AttributeValue) []Problem {
	fail := func(id domain.MsgID, args ...any) []Problem {
		return []Problem{Problemf(domain.CodeValidationFailed, id, args...)}
	}
	switch a.Type {
	case AttrText:
		if av.Text == nil {
			return fail(domain.MsgEngineAttrTextExpected, a.Code)
		}
		if strings.TrimSpace(*av.Text) == "" {
			return fail(domain.MsgEngineAttrTextEmpty, a.Code)
		}
	case AttrEnum:
		if av.Text == nil {
			return fail(domain.MsgEngineAttrEnumExpected, a.Code, strings.Join(a.EnumValues, ", "))
		}
		if !slices.Contains(a.EnumValues, *av.Text) {
			return fail(domain.MsgEngineAttrEnumInvalid, a.Code, *av.Text, strings.Join(a.EnumValues, ", "))
		}
	case AttrBool:
		if av.Bool == nil {
			return fail(domain.MsgEngineAttrBoolExpected, a.Code)
		}
	case AttrInt:
		if av.Num == nil {
			return fail(domain.MsgEngineAttrIntExpected, a.Code)
		}
		if !isWholeNumber(*av.Num) {
			return fail(domain.MsgEngineAttrWhole, a.Code)
		}
		if *av.Num <= 0 {
			return fail(domain.MsgEngineAttrPositive, a.Code)
		}
	case AttrNumber:
		if av.Num == nil {
			return fail(domain.MsgEngineAttrNumExpected, a.Code)
		}
		if *av.Num <= 0 {
			return fail(domain.MsgEngineAttrPositive, a.Code)
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
			probs = append(probs, Problemf(domain.CodeUnknownParameter, domain.MsgEngineParamUnknown, v.Parameter))
			continue
		}
		if !p.Active {
			probs = append(probs, Problemf(domain.CodeValidationFailed, domain.MsgEngineParamInactive, p.Code))
			continue
		}
		if !p.AppliesTo(kind) {
			probs = append(probs, Problemf(domain.CodeParameterNotApplicable, domain.MsgEngineParamNotApplicable,
				p.Code, string(kind)))
			continue
		}
		if v.Section != "" {
			if g, ok := e.snap.Group(p.Group); ok && g.SectionName != v.Section {
				probs = append(probs, Problemf(domain.CodeValidationFailed, domain.MsgEngineParamWrongSection,
					p.Code, v.Section, g.SectionName))
			}
		}
		probs = append(probs, checkValueShape(p, v)...)
		probs = append(probs, e.checkValueConditions(p, v)...)
		key := valueKey(p, v)
		if seen[key] {
			probs = append(probs, Problemf(domain.CodeValidationFailed, domain.MsgEngineParamDuplicate, p.Code))
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
// (docs/plan/03-data-model.md §6): exact → value; at_least → min
// (необязательная верхняя граница max); at_most → max; range → min и max;
// text/enum → text. Позитивность (если не allow_negative) и потолок
// проверяются для каждой числовой части.
func checkValueShape(p *ParameterDef, v *ParameterValue) []Problem {
	var probs []Problem
	add := func(id domain.MsgID, args ...any) {
		probs = append(probs, Problemf(domain.CodeValidationFailed, id, args...))
	}
	numKey := func(key string, val *float64) {
		if val == nil {
			return
		}
		if *val <= 0 && !p.AllowNegative {
			add(domain.MsgValueKeyPositive, p.Code, key)
		}
		if p.Ceiling != nil && *val > *p.Ceiling {
			add(domain.MsgValueKeyCeiling, p.Code, key, formatNum(*p.Ceiling))
		}
	}
	switch p.ValueType {
	case ValueExact:
		if v.Exact == nil {
			add(domain.MsgValueKeyRequired, p.Code, "exact", "value")
		}
		if v.Min != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "exact", "min")
		}
		if v.Max != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "exact", "max")
		}
		if v.Text != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "exact", "text")
		}
		numKey("value", v.Exact)
	case ValueAtLeast:
		if v.Min == nil {
			add(domain.MsgValueKeyRequired, p.Code, "at_least", "min")
		}
		if v.Exact != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "at_least", "value")
		}
		if v.Text != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "at_least", "text")
		}
		numKey("min", v.Min)
		numKey("max", v.Max)
		if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
			add(domain.MsgValueMinMax, p.Code)
		}
	case ValueAtMost:
		if v.Max == nil {
			add(domain.MsgValueKeyRequired, p.Code, "at_most", "max")
		}
		if v.Min != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "at_most", "min")
		}
		if v.Exact != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "at_most", "value")
		}
		if v.Text != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "at_most", "text")
		}
		numKey("max", v.Max)
	case ValueRange:
		if v.Min == nil || v.Max == nil {
			add(domain.MsgValueKeysPairRequired, p.Code)
		}
		if v.Exact != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "range", "value")
		}
		if v.Text != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "range", "text")
		}
		numKey("min", v.Min)
		numKey("max", v.Max)
		if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
			add(domain.MsgValueMinMax, p.Code)
		}
	case ValueText:
		if v.Text == nil {
			add(domain.MsgValueKeyRequired, p.Code, "text", "text")
		} else if strings.TrimSpace(*v.Text) == "" {
			add(domain.MsgEngineParamTextEmpty, p.Code)
		}
		if v.Exact != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "text", "value")
		}
		if v.Min != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "text", "min")
		}
		if v.Max != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "text", "max")
		}
	case ValueEnum:
		if v.Text == nil {
			add(domain.MsgValueKeyRequired, p.Code, "enum", "text")
		} else if !slices.Contains(p.EnumValues, *v.Text) {
			add(domain.MsgValueEnumInvalid, p.Code, *v.Text, strings.Join(p.EnumValues, ", "))
		}
		if v.Exact != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "enum", "value")
		}
		if v.Min != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "enum", "min")
		}
		if v.Max != nil {
			add(domain.MsgValueKeyForbidden, p.Code, "enum", "max")
		}
	}
	return probs
}

// checkValueConditions — условия значения: известность, позитивность
// (allow_negative у условия), соответствие одному из наборов условий
// параметра (required обязательны, optional допустимы, остальные
// запрещены; fixed_value — константа, которую можно опустить либо задать
// равной — docs/plan/02-database.md §2.2). У безусловного параметра условий
// быть не должно.
func (e *Engine) checkValueConditions(p *ParameterDef, v *ParameterValue) []Problem {
	var probs []Problem
	add := func(id domain.MsgID, args ...any) {
		probs = append(probs, Problemf(domain.CodeValidationFailed, id, args...))
	}
	conds := make(map[string]float64, len(v.Conditions))
	unknown := false
	for _, cv := range v.Conditions {
		if _, dup := conds[cv.Condition]; dup {
			add(domain.MsgCondDuplicate, p.Code, cv.Condition)
			continue
		}
		c, ok := e.snap.Condition(cv.Condition)
		if !ok {
			probs = append(probs, Problemf(domain.CodeUnknownCondition, domain.MsgCondUnknown,
				p.Code, cv.Condition))
			unknown = true
			continue
		}
		if cv.Value <= 0 && !c.AllowNegative {
			add(domain.MsgCondPositive, p.Code, cv.Condition)
		}
		conds[cv.Condition] = cv.Value
	}
	if unknown {
		return probs
	}
	if len(p.ConditionSets) == 0 {
		if len(conds) > 0 {
			add(domain.MsgCondNotAllowed, p.Code)
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
				add(domain.MsgCondFixed, p.Code, it.Condition, formatNum(*it.FixedValue))
				fixedReported = true
			}
		}
	}
	if !fixedReported {
		add(domain.MsgCondSetMismatch, p.Code)
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
// «параметр + одинаковые условия» (docs/plan/02-database.md §2.3).
// Enum-параметры многозначны (ряды номиналов): ключ включает значение.
func valueKey(p *ParameterDef, v *ParameterValue) string {
	parts := make([]string, 0, len(v.Conditions))
	for _, c := range v.Conditions {
		parts = append(parts, c.Condition+"="+strconv.FormatFloat(c.Value, 'g', -1, 64))
	}
	slices.Sort(parts)
	key := v.Parameter + "|" + strings.Join(parts, ";")
	if p.ValueType == ValueEnum && v.Text != nil {
		key += "|" + *v.Text
	}
	return key
}

// formatNum — компактная запись числа для сообщений (целое — без точки).
func formatNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
