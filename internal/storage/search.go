package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// FilterOp — операция сравнения числового фильтра.
type FilterOp string

// Реестр операций фильтров.
const (
	OpEq  FilterOp = "eq"
	OpGte FilterOp = "gte"
	OpLte FilterOp = "lte"
)

func (op FilterOp) sql() string {
	switch op {
	case OpGte:
		return ">="
	case OpLte:
		return "<="
	default:
		return "="
	}
}

// FieldCond — фильтр по полю разбора обозначения: текстовое значение (Text),
// набор текстовых значений с семантикой ИЛИ (Texts — словарные поля
// мультивыбора; приоритетнее Text) либо числовое со сравнением (Num + Op).
type FieldCond struct {
	Field  string
	Text   string
	Texts  []string
	Num    float64
	HasNum bool
	Op     FilterOp
}

// AttrCond — фильтр по атрибуту: текстовое либо числовое значение.
type AttrCond struct {
	Code   string
	Text   string
	Num    float64
	HasNum bool
	Op     FilterOp
}

// ParamCond — фильтр по параметру: нижняя/верхняя граница, точное значение
// либо текст/enum. Вариантные параметры применяются в пределах одного
// исполнения (EXISTS по device_variants) — перекрёстные сочетания значений
// разных исполнений не сопоставляются (docs/plan/04-module-functionality.md §1).
type ParamCond struct {
	Code              string
	Min, Max, Exact   float64
	HasMin, HasMax    bool
	HasExact, HasText bool
	Text              string
}

// SortKey — ключ сортировки: "designation", "id" либо код поля разбора
// (Numeric — числовая сортировка поля, иначе текстовая).
type SortKey struct {
	Key     string
	Numeric bool
	Desc    bool
}

// SearchRequest — параметры поиска без бизнес-правил: готовые условия
// (валидация применимости — сервисный слой). IDs — фильтр по набору
// стабильных id записей (пересечение с прочими условиями; без сортировки
// порядок выдачи — порядок передачи id).
type SearchRequest struct {
	Kind   string
	System string
	Query  string // подстрока обозначения (каноническая, без масок)
	IDs    []int64
	Fields []FieldCond
	Attrs  []AttrCond
	Params []ParamCond
	Sorts  []SortKey
	Limit  int
	Offset int
}

// SearchItemRow — строка результата поиска.
type SearchItemRow struct {
	ID          int64
	Kind        string
	System      string
	Designation string
	Fields      []anyField
}

// whereBuilder собирает условия и уникальные имена аргументов.
type whereBuilder struct {
	parts []string
	names map[string]any
}

func newWhere() *whereBuilder {
	return &whereBuilder{names: make(map[string]any)}
}

func (w *whereBuilder) add(cond string) {
	w.parts = append(w.parts, cond)
}

func (w *whereBuilder) arg(name string, value any) string {
	w.names[name] = value
	return "@" + name
}

func (w *whereBuilder) empty() bool { return len(w.parts) == 0 }

func (w *whereBuilder) clause() string {
	if w.empty() {
		return ""
	}
	return " WHERE " + strings.Join(w.parts, " AND ")
}

// escapeLike экранирует символы подстановки LIKE.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// buildSearchWhere собирает общее WHERE для поиска и подсчёта.
func buildSearchWhere(r SearchRequest) (*whereBuilder, string) {
	w := newWhere()
	if r.Kind != "" {
		w.add("d.kind_code = " + w.arg("f_kind", r.Kind))
	}
	if r.System != "" {
		w.add("d.system_code = " + w.arg("f_system", r.System))
	}
	if r.Query != "" {
		w.add("d.designation LIKE " + w.arg("f_query", "%"+escapeLike(r.Query)+"%") + ` ESCAPE '\'`)
	}
	if len(r.IDs) > 0 {
		parts := make([]string, len(r.IDs))
		for i, id := range r.IDs {
			parts[i] = w.arg(fmt.Sprintf("fid%d", i), id)
		}
		w.add("d.id IN (" + strings.Join(parts, ", ") + ")")
	}
	for i, f := range r.Fields {
		switch {
		case len(f.Texts) > 0:
			// Мультивыбор словарного поля (CC-11): IN-список именованными
			// аргументами самого конструктора WHERE — по пути фильтра id;
			// словари конечны, чанкование не требуется.
			parts := make([]string, len(f.Texts))
			for j, v := range f.Texts {
				parts[j] = w.arg(fmt.Sprintf("ft%d_%d", i, j), v)
			}
			w.add(fmt.Sprintf(
				`EXISTS (SELECT 1 FROM device_designation_fields df%d
WHERE df%d.device_id = d.id AND df%d.field = %s AND df%d.text_value IN (%s))`,
				i, i, i, w.arg(fmt.Sprintf("ff%d", i), f.Field), i, strings.Join(parts, ", ")))
		case f.HasNum:
			w.add(fmt.Sprintf(
				`EXISTS (SELECT 1 FROM device_designation_fields df%d
WHERE df%d.device_id = d.id AND df%d.field = %s AND df%d.num_value %s %s)`,
				i, i, i, w.arg(fmt.Sprintf("ff%d", i), f.Field), i, f.Op.sql(), w.arg(fmt.Sprintf("fn%d", i), f.Num)))
		default:
			w.add(fmt.Sprintf(
				`EXISTS (SELECT 1 FROM device_designation_fields df%d
WHERE df%d.device_id = d.id AND df%d.field = %s AND df%d.text_value = %s)`,
				i, i, i, w.arg(fmt.Sprintf("ff%d", i), f.Field), i, w.arg(fmt.Sprintf("ft%d", i), f.Text)))
		}
	}
	for i, a := range r.Attrs {
		if a.HasNum {
			w.add(fmt.Sprintf(
				`EXISTS (SELECT 1 FROM device_attribute_values av%d
WHERE av%d.device_id = d.id AND av%d.attribute_code = %s AND av%d.num_value %s %s)`,
				i, i, i, w.arg(fmt.Sprintf("ac%d", i), a.Code), i, a.Op.sql(), w.arg(fmt.Sprintf("an%d", i), a.Num)))
		} else {
			w.add(fmt.Sprintf(
				`EXISTS (SELECT 1 FROM device_attribute_values av%d
WHERE av%d.device_id = d.id AND av%d.attribute_code = %s AND av%d.text_value = %s)`,
				i, i, i, w.arg(fmt.Sprintf("ac%d", i), a.Code), i, w.arg(fmt.Sprintf("at%d", i), a.Text)))
		}
	}
	if len(r.Params) > 0 {
		w.add(buildParamScope(r.Params, w))
	}
	return w, w.clause()
}

// paramCondSQL — условие одного параметрового фильтра на псевдониме pvX:
// принадлежность параметру + равенство либо границы (границы применяются
// к value_min/value_max либо value_exact — точные значения участвуют
// в фильтрах границ).
func paramCondSQL(i int, p ParamCond, w *whereBuilder) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("pv%d.parameter_code = %s", i, w.arg(fmt.Sprintf("pcode%d", i), p.Code)))
	if p.HasMin {
		minArg := w.arg(fmt.Sprintf("pmin%d", i), p.Min)
		parts = append(parts, fmt.Sprintf("(pv%d.value_min >= %s OR pv%d.value_exact >= %s)", i, minArg, i, minArg))
	}
	if p.HasMax {
		maxArg := w.arg(fmt.Sprintf("pmax%d", i), p.Max)
		parts = append(parts, fmt.Sprintf("(pv%d.value_max <= %s OR pv%d.value_exact <= %s)", i, maxArg, i, maxArg))
	}
	if p.HasExact {
		parts = append(parts, fmt.Sprintf("pv%d.value_exact = %s", i, w.arg(fmt.Sprintf("pex%d", i), p.Exact)))
	}
	if p.HasText {
		parts = append(parts, fmt.Sprintf("pv%d.value_text = %s", i, w.arg(fmt.Sprintf("ptx%d", i), p.Text)))
	}
	return strings.Join(parts, " AND ")
}

// buildParamScope — вариантная семантика параметрических фильтров: все
// фильтры должны выполняться в пределах данных типа (variant_id IS NULL)
// либо одного исполнения вместе с данными типа; сочетание значений разных
// исполнений не допускается (R8).
func buildParamScope(params []ParamCond, w *whereBuilder) string {
	typeLevel := make([]string, 0, len(params))
	for i, p := range params {
		typeLevel = append(typeLevel, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM parameter_values pv%d
WHERE pv%d.device_id = d.id AND pv%d.variant_id IS NULL AND %s)`,
			i, i, i, paramCondSQL(i, p, w)))
	}
	variantLevel := make([]string, 0, len(params))
	for i, p := range params {
		variantLevel = append(variantLevel, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM parameter_values pv%d
WHERE (pv%d.variant_id = dv.id OR (pv%d.device_id = d.id AND pv%d.variant_id IS NULL)) AND %s)`,
			i, i, i, i, paramCondSQL(i, p, w)))
	}
	return "(" +
		"(" + strings.Join(typeLevel, " AND ") + ")" +
		" OR " +
		"EXISTS (SELECT 1 FROM device_variants dv WHERE dv.device_id = d.id AND " +
		strings.Join(variantLevel, " AND ") + ")" +
		")"
}

// orderJoinClause — соединение и сортировка по ключам: финальный ключ
// (kind, designation) обязателен (designation уникален только в пределах
// класса — без этого пагинация недетерминирована); NULL-значения полей —
// в конце независимо от направления (переносимо). Ключу сортировки по
// полю разбора соответствует LEFT JOIN device_designation_fields (PK
// (device_id, field) даёт не более одной строки — дублирования записей
// нет) вместо коррелированного подзапроса на каждую строку. Без ключей
// сортировки и с непустым набором ids первым ключом становится порядок
// передачи id (CASE d.id WHEN … THEN n — переносимо; финальный ключ не
// мешает — id уникальны), значения переиспользуют аргументы фильтра IN.
func orderJoinClause(sorts []SortKey, ids []int64, w *whereBuilder) (joins, order string) {
	var joinParts, orderParts []string
	if len(ids) > 0 && len(sorts) == 0 {
		whens := make([]string, len(ids))
		for i := range ids {
			whens[i] = fmt.Sprintf("WHEN @fid%d THEN %d", i, i)
		}
		orderParts = append(orderParts, "CASE d.id "+strings.Join(whens, " ")+" END")
	}
	for i, s := range sorts {
		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		switch s.Key {
		case "designation":
			orderParts = append(orderParts, "d.designation "+dir)
		case "id":
			orderParts = append(orderParts, "d.id "+dir)
		default:
			alias := fmt.Sprintf("so%d", i)
			joinParts = append(joinParts, fmt.Sprintf(
				"LEFT JOIN device_designation_fields %s ON %s.device_id = d.id AND %s.field = %s",
				alias, alias, alias, w.arg(fmt.Sprintf("sortkey%d", i), s.Key)))
			col := alias + ".text_value"
			if s.Numeric {
				col = alias + ".num_value"
			}
			orderParts = append(orderParts, "(CASE WHEN "+col+" IS NULL THEN 1 ELSE 0 END) ASC", col+" "+dir)
		}
	}
	orderParts = append(orderParts, "d.kind_code ASC", "d.designation ASC")
	return strings.Join(joinParts, " "), " ORDER BY " + strings.Join(orderParts, ", ")
}

// Search выполняет поиск записей: WHERE — общие условия, ORDER BY —
// сортировка с финальным (kind, designation), LIMIT/OFFSET — пагинация.
// Возвращает страницу и общее число записей под фильтрами одним
// оператором (COUNT(*) OVER() — оконная функция переносима: sqlite ≥ 3.25,
// postgres): total и страница — один снимок данных, тяжёлый WHERE
// вычисляется один раз. Пустая страница (фильтр без строк либо страница
// за концом выдачи) оконного COUNT не даёт — total добирается отдельным
// COUNT-запросом того же фильтра. Отрицательные limit/offset отвергаются
// доменной ошибкой — диалекты расходятся в их трактовке (postgres — ошибка
// протокола, sqlite молча «без лимита»).
func (d *DB) Search(ctx context.Context, r SearchRequest) ([]SearchItemRow, int, error) {
	if r.Limit < 0 || r.Offset < 0 {
		return nil, 0, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgStoragePageInvalid, r.Limit, r.Offset)
	}
	w, clause := buildSearchWhere(r)
	joins, order := orderJoinClause(r.Sorts, r.IDs, w)
	w.names["limit"] = r.Limit
	w.names["offset"] = r.Offset

	from := "FROM devices d"
	if joins != "" {
		from += " " + joins
	}
	query := `SELECT d.id, d.kind_code, d.system_code, d.designation, COUNT(*) OVER()
` + from + clause + order + ` LIMIT @limit OFFSET @offset`
	rows, err := d.query(ctx, query, w.names)
	if err != nil {
		return nil, 0, err
	}
	var out []SearchItemRow
	total := 0
	for rows.Next() {
		var it SearchItemRow
		var n int
		if err := rows.Scan(&it.ID, &it.Kind, &it.System, &it.Designation, &n); err != nil {
			rows.Close()
			return nil, 0, err
		}
		total = n
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()

	if len(out) == 0 {
		if err := d.queryRow(ctx,
			`SELECT COUNT(*) FROM devices d`+clause, w.names).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	if len(out) > 0 {
		if err := d.loadSearchFields(ctx, out); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

// loadSearchFields дозагружает поля разбора обозначений страницы результатов.
func (d *DB) loadSearchFields(ctx context.Context, items []SearchItemRow) error {
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	args := make(map[string]any, len(items))
	rows, err := d.query(ctx, `
SELECT device_id, field, text_value, num_value
FROM device_designation_fields WHERE device_id IN `+inList("sid", ids, args)+`
ORDER BY device_id, field`, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := make(map[int64][]anyField, len(items))
	for rows.Next() {
		var id int64
		var name string
		var text sql.NullString
		var num sql.NullFloat64
		if err := rows.Scan(&id, &name, &text, &num); err != nil {
			return err
		}
		f := anyField{Name: name}
		if text.Valid {
			s := text.String
			f.Text = &s
		}
		if num.Valid {
			v := num.Float64
			f.Num = &v
		}
		byID[id] = append(byID[id], f)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		if fs, ok := byID[items[i].ID]; ok {
			items[i].Fields = fs
		}
	}
	return nil
}

// anyField — поле разбора с nullable-значениями (для результатов поиска).
type anyField struct {
	Name string
	Text *string
	Num  *float64
}

// SuggestPrefix — автодополнение по префиксу обозначения (каноническому);
// kind — необязательный фильтр классом.
func (d *DB) SuggestPrefix(ctx context.Context, prefix string, kind string, limit int) ([]SearchItemRow, error) {
	args := map[string]any{"p": escapeLike(prefix) + "%", "limit": limit}
	q := `SELECT d.id, d.kind_code, d.system_code, d.designation FROM devices d
WHERE d.designation LIKE @p ESCAPE '\'`
	if kind != "" {
		q += ` AND d.kind_code = @kind`
		args["kind"] = kind
	}
	q += ` ORDER BY d.kind_code, d.designation LIMIT @limit`
	rows, err := d.query(ctx, q, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchItemRow
	for rows.Next() {
		var it SearchItemRow
		if err := rows.Scan(&it.ID, &it.Kind, &it.System, &it.Designation); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
