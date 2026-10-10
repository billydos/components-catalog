package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

// FindDeviceAnyKind ищет запись по обозначению без фильтра классом
// (разрешение аналогов, find).
func (d *DB) FindDeviceAnyKind(ctx context.Context, designation string) (*DeviceRow, error) {
	return findDeviceAnyKind(ctx, d.reads, designation)
}

// ListDeviceIDsByKind перечисляет id записей класса в порядке
// (designation) — единый снимок одного запроса: последовательный обход
// страниц по OFFSET на живой базе даёт дубли и пропуски (экспорт).
func (d *DB) ListDeviceIDsByKind(ctx context.Context, kind domain.Kind) ([]int64, error) {
	rows, err := queryContext(ctx, d.reads, `
SELECT id FROM devices
WHERE kind_code = @kind
ORDER BY designation`,
		map[string]any{"kind": string(kind)})
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck — чтение завершилось
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (t *Tx) FindDevice(ctx context.Context, kind domain.Kind, designation string) (*DeviceRow, error) {
	return findDevice(ctx, t.tx, kind, designation)
}

func findDevice(ctx context.Context, q queryer, kind domain.Kind, designation string) (*DeviceRow, error) {
	row := queryRowContext(ctx, q, `
SELECT id, kind_code, system_code, designation FROM devices
WHERE kind_code = @kind AND designation = @designation`,
		map[string]any{"kind": string(kind), "designation": designation})
	return scanDevice(row)
}

func findDeviceAnyKind(ctx context.Context, q queryer, designation string) (*DeviceRow, error) {
	row := queryRowContext(ctx, q, `
SELECT id, kind_code, system_code, designation FROM devices
WHERE designation = @designation
ORDER BY kind_code
LIMIT 1`,
		map[string]any{"designation": designation})
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
	rows, err := queryContext(ctx, q, `
SELECT field, text_value, num_value FROM device_designation_fields
WHERE device_id = @id ORDER BY field`,
		map[string]any{"id": deviceID})
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

func (t *Tx) LoadAttributes(ctx context.Context, deviceID int64) ([]AttrRow, error) {
	return loadAttributes(ctx, t.tx, deviceID)
}

func loadAttributes(ctx context.Context, q queryer, deviceID int64) ([]AttrRow, error) {
	rows, err := queryContext(ctx, q, `
SELECT attribute_code, text_value, num_value, bool_value FROM device_attribute_values
WHERE device_id = @id ORDER BY attribute_code`,
		map[string]any{"id": deviceID})
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
func (t *Tx) LoadValues(ctx context.Context, deviceID int64) ([]ValueRow, error) {
	return loadValues(ctx, t.tx, deviceID)
}

func loadValues(ctx context.Context, q queryer, deviceID int64) ([]ValueRow, error) {
	rows, err := queryContext(ctx, q, `
SELECT pv.id, pv.variant_id, pv.parameter_code, pv.value_exact, pv.value_min,
       pv.value_max, pv.value_text, pvc.condition_code, pvc.value
FROM parameter_values pv
LEFT JOIN parameter_value_conditions pvc ON pvc.parameter_value_id = pv.id
WHERE pv.device_id = @id
ORDER BY (CASE WHEN pv.variant_id IS NULL THEN 0 ELSE 1 END),
         pv.variant_id, pv.sort_order, pv.id, pvc.condition_code`,
		map[string]any{"id": deviceID})
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

func (t *Tx) LoadVariants(ctx context.Context, deviceID int64) ([]VariantRow, error) {
	return loadVariants(ctx, t.tx, deviceID)
}

func loadVariants(ctx context.Context, q queryer, deviceID int64) ([]VariantRow, error) {
	rows, err := queryContext(ctx, q, `
SELECT id, label, sort_order FROM device_variants
WHERE device_id = @id ORDER BY sort_order, id`,
		map[string]any{"id": deviceID})
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
	rows, err := queryContext(ctx, q, `
SELECT m.name FROM manufacturers m
JOIN device_manufacturers dm ON dm.manufacturer_id = m.id
WHERE dm.device_id = @id ORDER BY m.name`,
		map[string]any{"id": deviceID})
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

func (t *Tx) LoadOutgoingAnalogs(ctx context.Context, deviceID int64) ([]AnalogRow, error) {
	return loadOutgoingAnalogs(ctx, t.tx, deviceID)
}

func loadOutgoingAnalogs(ctx context.Context, q queryer, deviceID int64) ([]AnalogRow, error) {
	rows, err := queryContext(ctx, q, `
SELECT a.analog_device_id, t.designation, t.system_code, a.note
FROM device_analogs a
JOIN devices t ON t.id = a.analog_device_id
WHERE a.device_id = @id
ORDER BY t.designation`,
		map[string]any{"id": deviceID})
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

// Пакетное чтение карточек: по одному SELECT с device_id IN (…) на таблицу,
// группировка по записям — в Go (образец — loadSearchFields); порядок строк
// внутри записи — как у одиночных загрузчиков.

// FindDevicesByIDs — записи по списку стабильных id (параллельно удалённые
// в результате отсутствуют; порядок строк — по id).
func (d *DB) FindDevicesByIDs(ctx context.Context, ids []int64) ([]*DeviceRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT id, kind_code, system_code, designation FROM devices
WHERE id IN `+inList("id", ids, args)+` ORDER BY id`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DeviceRow
	for rows.Next() {
		var r DeviceRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.System, &r.Designation); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// LoadFieldsByIDs — поля разбора обозначений по списку записей.
func (d *DB) LoadFieldsByIDs(ctx context.Context, ids []int64) (map[int64][]domain.Field, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT device_id, field, text_value, num_value FROM device_designation_fields
WHERE device_id IN `+inList("id", ids, args)+` ORDER BY device_id, field`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]domain.Field, len(ids))
	for rows.Next() {
		var id int64
		var name string
		var text sql.NullString
		var num sql.NullFloat64
		if err := rows.Scan(&id, &name, &text, &num); err != nil {
			return nil, err
		}
		switch {
		case text.Valid:
			out[id] = append(out[id], domain.TextField(name, text.String))
		case num.Valid:
			out[id] = append(out[id], domain.NumField(name, num.Float64))
		default:
			out[id] = append(out[id], domain.TextField(name, ""))
		}
	}
	return out, rows.Err()
}

// LoadAttributesByIDs — значения атрибутов по списку записей.
func (d *DB) LoadAttributesByIDs(ctx context.Context, ids []int64) (map[int64][]AttrRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT device_id, attribute_code, text_value, num_value, bool_value
FROM device_attribute_values
WHERE device_id IN `+inList("id", ids, args)+` ORDER BY device_id, attribute_code`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]AttrRow, len(ids))
	for rows.Next() {
		var id int64
		var r AttrRow
		var text sql.NullString
		var num sql.NullFloat64
		var b sql.NullInt64
		if err := rows.Scan(&id, &r.Attribute, &text, &num, &b); err != nil {
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
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

// LoadValuesByIDs — все значения параметров (тип в целом и исполнения)
// с условиями по списку записей; порядок строк внутри записи — как у
// LoadValues.
func (d *DB) LoadValuesByIDs(ctx context.Context, ids []int64) (map[int64][]ValueRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT pv.device_id, pv.id, pv.variant_id, pv.parameter_code, pv.value_exact, pv.value_min,
       pv.value_max, pv.value_text, pvc.condition_code, pvc.value
FROM parameter_values pv
LEFT JOIN parameter_value_conditions pvc ON pvc.parameter_value_id = pv.id
WHERE pv.device_id IN `+inList("id", ids, args)+`
ORDER BY pv.device_id, (CASE WHEN pv.variant_id IS NULL THEN 0 ELSE 1 END),
         pv.variant_id, pv.sort_order, pv.id, pvc.condition_code`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]ValueRow, len(ids))
	pos := make(map[int64]int) // parameter_values.id → индекс в out[device_id]
	for rows.Next() {
		var devID, id int64
		var variant sql.NullInt64
		var parameter string
		var exact, minv, maxv sql.NullFloat64
		var text sql.NullString
		var condCode sql.NullString
		var condValue sql.NullFloat64
		if err := rows.Scan(&devID, &id, &variant, &parameter, &exact, &minv, &maxv, &text,
			&condCode, &condValue); err != nil {
			return nil, err
		}
		idx, ok := pos[id]
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
			out[devID] = append(out[devID], vr)
			pos[id] = len(out[devID]) - 1
			idx = pos[id]
		}
		if condCode.Valid && condValue.Valid {
			slice := out[devID]
			slice[idx].Value.Conditions = append(slice[idx].Value.Conditions,
				Cond{Code: condCode.String, Value: condValue.Float64})
		}
	}
	return out, rows.Err()
}

// LoadVariantsByIDs — исполнения по списку записей.
func (d *DB) LoadVariantsByIDs(ctx context.Context, ids []int64) (map[int64][]VariantRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT device_id, id, label, sort_order FROM device_variants
WHERE device_id IN `+inList("id", ids, args)+` ORDER BY device_id, sort_order, id`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]VariantRow, len(ids))
	for rows.Next() {
		var id int64
		var r VariantRow
		var label sql.NullString
		if err := rows.Scan(&id, &r.ID, &label, &r.SortOrder); err != nil {
			return nil, err
		}
		r.Label = nullableStr(label)
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

// LoadManufacturersByIDs — производители по списку записей.
func (d *DB) LoadManufacturersByIDs(ctx context.Context, ids []int64) (map[int64][]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT dm.device_id, m.name FROM manufacturers m
JOIN device_manufacturers dm ON dm.manufacturer_id = m.id
WHERE dm.device_id IN `+inList("id", ids, args)+` ORDER BY dm.device_id, m.name`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]string, len(ids))
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}

// LoadOutgoingAnalogsByIDs — исходящие ссылки-аналоги по списку записей.
func (d *DB) LoadOutgoingAnalogsByIDs(ctx context.Context, ids []int64) (map[int64][]AnalogRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT a.device_id, a.analog_device_id, t.designation, t.system_code, a.note
FROM device_analogs a
JOIN devices t ON t.id = a.analog_device_id
WHERE a.device_id IN `+inList("id", ids, args)+` ORDER BY a.device_id, t.designation`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]AnalogRow, len(ids))
	for rows.Next() {
		var id int64
		var r AnalogRow
		var note sql.NullString
		if err := rows.Scan(&id, &r.TargetID, &r.Designation, &r.System, &note); err != nil {
			return nil, err
		}
		r.Note = nullableStr(note)
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

// LoadBacklinksByIDs — встречные ссылки («кто указал запись аналогом») по
// списку записей, индекс по analog_device_id (docs/plan/02-database.md §3).
func (d *DB) LoadBacklinksByIDs(ctx context.Context, ids []int64) (map[int64][]BacklinkRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make(map[string]any, len(ids))
	rows, err := d.query(ctx, `
SELECT a.analog_device_id, a.device_id, s.kind_code, s.designation, s.system_code, a.note
FROM device_analogs a
JOIN devices s ON s.id = a.device_id
WHERE a.analog_device_id IN `+inList("id", ids, args)+`
ORDER BY a.analog_device_id, s.designation`, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]BacklinkRow, len(ids))
	for rows.Next() {
		var id int64
		var r BacklinkRow
		var note sql.NullString
		if err := rows.Scan(&id, &r.SourceID, &r.Kind, &r.Designation, &r.System, &note); err != nil {
			return nil, err
		}
		r.Note = nullableStr(note)
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

// Запись (только внутри транзакции).

// InsertDevice вставляет запись, если её нет, и возвращает id вместе
// с признаком вставки. Вставка атомарно подавляет конфликт уникальности
// (kind_code, designation): параллельный писатель (postgres — общий пул
// соединений; sqlite-писатель один) успел вставить то же обозначение —
// вставки не происходит, id перечитывается по ключу.
func (t *Tx) InsertDevice(ctx context.Context, kind domain.Kind, system domain.System, designation string) (int64, bool, error) {
	id, inserted, err := t.InsertIfAbsentReturningID(ctx, `
INSERT INTO devices(kind_code, system_code, designation)
VALUES (@kind, @system, @designation)`,
		map[string]any{"kind": string(kind), "system": string(system), "designation": designation})
	if err != nil {
		return 0, false, err
	}
	if inserted {
		return id, true, nil
	}
	err = t.queryRow(ctx, `
SELECT id FROM devices WHERE kind_code = @kind AND designation = @designation`,
		map[string]any{"kind": string(kind), "designation": designation}).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id, false, nil
}

// InsertDesignationFields записывает поля разбора обозначения; поля,
// уже существующие у записи (параллельный писатель), не затрагиваются —
// конфликт уникальности (device_id, field) подавляется атомарно. Вставка
// многострочная: число операторов не зависит от числа полей.
func (t *Tx) InsertDesignationFields(ctx context.Context, deviceID int64, fields []domain.Field) error {
	const chunk = 100 // аргументов в операторе ≤ 3·100 + 1 — в пределах лимита sqlite
	for start := 0; start < len(fields); start += chunk {
		end := min(start+chunk, len(fields))
		var sb strings.Builder
		sb.WriteString(`
INSERT INTO device_designation_fields(device_id, field, text_value, num_value)
VALUES `)
		args := make(map[string]any, (end-start)*3+1)
		args["id"] = deviceID
		for i := start; i < end; i++ {
			if i > start {
				sb.WriteString(", ")
			}
			f := fields[i]
			fmt.Fprintf(&sb, "(@id, @f%d, @t%d, @n%d)", i, i, i)
			args[fmt.Sprintf("f%d", i)] = f.Name
			if f.IsNum {
				args[fmt.Sprintf("t%d", i)] = nil
				args[fmt.Sprintf("n%d", i)] = f.Num
			} else {
				args[fmt.Sprintf("t%d", i)] = f.Text
				args[fmt.Sprintf("n%d", i)] = nil
			}
		}
		if _, err := t.exec(ctx, sb.String()+" ON CONFLICT DO NOTHING", args); err != nil {
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

// ReplaceAttributes заменяет значения атрибутов целиком (секция attributes);
// значения, вставленные параллельным писателем в той же записи и не видимые
// DELETE этого вызова, сохраняются.
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
		if _, err := t.InsertIfAbsent(ctx, `
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
// nil) либо исполнения; порядок среза — sort_order. Значения без условий
// идут многострочной вставкой (id строк не нужен), значения с условиями —
// построчно (id нужен для условий), условия одного значения — одной
// многострочной вставкой.
func (t *Tx) InsertValues(ctx context.Context, deviceID int64, variantID *int64, values []ParamValue) error {
	plain := make([]int, 0, len(values))
	for i, v := range values {
		if len(v.Conditions) == 0 {
			plain = append(plain, i)
		}
	}
	if err := t.insertPlainValues(ctx, deviceID, variantID, values, plain); err != nil {
		return err
	}
	for i, v := range values {
		if len(v.Conditions) == 0 {
			continue
		}
		id, err := t.insertValueRow(ctx, deviceID, variantID, v, i)
		if err != nil {
			return err
		}
		if err := t.insertValueConditions(ctx, id, v.Conditions); err != nil {
			return err
		}
	}
	return nil
}

// insertPlainValues — многострочная вставка значений без условий по списку
// их индексов (чанки ограничивают число аргументов оператора).
func (t *Tx) insertPlainValues(ctx context.Context, deviceID int64, variantID *int64,
	values []ParamValue, idx []int) error {
	const chunk = 50 // аргументов в операторе ≤ 6·50 + 2 — в пределах лимита sqlite
	for start := 0; start < len(idx); start += chunk {
		end := min(start+chunk, len(idx))
		var sb strings.Builder
		sb.WriteString(`
INSERT INTO parameter_values(device_id, variant_id, parameter_code,
    value_exact, value_min, value_max, value_text, sort_order)
VALUES `)
		args := make(map[string]any, (end-start)*6+2)
		args["device"] = deviceID
		args["variant"] = variantID
		for j := start; j < end; j++ {
			if j > start {
				sb.WriteString(", ")
			}
			i := idx[j]
			v := values[i]
			fmt.Fprintf(&sb, "(@device, @variant, @p%d, @e%d, @m%d, @x%d, @t%d, @s%d)", i, i, i, i, i, i)
			args[fmt.Sprintf("p%d", i)] = v.Parameter
			args[fmt.Sprintf("e%d", i)] = optValue(v.Exact)
			args[fmt.Sprintf("m%d", i)] = optValue(v.Min)
			args[fmt.Sprintf("x%d", i)] = optValue(v.Max)
			args[fmt.Sprintf("t%d", i)] = optValue(v.Text)
			args[fmt.Sprintf("s%d", i)] = i
		}
		if _, err := t.exec(ctx, sb.String(), args); err != nil {
			return err
		}
	}
	return nil
}

// insertValueRow — одно значение с возвратом сгенерированного id (метод
// диалекта — план 02 §4).
func (t *Tx) insertValueRow(ctx context.Context, deviceID int64, variantID *int64,
	v ParamValue, sort int) (int64, error) {
	return t.InsertReturningID(ctx, `
INSERT INTO parameter_values(device_id, variant_id, parameter_code,
    value_exact, value_min, value_max, value_text, sort_order)
VALUES (@device, @variant, @code, @exact, @min, @max, @text, @sort)`,
		map[string]any{
			"device": deviceID, "variant": variantID, "code": v.Parameter,
			"exact": optValue(v.Exact), "min": optValue(v.Min), "max": optValue(v.Max),
			"text": optValue(v.Text), "sort": sort,
		})
}

// insertValueConditions — условия одного значения одной многострочной
// вставкой.
func (t *Tx) insertValueConditions(ctx context.Context, id int64, conds []Cond) error {
	var sb strings.Builder
	sb.WriteString(`
INSERT INTO parameter_value_conditions(parameter_value_id, condition_code, value)
VALUES `)
	args := make(map[string]any, len(conds)*2+1)
	args["id"] = id
	for j, c := range conds {
		if j > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "(@id, @c%d, @v%d)", j, j)
		args[fmt.Sprintf("c%d", j)] = c.Code
		args[fmt.Sprintf("v%d", j)] = c.Value
	}
	_, err := t.exec(ctx, sb.String(), args)
	return err
}

// optValue — значение-указатель как аргумент вставки: nil — NULL.
func optValue[T any](v *T) any {
	if v == nil {
		return nil
	}
	return *v
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
// без единой связи удаляются (чистка сирот). Создание строки производителя
// атомарно подавляет конфликт уникальности имени: параллельный писатель
// успел вставить то же имя — id перечитывается по имени.
func (t *Tx) ReplaceManufacturers(ctx context.Context, deviceID int64, names []string) error {
	if _, err := t.exec(ctx,
		`DELETE FROM device_manufacturers WHERE device_id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	for _, name := range names {
		mid, err := t.manufacturerID(ctx, name)
		if err != nil {
			return err
		}
		if _, err := t.InsertIfAbsent(ctx, `
INSERT INTO device_manufacturers(device_id, manufacturer_id)
VALUES (@id, @mid)`,
			map[string]any{"id": deviceID, "mid": mid}); err != nil {
			return err
		}
	}
	return t.deleteOrphanManufacturers(ctx)
}

// manufacturerID возвращает id производителя по имени, создавая строку при
// отсутствии; конкурентное создание той же строки подавляется (проигранная
// гонка), id перечитывается по имени.
func (t *Tx) manufacturerID(ctx context.Context, name string) (int64, error) {
	id, inserted, err := t.InsertIfAbsentReturningID(ctx,
		`INSERT INTO manufacturers(name) VALUES (@name)`,
		map[string]any{"name": name})
	if err != nil {
		return 0, err
	}
	if inserted {
		return id, nil
	}
	err = t.queryRow(ctx,
		`SELECT id FROM manufacturers WHERE name = @name`,
		map[string]any{"name": name}).Scan(&id)
	return id, err
}

// ReplaceAnalogs заменяет исходящие ссылки-аналоги целиком (встречные
// ссылки других записей не затрагиваются — D8); ссылки, вставленные
// параллельным писателем в той же записи и не видимые DELETE этого вызова,
// сохраняются.
func (t *Tx) ReplaceAnalogs(ctx context.Context, deviceID int64, targets []AnalogRow) error {
	if _, err := t.exec(ctx,
		`DELETE FROM device_analogs WHERE device_id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	for _, a := range targets {
		if _, err := t.InsertIfAbsent(ctx, `
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

// DeleteDeviceByKey удаляет запись по ключу (kind_code, designation)
// в текущей транзакции и чистит сирот производителей; возвращает число
// удалённых строк (0 — записи нет). Поиск и удаление атомарны в одном
// операторе: конкурентное удаление того же ключа даёт ноль строк — без
// «пустого» успеха и лишнего инкремента ревизии (TOCTOU отдельного
// поиска и удаления).
func (t *Tx) DeleteDeviceByKey(ctx context.Context, kind domain.Kind, designation string) (int64, error) {
	return t.deleteDeviceWhere(ctx, `kind_code = @kind AND designation = @designation`,
		map[string]any{"kind": string(kind), "designation": designation})
}

// DeleteDeviceAnyKindByKey удаляет запись по обозначению без фильтра
// классом — детерминированно первая по kind_code, как FindDeviceAnyKind
// (подзапрос переносим: sqlite и postgres).
func (t *Tx) DeleteDeviceAnyKindByKey(ctx context.Context, designation string) (int64, error) {
	return t.deleteDeviceWhere(ctx, `id = (
SELECT id FROM devices WHERE designation = @designation ORDER BY kind_code LIMIT 1)`,
		map[string]any{"designation": designation})
}

func (t *Tx) deleteDeviceWhere(ctx context.Context, cond string, args map[string]any) (int64, error) {
	res, err := t.exec(ctx, `DELETE FROM devices WHERE `+cond, args)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	return n, t.deleteOrphanManufacturers(ctx)
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

// CountDevicesByKind — числа записей по классам одним GROUP BY-запросом
// (агрегат /stats); классы без записей в результате отсутствуют.
func (d *DB) CountDevicesByKind(ctx context.Context) (map[string]int, error) {
	rows, err := d.query(ctx, `
SELECT kind_code, COUNT(*) FROM devices GROUP BY kind_code`, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck — чтение завершилось
	out := make(map[string]int)
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, err
		}
		out[kind] = n
	}
	return out, rows.Err()
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
