package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// WriteCatalog применяет строки каталога upsert'ом по коду внутри транзакции
// записи (docs/plan/02-database.md §5.4): вставка новой строки либо обновление полей
// существующей; enum-значения, наборы условий и применимость к классам
// замещаются целиком; удаления строк каталога нет — только деактивация
// (is_active = 0) самим входом. Валидацию метасхемы выполняет сервисный слой
// до записи; порядок таблиц учитывает FK.
func (t *Tx) WriteCatalog(ctx context.Context, in catalog.Input) error {
	if len(in.Kinds) > 0 {
		rows := make([][]any, len(in.Kinds))
		for i, r := range in.Kinds {
			rows[i] = []any{string(r.Code)}
		}
		if err := t.insertMulti(ctx, "kinds", "code", rows); err != nil {
			return err
		}
	}
	if len(in.Systems) > 0 {
		rows := make([][]any, len(in.Systems))
		for i, r := range in.Systems {
			rows[i] = []any{string(r.Code)}
		}
		if err := t.insertMulti(ctx, "designation_systems", "code", rows); err != nil {
			return err
		}
	}
	if len(in.SystemKinds) > 0 {
		rows := make([][]any, len(in.SystemKinds))
		for i, r := range in.SystemKinds {
			rows[i] = []any{string(r.System), string(r.Kind)}
		}
		if err := t.insertMulti(ctx, "designation_system_kinds", "system_code, kind_code", rows); err != nil {
			return err
		}
	}
	if len(in.Units) > 0 {
		rows := make([][]any, len(in.Units))
		for i, r := range in.Units {
			rows[i] = []any{r.Code}
		}
		if err := t.insertMulti(ctx, "units", "code", rows); err != nil {
			return err
		}
	}
	if len(in.Categories) > 0 {
		rows := make([][]any, len(in.Categories))
		for i, r := range in.Categories {
			rows[i] = []any{r.Code}
		}
		if err := t.insertMulti(ctx, "categories", "code", rows); err != nil {
			return err
		}
	}
	for _, r := range in.Conditions {
		if err := t.upsert(ctx,
			`UPDATE conditions SET unit_code = @unit, allow_negative = @neg
WHERE code = @code`,
			`INSERT INTO conditions(code, unit_code, allow_negative)
VALUES (@code, @unit, @neg)`,
			map[string]any{
				"code": r.Code,
				"unit": nilIfEmpty(r.Unit), "neg": boolInt(r.AllowNegative),
			}); err != nil {
			return err
		}
	}
	for _, r := range in.Groups {
		if err := t.upsert(ctx,
			`UPDATE parameter_groups SET section_name = @section, sort_order = @sort
WHERE code = @code`,
			`INSERT INTO parameter_groups(code, section_name, sort_order)
VALUES (@code, @section, @sort)`,
			map[string]any{
				"code": r.Code, "section": r.SectionName, "sort": r.SortOrder,
			}); err != nil {
			return err
		}
	}
	if len(in.Rules) > 0 {
		rows := make([][]any, len(in.Rules))
		for i, r := range in.Rules {
			rows[i] = []any{r.Code}
		}
		if err := t.insertMulti(ctx, "validation_rules", "code", rows); err != nil {
			return err
		}
	}
	if err := t.writeParameters(ctx, in.Parameters); err != nil {
		return err
	}
	if err := t.writeAttributes(ctx, in.Attributes); err != nil {
		return err
	}
	if len(in.KindRules) > 0 {
		rows := make([][]any, len(in.KindRules))
		for i, r := range in.KindRules {
			rows[i] = []any{string(r.Kind), r.Rule}
		}
		if err := t.insertMulti(ctx, "kind_validation_rules", "kind_code, validation_rule", rows); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) writeParameters(ctx context.Context, rows []catalog.ParameterDef) error {
	for _, r := range rows {
		if err := t.upsert(ctx, `
UPDATE parameters SET group_code = @group, unit_code = @unit,
       value_type = @vtype, value_ceiling = @ceiling, allow_negative = @neg,
       validation_rule = @rule, sort_order = @sort, is_active = @active
WHERE code = @code`,
			`INSERT INTO parameters(code, group_code, unit_code, value_type,
    value_ceiling, allow_negative, validation_rule, sort_order, is_active)
VALUES (@code, @group, @unit, @vtype, @ceiling, @neg, @rule, @sort, @active)`,
			map[string]any{
				"code": r.Code, "group": r.Group,
				"unit": nilIfEmpty(r.Unit), "vtype": string(r.ValueType),
				"ceiling": r.Ceiling, "neg": boolInt(r.AllowNegative),
				"rule": nilIfEmpty(r.ValidationRule), "sort": r.SortOrder,
				"active": boolInt(r.Active),
			}); err != nil {
			return err
		}
		// Применимость к классам замещается целиком (батч-вставкой).
		if _, err := t.exec(ctx,
			`DELETE FROM parameter_kinds WHERE parameter_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		if len(r.Kinds) > 0 {
			rows := make([][]any, len(r.Kinds))
			for i, k := range r.Kinds {
				rows[i] = []any{r.Code, string(k)}
			}
			if err := t.insertMulti(ctx, "parameter_kinds", "parameter_code, kind_code", rows); err != nil {
				return err
			}
		}
		// Enum-значения замещаются целиком (батч-вставкой).
		if _, err := t.exec(ctx,
			`DELETE FROM parameter_enum_values WHERE parameter_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		if len(r.EnumValues) > 0 {
			rows := make([][]any, len(r.EnumValues))
			for i, v := range r.EnumValues {
				rows[i] = []any{r.Code, v}
			}
			if err := t.insertMulti(ctx, "parameter_enum_values", "parameter_code, value", rows); err != nil {
				return err
			}
		}
		// Наборы условий замещаются целиком (элементы — каскадом по FK);
		// строки наборов и элементов — батч-вставками по параметру.
		if _, err := t.exec(ctx,
			`DELETE FROM parameter_condition_sets WHERE parameter_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		if len(r.ConditionSets) > 0 {
			setRows := make([][]any, 0, len(r.ConditionSets))
			itemRows := make([][]any, 0, 8)
			for _, set := range r.ConditionSets {
				setRows = append(setRows, []any{r.Code, set.No})
				for _, it := range set.Items {
					itemRows = append(itemRows, []any{
						r.Code, set.No, it.Condition, string(it.Mode), it.FixedValue,
					})
				}
			}
			if err := t.insertMulti(ctx, "parameter_condition_sets",
				"parameter_code, set_no", setRows); err != nil {
				return err
			}
			if err := t.insertMulti(ctx, "parameter_condition_set_items",
				"parameter_code, set_no, condition_code, mode, fixed_value", itemRows); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Tx) writeAttributes(ctx context.Context, rows []catalog.AttributeDef) error {
	for _, r := range rows {
		if err := t.upsert(ctx, `
UPDATE attributes SET "group" = @group, value_type = @vtype,
       unit_code = @unit, validation_rule = @rule, sort_order = @sort, is_active = @active
WHERE code = @code`,
			`INSERT INTO attributes(code, "group", value_type, unit_code,
    validation_rule, sort_order, is_active)
VALUES (@code, @group, @vtype, @unit, @rule, @sort, @active)`,
			map[string]any{
				"code": r.Code, "group": nilIfEmpty(r.GroupName),
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
		if len(r.Kinds) > 0 {
			rows := make([][]any, len(r.Kinds))
			for i, k := range r.Kinds {
				rows[i] = []any{r.Code, string(k)}
			}
			if err := t.insertMulti(ctx, "attribute_kinds", "attribute_code, kind_code", rows); err != nil {
				return err
			}
		}
		if _, err := t.exec(ctx,
			`DELETE FROM attribute_enum_values WHERE attribute_code = @code`,
			map[string]any{"code": r.Code}); err != nil {
			return err
		}
		if len(r.EnumValues) > 0 {
			rows := make([][]any, len(r.EnumValues))
			for i, v := range r.EnumValues {
				rows[i] = []any{r.Code, v}
			}
			if err := t.insertMulti(ctx, "attribute_enum_values", "attribute_code, value", rows); err != nil {
				return err
			}
		}
	}
	return nil
}

// insertMulti — многострочная вставка INSERT INTO … VALUES … с атомарным
// подавлением конфликта уникальности (ON CONFLICT DO NOTHING — параллельный
// писатель postgres); чанки ограничивают число аргументов оператора, как
// батч-вставки devices.go: сидирование каталога — сотни строк, построчные
// вставки стоили бы сотен round-trip'ов.
func (t *Tx) insertMulti(ctx context.Context, table, cols string, rows [][]any) error {
	const rowsPerStmt = 100 // аргументов в операторе ≤ 5·100 — в пределах лимита sqlite
	for start := 0; start < len(rows); start += rowsPerStmt {
		end := min(start+rowsPerStmt, len(rows))
		var sb strings.Builder
		fmt.Fprintf(&sb, "INSERT INTO %s(%s) VALUES ", table, cols)
		args := make(map[string]any, (end-start)*5)
		for i := start; i < end; i++ {
			if i > start {
				sb.WriteString(", ")
			}
			sb.WriteString("(")
			for j, v := range rows[i] {
				if j > 0 {
					sb.WriteString(", ")
				}
				name := fmt.Sprintf("r%dc%d", i, j)
				sb.WriteString("@" + name)
				args[name] = v
			}
			sb.WriteString(")")
		}
		if _, err := t.exec(ctx, withOnConflict(sb.String()), args); err != nil {
			return err
		}
	}
	return nil
}

// upsert выполняет обновление строки по ключу, а при отсутствии изменения —
// вставку. Порядок UPDATE-then-INSERT переносим; писатели параллельны
// только на postgres (sqlite — одно выделенное соединение записи), поэтому
// вставка атомарно подавляет конфликт уникальности (ON CONFLICT DO NOTHING):
// подавленная вставка — строка создана конкурирующей транзакцией, и
// обновление повторяется по закоммиченной строке.
func (t *Tx) upsert(ctx context.Context, update, insert string, args map[string]any) error {
	res, err := t.exec(ctx, update, args)
	if err != nil {
		return domain.NewErrorf(domain.CodeInternal, domain.MsgStorageCatalogWrite, err.Error())
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	res, err = t.exec(ctx, withOnConflict(insert), args)
	if err != nil {
		return domain.NewErrorf(domain.CodeInternal, domain.MsgStorageCatalogWrite, err.Error())
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	if _, err := t.exec(ctx, update, args); err != nil {
		return domain.NewErrorf(domain.CodeInternal, domain.MsgStorageCatalogWrite, err.Error())
	}
	return nil
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
