package storage

import (
	"context"
	"fmt"

	"github.com/billydos/components-catalog/internal/catalog"
)

// WriteCatalog применяет строки каталога upsert'ом по коду внутри транзакции
// записи (docs/plan/02-database.md §5.4): вставка новой строки либо обновление полей
// существующей; enum-значения, наборы условий и применимость к классам
// замещаются целиком; удаления строк каталога нет — только деактивация
// (is_active = 0) самим входом. Валидацию метасхемы выполняет сервисный слой
// до записи; порядок таблиц учитывает FK.
func (t *Tx) WriteCatalog(ctx context.Context, in catalog.Input) error {
	for _, r := range in.Kinds {
		if err := t.upsert(ctx,
			`UPDATE kinds SET name = @name WHERE code = @code`,
			`INSERT INTO kinds(code, name) VALUES (@code, @name)`,
			map[string]any{"code": string(r.Code), "name": r.Name}); err != nil {
			return err
		}
	}
	for _, r := range in.Systems {
		if err := t.upsert(ctx,
			`UPDATE designation_systems SET name = @name, description = @description WHERE code = @code`,
			`INSERT INTO designation_systems(code, name, description) VALUES (@code, @name, @description)`,
			map[string]any{"code": string(r.Code), "name": r.Name, "description": nilIfEmpty(r.Description)}); err != nil {
			return err
		}
	}
	for _, r := range in.SystemKinds {
		if err := t.insertIfAbsent(ctx,
			`INSERT INTO designation_system_kinds(system_code, kind_code)
SELECT @system, @kind
WHERE NOT EXISTS (
    SELECT 1 FROM designation_system_kinds
    WHERE system_code = @system AND kind_code = @kind)`,
			map[string]any{"system": string(r.System), "kind": string(r.Kind)}); err != nil {
			return err
		}
	}
	for _, r := range in.SeriesFamilies {
		if err := t.upsert(ctx,
			`UPDATE series_families SET name = @name, tail_semantic = @tail
WHERE series = @series AND kind_code = @kind`,
			`INSERT INTO series_families(series, kind_code, name, tail_semantic)
VALUES (@series, @kind, @name, @tail)`,
			map[string]any{
				"series": r.Series, "kind": string(r.Kind),
				"name": nilIfEmpty(r.Name), "tail": nilIfEmpty(r.TailSemantic),
			}); err != nil {
			return err
		}
	}
	for _, r := range in.Units {
		if err := t.upsert(ctx,
			`UPDATE units SET name = @name, symbol = @symbol WHERE code = @code`,
			`INSERT INTO units(code, name, symbol) VALUES (@code, @name, @symbol)`,
			map[string]any{"code": r.Code, "name": r.Name, "symbol": r.Symbol}); err != nil {
			return err
		}
	}
	for _, r := range in.Conditions {
		if err := t.upsert(ctx,
			`UPDATE conditions SET name = @name, unit_code = @unit, allow_negative = @neg
WHERE code = @code`,
			`INSERT INTO conditions(code, name, unit_code, allow_negative)
VALUES (@code, @name, @unit, @neg)`,
			map[string]any{
				"code": r.Code, "name": r.Name,
				"unit": nilIfEmpty(r.Unit), "neg": boolInt(r.AllowNegative),
			}); err != nil {
			return err
		}
	}
	for _, r := range in.Groups {
		if err := t.upsert(ctx,
			`UPDATE parameter_groups SET section_name = @section, display_name = @display, sort_order = @sort
WHERE code = @code`,
			`INSERT INTO parameter_groups(code, section_name, display_name, sort_order)
VALUES (@code, @section, @display, @sort)`,
			map[string]any{
				"code": r.Code, "section": r.SectionName,
				"display": r.DisplayName, "sort": r.SortOrder,
			}); err != nil {
			return err
		}
	}
	for _, r := range in.Rules {
		if err := t.upsert(ctx,
			`UPDATE validation_rules SET description = @description WHERE code = @code`,
			`INSERT INTO validation_rules(code, description) VALUES (@code, @description)`,
			map[string]any{"code": r.Code, "description": r.Description}); err != nil {
			return err
		}
	}
	if err := t.writeParameters(ctx, in.Parameters); err != nil {
		return err
	}
	if err := t.writeAttributes(ctx, in.Attributes); err != nil {
		return err
	}
	for _, r := range in.KindRules {
		if err := t.insertIfAbsent(ctx,
			`INSERT INTO kind_validation_rules(kind_code, validation_rule)
SELECT @kind, @rule
WHERE NOT EXISTS (
    SELECT 1 FROM kind_validation_rules
    WHERE kind_code = @kind AND validation_rule = @rule)`,
			map[string]any{"kind": string(r.Kind), "rule": r.Rule}); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) writeParameters(ctx context.Context, rows []catalog.ParameterDef) error {
	for _, r := range rows {
		if err := t.upsert(ctx, `
UPDATE parameters SET group_code = @group, display_name = @display, unit_code = @unit,
       value_type = @vtype, value_ceiling = @ceiling, allow_negative = @neg,
       validation_rule = @rule, sort_order = @sort, is_active = @active
WHERE code = @code`,
			`INSERT INTO parameters(code, group_code, display_name, unit_code, value_type,
    value_ceiling, allow_negative, validation_rule, sort_order, is_active)
VALUES (@code, @group, @display, @unit, @vtype, @ceiling, @neg, @rule, @sort, @active)`,
			map[string]any{
				"code": r.Code, "group": r.Group, "display": r.DisplayName,
				"unit": nilIfEmpty(r.Unit), "vtype": string(r.ValueType),
				"ceiling": r.Ceiling, "neg": boolInt(r.AllowNegative),
				"rule": nilIfEmpty(r.ValidationRule), "sort": r.SortOrder,
				"active": boolInt(r.Active),
			}); err != nil {
			return err
		}
		// Применимость к классам замещается целиком.
		if _, err := t.exec(ctx,
			`DELETE FROM parameter_kinds WHERE parameter_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		for _, k := range r.Kinds {
			if err := t.insertIfAbsent(ctx,
				`INSERT INTO parameter_kinds(parameter_code, kind_code)
SELECT @code, @kind
WHERE NOT EXISTS (
    SELECT 1 FROM parameter_kinds
    WHERE parameter_code = @code AND kind_code = @kind)`,
				map[string]any{"code": r.Code, "kind": string(k)}); err != nil {
				return err
			}
		}
		// Enum-значения замещаются целиком.
		if _, err := t.exec(ctx,
			`DELETE FROM parameter_enum_values WHERE parameter_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		for _, v := range r.EnumValues {
			if _, err := t.exec(ctx,
				`INSERT INTO parameter_enum_values(parameter_code, value) VALUES (@code, @value)`,
				map[string]any{"code": r.Code, "value": v}); err != nil {
				return err
			}
		}
		// Наборы условий замещаются целиком (элементы — каскадом по FK).
		if _, err := t.exec(ctx,
			`DELETE FROM parameter_condition_sets WHERE parameter_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		for _, set := range r.ConditionSets {
			if _, err := t.exec(ctx,
				`INSERT INTO parameter_condition_sets(parameter_code, set_no) VALUES (@code, @no)`,
				map[string]any{"code": r.Code, "no": set.No}); err != nil {
				return err
			}
			for _, it := range set.Items {
				if _, err := t.exec(ctx, `
INSERT INTO parameter_condition_set_items(parameter_code, set_no, condition_code, mode, fixed_value)
VALUES (@code, @no, @cond, @mode, @fixed)`,
					map[string]any{
						"code": r.Code, "no": set.No, "cond": it.Condition,
						"mode": string(it.Mode), "fixed": it.FixedValue,
					}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (t *Tx) writeAttributes(ctx context.Context, rows []catalog.AttributeDef) error {
	for _, r := range rows {
		if err := t.upsert(ctx, `
UPDATE attributes SET display_name = @display, "group" = @group, value_type = @vtype,
       unit_code = @unit, validation_rule = @rule, sort_order = @sort, is_active = @active
WHERE code = @code`,
			`INSERT INTO attributes(code, display_name, "group", value_type, unit_code,
    validation_rule, sort_order, is_active)
VALUES (@code, @display, @group, @vtype, @unit, @rule, @sort, @active)`,
			map[string]any{
				"code": r.Code, "display": r.DisplayName, "group": nilIfEmpty(r.GroupName),
				"vtype": string(r.Type), "unit": nilIfEmpty(r.Unit),
				"rule": nilIfEmpty(r.ValidationRule), "sort": r.SortOrder,
				"active": boolInt(r.Active),
			}); err != nil {
			return err
		}
		if _, err := t.exec(ctx,
			`DELETE FROM attribute_kinds WHERE attribute_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		for _, k := range r.Kinds {
			if err := t.insertIfAbsent(ctx,
				`INSERT INTO attribute_kinds(attribute_code, kind_code)
SELECT @code, @kind
WHERE NOT EXISTS (
    SELECT 1 FROM attribute_kinds
    WHERE attribute_code = @code AND kind_code = @kind)`,
				map[string]any{"code": r.Code, "kind": string(k)}); err != nil {
				return err
			}
		}
		if _, err := t.exec(ctx,
			`DELETE FROM attribute_enum_values WHERE attribute_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		for _, v := range r.EnumValues {
			if _, err := t.exec(ctx,
				`INSERT INTO attribute_enum_values(attribute_code, value) VALUES (@code, @value)`,
				map[string]any{"code": r.Code, "value": v}); err != nil {
				return err
			}
		}
	}
	return nil
}

// upsert выполняет обновление строки по ключу, а при отсутствии изменения —
// вставку (порядок UPDATE-then-INSERT переносим; конкуренция писателей
// отсутствует по построению: одна точка записи).
func (t *Tx) upsert(ctx context.Context, update, insert string, args map[string]any) error {
	res, err := t.exec(ctx, update, args)
	if err != nil {
		return fmt.Errorf("хранилище: каталог: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = t.exec(ctx, insert, args)
	return err
}

// insertIfAbsent выполняет вставку связи, если её ещё нет.
func (t *Tx) insertIfAbsent(ctx context.Context, query string, args map[string]any) error {
	_, err := t.exec(ctx, query, args)
	return err
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CountKinds возвращает число строк kinds (признак пустого каталога для
// сидирования при EnsureCreated).
func (d *DB) CountKinds(ctx context.Context) (int, error) {
	var n int
	if err := d.queryRow(ctx, `SELECT COUNT(*) FROM kinds`, nil).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
