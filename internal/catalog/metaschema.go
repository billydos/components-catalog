package catalog

import (
	"github.com/billydos/components-catalog/internal/domain"
)

// ApplyCatalog применяет каталог in поверх снимка base и валидирует
// метасхему результата (план работ 2.4). Применение — upsert по коду
// (docs/plan/02-database.md §5.4): вставка новой строки либо обновление полей
// существующей; enum-значения, наборы условий и применимость к классам
// замещаются целиком; удаление строк каталога входом не поддерживается —
// только деактивация (is_active = 0). При пустом списке Problem результат
// пригоден к записи; непустой — вход отвергается целиком (частичное
// применение запрещено): проблемы самопроверки входа исключают и перенос
// строк в снимок, и проверку метасхемы — вердикт (непустой список проблем)
// от этого не меняется. base = nil означает пустой каталог (сидирование).
func ApplyCatalog(base *Snapshot, in Input) (*Snapshot, []Problem) {
	out := base.Clone()
	probs := validateInputSelf(in)
	if len(probs) > 0 {
		return out, probs
	}
	applyInput(out, in)
	return out, append(probs, validateMetaschema(out)...)
}

// metaProblem — проблема метасхемы: ошибка импорта каталога.
func metaProblem(id domain.MsgID, a ...any) Problem {
	return Problemf(domain.CodeInvalidImportFile, id, a...)
}

// validateInputSelf — внутренняя согласованность входа: непустые и
// уникальные коды в каждом разделе, известность правил, допустимость
// литеральных перечислений (типы значений, режимы условий).
func validateInputSelf(in Input) []Problem {
	var probs []Problem
	probs = append(probs, checkCodes("kinds", in.Kinds, func(r KindDef) string { return string(r.Code) })...)
	probs = append(probs, checkCodes("designation_systems", in.Systems, func(r SystemDef) string { return string(r.Code) })...)
	probs = append(probs, checkPairCodes("designation_system_kinds", in.SystemKinds,
		func(r SystemKindRef) string { return string(r.System) + "+" + string(r.Kind) })...)
	probs = append(probs, checkCodes("units", in.Units, func(r UnitDef) string { return r.Code })...)
	probs = append(probs, checkCodes("categories", in.Categories, func(r CategoryDef) string { return r.Code })...)
	probs = append(probs, checkCodes("conditions", in.Conditions, func(r ConditionDef) string { return r.Code })...)
	probs = append(probs, checkCodes("parameter_groups", in.Groups, func(r GroupDef) string { return r.Code })...)
	probs = append(probs, checkCodes("parameters", in.Parameters, func(r ParameterDef) string { return r.Code })...)
	probs = append(probs, checkCodes("attributes", in.Attributes, func(r AttributeDef) string { return r.Code })...)
	probs = append(probs, checkCodes("validation_rules", in.Rules, func(r RuleDef) string { return r.Code })...)
	probs = append(probs, checkPairCodes("kind_validation_rules", in.KindRules,
		func(r KindRuleRef) string { return string(r.Kind) + "+" + r.Rule })...)

	for _, r := range in.Rules {
		if r.Code == "" {
			continue
		}
		if _, ok := RuleByCode(r.Code); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaRuleUnknownSection, r.Code))
		}
	}
	for i := range in.Parameters {
		p := &in.Parameters[i]
		if p.Code == "" {
			continue
		}
		if !p.ValueType.Valid() {
			probs = append(probs, metaProblem(domain.MsgMetaParamValueType, p.Code, string(p.ValueType)))
		}
		if p.ValidationRule != "" {
			if _, ok := RuleByCode(p.ValidationRule); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaParamRuleUnknown, p.Code, p.ValidationRule))
			}
		}
		for j := range p.ConditionSets {
			for _, it := range p.ConditionSets[j].Items {
				if !it.Mode.Valid() {
					probs = append(probs, metaProblem(domain.MsgMetaCondModeUnknown,
						p.Code, p.ConditionSets[j].No, string(it.Mode)))
				}
			}
		}
	}
	for i := range in.Attributes {
		a := &in.Attributes[i]
		if a.Code == "" {
			continue
		}
		if !a.Type.Valid() {
			probs = append(probs, metaProblem(domain.MsgMetaAttrValueType, a.Code, string(a.Type)))
		}
		if a.ValidationRule != "" {
			if _, ok := RuleByCode(a.ValidationRule); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaAttrRuleUnknown, a.Code, a.ValidationRule))
			}
		}
	}
	return probs
}

func checkCodes[T any](section string, rows []T, code func(T) string) []Problem {
	var probs []Problem
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		c := code(r)
		if c == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, section))
			continue
		}
		if seen[c] {
			probs = append(probs, metaProblem(domain.MsgMetaSectionDupCode, section, c))
		}
		seen[c] = true
	}
	return probs
}

// checkPairCodes — контроль уникальности составных ключей
// (система+класс, семейство+класс, класс+правило).
func checkPairCodes[T any](section string, rows []T, key func(T) string) []Problem {
	return checkCodes(section, rows, key)
}

// kindRuleKey — составной ключ привязки правила к классу
// (kind_validation_rules).
type kindRuleKey struct {
	kind domain.Kind
	rule string
}

// indexRows строит индекс «ключ → позиция строки» (при повторах ключа
// в снимке побеждает первое вхождение — как линейный поиск). Индексы
// применяются к строкам входа вместо линейного поиска на каждую строку:
// применение растёт как O(строк входа + снимок), а не O(строк×снимок).
func indexRows[K comparable, V any](rows []V, key func(V) K) map[K]int {
	idx := make(map[K]int, len(rows))
	for i := range rows {
		if _, ok := idx[key(rows[i])]; !ok {
			idx[key(rows[i])] = i
		}
	}
	return idx
}

// applyInput — перенос строк входа в снимок (upsert по коду; при дубликатах
// в одном входе побеждает последняя строка — вход уже отвергнут проверкой).
// Вызывается только при пустом списке проблем validateInputSelf: дубликатов
// ключей во входе нет, поэтому индексы снимка, построенные до применения,
// эквивалентны поиску по текущему состоянию среза на каждом шаге.
func applyInput(out *Snapshot, in Input) {
	kinds := indexRows(out.Kinds, func(r KindDef) domain.Kind { return r.Code })
	systems := indexRows(out.Systems, func(r SystemDef) domain.System { return r.Code })
	sysKinds := indexRows(out.SystemKinds, func(r SystemKindRef) systemKindKey {
		return systemKindKey{system: r.System, kind: r.Kind}
	})
	units := indexRows(out.Units, func(r UnitDef) string { return r.Code })
	categories := indexRows(out.Categories, func(r CategoryDef) string { return r.Code })
	conditions := indexRows(out.Conditions, func(r ConditionDef) string { return r.Code })
	groups := indexRows(out.Groups, func(r GroupDef) string { return r.Code })
	parameters := indexRows(out.Parameters, func(r ParameterDef) string { return r.Code })
	attributes := indexRows(out.Attributes, func(r AttributeDef) string { return r.Code })
	rules := indexRows(out.Rules, func(r RuleDef) string { return r.Code })
	kindRules := indexRows(out.KindRules, func(r KindRuleRef) kindRuleKey {
		return kindRuleKey{kind: r.Kind, rule: r.Rule}
	})

	for _, r := range in.Kinds {
		if i, ok := kinds[r.Code]; ok {
			out.Kinds[i] = r
		} else {
			out.Kinds = append(out.Kinds, r)
		}
	}
	for _, r := range in.Systems {
		if i, ok := systems[r.Code]; ok {
			out.Systems[i] = r
		} else {
			out.Systems = append(out.Systems, r)
		}
	}
	for _, r := range in.SystemKinds {
		if _, ok := sysKinds[systemKindKey{system: r.System, kind: r.Kind}]; !ok {
			out.SystemKinds = append(out.SystemKinds, r)
		}
	}
	for _, r := range in.Units {
		if i, ok := units[r.Code]; ok {
			out.Units[i] = r
		} else {
			out.Units = append(out.Units, r)
		}
	}
	for _, r := range in.Categories {
		if i, ok := categories[r.Code]; ok {
			out.Categories[i] = r
		} else {
			out.Categories = append(out.Categories, r)
		}
	}
	for _, r := range in.Conditions {
		if i, ok := conditions[r.Code]; ok {
			out.Conditions[i] = r
		} else {
			out.Conditions = append(out.Conditions, r)
		}
	}
	for _, r := range in.Groups {
		if i, ok := groups[r.Code]; ok {
			out.Groups[i] = r
		} else {
			out.Groups = append(out.Groups, r)
		}
	}
	for _, r := range in.Parameters {
		if i, ok := parameters[r.Code]; ok {
			out.Parameters[i] = r
		} else {
			out.Parameters = append(out.Parameters, r)
		}
	}
	for _, r := range in.Attributes {
		if i, ok := attributes[r.Code]; ok {
			out.Attributes[i] = r
		} else {
			out.Attributes = append(out.Attributes, r)
		}
	}
	for _, r := range in.Rules {
		if i, ok := rules[r.Code]; ok {
			out.Rules[i] = r
		} else {
			out.Rules = append(out.Rules, r)
		}
	}
	for _, r := range in.KindRules {
		if _, ok := kindRules[kindRuleKey{kind: r.Kind, rule: r.Rule}]; !ok {
			out.KindRules = append(out.KindRules, r)
		}
	}
}

// validateMetaschema — валидация целостности определений каталога
// (docs/plan/03-data-model.md §11): ссылки (единицы/условия/группы/правила/
// классы) существуют, enum непуст для enum-типа, text/enum без единиц,
// наборы условий непусты и корректны, потолок положителен, порядок
// неотрицателен, правило подходит уровню привязки, коды условий и имена
// секций групп не совпадают с зарезервированными ключами формата
// наполнения.
func validateMetaschema(out *Snapshot) []Problem {
	var probs []Problem
	for i := range out.Kinds {
		r := &out.Kinds[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "kinds"))
		}
	}
	for i := range out.Systems {
		r := &out.Systems[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "designation_systems"))
		}
	}
	for i := range out.SystemKinds {
		r := &out.SystemKinds[i]
		if _, ok := out.System(r.System); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaSystemRefMissing, string(r.System)))
		}
		if _, ok := out.Kind(r.Kind); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaSystemKindMissing, string(r.Kind)))
		}
	}
	for i := range out.Units {
		r := &out.Units[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "units"))
		}
	}
	for i := range out.Categories {
		r := &out.Categories[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "categories"))
		}
	}
	for i := range out.Conditions {
		r := &out.Conditions[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "conditions"))
			continue
		}
		if ValueKeyReserved(r.Code) {
			probs = append(probs, metaProblem(domain.MsgMetaCondCodeReserved, r.Code))
		}
		if r.Unit != "" {
			if _, ok := out.Unit(r.Unit); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaCondUnitMissing, r.Code, r.Unit))
			}
		}
	}
	for i := range out.Groups {
		r := &out.Groups[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "parameter_groups"))
			continue
		}
		if r.SectionName == "" {
			probs = append(probs, metaProblem(domain.MsgMetaGroupNoSection, r.Code))
		}
		if reservedSectionName(out.Kinds, r.SectionName) {
			probs = append(probs, metaProblem(domain.MsgMetaGroupSectionReserved, r.Code, r.SectionName))
		}
		if r.SortOrder < 0 {
			probs = append(probs, metaProblem(domain.MsgMetaGroupSortNegative, r.Code))
		}
		for j := 0; j < i; j++ {
			if out.Groups[j].SectionName == r.SectionName {
				probs = append(probs, metaProblem(domain.MsgMetaGroupSectionDup,
					r.Code, r.SectionName, out.Groups[j].Code))
				break
			}
		}
	}
	for i := range out.Rules {
		r := &out.Rules[i]
		if r.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "validation_rules"))
			continue
		}
		if _, ok := RuleByCode(r.Code); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaRuleUnknown, r.Code))
		}
	}
	for i := range out.KindRules {
		r := &out.KindRules[i]
		if _, ok := out.Kind(r.Kind); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaKindRuleKindMissing, string(r.Kind)))
		}
		reg, known := RuleByCode(r.Rule)
		if !known {
			probs = append(probs, metaProblem(domain.MsgMetaKindRuleUnknown, string(r.Kind), r.Rule))
			continue
		}
		if _, ok := indexOfRuleRow(out.Rules, r.Rule); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaKindRuleRowMissing, string(r.Kind), r.Rule))
		}
		if _, is := reg.(DeviceRule); !is {
			probs = append(probs, metaProblem(domain.MsgMetaKindRuleNotDevice, string(r.Kind), r.Rule))
		}
	}
	for i := range out.Parameters {
		p := &out.Parameters[i]
		if p.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "parameters"))
			continue
		}
		if _, ok := out.Group(p.Group); !ok {
			probs = append(probs, metaProblem(domain.MsgMetaParamGroupMissing, p.Code, p.Group))
		}
		if p.Unit != "" {
			if _, ok := out.Unit(p.Unit); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaParamUnitMissing, p.Code, p.Unit))
			}
		}
		if !p.ValueType.Valid() {
			probs = append(probs, metaProblem(domain.MsgMetaParamValueType, p.Code, string(p.ValueType)))
		} else {
			if (p.ValueType == ValueText || p.ValueType == ValueEnum) && p.Unit != "" {
				probs = append(probs, metaProblem(domain.MsgMetaParamUnitForbidden, p.Code, string(p.ValueType)))
			}
			if p.ValueType == ValueEnum && len(p.EnumValues) == 0 {
				probs = append(probs, metaProblem(domain.MsgMetaParamEnumRequired, p.Code))
			}
			if p.ValueType != ValueEnum && len(p.EnumValues) > 0 {
				probs = append(probs, metaProblem(domain.MsgMetaParamEnumOnly, p.Code))
			}
		}
		if p.Ceiling != nil && *p.Ceiling <= 0 {
			probs = append(probs, metaProblem(domain.MsgMetaParamCeiling, p.Code))
		}
		if p.SortOrder < 0 {
			probs = append(probs, metaProblem(domain.MsgMetaParamSortNegative, p.Code))
		}
		for _, k := range p.Kinds {
			if _, ok := out.Kind(k); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaParamKindMissing, p.Code, string(k)))
			}
		}
		if p.ValidationRule != "" {
			if _, ok := indexOfRuleRow(out.Rules, p.ValidationRule); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaParamRuleRowMissing, p.Code, p.ValidationRule))
			}
			if reg, ok := RuleByCode(p.ValidationRule); ok {
				if _, is := reg.(ParameterRule); !is {
					probs = append(probs, metaProblem(domain.MsgMetaParamRuleWrongLevel, p.Code, p.ValidationRule))
				}
			}
		}
		for j := range p.ConditionSets {
			set := &p.ConditionSets[j]
			if len(set.Items) == 0 {
				probs = append(probs, metaProblem(domain.MsgMetaCondSetEmpty, p.Code, set.No))
				continue
			}
			for _, it := range set.Items {
				if _, ok := out.Condition(it.Condition); !ok {
					probs = append(probs, metaProblem(domain.MsgMetaCondMissing, p.Code, set.No, it.Condition))
				}
				if it.FixedValue != nil && it.Mode != ModeRequired {
					probs = append(probs, metaProblem(domain.MsgMetaCondFixedOptional, p.Code, set.No, it.Condition))
				}
			}
		}
	}
	for i := range out.Attributes {
		a := &out.Attributes[i]
		if a.Code == "" {
			probs = append(probs, metaProblem(domain.MsgMetaSectionNoCode, "attributes"))
			continue
		}
		if a.Unit != "" {
			if _, ok := out.Unit(a.Unit); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaAttrUnitMissing, a.Code, a.Unit))
			}
		}
		if !a.Type.Valid() {
			probs = append(probs, metaProblem(domain.MsgMetaAttrValueType, a.Code, string(a.Type)))
		} else {
			if (a.Type == AttrText || a.Type == AttrEnum) && a.Unit != "" {
				probs = append(probs, metaProblem(domain.MsgMetaAttrUnitForbidden, a.Code, string(a.Type)))
			}
			if a.Type == AttrEnum && len(a.EnumValues) == 0 {
				probs = append(probs, metaProblem(domain.MsgMetaAttrEnumRequired, a.Code))
			}
			if a.Type != AttrEnum && len(a.EnumValues) > 0 {
				probs = append(probs, metaProblem(domain.MsgMetaAttrEnumOnly, a.Code))
			}
		}
		if a.SortOrder < 0 {
			probs = append(probs, metaProblem(domain.MsgMetaAttrSortNegative, a.Code))
		}
		for _, k := range a.Kinds {
			if _, ok := out.Kind(k); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaAttrKindMissing, a.Code, string(k)))
			}
		}
		if a.ValidationRule != "" {
			if _, ok := indexOfRuleRow(out.Rules, a.ValidationRule); !ok {
				probs = append(probs, metaProblem(domain.MsgMetaAttrRuleRowMissing, a.Code, a.ValidationRule))
			}
			if reg, ok := RuleByCode(a.ValidationRule); ok {
				if _, is := reg.(AttributeRule); !is {
					probs = append(probs, metaProblem(domain.MsgMetaAttrRuleWrongLevel, a.Code, a.ValidationRule))
				}
			}
		}
	}
	return probs
}

// indexOfRuleRow сообщает, есть ли строка правила в данных каталога
// (validation_rules — FK привязок параметров/атрибутов/классов).
func indexOfRuleRow(rows []RuleDef, code string) (int, bool) {
	for i := range rows {
		if rows[i].Code == code {
			return i, true
		}
	}
	return -1, false
}
