package storage

import (
	"context"
	"database/sql"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// LoadSnapshot загружает полный снимок каталога одной транзакцией чтения
// из пула чтения (транспорт каталога — docs/plan/01-architecture.md §2.2;
// реализация catalog.Store). Все SELECT и чтение catalog_revision идут из
// одного снапшота СУБД: конкурентный импорт каталога не может смешать в
// снимке старые и новые определения, а Revision соответствует загруженным
// строкам. Уровень REPEATABLE READ переносим: pgx задаёт его явно,
// sqlite-драйвер игнорирует — WAL-транзакция чтения и так держит стабильный
// снапшот на всё время транзакции. Порядок строк детерминирован (ORDER BY),
// чтобы снимки одной базы были сравнимы.
func (d *DB) LoadSnapshot(ctx context.Context) (*catalog.Snapshot, error) {
	tx, err := d.reads.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck — чтение без побочных эффектов

	rev, err := catalogRevision(ctx, tx)
	if err != nil {
		return nil, err
	}
	snap := &catalog.Snapshot{Revision: rev}
	if snap.Kinds, err = loadKinds(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Systems, err = loadSystems(ctx, tx); err != nil {
		return nil, err
	}
	if snap.SystemKinds, err = loadSystemKinds(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Units, err = loadUnits(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Categories, err = loadCategories(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Conditions, err = loadConditions(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Groups, err = loadGroups(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Rules, err = loadRules(ctx, tx); err != nil {
		return nil, err
	}
	if snap.KindRules, err = loadKindRules(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Parameters, err = loadParameters(ctx, tx); err != nil {
		return nil, err
	}
	if snap.Attributes, err = loadAttributeDefs(ctx, tx); err != nil {
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

func loadKinds(ctx context.Context, q queryer) ([]catalog.KindDef, error) {
	rows, err := q.QueryContext(ctx, `SELECT code FROM kinds ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.KindDef
	for rows.Next() {
		var r catalog.KindDef
		if err := rows.Scan(&r.Code); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadSystems(ctx context.Context, q queryer) ([]catalog.SystemDef, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT code FROM designation_systems ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.SystemDef
	for rows.Next() {
		var r catalog.SystemDef
		if err := rows.Scan(&r.Code); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadSystemKinds(ctx context.Context, q queryer) ([]catalog.SystemKindRef, error) {
	rows, err := q.QueryContext(ctx, `
SELECT system_code, kind_code FROM designation_system_kinds
ORDER BY system_code, kind_code`)
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

func loadUnits(ctx context.Context, q queryer) ([]catalog.UnitDef, error) {
	rows, err := q.QueryContext(ctx, `SELECT code FROM units ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.UnitDef
	for rows.Next() {
		var r catalog.UnitDef
		if err := rows.Scan(&r.Code); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadCategories(ctx context.Context, q queryer) ([]catalog.CategoryDef, error) {
	rows, err := q.QueryContext(ctx, `SELECT code FROM categories ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.CategoryDef
	for rows.Next() {
		var r catalog.CategoryDef
		if err := rows.Scan(&r.Code); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadConditions(ctx context.Context, q queryer) ([]catalog.ConditionDef, error) {
	rows, err := q.QueryContext(ctx, `
SELECT code, unit_code, allow_negative FROM conditions ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.ConditionDef
	for rows.Next() {
		var r catalog.ConditionDef
		var unit sql.NullString
		var allow int
		if err := rows.Scan(&r.Code, &unit, &allow); err != nil {
			return nil, err
		}
		r.Unit = nullableStr(unit)
		r.AllowNegative = allow != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadGroups(ctx context.Context, q queryer) ([]catalog.GroupDef, error) {
	rows, err := q.QueryContext(ctx, `
SELECT code, section_name, sort_order FROM parameter_groups
ORDER BY sort_order, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.GroupDef
	for rows.Next() {
		var r catalog.GroupDef
		if err := rows.Scan(&r.Code, &r.SectionName, &r.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadRules(ctx context.Context, q queryer) ([]catalog.RuleDef, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT code FROM validation_rules ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.RuleDef
	for rows.Next() {
		var r catalog.RuleDef
		if err := rows.Scan(&r.Code); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadKindRules(ctx context.Context, q queryer) ([]catalog.KindRuleRef, error) {
	rows, err := q.QueryContext(ctx, `
SELECT kind_code, validation_rule FROM kind_validation_rules
ORDER BY kind_code, validation_rule`)
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

func loadParameters(ctx context.Context, q queryer) ([]catalog.ParameterDef, error) {
	rows, err := q.QueryContext(ctx, `
SELECT code, group_code, unit_code, value_type,
       value_ceiling, allow_negative, validation_rule, sort_order, is_active
FROM parameters ORDER BY code`)
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
		if err := rows.Scan(&r.Code, &r.Group, &unit, &r.ValueType,
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

	// Индекс кодов параметров: применимость, enum и наборы условий
	// сливаются со списком одним поиском на строку.
	idx := make(map[string]int, len(out))
	for i := range out {
		idx[out[i].Code] = i
	}
	kinds, err := loadParameterKinds(ctx, q)
	if err != nil {
		return nil, err
	}
	for code, k := range kinds {
		if i, ok := idx[code]; ok {
			out[i].Kinds = append(out[i].Kinds, k...)
		}
	}
	enums, err := loadParameterEnums(ctx, q)
	if err != nil {
		return nil, err
	}
	for code, v := range enums {
		if i, ok := idx[code]; ok {
			out[i].EnumValues = append(out[i].EnumValues, v...)
		}
	}
	sets, err := loadConditionSets(ctx, q)
	if err != nil {
		return nil, err
	}
	for code, ss := range sets {
		if i, ok := idx[code]; ok {
			out[i].ConditionSets = ss
		}
	}
	return out, nil
}

func loadParameterKinds(ctx context.Context, q queryer) (map[string][]domain.Kind, error) {
	rows, err := q.QueryContext(ctx, `
SELECT parameter_code, kind_code FROM parameter_kinds
ORDER BY parameter_code, kind_code`)
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

func loadParameterEnums(ctx context.Context, q queryer) (map[string][]string, error) {
	rows, err := q.QueryContext(ctx, `
SELECT parameter_code, value FROM parameter_enum_values
ORDER BY parameter_code, value`)
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
func loadConditionSets(ctx context.Context, q queryer) (map[string][]catalog.ConditionSet, error) {
	rows, err := q.QueryContext(ctx, `
SELECT parameter_code, set_no, condition_code, mode, fixed_value
FROM parameter_condition_set_items
ORDER BY parameter_code, set_no, condition_code`)
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

func loadAttributeDefs(ctx context.Context, q queryer) ([]catalog.AttributeDef, error) {
	rows, err := q.QueryContext(ctx, `
SELECT code, "group", value_type, unit_code,
       validation_rule, sort_order, is_active
FROM attributes ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.AttributeDef
	for rows.Next() {
		var r catalog.AttributeDef
		var group, unit, rule sql.NullString
		var active int
		if err := rows.Scan(&r.Code, &group, &r.Type, &unit,
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

	// Индекс кодов атрибутов: применимость и enum сливаются со списком
	// одним поиском на строку.
	idx := make(map[string]int, len(out))
	for i := range out {
		idx[out[i].Code] = i
	}

	rows2, err := q.QueryContext(ctx, `
SELECT attribute_code, kind_code FROM attribute_kinds
ORDER BY attribute_code, kind_code`)
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
		if i, ok := idx[code]; ok {
			out[i].Kinds = append(out[i].Kinds, kind)
		}
	}
	rows2.Close()
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	rows3, err := q.QueryContext(ctx, `
SELECT attribute_code, value FROM attribute_enum_values
ORDER BY attribute_code, value`)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	for rows3.Next() {
		var code, value string
		if err := rows3.Scan(&code, &value); err != nil {
			return nil, err
		}
		if i, ok := idx[code]; ok {
			out[i].EnumValues = append(out[i].EnumValues, value)
		}
	}
	return out, rows3.Err()
}
