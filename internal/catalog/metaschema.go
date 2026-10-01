package catalog

import (
	"fmt"

	"github.com/billydos/components-catalog/internal/domain"
)

// ApplyCatalog применяет каталог in поверх снимка base и валидирует
// метасхему результата (план работ 2.4). Применение — upsert по коду
// (plan/02-database.md §5.4): вставка новой строки либо обновление полей
// существующей; enum-значения, наборы условий и применимость к классам
// замещаются целиком; удаление строк каталога входом не поддерживается —
// только деактивация (is_active = 0). При пустом списке Problem результат
// пригоден к записи; непустой — вход отвергается целиком (частичное
// применение запрещено). base = nil означает пустой каталог (сидирование).
func ApplyCatalog(base *Snapshot, in Input) (*Snapshot, []Problem) {
	out := base.Clone()
	probs := validateInputSelf(in)
	applyInput(out, in)
	probs = append(probs, validateMetaschema(out)...)
	return out, probs
}

// metaProblem — проблема метасхемы: ошибка импорта каталога.
func metaProblem(format string, a ...any) Problem {
	return Problem{Code: domain.CodeInvalidImportFile, Message: fmt.Sprintf(format, a...)}
}

// validateInputSelf — внутренняя согласованность входа: непустые и
// уникальные коды в каждом разделе, известность правил, допустимость
// литеральных перечислений (типы значений, режимы условий, семантика
// хвоста семейств).
func validateInputSelf(in Input) []Problem {
	var probs []Problem
	probs = append(probs, checkCodes("kinds", in.Kinds, func(r KindDef) string { return string(r.Code) })...)
	probs = append(probs, checkCodes("designation_systems", in.Systems, func(r SystemDef) string { return string(r.Code) })...)
	probs = append(probs, checkPairCodes("designation_system_kinds", in.SystemKinds,
		func(r SystemKindRef) string { return string(r.System) + "+" + string(r.Kind) })...)
	probs = append(probs, checkPairCodes("series_families", in.SeriesFamilies,
		func(r SeriesFamilyDef) string { return r.Series + "+" + string(r.Kind) })...)
	probs = append(probs, checkCodes("units", in.Units, func(r UnitDef) string { return r.Code })...)
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
			probs = append(probs, metaProblem("каталог: раздел validation_rules: неизвестное правило «%s»", r.Code))
		}
	}
	for i := range in.Parameters {
		p := &in.Parameters[i]
		if p.Code == "" {
			continue
		}
		if !p.ValueType.Valid() {
			probs = append(probs, metaProblem("каталог: параметр «%s»: неизвестный тип значения «%s»", p.Code, string(p.ValueType)))
		}
		if p.ValidationRule != "" {
			if _, ok := RuleByCode(p.ValidationRule); !ok {
				probs = append(probs, metaProblem("каталог: параметр «%s»: правило «%s» неизвестно", p.Code, p.ValidationRule))
			}
		}
		for j := range p.ConditionSets {
			for _, it := range p.ConditionSets[j].Items {
				if !it.Mode.Valid() {
					probs = append(probs, metaProblem(
						"каталог: параметр «%s»: набор условий %d: неизвестный режим условия «%s»",
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
			probs = append(probs, metaProblem("каталог: атрибут «%s»: неизвестный тип значения «%s»", a.Code, string(a.Type)))
		}
		if a.ValidationRule != "" {
			if _, ok := RuleByCode(a.ValidationRule); !ok {
				probs = append(probs, metaProblem("каталог: атрибут «%s»: правило «%s» неизвестно", a.Code, a.ValidationRule))
			}
		}
	}
	for i := range in.SeriesFamilies {
		f := &in.SeriesFamilies[i]
		if f.Series == "" {
			continue
		}
		if f.TailSemantic != "" && f.TailSemantic != TailSemanticPower {
			probs = append(probs, metaProblem(
				"каталог: семейство «%s» (класс %s): неизвестная семантика хвоста «%s»",
				f.Series, string(f.Kind), f.TailSemantic))
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
			probs = append(probs, metaProblem("каталог: раздел %s: не задан код", section))
			continue
		}
		if seen[c] {
			probs = append(probs, metaProblem("каталог: раздел %s: дубликат кода «%s»", section, c))
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

// applyInput — перенос строк входа в снимок (upsert по коду; при дубликатах
// в одном входе побеждает последняя строка — вход уже отвергнут проверкой).
func applyInput(out *Snapshot, in Input) {
	for _, r := range in.Kinds {
		if i := indexOfKind(out.Kinds, r.Code); i >= 0 {
			out.Kinds[i] = r
		} else {
			out.Kinds = append(out.Kinds, r)
		}
	}
	for _, r := range in.Systems {
		if i := indexOfSystem(out.Systems, r.Code); i >= 0 {
			out.Systems[i] = r
		} else {
			out.Systems = append(out.Systems, r)
		}
	}
	for _, r := range in.SystemKinds {
		if !containsSystemKind(out.SystemKinds, r) {
			out.SystemKinds = append(out.SystemKinds, r)
		}
	}
	for _, r := range in.SeriesFamilies {
		if i := indexOfFamily(out.SeriesFamilies, r.Series, r.Kind); i >= 0 {
			out.SeriesFamilies[i] = r
		} else {
			out.SeriesFamilies = append(out.SeriesFamilies, r)
		}
	}
	for _, r := range in.Units {
		if i := indexOfUnit(out.Units, r.Code); i >= 0 {
			out.Units[i] = r
		} else {
			out.Units = append(out.Units, r)
		}
	}
	for _, r := range in.Conditions {
		if i := indexOfCondition(out.Conditions, r.Code); i >= 0 {
			out.Conditions[i] = r
		} else {
			out.Conditions = append(out.Conditions, r)
		}
	}
	for _, r := range in.Groups {
		if i := indexOfGroup(out.Groups, r.Code); i >= 0 {
			out.Groups[i] = r
		} else {
			out.Groups = append(out.Groups, r)
		}
	}
	for _, r := range in.Parameters {
		if i := indexOfParameter(out.Parameters, r.Code); i >= 0 {
			out.Parameters[i] = r
		} else {
			out.Parameters = append(out.Parameters, r)
		}
	}
	for _, r := range in.Attributes {
		if i := indexOfAttribute(out.Attributes, r.Code); i >= 0 {
			out.Attributes[i] = r
		} else {
			out.Attributes = append(out.Attributes, r)
		}
	}
	for _, r := range in.Rules {
		if i := indexOfRule(out.Rules, r.Code); i >= 0 {
			out.Rules[i] = r
		} else {
			out.Rules = append(out.Rules, r)
		}
	}
	for _, r := range in.KindRules {
		if !containsKindRule(out.KindRules, r) {
			out.KindRules = append(out.KindRules, r)
		}
	}
}

func indexOfKind(rows []KindDef, code domain.Kind) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func indexOfSystem(rows []SystemDef, code domain.System) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func containsSystemKind(rows []SystemKindRef, r SystemKindRef) bool {
	for i := range rows {
		if rows[i] == r {
			return true
		}
	}
	return false
}

func indexOfFamily(rows []SeriesFamilyDef, series string, kind domain.Kind) int {
	for i := range rows {
		if rows[i].Series == series && rows[i].Kind == kind {
			return i
		}
	}
	return -1
}

func indexOfUnit(rows []UnitDef, code string) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func indexOfCondition(rows []ConditionDef, code string) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func indexOfGroup(rows []GroupDef, code string) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func indexOfParameter(rows []ParameterDef, code string) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func indexOfAttribute(rows []AttributeDef, code string) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func indexOfRule(rows []RuleDef, code string) int {
	for i := range rows {
		if rows[i].Code == code {
			return i
		}
	}
	return -1
}

func containsKindRule(rows []KindRuleRef, r KindRuleRef) bool {
	for i := range rows {
		if rows[i] == r {
			return true
		}
	}
	return false
}

// validateMetaschema — валидация целостности определений каталога
// (plan/03-data-model.md §11): ссылки (единицы/условия/группы/правила/
// классы) существуют, enum непуст для enum-типа, text/enum без единиц,
// наборы условий непусты и корректны, потолок положителен, порядок
// неотрицателен, правило подходит уровню привязки, семейства series
// не разбираются строгими системами.
func validateMetaschema(out *Snapshot) []Problem {
	var probs []Problem
	for i := range out.Kinds {
		r := &out.Kinds[i]
		if r.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел kinds: не задан код"))
		} else if r.Name == "" {
			probs = append(probs, metaProblem("каталог: класс «%s»: не задано название", string(r.Code)))
		}
	}
	for i := range out.Systems {
		r := &out.Systems[i]
		if r.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел designation_systems: не задан код"))
		} else if r.Name == "" {
			probs = append(probs, metaProblem("каталог: система «%s»: не задано название", string(r.Code)))
		}
	}
	for i := range out.SystemKinds {
		r := &out.SystemKinds[i]
		if _, ok := out.System(r.System); !ok {
			probs = append(probs, metaProblem(
				"каталог: применимость систем: система «%s» не существует", string(r.System)))
		}
		if _, ok := out.Kind(r.Kind); !ok {
			probs = append(probs, metaProblem(
				"каталог: применимость систем: класс «%s» не существует", string(r.Kind)))
		}
	}
	for i := range out.SeriesFamilies {
		f := &out.SeriesFamilies[i]
		if f.Series == "" {
			probs = append(probs, metaProblem("каталог: раздел series_families: не задано семейство"))
			continue
		}
		if _, ok := out.Kind(f.Kind); !ok {
			probs = append(probs, metaProblem(
				"каталог: семейство «%s»: класс «%s» не существует", f.Series, string(f.Kind)))
		}
		if f.TailSemantic != "" && f.TailSemantic != TailSemanticPower {
			probs = append(probs, metaProblem(
				"каталог: семейство «%s» (класс %s): неизвестная семантика хвоста «%s»",
				f.Series, string(f.Kind), f.TailSemantic))
		}
		if sys, strict := seriesParsedStrictly(f.Series); strict {
			probs = append(probs, metaProblem(
				"каталог: семейство «%s»: код разбирается строгой системой «%s» — нарушен инвариант реестра series",
				f.Series, string(sys)))
		}
	}
	for i := range out.Units {
		r := &out.Units[i]
		if r.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел units: не задан код"))
		} else if r.Name == "" || r.Symbol == "" {
			probs = append(probs, metaProblem("каталог: единица «%s»: не заданы название и символ", r.Code))
		}
	}
	for i := range out.Conditions {
		r := &out.Conditions[i]
		if r.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел conditions: не задан код"))
			continue
		}
		if r.Name == "" {
			probs = append(probs, metaProblem("каталог: условие «%s»: не задано название", r.Code))
		}
		if r.Unit != "" {
			if _, ok := out.Unit(r.Unit); !ok {
				probs = append(probs, metaProblem("каталог: условие «%s»: единица «%s» не существует", r.Code, r.Unit))
			}
		}
	}
	for i := range out.Groups {
		r := &out.Groups[i]
		if r.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел parameter_groups: не задан код"))
			continue
		}
		if r.SectionName == "" || r.DisplayName == "" {
			probs = append(probs, metaProblem(
				"каталог: группа «%s»: не заданы имя секции и отображаемое название", r.Code))
		}
		if r.SortOrder < 0 {
			probs = append(probs, metaProblem("каталог: группа «%s»: порядок сортировки должен быть неотрицательным", r.Code))
		}
		for j := 0; j < i; j++ {
			if out.Groups[j].SectionName == r.SectionName {
				probs = append(probs, metaProblem(
					"каталог: группа «%s»: имя секции «%s» уже используется группой «%s»",
					r.Code, r.SectionName, out.Groups[j].Code))
				break
			}
		}
	}
	for i := range out.Rules {
		r := &out.Rules[i]
		if r.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел validation_rules: не задан код"))
			continue
		}
		reg, ok := RuleByCode(r.Code)
		if !ok {
			probs = append(probs, metaProblem("каталог: правило «%s» неизвестно", r.Code))
		} else if reg.Description() != r.Description {
			probs = append(probs, metaProblem(
				"каталог: правило «%s»: описание расходится с реализацией (реестр: «%s»)",
				r.Code, reg.Description()))
		}
	}
	for i := range out.KindRules {
		r := &out.KindRules[i]
		if _, ok := out.Kind(r.Kind); !ok {
			probs = append(probs, metaProblem(
				"каталог: правило класса записей: класс «%s» не существует", string(r.Kind)))
		}
		reg, known := RuleByCode(r.Rule)
		if !known {
			probs = append(probs, metaProblem(
				"каталог: правило класса записей (%s): правило «%s» не существует", string(r.Kind), r.Rule))
			continue
		}
		if _, ok := indexOfRuleRow(out.Rules, r.Rule); !ok {
			probs = append(probs, metaProblem(
				"каталог: правило класса записей (%s): строка правила «%s» отсутствует в разделе validation_rules",
				string(r.Kind), r.Rule))
		}
		if _, is := reg.(DeviceRule); !is {
			probs = append(probs, metaProblem(
				"каталог: правило класса записей (%s): правило «%s» не применяется к записям класса",
				string(r.Kind), r.Rule))
		}
	}
	for i := range out.Parameters {
		p := &out.Parameters[i]
		if p.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел parameters: не задан код"))
			continue
		}
		if p.DisplayName == "" {
			probs = append(probs, metaProblem("каталог: параметр «%s»: не задано название", p.Code))
		}
		if _, ok := out.Group(p.Group); !ok {
			probs = append(probs, metaProblem("каталог: параметр «%s»: группа «%s» не существует", p.Code, p.Group))
		}
		if p.Unit != "" {
			if _, ok := out.Unit(p.Unit); !ok {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s»: единица «%s» не существует", p.Code, p.Unit))
			}
		}
		if !p.ValueType.Valid() {
			probs = append(probs, metaProblem(
				"каталог: параметр «%s»: неизвестный тип значения «%s»", p.Code, string(p.ValueType)))
		} else {
			if (p.ValueType == ValueText || p.ValueType == ValueEnum) && p.Unit != "" {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s» типа %s не должен иметь единицу измерения", p.Code, string(p.ValueType)))
			}
			if p.ValueType == ValueEnum && len(p.EnumValues) == 0 {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s»: тип enum требует непустой список значений", p.Code))
			}
			if p.ValueType != ValueEnum && len(p.EnumValues) > 0 {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s»: enum-значения допустимы только для типа enum", p.Code))
			}
		}
		if p.Ceiling != nil && *p.Ceiling <= 0 {
			probs = append(probs, metaProblem("каталог: параметр «%s»: потолок должен быть положительным", p.Code))
		}
		if p.SortOrder < 0 {
			probs = append(probs, metaProblem(
				"каталог: параметр «%s»: порядок сортировки должен быть неотрицательным", p.Code))
		}
		for _, k := range p.Kinds {
			if _, ok := out.Kind(k); !ok {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s»: класс применимости «%s» не существует", p.Code, string(k)))
			}
		}
		if p.ValidationRule != "" {
			if _, ok := indexOfRuleRow(out.Rules, p.ValidationRule); !ok {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s»: строка правила «%s» отсутствует в разделе validation_rules",
					p.Code, p.ValidationRule))
			}
			if reg, ok := RuleByCode(p.ValidationRule); ok {
				if _, is := reg.(ParameterRule); !is {
					probs = append(probs, metaProblem(
						"каталог: параметр «%s»: правило «%s» не применяется к параметрам", p.Code, p.ValidationRule))
				}
			}
		}
		for j := range p.ConditionSets {
			set := &p.ConditionSets[j]
			if len(set.Items) == 0 {
				probs = append(probs, metaProblem(
					"каталог: параметр «%s»: набор условий %d пуст", p.Code, set.No))
				continue
			}
			for _, it := range set.Items {
				if _, ok := out.Condition(it.Condition); !ok {
					probs = append(probs, metaProblem(
						"каталог: параметр «%s»: набор условий %d: условие «%s» не существует",
						p.Code, set.No, it.Condition))
				}
				if it.FixedValue != nil && it.Mode != ModeRequired {
					probs = append(probs, metaProblem(
						"каталог: параметр «%s»: набор условий %d, условие «%s»: fixed_value допустим только у обязательного условия",
						p.Code, set.No, it.Condition))
				}
			}
		}
	}
	for i := range out.Attributes {
		a := &out.Attributes[i]
		if a.Code == "" {
			probs = append(probs, metaProblem("каталог: раздел attributes: не задан код"))
			continue
		}
		if a.DisplayName == "" {
			probs = append(probs, metaProblem("каталог: атрибут «%s»: не задано название", a.Code))
		}
		if a.Unit != "" {
			if _, ok := out.Unit(a.Unit); !ok {
				probs = append(probs, metaProblem(
					"каталог: атрибут «%s»: единица «%s» не существует", a.Code, a.Unit))
			}
		}
		if !a.Type.Valid() {
			probs = append(probs, metaProblem(
				"каталог: атрибут «%s»: неизвестный тип значения «%s»", a.Code, string(a.Type)))
		} else {
			if (a.Type == AttrText || a.Type == AttrEnum) && a.Unit != "" {
				probs = append(probs, metaProblem(
					"каталог: атрибут «%s» типа %s не должен иметь единицу измерения", a.Code, string(a.Type)))
			}
			if a.Type == AttrEnum && len(a.EnumValues) == 0 {
				probs = append(probs, metaProblem(
					"каталог: атрибут «%s»: тип enum требует непустой список значений", a.Code))
			}
			if a.Type != AttrEnum && len(a.EnumValues) > 0 {
				probs = append(probs, metaProblem(
					"каталог: атрибут «%s»: enum-значения допустимы только для типа enum", a.Code))
			}
		}
		if a.SortOrder < 0 {
			probs = append(probs, metaProblem(
				"каталог: атрибут «%s»: порядок сортировки должен быть неотрицательным", a.Code))
		}
		for _, k := range a.Kinds {
			if _, ok := out.Kind(k); !ok {
				probs = append(probs, metaProblem(
					"каталог: атрибут «%s»: класс применимости «%s» не существует", a.Code, string(k)))
			}
		}
		if a.ValidationRule != "" {
			if _, ok := indexOfRuleRow(out.Rules, a.ValidationRule); !ok {
				probs = append(probs, metaProblem(
					"каталог: атрибут «%s»: строка правила «%s» отсутствует в разделе validation_rules",
					a.Code, a.ValidationRule))
			}
			if reg, ok := RuleByCode(a.ValidationRule); ok {
				if _, is := reg.(AttributeRule); !is {
					probs = append(probs, metaProblem(
						"каталог: атрибут «%s»: правило «%s» не применяется к атрибутам", a.Code, a.ValidationRule))
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

// seriesParsedStrictly — проверка инварианта реестра series для кода
// семейства (plan/03-data-model.md §2.4): код не должен разбираться
// строгими системами. Проверка по самому коду — огрубление: полная
// проверка инварианта реестра — предметные тесты примеров обозначений
// (internal/domain/series_test.go).
func seriesParsedStrictly(series string) (domain.System, bool) {
	for _, sys := range strictSystems {
		if _, err := domain.ParseDesignationForSystem(series, sys, ""); err == nil {
			return sys, true
		}
	}
	return "", false
}
