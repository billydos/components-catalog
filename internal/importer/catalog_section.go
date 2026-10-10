package importer

import (
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Секция catalog файла наполнения — расширение каталога (метасхема,
// docs/plan/04-module-functionality.md §4). Форма подразделов повторяет
// catalog.Input; целостность определений (ссылки, типы, enum) проверяет
// ApplyCatalog — единственная точка валидации метасхемы; здесь только
// форма: типы ключей и известность подразделов.

// catalogSubsections — подразделы секции catalog (порядок экспорта).
var catalogSubsections = []string{
	"kinds", "designation_systems", "designation_system_kinds",
	"units", "categories", "conditions", "parameter_groups", "parameters",
	"attributes", "validation_rules", "kind_validation_rules",
}

// ReadCatalogSection читает дерево секции catalog в каталог. Значение
// null/отсутствие — пустой вход. Строки NDJSON ({"catalog": {"<подраздел>":
// […]}}) читаются тем же читателем и сливаются mergeCatalogTrees.
func ReadCatalogSection(v value) (catalog.Input, []Issue) {
	r := &catReader{}
	if v.kind == kindNull {
		return catalog.Input{}, nil
	}
	if v.kind != kindObject {
		r.fail(domain.MsgImportCatalogObject, strings.Join(catalogSubsections, ", "))
		return catalog.Input{}, r.issues
	}
	for _, m := range v.members {
		switch m.name {
		case "kinds":
			r.readKinds(m.value)
		case "designation_systems":
			r.readSystems(m.value)
		case "designation_system_kinds":
			r.readSystemKinds(m.value)
		case "units":
			r.readUnits(m.value)
		case "categories":
			r.readCategories(m.value)
		case "conditions":
			r.readConditions(m.value)
		case "parameter_groups":
			r.readGroups(m.value)
		case "parameters":
			r.readParameters(m.value)
		case "attributes":
			r.readAttributes(m.value)
		case "validation_rules":
			r.readRules(m.value)
		case "kind_validation_rules":
			r.readKindRules(m.value)
		default:
			r.fail(domain.MsgImportCatalogSubsection, m.name, strings.Join(catalogSubsections, ", "))
		}
	}
	return r.in, r.issues
}

type catReader struct {
	in     catalog.Input
	issues []Issue
}

func (r *catReader) fail(id domain.MsgID, a ...any) {
	r.issues = append(r.issues, issuef(0, id, a...))
}

// rows — массив объектов подраздела; имя — для сообщений.
func (r *catReader) rows(section string, v value) ([]value, bool) {
	if v.kind == kindNull {
		return nil, true
	}
	if v.kind != kindArray {
		r.fail(domain.MsgImportCatalogRowsArray, section)
		return nil, false
	}
	return v.items, true
}

// checkKeys — известность полей объекта строки подраздела.
func (r *catReader) checkKeys(section string, v value, code string, allowed ...string) bool {
	if v.kind != kindObject {
		r.fail(domain.MsgImportCatalogRowObject, section)
		return false
	}
	known := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		known[k] = true
	}
	for _, m := range v.members {
		if !known[m.name] {
			r.fail(domain.MsgImportCatalogUnknownField, rowName(section, code), m.name, strings.Join(allowed, ", "))
			return false
		}
	}
	return true
}

func rowName(section, code string) any {
	if code == "" {
		return domain.MsgArg(domain.MsgImportCatalogRow, section)
	}
	return domain.MsgArg(domain.MsgImportCatalogRowCode, section, code)
}

// Скалярные поля со строгими типами; обязательность проверяют читатели
// подразделов (или метасхема — для ссылок).

func (r *catReader) str(v value, section, code, key string, required bool) (string, bool) {
	val, has := v.has(key)
	if !has || val.kind == kindNull {
		if required {
			r.fail(domain.MsgImportCatalogFieldRequired, rowName(section, code), key)
			return "", false
		}
		return "", true
	}
	if val.kind != kindString {
		r.fail(domain.MsgImportCatalogFieldString, rowName(section, code), key)
		return "", false
	}
	return strings.TrimSpace(val.str), true
}

func (r *catReader) num(v value, section, code, key string) (*float64, bool) {
	val, has := v.has(key)
	if !has || val.kind == kindNull {
		return nil, true
	}
	if val.kind != kindNumber {
		r.fail(domain.MsgImportCatalogFieldNumber, rowName(section, code), key)
		return nil, false
	}
	f, err := strconv.ParseFloat(val.num, 64)
	if err != nil {
		r.fail(domain.MsgImportCatalogFieldNumber, rowName(section, code), key)
		return nil, false
	}
	return &f, true
}

func (r *catReader) integer(v value, section, code, key string) (int, bool) {
	f, ok := r.num(v, section, code, key)
	if !ok || f == nil {
		return 0, ok
	}
	if *f != float64(int(*f)) {
		r.fail(domain.MsgImportCatalogFieldInteger, rowName(section, code), key)
		return 0, false
	}
	return int(*f), true
}

func (r *catReader) boolean(v value, section, code, key string) (bool, bool) {
	val, has := v.has(key)
	if !has || val.kind == kindNull {
		return false, true
	}
	if val.kind != kindBool {
		r.fail(domain.MsgImportCatalogFieldBool, rowName(section, code), key)
		return false, false
	}
	return val.boolean, true
}

func (r *catReader) strSlice(v value, section, code, key string) ([]string, bool) {
	val, has := v.has(key)
	if !has || val.kind == kindNull {
		return nil, true
	}
	if val.kind != kindArray {
		r.fail(domain.MsgImportCatalogFieldStrings, rowName(section, code), key)
		return nil, false
	}
	out := make([]string, 0, len(val.items))
	for _, item := range val.items {
		if item.kind != kindString {
			r.fail(domain.MsgImportCatalogItemString, rowName(section, code), key)
			return nil, false
		}
		out = append(out, strings.TrimSpace(item.str))
	}
	return out, true
}

func (r *catReader) readKinds(v value) {
	rows, ok := r.rows("kinds", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "kinds", "", "code", false)
		if !r.checkKeys("kinds", row, code, "code") {
			continue
		}
		r.in.Kinds = append(r.in.Kinds, catalog.KindDef{Code: domain.Kind(code)})
	}
}

func (r *catReader) readSystems(v value) {
	rows, ok := r.rows("designation_systems", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "designation_systems", "", "code", false)
		if !r.checkKeys("designation_systems", row, code, "code") {
			continue
		}
		r.in.Systems = append(r.in.Systems, catalog.SystemDef{Code: domain.System(code)})
	}
}

func (r *catReader) readSystemKinds(v value) {
	rows, ok := r.rows("designation_system_kinds", v)
	if !ok {
		return
	}
	for _, row := range rows {
		if !r.checkKeys("designation_system_kinds", row, "", "system", "kind") {
			continue
		}
		sys, ok := r.str(row, "designation_system_kinds", "", "system", true)
		if !ok {
			continue
		}
		kind, ok := r.str(row, "designation_system_kinds", sys, "kind", true)
		if !ok {
			continue
		}
		r.in.SystemKinds = append(r.in.SystemKinds, catalog.SystemKindRef{
			System: domain.System(sys), Kind: domain.Kind(kind),
		})
	}
}

func (r *catReader) readUnits(v value) {
	rows, ok := r.rows("units", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "units", "", "code", false)
		if !r.checkKeys("units", row, code, "code") {
			continue
		}
		r.in.Units = append(r.in.Units, catalog.UnitDef{Code: code})
	}
}

func (r *catReader) readCategories(v value) {
	rows, ok := r.rows("categories", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "categories", "", "code", false)
		if !r.checkKeys("categories", row, code, "code") {
			continue
		}
		r.in.Categories = append(r.in.Categories, catalog.CategoryDef{Code: code})
	}
}

func (r *catReader) readConditions(v value) {
	rows, ok := r.rows("conditions", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "conditions", "", "code", false)
		if !r.checkKeys("conditions", row, code, "code", "unit", "allow_negative") {
			continue
		}
		unit, ok := r.str(row, "conditions", code, "unit", false)
		if !ok {
			continue
		}
		neg, ok := r.boolean(row, "conditions", code, "allow_negative")
		if !ok {
			continue
		}
		r.in.Conditions = append(r.in.Conditions, catalog.ConditionDef{
			Code: code, Unit: unit, AllowNegative: neg,
		})
	}
}

func (r *catReader) readGroups(v value) {
	rows, ok := r.rows("parameter_groups", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "parameter_groups", "", "code", false)
		if !r.checkKeys("parameter_groups", row, code, "code", "section", "sort_order") {
			continue
		}
		section, ok := r.str(row, "parameter_groups", code, "section", true)
		if !ok {
			continue
		}
		sortOrder, ok := r.integer(row, "parameter_groups", code, "sort_order")
		if !ok {
			continue
		}
		r.in.Groups = append(r.in.Groups, catalog.GroupDef{
			Code: code, SectionName: section, SortOrder: sortOrder,
		})
	}
}

func (r *catReader) readParameters(v value) {
	rows, ok := r.rows("parameters", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "parameters", "", "code", false)
		if !r.checkKeys("parameters", row, code,
			"code", "group", "unit", "value_type", "kinds", "enum_values",
			"condition_sets", "ceiling", "allow_negative", "rule", "sort_order", "is_active") {
			continue
		}
		p := catalog.ParameterDef{Code: code}
		group, ok := r.str(row, "parameters", code, "group", true)
		if !ok {
			continue
		}
		p.Group = group
		unit, ok := r.str(row, "parameters", code, "unit", false)
		if !ok {
			continue
		}
		p.Unit = unit
		vt, ok := r.str(row, "parameters", code, "value_type", true)
		if !ok {
			continue
		}
		p.ValueType = catalog.ValueType(vt)
		if kinds, ok := r.strSlice(row, "parameters", code, "kinds"); ok {
			for _, k := range kinds {
				p.Kinds = append(p.Kinds, domain.Kind(k))
			}
		} else {
			continue
		}
		if enum, ok := r.strSlice(row, "parameters", code, "enum_values"); ok {
			p.EnumValues = enum
		} else {
			continue
		}
		if ceiling, ok := r.num(row, "parameters", code, "ceiling"); ok {
			p.Ceiling = ceiling
		} else {
			continue
		}
		if neg, ok := r.boolean(row, "parameters", code, "allow_negative"); ok {
			p.AllowNegative = neg
		} else {
			continue
		}
		rule, ok := r.str(row, "parameters", code, "rule", false)
		if !ok {
			continue
		}
		p.ValidationRule = rule
		if sortOrder, ok := r.integer(row, "parameters", code, "sort_order"); ok {
			p.SortOrder = sortOrder
		} else {
			continue
		}
		// is_active отсутствует либо null — строка активна; деактивация —
		// явным "is_active": false.
		if activeVal, has := row.has("is_active"); has && activeVal.kind != kindNull {
			if activeVal.kind != kindBool {
				r.fail(domain.MsgImportCatalogParamActiveBool, code)
				continue
			}
			p.Active = activeVal.boolean
		} else {
			p.Active = true
		}
		if sets, ok := r.conditionSets(row, code); ok {
			p.ConditionSets = sets
		} else {
			continue
		}
		r.in.Parameters = append(r.in.Parameters, p)
	}
}

func (r *catReader) readAttributes(v value) {
	rows, ok := r.rows("attributes", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "attributes", "", "code", false)
		if !r.checkKeys("attributes", row, code,
			"code", "group", "type", "unit", "kinds", "enum_values",
			"rule", "sort_order", "is_active") {
			continue
		}
		a := catalog.AttributeDef{Code: code}
		group, ok := r.str(row, "attributes", code, "group", false)
		if !ok {
			continue
		}
		a.GroupName = group
		typ, ok := r.str(row, "attributes", code, "type", true)
		if !ok {
			continue
		}
		a.Type = catalog.AttrType(typ)
		unit, ok := r.str(row, "attributes", code, "unit", false)
		if !ok {
			continue
		}
		a.Unit = unit
		if kinds, ok := r.strSlice(row, "attributes", code, "kinds"); ok {
			for _, k := range kinds {
				a.Kinds = append(a.Kinds, domain.Kind(k))
			}
		} else {
			continue
		}
		if enum, ok := r.strSlice(row, "attributes", code, "enum_values"); ok {
			a.EnumValues = enum
		} else {
			continue
		}
		rule, ok := r.str(row, "attributes", code, "rule", false)
		if !ok {
			continue
		}
		a.ValidationRule = rule
		if sortOrder, ok := r.integer(row, "attributes", code, "sort_order"); ok {
			a.SortOrder = sortOrder
		} else {
			continue
		}
		// is_active отсутствует либо null — строка активна; деактивация —
		// явным "is_active": false.
		if activeVal, has := row.has("is_active"); has && activeVal.kind != kindNull {
			if activeVal.kind != kindBool {
				r.fail(domain.MsgImportCatalogAttrActiveBool, code)
				continue
			}
			a.Active = activeVal.boolean
		} else {
			a.Active = true
		}
		r.in.Attributes = append(r.in.Attributes, a)
	}
}

func (r *catReader) readRules(v value) {
	rows, ok := r.rows("validation_rules", v)
	if !ok {
		return
	}
	for _, row := range rows {
		code, _ := r.str(row, "validation_rules", "", "code", false)
		if !r.checkKeys("validation_rules", row, code, "code") {
			continue
		}
		r.in.Rules = append(r.in.Rules, catalog.RuleDef{Code: code})
	}
}

func (r *catReader) readKindRules(v value) {
	rows, ok := r.rows("kind_validation_rules", v)
	if !ok {
		return
	}
	for _, row := range rows {
		if !r.checkKeys("kind_validation_rules", row, "", "kind", "rule") {
			continue
		}
		kind, ok := r.str(row, "kind_validation_rules", "", "kind", true)
		if !ok {
			continue
		}
		rule, ok := r.str(row, "kind_validation_rules", kind, "rule", true)
		if !ok {
			continue
		}
		r.in.KindRules = append(r.in.KindRules, catalog.KindRuleRef{
			Kind: domain.Kind(kind), Rule: rule,
		})
	}
}

// conditionSets читает наборы условий параметра: [{"items": [{"condition",
// "mode", "fixed_value"?}]}]; номер набора — по порядку.
func (r *catReader) conditionSets(v value, code string) ([]catalog.ConditionSet, bool) {
	val, has := v.has("condition_sets")
	if !has || val.kind == kindNull {
		return nil, true
	}
	if val.kind != kindArray {
		r.fail(domain.MsgImportCatalogCondsetsArray, code)
		return nil, false
	}
	var sets []catalog.ConditionSet
	for i, setVal := range val.items {
		set := catalog.ConditionSet{No: i + 1}
		if setVal.kind != kindObject {
			r.fail(domain.MsgImportCatalogCondsetObject, code, i+1)
			return nil, false
		}
		itemsVal, hasItems := setVal.has("items")
		if !hasItems || itemsVal.kind != kindArray {
			r.fail(domain.MsgImportCatalogCondsetItems, code, i+1)
			return nil, false
		}
		for j, itemVal := range itemsVal.items {
			if itemVal.kind != kindObject {
				r.fail(domain.MsgImportCatalogCondsetItemObject, code, i+1, j+1)
				return nil, false
			}
			if !r.checkKeys("parameters", itemVal, code, "condition", "mode", "fixed_value") {
				return nil, false
			}
			cond, ok := r.str(itemVal, "parameters", code, "condition", true)
			if !ok {
				return nil, false
			}
			mode, ok := r.str(itemVal, "parameters", code, "mode", true)
			if !ok {
				return nil, false
			}
			item := catalog.ConditionSetItem{Condition: cond, Mode: catalog.ConditionMode(mode)}
			if f, ok := r.num(itemVal, "parameters", code, "fixed_value"); ok {
				item.FixedValue = f
			} else {
				return nil, false
			}
			set.Items = append(set.Items, item)
		}
		if len(set.Items) == 0 {
			r.fail(domain.MsgImportCatalogCondsetEmpty, code, i+1)
			return nil, false
		}
		sets = append(sets, set)
	}
	return sets, true
}
