package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Источники данных записи (план 03 §13, D13): реестр источников
// (библиографическое описание — редактируемые данные, не каталожные
// строки), лица (авторы/редакторы) и ссылки на документацию — по строке
// на элемент, ссылки записей — с заметкой места в источнике. Чистка
// сирот — по образцу производителей (адресная, после замены/удаления).

// SourceRow — строка реестра sources вместе с лицами и ссылками
// (Year/Publisher/Edition/ISBN nil/"" — не задано; Authors/Editors/URLs —
// упорядоченные списки, nil — список не задан).
type SourceRow struct {
	ID        int64
	Code      string
	Title     string
	Type      string
	Year      *int
	Publisher string
	Edition   string
	ISBN      string
	Authors   []string
	Editors   []string
	URLs      []string
}

// SourceLinkRow — ссылка записи на источник: код реестра и заметка места
// в источнике (слитое состояние Upsert; порядок — по коду, канонический).
type SourceLinkRow struct {
	Code string
	Note string
}

// DeviceSourceRef — ссылка записи на источник для применения: id строки
// реестра и заметка; порядок среза — sort_order.
type DeviceSourceRef struct {
	SourceID int64
	Note     string
}

// DeviceSourceRow — ссылка записи на источник с полным определением
// (карточка, экспорт): Note — место в источнике, порядок — sort_order.
type DeviceSourceRow struct {
	SourceRow
	Note string
}

// LoadSources читает весь реестр источников с лицами и ссылками в
// порядке кода (одно соединение чтения; служебное чтение и экспорт).
func (d *DB) LoadSources(ctx context.Context) ([]SourceRow, error) {
	tx, err := d.BeginReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck — чтение без побочных эффектов
	rows, err := queryContext(ctx, tx.tx, `SELECT id, code FROM sources ORDER BY code`, nil)
	if err != nil {
		return nil, err
	}
	type idCode struct {
		id   int64
		code string
	}
	var keys []idCode
	for rows.Next() {
		var k idCode
		if err := rows.Scan(&k.id, &k.code); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]SourceRow, 0, len(keys))
	bySource := make(map[int64]*SourceRow, len(keys))
	for _, k := range keys {
		r := SourceRow{ID: k.id, Code: k.code}
		bySource[k.id] = &r
		out = append(out, r)
	}
	// Дозаполнение полей и списков: указатели в bySource указывают
	// на элементы out — после последнего append срез не растёт.
	if err := tx.loadSourceRows(ctx, bySource); err != nil {
		return nil, err
	}
	for i := range out {
		src := bySource[out[i].ID]
		out[i] = *src
	}
	return out, nil
}

// sourceSelectCols — колонки чтения определения источника (id, код
// и скалярные поля; лица и ссылки читаются отдельно по id): порядок
// колонок закреплён сканером scanSourceRow — все читатели реестра
// собирают строку одной точкой.
const sourceSelectCols = "id, code, title, type_code, year, publisher, edition, isbn"

// scanSourceRow сканирует строку реестра в порядке sourceSelectCols:
// Null-обработка года и текстовых полей — в одном месте; extra —
// завершающие цели скана вызывающего (оконный total и т.п.).
func scanSourceRow(sc interface{ Scan(dest ...any) error }, r *SourceRow, extra ...any) error {
	var year sql.NullInt64
	var publisher, edition, isbn sql.NullString
	dest := []any{&r.ID, &r.Code, &r.Title, &r.Type,
		&year, &publisher, &edition, &isbn}
	if err := sc.Scan(append(dest, extra...)...); err != nil {
		return err
	}
	if year.Valid {
		y := int(year.Int64)
		r.Year = &y
	}
	r.Publisher = nullableStr(publisher)
	r.Edition = nullableStr(edition)
	r.ISBN = nullableStr(isbn)
	return nil
}

// LoadSourcePage читает страницу реестра источников в порядке кода
// с общим числом строк под фильтром (по образцу поиска записей):
// total и страница — один оператор с оконным COUNT(*) OVER(), пустая
// страница (фильтр без строк либо страница за концом выдачи) добирается
// отдельным COUNT-запросом того же фильтра. q — подстрока по коду либо
// названию, регистронезависимо в пределах возможностей диалекта
// (postgres — все алфавиты; sqlite — латиница, кириллица в точном
// регистре запасной веткой). Символы подстановки LIKE в q
// экранируются. Лица и ссылки читаются только по строкам страницы.
func (d *DB) LoadSourcePage(ctx context.Context, q string, limit, offset int) ([]SourceRow, int, error) {
	if limit < 0 || offset < 0 {
		return nil, 0, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgStoragePageInvalid, limit, offset)
	}
	tx, err := d.BeginReadOnly(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck — чтение без побочных эффектов
	args := map[string]any{"limit": limit, "offset": offset}
	where := ""
	if q != "" {
		// Три ветки предиката: LOWER с обеих сторон даёт полную
		// регистронезависимость на postgres (свёртка всех алфавитов)
		// и латиницу на sqlite; точный регистр запасной веткой —
		// кириллические названия на sqlite (LOWER сворачивает только
		// латиницу) ищутся в точном регистре, а не пропадают вовсе.
		args["pat"] = "%" + escapeLike(strings.ToLower(q)) + "%"
		args["raw"] = "%" + escapeLike(q) + "%"
		where = ` WHERE (LOWER(code) LIKE @pat ESCAPE '\' OR LOWER(title) LIKE @pat ESCAPE '\' OR title LIKE @raw ESCAPE '\')`
	}
	rows, err := tx.query(ctx, "SELECT "+sourceSelectCols+`, COUNT(*) OVER()
FROM sources`+where+` ORDER BY code LIMIT @limit OFFSET @offset`, args)
	if err != nil {
		return nil, 0, err
	}
	var out []SourceRow
	bySource := make(map[int64]*SourceRow)
	total := 0
	for rows.Next() {
		var r SourceRow
		if err := scanSourceRow(rows, &r, &total); err != nil {
			rows.Close()
			return nil, 0, err
		}
		// Независимая копия строки в map: списки дозаполняются в map,
		// затем значения копируются обратно в out по id.
		bySource[r.ID] = &r
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	if len(out) == 0 {
		if err := tx.queryRow(ctx,
			`SELECT COUNT(*) FROM sources`+where, args).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	if len(bySource) > 0 {
		if err := tx.loadSourceLists(ctx, sourceIDsOf(bySource), bySource); err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i] = *bySource[out[i].ID]
		}
	}
	return out, total, nil
}

// LoadSourcesByCodes читает определения источников по списку кодов
// (пакетное чтение меток селекторов по образцу карточек по id):
// повтор кода входа игнорируется (учитывается первое вхождение),
// порядок результата — порядок кодов входа, неизвестный код — дырка
// (пропуск). SELECT по чанкам списка (потолок аргументов одного
// оператора), лица и ссылки — одним проходом по вовлечённым строкам.
func (d *DB) LoadSourcesByCodes(ctx context.Context, codes []string) ([]SourceRow, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(codes))
	uniq := make([]string, 0, len(codes))
	for _, code := range codes {
		if seen[code] {
			continue
		}
		seen[code] = true
		uniq = append(uniq, code)
	}
	tx, err := d.BeginReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck — чтение без побочных эффектов
	byCode := make(map[string]*SourceRow, len(uniq))
	bySource := make(map[int64]*SourceRow, len(uniq))
	for start := 0; start < len(uniq); start += maxInList {
		end := min(start+maxInList, len(uniq))
		chunk := uniq[start:end]
		args := make(map[string]any, len(chunk))
		rows, err := tx.query(ctx, "SELECT "+sourceSelectCols+`
FROM sources WHERE code IN `+inTextList("code", chunk, args), args)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r SourceRow
			if err := scanSourceRow(rows, &r); err != nil {
				rows.Close()
				return nil, err
			}
			// Строка живёт в map как независимая копия (r — переменная
			// итерации): списки дозаполняются в map, финальная сборка
			// читает значения по кодам входа.
			byCode[r.Code] = &r
			bySource[r.ID] = &r
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	if len(bySource) > 0 {
		if err := tx.loadSourceLists(ctx, sourceIDsOf(bySource), bySource); err != nil {
			return nil, err
		}
	}
	out := make([]SourceRow, 0, len(uniq))
	for _, code := range uniq {
		if r, ok := byCode[code]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

// loadSourceRows читает скалярные поля источников по id и дозаполняет
// списки (лица, ссылки).
func (t *Tx) loadSourceRows(ctx context.Context, bySource map[int64]*SourceRow) error {
	ids := make([]int64, 0, len(bySource))
	for id := range bySource {
		ids = append(ids, id)
	}
	err := eachIDChunk(ids, func(chunk []int64) error {
		args := make(map[string]any, len(chunk))
		rows, err := t.query(ctx, `
SELECT id, code, title, type_code, year, publisher, edition, isbn
FROM sources WHERE id IN `+inList("id", chunk, args), args)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r SourceRow
			if err := scanSourceRow(rows, &r); err != nil {
				return err
			}
			// Скалярные поля перезаписывают строку-владельца в map
			// (Code — то же значение, что у владельца из LoadSources).
			if dst, ok := bySource[r.ID]; ok {
				*dst = r
			}
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	return t.loadSourceLists(ctx, ids, bySource)
}

// Чтение.

// FindSourceID ищет строку реестра по коду (разрешение ссылок записей);
// false — код не найден.
func (t *Tx) FindSourceID(ctx context.Context, code string) (int64, bool, error) {
	var id int64
	err := t.queryRow(ctx, `SELECT id FROM sources WHERE code = @code`,
		map[string]any{"code": code}).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// LoadSourceByCode читает определение источника по коду с лицами
// и ссылками; nil — код не найден.
func (t *Tx) LoadSourceByCode(ctx context.Context, code string) (*SourceRow, error) {
	row := t.queryRow(ctx, "SELECT "+sourceSelectCols+`
FROM sources WHERE code = @code`,
		map[string]any{"code": code})
	var r SourceRow
	if err := scanSourceRow(row, &r); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := t.loadSourceLists(ctx, []int64{r.ID}, map[int64]*SourceRow{r.ID: &r}); err != nil {
		return nil, err
	}
	return &r, nil
}

// LoadDeviceSources — ссылки записи на источники в каноническом порядке
// (по коду — сравнение состояний не зависит от порядка чтения).
func (t *Tx) LoadDeviceSources(ctx context.Context, deviceID int64) ([]SourceLinkRow, error) {
	rows, err := t.query(ctx, `
SELECT s.code, ds.note
FROM device_sources ds
JOIN sources s ON s.id = ds.source_id
WHERE ds.device_id = @id
ORDER BY s.code`,
		map[string]any{"id": deviceID})
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck — чтение завершилось
	out := make([]SourceLinkRow, 0, 4)
	for rows.Next() {
		var r SourceLinkRow
		var note sql.NullString
		if err := rows.Scan(&r.Code, &note); err != nil {
			return nil, err
		}
		r.Note = nullableStr(note)
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadDeviceSourcesByIDs — ссылки записей на источники с полными
// определениями (пакетное чтение карточек): SELECT по чанкам списка
// записей (повторяющиеся id входа игнорируются — учитывается первое
// вхождение), лица и ссылки — одним проходом по вовлечённым источникам;
// порядок внутри записи — как у LoadDeviceSources.
func (t *Tx) LoadDeviceSourcesByIDs(ctx context.Context, ids []int64) (map[int64][]DeviceSourceRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Ссылки собираются во временный список: указатели на элементы
	// out[devID] между append'ами нестабильны (переразмещение), поэтому
	// карты определений строятся после финальной сборки слайсов.
	type link struct {
		devID int64
		row   DeviceSourceRow
	}
	var links []link
	err := eachIDChunk(ids, func(chunk []int64) error {
		args := make(map[string]any, len(chunk))
		rows, err := t.query(ctx, `
SELECT ds.device_id, s.id, s.code, s.title, s.type_code, s.year,
       s.publisher, s.edition, s.isbn, ds.note
FROM device_sources ds
JOIN sources s ON s.id = ds.source_id
WHERE ds.device_id IN `+inList("id", chunk, args)+`
ORDER BY ds.device_id, s.code`, args)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l link
			var year sql.NullInt64
			var publisher, edition, isbn, note sql.NullString
			if err := rows.Scan(&l.devID, &l.row.ID, &l.row.Code, &l.row.Title, &l.row.Type,
				&year, &publisher, &edition, &isbn, &note); err != nil {
				return err
			}
			if year.Valid {
				y := int(year.Int64)
				l.row.Year = &y
			}
			l.row.Publisher = nullableStr(publisher)
			l.row.Edition = nullableStr(edition)
			l.row.ISBN = nullableStr(isbn)
			l.row.Note = nullableStr(note)
			links = append(links, l)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}
	out := make(map[int64][]DeviceSourceRow, len(ids))
	for _, l := range links {
		out[l.devID] = append(out[l.devID], l.row)
	}
	bySource := make(map[int64]*SourceRow, len(links))
	for devID := range out {
		rows := out[devID]
		for i := range rows {
			bySource[rows[i].ID] = &rows[i].SourceRow
		}
	}
	if err := t.loadSourceLists(ctx, sourceIDsOf(bySource), bySource); err != nil {
		return nil, err
	}
	// Источник, общий для нескольких записей партии, представлен
	// отдельной копией строки в каждой: loadSourceLists заполняет списки
	// только в одну копию карты — раздаём заполненное всем копиям
	// (образец copy-back — LoadSources).
	for devID := range out {
		rows := out[devID]
		for i := range rows {
			if src, ok := bySource[rows[i].ID]; ok {
				rows[i].Authors, rows[i].Editors, rows[i].URLs = src.Authors, src.Editors, src.URLs
			}
		}
	}
	return out, nil
}

// sourceIDsOf — id вовлечённых источников одним срезом.
func sourceIDsOf(bySource map[int64]*SourceRow) []int64 {
	ids := make([]int64, 0, len(bySource))
	for id := range bySource {
		ids = append(ids, id)
	}
	return ids
}

// loadSourceLists дозаполняет лица и ссылки источников по списку id:
// строки упорядочены по (source_id, pos), порядок внутри списка значим.
func (t *Tx) loadSourceLists(ctx context.Context, ids []int64, bySource map[int64]*SourceRow) error {
	if len(ids) == 0 {
		return nil
	}
	err := eachIDChunk(ids, func(chunk []int64) error {
		args := make(map[string]any, len(chunk))
		rows, err := t.query(ctx, `
SELECT source_id, role, name FROM source_persons
WHERE source_id IN `+inList("id", chunk, args)+`
ORDER BY source_id, pos`, args)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var role, name string
			if err := rows.Scan(&id, &role, &name); err != nil {
				return err
			}
			r, ok := bySource[id]
			if !ok {
				continue
			}
			if role == "editor" {
				r.Editors = append(r.Editors, name)
				continue
			}
			r.Authors = append(r.Authors, name)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	return eachIDChunk(ids, func(chunk []int64) error {
		args := make(map[string]any, len(chunk))
		rows, err := t.query(ctx, `
SELECT source_id, url FROM source_urls
WHERE source_id IN `+inList("id", chunk, args)+`
ORDER BY source_id, pos`, args)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var u string
			if err := rows.Scan(&id, &u); err != nil {
				return err
			}
			if r, ok := bySource[id]; ok {
				r.URLs = append(r.URLs, u)
			}
		}
		return rows.Err()
	})
}

// Запись (только внутри транзакции).

// InsertSource вставляет определение источника (строку реестра, лица
// и ссылки) и возвращает id с признаком вставки; конфликт уникальности
// кода при параллельном писателе подавляется атомарно — определение
// конкурента не перезаписывается, id перечитывается по коду, inserted =
// false (строка, не видная перечитыванию, — доменная ошибка, как у
// производителей).
func (t *Tx) InsertSource(ctx context.Context, r SourceRow) (int64, bool, error) {
	id, inserted, err := t.InsertIfAbsentReturningID(ctx, `
INSERT INTO sources(code, title, type_code, year, publisher, edition, isbn)
VALUES (@code, @title, @type, @year, @publisher, @edition, @isbn)`,
		map[string]any{
			"code": r.Code, "title": r.Title, "type": r.Type, "year": r.Year,
			"publisher": nilIfEmpty(r.Publisher), "edition": nilIfEmpty(r.Edition),
			"isbn": nilIfEmpty(r.ISBN),
		})
	if err != nil {
		return 0, false, err
	}
	if !inserted {
		err = t.queryRow(ctx, `SELECT id FROM sources WHERE code = @code`,
			map[string]any{"code": r.Code}).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, domain.NewErrorf(domain.CodeInternal, domain.MsgStorageRereadMissing,
				"sources(code = "+r.Code+")")
		}
		if err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	if err := t.writeSourceLists(ctx, id, r); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// UpdateSource обновляет поля строки реестра; лица и ссылки замещаются
// целиком (upsert определения по коду — план 03 §13).
func (t *Tx) UpdateSource(ctx context.Context, id int64, r SourceRow) error {
	if _, err := t.exec(ctx, `
UPDATE sources SET title = @title, type_code = @type, year = @year,
    publisher = @publisher, edition = @edition, isbn = @isbn
WHERE id = @id`,
		map[string]any{
			"id": id, "title": r.Title, "type": r.Type, "year": r.Year,
			"publisher": nilIfEmpty(r.Publisher), "edition": nilIfEmpty(r.Edition),
			"isbn": nilIfEmpty(r.ISBN),
		}); err != nil {
		return err
	}
	if _, err := t.exec(ctx, `DELETE FROM source_persons WHERE source_id = @id`,
		map[string]any{"id": id}); err != nil {
		return err
	}
	if _, err := t.exec(ctx, `DELETE FROM source_urls WHERE source_id = @id`,
		map[string]any{"id": id}); err != nil {
		return err
	}
	return t.writeSourceLists(ctx, id, r)
}

// writeSourceLists пишет лица и ссылки источника после очистки: pos —
// сквозной порядок списка (авторы, затем редакторы) и ссылок.
func (t *Tx) writeSourceLists(ctx context.Context, id int64, r SourceRow) error {
	pos := 0
	for _, name := range r.Authors {
		if _, err := t.InsertIfAbsent(ctx, `
INSERT INTO source_persons(source_id, pos, role, name)
VALUES (@id, @pos, 'author', @name)`,
			map[string]any{"id": id, "pos": pos, "name": name}); err != nil {
			return err
		}
		pos++
	}
	for _, name := range r.Editors {
		if _, err := t.InsertIfAbsent(ctx, `
INSERT INTO source_persons(source_id, pos, role, name)
VALUES (@id, @pos, 'editor', @name)`,
			map[string]any{"id": id, "pos": pos, "name": name}); err != nil {
			return err
		}
		pos++
	}
	for i, u := range r.URLs {
		if _, err := t.InsertIfAbsent(ctx, `
INSERT INTO source_urls(source_id, pos, url)
VALUES (@id, @pos, @url)`,
			map[string]any{"id": id, "pos": i, "url": u}); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceDeviceSources заменяет ссылки записи на источники целиком
// (секция sources записи): связи пересоздаются, источники этой записи,
// оставшиеся без единой связи, удаляются (адресная чистка сирот —
// по образцу производителей; индекс idx_device_sources_source держит
// проверку «без связей» точечной). Строки, вставленные параллельным
// писателем в той же записи и не видные DELETE этого вызова,
// сохраняются.
func (t *Tx) ReplaceDeviceSources(ctx context.Context, deviceID int64, refs []DeviceSourceRef) error {
	old, err := t.sourcesOf(ctx, deviceID)
	if err != nil {
		return err
	}
	if _, err := t.exec(ctx,
		`DELETE FROM device_sources WHERE device_id = @id`,
		map[string]any{"id": deviceID}); err != nil {
		return err
	}
	linked := make(map[int64]bool, len(refs))
	for _, ref := range refs {
		if _, err := t.InsertIfAbsent(ctx, `
INSERT INTO device_sources(device_id, source_id, note, sort_order)
VALUES (@id, @source, @note, @sort)`,
			map[string]any{
				"id": deviceID, "source": ref.SourceID,
				"note": nilIfEmpty(ref.Note), "sort": len(linked),
			}); err != nil {
			return err
		}
		linked[ref.SourceID] = true
	}
	candidates := make([]int64, 0, len(old))
	for _, sid := range old {
		if !linked[sid] {
			candidates = append(candidates, sid)
		}
	}
	return t.deleteOrphanSources(ctx, candidates)
}

// sourcesOf возвращает id источников записи до замены/удаления —
// кандидаты адресной чистки сирот.
func (t *Tx) sourcesOf(ctx context.Context, deviceID int64) ([]int64, error) {
	rows, err := t.query(ctx,
		`SELECT source_id FROM device_sources WHERE device_id = @id`,
		map[string]any{"id": deviceID})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			return nil, err
		}
		out = append(out, sid)
	}
	return out, rows.Err()
}

// deleteOrphanSources удаляет перечисленные источники без единой связи:
// адресная проверка NOT EXISTS по индексу device_sources(source_id),
// без полного анти-джойна таблицы; лица и ссылки уходят каскадом FK.
func (t *Tx) deleteOrphanSources(ctx context.Context, ids []int64) error {
	seen := make(map[int64]bool, len(ids))
	for _, sid := range ids {
		if seen[sid] {
			continue
		}
		seen[sid] = true
		if _, err := t.exec(ctx, `
DELETE FROM sources
WHERE id = @id AND NOT EXISTS (
    SELECT 1 FROM device_sources ds WHERE ds.source_id = @id)`,
			map[string]any{"id": sid}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSourceByCode удаляет строку реестра по коду, если на неё нет
// ссылок записей: DELETE с NOT EXISTS атомарен — ссылка, добавленная
// параллельным писателем между чтением и удалением, блокирует удаление
// (как адресная чистка сирот). Классификация отказа проверяет сами
// связи, а не существование строки: на postgres (READ COMMITTED)
// каждый оператор видит свежий снимок — между DELETE и проверкой
// параллельный писатель может закоммитить новый несвязанный источник,
// и существование строки не доказывает связанности. Повтор DELETE
// закрывает это окно; остаточный случай (строка появилась между
// повтором и его проверкой связей) классифицируется как отсутствие —
// безвредный 404, повтор запроса удаляет. Лица и ссылки уходят
// каскадом FK. Контракт возврата: deleted — строка удалена;
// deleted=false, inUse — источник связан записями (отказ вызывающего
// уровня); deleted=false, !inUse — кода в реестре нет.
func (t *Tx) DeleteSourceByCode(ctx context.Context, code string) (deleted, inUse bool, err error) {
	for range 2 {
		res, err := t.exec(ctx, `
DELETE FROM sources
WHERE code = @code AND NOT EXISTS (
    SELECT 1 FROM device_sources ds WHERE ds.source_id = sources.id)`,
			map[string]any{"code": code})
		if err != nil {
			return false, false, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return false, false, err
		}
		if n > 0 {
			return true, false, nil
		}
		linked, err := t.sourceLinked(ctx, code)
		if err != nil {
			return false, false, err
		}
		if linked {
			return false, true, nil
		}
		// Не удалён и не связан: строки либо нет, либо она появилась
		// после DELETE (postgres) — повтор закрывает окно.
	}
	return false, false, nil
}

// sourceLinked — есть ли у источника с кодом хотя бы одна ссылка записи
// (индекс device_sources(source_id) держит проверку точечной).
func (t *Tx) sourceLinked(ctx context.Context, code string) (bool, error) {
	var one int
	err := t.queryRow(ctx, `
SELECT 1 FROM sources s
WHERE s.code = @code AND EXISTS (
    SELECT 1 FROM device_sources ds WHERE ds.source_id = s.id)`,
		map[string]any{"code": code}).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
