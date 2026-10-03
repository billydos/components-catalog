package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/billydos/components-catalog/internal/domain"
)

// Строчные типы транспорта устройств: только данные, без семантики секций
// (она — сервисный слой).

// DeviceRow — строка devices.
type DeviceRow struct {
	ID          int64
	Kind        domain.Kind
	System      domain.System
	Designation string
}

// ParamValue — значение параметра с условиями (строка parameter_values
// вместе с parameter_value_conditions).
type ParamValue struct {
	Parameter  string
	Exact      *float64
	Min        *float64
	Max        *float64
	Text       *string
	Conditions []Cond
}

// Cond — условие измерения со значением.
type Cond struct {
	Code  string
	Value float64
}

// ValueRow — значение в контексте устройства: данные типа (VariantID = nil)
// либо исполнения (VariantID = id варианта).
type ValueRow struct {
	VariantID *int64
	Value     ParamValue
}

// VariantRow — строка device_variants.
type VariantRow struct {
	ID        int64
	Label     string
	SortOrder int
}

// AttrRow — значение атрибута записи (ровно одно из Text/Num/Bool задано).
type AttrRow struct {
	Attribute string
	Text      *string
	Num       *float64
	Bool      *bool
}

// AnalogRow — исходящая ссылка-аналог: цель с обозначением и системой
// (направленность — D8: строка принадлежит владельцу device_id).
type AnalogRow struct {
	TargetID    int64
	Designation string
	System      domain.System
	Note        string
}

// BacklinkRow — встречная ссылка: запись, указавшая устройство аналогом.
type BacklinkRow struct {
	SourceID    int64
	Kind        domain.Kind
	Designation string
	System      domain.System
	Note        string
}

// Чтение (пул чтения вне транзакций либо соединение транзакции записи).

func (d *DB) FindDevice(ctx context.Context, kind domain.Kind, designation string) (*DeviceRow, error) {
	return findDevice(ctx, d.reads, kind, designation)
}

func (d *DB) FindDeviceByID(ctx context.Context, id int64) (*DeviceRow, error) {
	return findDeviceByID(ctx, d.reads, id)
}

// FindDeviceAnyKind ищет запись по обозначению без фильтра классом
// (разрешение аналогов, find).
func (d *DB) FindDeviceAnyKind(ctx context.Context, designation string) (*DeviceRow, error) {
	return findDeviceAnyKind(ctx, d.reads, designation)
}

func (t *Tx) FindDevice(ctx context.Context, kind domain.Kind, designation string) (*DeviceRow, error) {
	return findDevice(ctx, t.tx, kind, designation)
}

func findDevice(ctx context.Context, q queryer, kind domain.Kind, designation string) (*DeviceRow, error) {
	row := q.QueryRowContext(ctx, `
SELECT id, kind_code, system_code, designation FROM devices
WHERE kind_code = @kind AND designation = @designation`,
		named(map[string]any{"kind": string(kind), "designation": designation})...)
	return scanDevice(row)
}

func findDeviceByID(ctx context.Context, q queryer, id int64) (*DeviceRow, error) {
	row := q.QueryRowContext(ctx, `
SELECT id, kind_code, system_code, designation FROM devices WHERE id = @id`,
		named(map[string]any{"id": id})...)
	return scanDevice(row)
}

func findDeviceAnyKind(ctx context.Context, q queryer, designation string) (*DeviceRow, error) {
	row := q.QueryRowContext(ctx, `
SELECT id, kind_code, system_code, designation FROM devices
WHERE designation = @designation
ORDER BY kind_code
LIMIT 1`,
		named(map[string]any{"designation": designation})...)
	return scanDevice(row)
}

func scanDevice(row *sql.Row) (*DeviceRow, error) {
	var r DeviceRow
	err := row.Scan(&r.ID, &r.Kind, &r.System, &r.Designation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// LoadFields — поля разбора обозначения (порядок алфавитный — стабильно
// для карточки и сравнения состояний).
func (d *DB) LoadFields(ctx context.Context, deviceID int64) ([]domain.Field, error) {
	return loadFields(ctx, d.reads, deviceID)
}

func (t *Tx) LoadFields(ctx context.Context, deviceID int64) ([]domain.Field, error) {
	return loadFields(ctx, t.tx, deviceID)
}

func loadFields(ctx context.Context, q queryer, deviceID int64) ([]domain.Field, error) {
	rows, err := q.QueryContext(ctx, `
SELECT field, text_value, num_value FROM device_designation_fields
WHERE device_id = @id ORDER BY field`,
		named(map[string]any{"id": deviceID})...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Field
	for rows.Next() {
		var name string
		var text sql.NullString
		var num sql.NullFloat64
		if err := rows.Scan(&name, &text, &num); err != nil {
			return nil, err
		}
		switch {
		case text.Valid:
			out = append(out, domain.TextField(name, text.String))
		case num.Valid:
			out = append(out, domain.NumField(name, num.Float64))
		default:
			out = append(out, domain.TextField(name, ""))
		}
	}
	return out, rows.Err()
}

func (d *DB) LoadAttributes(ctx context.Context, deviceID int64) ([]AttrRow, error) {
	return loadAttributes(ctx, d.reads, deviceID)
}

func (t *Tx) LoadAttributes(ctx context.Context, deviceID int64) ([]AttrRow, error) {
	return loadAttributes(ctx, t.tx, deviceID)
}

func loadAttributes(ctx context.Context, q queryer, deviceID int64) ([]AttrRow, error) {
	rows, err := q.QueryContext(ctx, `
SELECT attribute_code, text_value, num_value, bool_value FROM device_attribute_values
WHERE device_id = @id ORDER BY attribute_code`,
		named(map[string]any{"id": deviceID})...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttrRow
	for rows.Next() {
		var r AttrRow
		var text sql.NullString
		var num sql.NullFloat64
		var b sql.NullInt64
		if err := rows.Scan(&r.Attribute, &text, &num, &b); err != nil {
			return nil, err
		}
		if text.Valid {
			s := text.String
			r.Text = &s
		}
		if num.Valid {
			f := num.Float64
			r.Num = &f
		}
		if b.Valid {
			v := b.Int64 != 0
			r.Bool = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadValues — все значения параметров устройства (тип в целом и исполнения)
// с условиями; порядок детерминирован: сначала данные типа, затем значения
// вариантов по (variant_id, sort_order, id); условия — по коду.
func (d *DB) LoadValues(ctx context.Context, deviceID int64) ([]ValueRow, error) {
	return loadValues(ctx, d.reads, deviceID)
}

func (t *Tx) LoadValues(ctx context.Context, deviceID int64) ([]ValueRow, error) {
	return loadValues(ctx, t.tx, deviceID)
}

func loadValues(ctx context.Context, q queryer, deviceID int64) ([]ValueRow, error) {
	rows, err := q.QueryContext(ctx, `
SELECT pv.id, pv.variant_id, pv.parameter_code, pv.value_exact, pv.value_min,
       pv.value_max, pv.value_text, pvc.condition_code, pvc.value
FROM parameter_values pv
LEFT JOIN parameter_value_conditions pvc ON pvc.parameter_value_id = pv.id
WHERE pv.device_id = @id
ORDER BY (CASE WHEN pv.variant_id IS NULL THEN 0 ELSE 1 END),
         pv.variant_id, pv.sort_order, pv.id, pvc.condition_code`,
		named(map[string]any{"id": deviceID})...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ValueRow
	byID := make(map[int64]int)
	for rows.Next() {
		var id int64
		var variant sql.NullInt64
		var parameter string
		var exact, minv, maxv sql.NullFloat64
		var text sql.NullString
		var condCode sql.NullString
		var condValue sql.NullFloat64
		if err := rows.Scan(&id, &variant, &parameter, &exact, &minv, &maxv, &text,
			&condCode, &condValue); err != nil {
			return nil, err
		}
		idx, ok := byID[id]
		if !ok {
			vr := ValueRow{Value: ParamValue{Parameter: parameter}}
			if variant.Valid {
				vid := variant.Int64
				vr.VariantID = &vid
			}
			if exact.Valid {
				f := exact.Float64
				vr.Value.Exact = &f
			}
			if minv.Valid {
				f := minv.Float64
				vr.Value.Min = &f
			}
			if maxv.Valid {
				f := maxv.Float64
				vr.Value.Max = &f
			}
			if text.Valid {
				s := text.String
				vr.Value.Text = &s
			}
			out = append(out, vr)
			byID[id] = len(out) - 1
			idx = byID[id]
		}
		if condCode.Valid && condValue.Valid {
			out[idx].Value.Conditions = append(out[idx].Value.Conditions,
				Cond{Code: condCode.String, Value: condValue.Float64})
		}
	}
	return out, rows.Err()
}

func (d *DB) LoadVariants(ctx context.Context, deviceID int64) ([]VariantRow, error) {
	return loadVariants(ctx, d.reads, deviceID)
}

func (t *Tx) LoadVariants(ctx context.Context, deviceID int64) ([]VariantRow, error) {
	return loadVariants(ctx, t.tx, deviceID)
}

func loadVariants(ctx context.Context, q queryer, deviceID int64) ([]VariantRow, error) {
	rows, err := q.QueryContext(ctx, `
SELECT id, label, sort_order FROM device_variants
WHERE device_id = @id ORDER BY sort_order, id`,
		named(map[string]any{"id": deviceID})...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VariantRow
	for rows.Next() {
		var r VariantRow
		var label sql.NullString
		if err := rows.Scan(&r.ID, &label, &r.SortOrder); err != nil {
			return nil, err
		}
		r.Label = nullableStr(label)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) LoadManufacturers(ctx context.Context, deviceID int64) ([]string, error) {
	return loadManufacturers(ctx, d.reads, deviceID)
}

func (t *Tx) LoadManufacturers(ctx context.Context, deviceID int64) ([]string, error) {
	return loadManufacturers(ctx, t.tx, deviceID)
}

func loadManufacturers(ctx context.Context, q queryer, deviceID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
SELECT m.name FROM manufacturers m
JOIN device_manufacturers dm ON dm.manufacturer_id = m.id
WHERE dm.device_id = @id ORDER BY m.name`,
		named(map[string]any{"id": deviceID})...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (d *DB) LoadOutgoingAnalogs(ctx context.Context, deviceID int64) ([]AnalogRow, error) {
	return loadOutgoingAnalogs(ctx, d.reads, deviceID)
}

func (t *Tx) LoadOutgoingAnalogs(ctx context.Context, deviceID int64) ([]AnalogRow, error) {
	return loadOutgoingAnalogs(ctx, t.tx, deviceID)
}

func loadOutgoingAnalogs(ctx context.Context, q queryer, deviceID int64) ([]AnalogRow, error) {
	rows, err := q.QueryContext(ctx, `
SELECT a.analog_device_id, t.designation, t.system_code, a.note
FROM device_analogs a
JOIN devices t ON t.id = a.analog_device_id
WHERE a.device_id = @id
ORDER BY t.designation`,
		named(map[string]any{"id": deviceID})...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AnalogRow
	for rows.Next() {
		var r AnalogRow
		var note sql.NullString
		if err := rows.Scan(&r.TargetID, &r.Designation, &r.System, &note); err != nil {
			return nil, err
		}
		r.Note = nullableStr(note)
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadBacklinks — встречные ссылки («кто указал запись аналогом»), индекс
// по analog_device_id (docs/plan/02-database.md §3).
func (d *DB) LoadBacklinks(ctx context.Context, deviceID int64) ([]BacklinkRow, error) {
	rows, err := d.query(ctx, `
SELECT a.device_id, s.kind_code, s.designation, s.system_code, a.note
FROM device_analogs a
JOIN devices s ON s.id = a.device_id
WHERE a.analog_device_id = @id
ORDER BY s.designation`,
		map[string]any{"id": deviceID})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BacklinkRow
	for rows.Next() {
		var r BacklinkRow
		var note sql.NullString
		if err := rows.Scan(&r.SourceID, &r.Kind, &r.Designation, &r.System, &note); err != nil {
			return nil, err
		}
		r.Note = nullableStr(note)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Запись (только внутри транзакции).

// InsertDevice вставляет запись, если её нет (INSERT … SELECT … WHERE NOT
// EXISTS), и возвращает id вместе с признаком вставки.
func (t *Tx) InsertDevice(ctx context.Context, kind domain.Kind, system domain.System, designation string) (int64, bool, error) {
	res, err := t.exec(ctx, `
INSERT INTO devices(kind_code, system_code, designation)
SELECT @kind, @system, @designation
WHERE NOT EXISTS (
    SELECT 1 FROM devices
    WHERE kind_code = @kind AND designation = @designation)`,
		map[string]any{"kind": string(kind), "system": string(system), "designation": designation})
	if err != nil {
		return 0, false, err
	}
	created := false
	if n, _ := res.RowsAffected(); n > 0 {
		created = true
	}
	row := t.queryRow(ctx, `
SELECT id FROM devices WHERE kind_code = @kind AND designation = @designation`,
		map[string]any{"kind": string(kind), "designation": designation})
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, false, err
	}
	return id, created, nil
}

// InsertDesignationFields записывает поля разбора обозначения.
func (t *Tx) InsertDesignationFields(ctx context.Context, deviceID int64, fields []domain.Field) error {
	for _, f := range fields {
		var text, num any
		if f.IsNum {
			num = f.Num
		} else {
			text = f.Text
		}
		if _, err := t.exec(ctx, `
INSERT INTO device_designation_fields(device_id, field, text_value, num_value)
VALUES (@id, @field, @text, @num)`,
			map[string]any{"id": deviceID, "field": f.Name, "text": text, "num": num}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDesignationFields удаляет именованные поля разбора записи (замена
// набора явных классификационных полей секцией fields; поля грамматики
// парсера не затрагиваются).
func (t *Tx) DeleteDesignationFields(ctx context.Context, deviceID int64, names []string) error {
	for _, name := range names {
		if _, err := t.exec(ctx,
			`DELETE FROM device_designation_fields WHERE device_id = @id AND field = @field`,
			map[string]any{"id": deviceID, "field": name}); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceAttributes заменяет значения атрибутов целиком (секция attributes).
func (t *Tx) ReplaceAttributes(ctx context.Context, deviceID int64, rows []AttrRow) error {
	if _, err := t.exec(ctx,
		`DELETE FROM device_attribute_values WHERE device_id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	for _, r := range rows {
		var text, num, b any
		if r.Text != nil {
			text = *r.Text
		}
		if r.Num != nil {
			num = *r.Num
		}
		if r.Bool != nil {
			b = boolInt(*r.Bool)
		}
		if _, err := t.exec(ctx, `
INSERT INTO device_attribute_values(device_id, attribute_code, text_value, num_value, bool_value)
VALUES (@id, @code, @text, @num, @bool)`,
			map[string]any{"id": deviceID, "code": r.Attribute, "text": text, "num": num, "bool": b}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteTypeGroupValues удаляет данные типа в целом (variant_id NULL)
// по группе каталога — замена секции группы.
func (t *Tx) DeleteTypeGroupValues(ctx context.Context, deviceID int64, groupCode string) error {
	_, err := t.exec(ctx, `
DELETE FROM parameter_values
WHERE device_id = @id AND variant_id IS NULL
  AND parameter_code IN (SELECT code FROM parameters WHERE group_code = @group)`,
		map[string]any{"id": deviceID, "group": groupCode})
	return err
}

// InsertValues вставляет значения параметров в контексте типа (variantID =
// nil) либо исполнения; порядок среза — sort_order.
func (t *Tx) InsertValues(ctx context.Context, deviceID int64, variantID *int64, values []ParamValue) error {
	for i, v := range values {
		var exact, minv, maxv, text any
		if v.Exact != nil {
			exact = *v.Exact
		}
		if v.Min != nil {
			minv = *v.Min
		}
		if v.Max != nil {
			maxv = *v.Max
		}
		if v.Text != nil {
			text = *v.Text
		}
		id, err := t.InsertReturningID(ctx, `
INSERT INTO parameter_values(device_id, variant_id, parameter_code,
    value_exact, value_min, value_max, value_text, sort_order)
VALUES (@device, @variant, @code, @exact, @min, @max, @text, @sort)`,
			map[string]any{
				"device": deviceID, "variant": variantID, "code": v.Parameter,
				"exact": exact, "min": minv, "max": maxv, "text": text, "sort": i,
			})
		if err != nil {
			return err
		}
		for _, c := range v.Conditions {
			if _, err := t.exec(ctx, `
INSERT INTO parameter_value_conditions(parameter_value_id, condition_code, value)
VALUES (@id, @code, @value)`,
				map[string]any{"id": id, "code": c.Code, "value": c.Value}); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteVariants удаляет исполнения записи вместе с их значениями (каскад
// FK parameter_values.variant_id).
func (t *Tx) DeleteVariants(ctx context.Context, deviceID int64) error {
	_, err := t.exec(ctx,
		`DELETE FROM device_variants WHERE device_id = @id`,
		map[string]any{"id": deviceID})
	return err
}

// InsertVariant вставляет исполнение и возвращает его id.
func (t *Tx) InsertVariant(ctx context.Context, deviceID int64, label string, sortOrder int) (int64, error) {
	return t.InsertReturningID(ctx, `
INSERT INTO device_variants(device_id, label, sort_order)
VALUES (@id, @label, @sort)`,
		map[string]any{"id": deviceID, "label": nilIfEmpty(label), "sort": sortOrder})
}

// ReplaceManufacturers заменяет список производителей целиком: связи
// пересоздаются, недостающие строки manufacturers создаются, производители
// без единой связи удаляются (чистка сирот).
func (t *Tx) ReplaceManufacturers(ctx context.Context, deviceID int64, names []string) error {
	if _, err := t.exec(ctx,
		`DELETE FROM device_manufacturers WHERE device_id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	for _, name := range names {
		var mid int64
		err := t.queryRow(ctx,
			`SELECT id FROM manufacturers WHERE name = @name`,
			map[string]any{"name": name}).Scan(&mid)
		if errors.Is(err, sql.ErrNoRows) {
			mid, err = t.InsertReturningID(ctx,
				`INSERT INTO manufacturers(name) VALUES (@name)`,
				map[string]any{"name": name})
		}
		if err != nil {
			return err
		}
		if _, err := t.exec(ctx, `
INSERT INTO device_manufacturers(device_id, manufacturer_id)
SELECT @id, @mid
WHERE NOT EXISTS (
    SELECT 1 FROM device_manufacturers
    WHERE device_id = @id AND manufacturer_id = @mid)`,
			map[string]any{"id": deviceID, "mid": mid}); err != nil {
			return err
		}
	}
	return t.deleteOrphanManufacturers(ctx)
}

// ReplaceAnalogs заменяет исходящие ссылки-аналоги целиком (встречные
// ссылки других записей не затрагиваются — D8).
func (t *Tx) ReplaceAnalogs(ctx context.Context, deviceID int64, targets []AnalogRow) error {
	if _, err := t.exec(ctx,
		`DELETE FROM device_analogs WHERE device_id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	for _, a := range targets {
		if _, err := t.exec(ctx, `
INSERT INTO device_analogs(device_id, analog_device_id, note)
VALUES (@id, @target, @note)`,
			map[string]any{"id": deviceID, "target": a.TargetID, "note": nilIfEmpty(a.Note)}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDevice удаляет запись (каскад FK по всем дочерним таблицам,
// включая обе стороны device_analogs) и чистит сирот производителей.
func (t *Tx) DeleteDevice(ctx context.Context, deviceID int64) error {
	if _, err := t.exec(ctx,
		`DELETE FROM devices WHERE id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	return t.deleteOrphanManufacturers(ctx)
}

// deleteOrphanManufacturers удаляет производителей без единой связи
// (NOT IN по device_manufacturers — переносимо).
func (t *Tx) deleteOrphanManufacturers(ctx context.Context) error {
	_, err := t.exec(ctx, `
DELETE FROM manufacturers
WHERE id NOT IN (SELECT manufacturer_id FROM device_manufacturers)`, nil)
	return err
}

// CountDevices возвращает число записей (класса, если задан).
func (d *DB) CountDevices(ctx context.Context, kind domain.Kind) (int, error) {
	q := `SELECT COUNT(*) FROM devices`
	args := map[string]any{}
	if kind != "" {
		q += ` WHERE kind_code = @kind`
		args["kind"] = string(kind)
	}
	var n int
	if err := d.queryRow(ctx, q, args).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// CountManufacturers возвращает число строк manufacturers (контроль чистки
// сирот в тестах и диагностике).
func (d *DB) CountManufacturers(ctx context.Context) (int, error) {
	var n int
	if err := d.queryRow(ctx, `SELECT COUNT(*) FROM manufacturers`, nil).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
