package storage

import (
	"context"
	"database/sql"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// LoadSnapshot загружает полный снимок каталога одним согласованным чтением
// из пула чтения (транспорт каталога — docs/plan/01-architecture.md §2.2;
// реализация catalog.Store). Порядок строк детерминирован (ORDER BY),
// чтобы снимки одной базы были сравнимы.
func (d *DB) LoadSnapshot(ctx context.Context) (*catalog.Snapshot, error) {
	snap := &catalog.Snapshot{}
	var err error
	if snap.Kinds, err = d.loadKinds(ctx); err != nil {
		return nil, err
	}
	if snap.Systems, err = d.loadSystems(ctx); err != nil {
		return nil, err
	}
	if snap.SystemKinds, err = d.loadSystemKinds(ctx); err != nil {
		return nil, err
	}
	if snap.SeriesFamilies, err = d.loadSeriesFamilies(ctx); err != nil {
		return nil, err
	}
	if snap.Units, err = d.loadUnits(ctx); err != nil {
		return nil, err
	}
	if snap.Conditions, err = d.loadConditions(ctx); err != nil {
		return nil, err
	}
	if snap.Groups, err = d.loadGroups(ctx); err != nil {
		return nil, err
	}
	if snap.Rules, err = d.loadRules(ctx); err != nil {
		return nil, err
	}
	if snap.KindRules, err = d.loadKindRules(ctx); err != nil {
		return nil, err
	}
	if snap.Parameters, err = d.loadParameters(ctx); err != nil {
		return nil, err
	}
	if snap.Attributes, err = d.loadAttributes(ctx); err != nil {
		return nil, err
	}
	return snap, nil
}

// nullableStr — NULL-колонка TEXT в строку (NULL — «не задано» — "").
func nullableStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// nullableFloat — NULL-колонка REAL в *float64 (NULL — nil).
func nullableFloat(v sql.NullFloat64) *float64 {
	if v.Valid {
		f := v.Float64
		return &f
	}
	return nil
}

func (d *DB) loadKinds(ctx context.Context) ([]catalog.KindDef, error) {
	rows, err := d.query(ctx, `SELECT code, name FROM kinds ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.KindDef
	for rows.Next() {
		var r catalog.KindDef
		if err := rows.Scan(&r.Code, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadSystems(ctx context.Context) ([]catalog.SystemDef, error) {
	rows, err := d.query(ctx,
		`SELECT code, name, description FROM designation_systems ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.SystemDef
	for rows.Next() {
		var r catalog.SystemDef
		var desc sql.NullString
		if err := rows.Scan(&r.Code, &r.Name, &desc); err != nil {
			return nil, err
		}
		r.Description = nullableStr(desc)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadSystemKinds(ctx context.Context) ([]catalog.SystemKindRef, error) {
	rows, err := d.query(ctx, `
SELECT system_code, kind_code FROM designation_system_kinds
ORDER BY system_code, kind_code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.SystemKindRef
	for rows.Next() {
		var r catalog.SystemKindRef
		if err := rows.Scan(&r.System, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadSeriesFamilies(ctx context.Context) ([]catalog.SeriesFamilyDef, error) {
	rows, err := d.query(ctx, `
SELECT series, kind_code, name, tail_semantic FROM series_families
ORDER BY series, kind_code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.SeriesFamilyDef
	for rows.Next() {
		var r catalog.SeriesFamilyDef
		var name, tail sql.NullString
		if err := rows.Scan(&r.Series, &r.Kind, &name, &tail); err != nil {
			return nil, err
		}
		r.Name = nullableStr(name)
		r.TailSemantic = nullableStr(tail)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadUnits(ctx context.Context) ([]catalog.UnitDef, error) {
	rows, err := d.query(ctx, `SELECT code, name, symbol FROM units ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.UnitDef
	for rows.Next() {
		var r catalog.UnitDef
		if err := rows.Scan(&r.Code, &r.Name, &r.Symbol); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadConditions(ctx context.Context) ([]catalog.ConditionDef, error) {
	rows, err := d.query(ctx, `
SELECT code, name, unit_code, allow_negative FROM conditions ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.ConditionDef
	for rows.Next() {
		var r catalog.ConditionDef
		var unit sql.NullString
		var allow int
		if err := rows.Scan(&r.Code, &r.Name, &unit, &allow); err != nil {
			return nil, err
		}
		r.Unit = nullableStr(unit)
		r.AllowNegative = allow != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadGroups(ctx context.Context) ([]catalog.GroupDef, error) {
	rows, err := d.query(ctx, `
SELECT code, section_name, display_name, sort_order FROM parameter_groups
ORDER BY sort_order, code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.GroupDef
	for rows.Next() {
		var r catalog.GroupDef
		if err := rows.Scan(&r.Code, &r.SectionName, &r.DisplayName, &r.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadRules(ctx context.Context) ([]catalog.RuleDef, error) {
	rows, err := d.query(ctx,
		`SELECT code, description FROM validation_rules ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.RuleDef
	for rows.Next() {
		var r catalog.RuleDef
		if err := rows.Scan(&r.Code, &r.Description); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadKindRules(ctx context.Context) ([]catalog.KindRuleRef, error) {
	rows, err := d.query(ctx, `
SELECT kind_code, validation_rule FROM kind_validation_rules
ORDER BY kind_code, validation_rule`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.KindRuleRef
	for rows.Next() {
		var r catalog.KindRuleRef
		if err := rows.Scan(&r.Kind, &r.Rule); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) loadParameters(ctx context.Context) ([]catalog.ParameterDef, error) {
	rows, err := d.query(ctx, `
SELECT code, group_code, display_name, unit_code, value_type,
       value_ceiling, allow_negative, validation_rule, sort_order, is_active
FROM parameters ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.ParameterDef
	for rows.Next() {
		var r catalog.ParameterDef
		var unit, rule sql.NullString
		var ceiling sql.NullFloat64
		var allow, active int
		if err := rows.Scan(&r.Code, &r.Group, &r.DisplayName, &unit, &r.ValueType,
			&ceiling, &allow, &rule, &r.SortOrder, &active); err != nil {
			return nil, err
		}
		r.Unit = nullableStr(unit)
		r.Ceiling = nullableFloat(ceiling)
		r.AllowNegative = allow != 0
		r.ValidationRule = nullableStr(rule)
		r.Active = active != 0
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	kinds, err := d.loadParameterKinds(ctx)
	if err != nil {
		return nil, err
	}
	for code, k := range kinds {
		for i := range out {
			if out[i].Code == code {
				out[i].Kinds = append(out[i].Kinds, k...)
			}
		}
	}
	enums, err := d.loadParameterEnums(ctx)
	if err != nil {
		return nil, err
	}
	for code, v := range enums {
		for i := range out {
			if out[i].Code == code {
				out[i].EnumValues = append(out[i].EnumValues, v...)
			}
		}
	}
	sets, err := d.loadConditionSets(ctx)
	if err != nil {
		return nil, err
	}
	for code, ss := range sets {
		for i := range out {
			if out[i].Code == code {
				out[i].ConditionSets = ss
			}
		}
	}
	return out, nil
}

func (d *DB) loadParameterKinds(ctx context.Context) (map[string][]domain.Kind, error) {
	rows, err := d.query(ctx, `
SELECT parameter_code, kind_code FROM parameter_kinds
ORDER BY parameter_code, kind_code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]domain.Kind)
	for rows.Next() {
		var code string
		var kind domain.Kind
		if err := rows.Scan(&code, &kind); err != nil {
			return nil, err
		}
		out[code] = append(out[code], kind)
	}
	return out, rows.Err()
}

func (d *DB) loadParameterEnums(ctx context.Context) (map[string][]string, error) {
	rows, err := d.query(ctx, `
SELECT parameter_code, value FROM parameter_enum_values
ORDER BY parameter_code, value`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var code, value string
		if err := rows.Scan(&code, &value); err != nil {
			return nil, err
		}
		out[code] = append(out[code], value)
	}
	return out, rows.Err()
}

// loadConditionSets загружает наборы условий параметров с элементами;
// возврат — по коду параметра, наборы и элементы упорядочены.
func (d *DB) loadConditionSets(ctx context.Context) (map[string][]catalog.ConditionSet, error) {
	rows, err := d.query(ctx, `
SELECT parameter_code, set_no, condition_code, mode, fixed_value
FROM parameter_condition_set_items
ORDER BY parameter_code, set_no, condition_code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]catalog.ConditionSet)
	opened := make(map[string]map[int]bool)
	for rows.Next() {
		var code, condition, mode string
		var setNo int
		var fixed sql.NullFloat64
		if err := rows.Scan(&code, &setNo, &condition, &mode, &fixed); err != nil {
			return nil, err
		}
		if !opened[code][setNo] {
			if opened[code] == nil {
				opened[code] = make(map[int]bool)
			}
			opened[code][setNo] = true
			out[code] = append(out[code], catalog.ConditionSet{No: setNo})
		}
		item := catalog.ConditionSetItem{Condition: condition, Mode: catalog.ConditionMode(mode)}
		if f := nullableFloat(fixed); f != nil {
			item.FixedValue = f
		}
		sets := out[code]
		sets[len(sets)-1].Items = append(sets[len(sets)-1].Items, item)
	}
	return out, rows.Err()
}

func (d *DB) loadAttributes(ctx context.Context) ([]catalog.AttributeDef, error) {
	rows, err := d.query(ctx, `
SELECT code, display_name, "group", value_type, unit_code,
       validation_rule, sort_order, is_active
FROM attributes ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.AttributeDef
	for rows.Next() {
		var r catalog.AttributeDef
		var group, unit, rule sql.NullString
		var active int
		if err := rows.Scan(&r.Code, &r.DisplayName, &group, &r.Type, &unit,
			&rule, &r.SortOrder, &active); err != nil {
			return nil, err
		}
		r.GroupName = nullableStr(group)
		r.Unit = nullableStr(unit)
		r.ValidationRule = nullableStr(rule)
		r.Active = active != 0
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows2, err := d.query(ctx, `
SELECT attribute_code, kind_code FROM attribute_kinds
ORDER BY attribute_code, kind_code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var code string
		var kind domain.Kind
		if err := rows2.Scan(&code, &kind); err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].Code == code {
				out[i].Kinds = append(out[i].Kinds, kind)
			}
		}
	}
	rows2.Close()
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	rows3, err := d.query(ctx, `
SELECT attribute_code, value FROM attribute_enum_values
ORDER BY attribute_code, value`, nil)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	for rows3.Next() {
		var code, value string
		if err := rows3.Scan(&code, &value); err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].Code == code {
				out[i].EnumValues = append(out[i].EnumValues, value)
			}
		}
	}
	return out, rows3.Err()
}
