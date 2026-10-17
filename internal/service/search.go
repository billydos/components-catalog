package service

import (
	"context"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// CatalogService — операции с каталогом: снимок для UI и валидации,
// импорт (расширение каталога — план 02 §5.4). Экспорт — этап 4
// (реестр форматов наполнения).
type CatalogService struct {
	app *App
}

// Snapshot возвращает актуальный снимок каталога (кэш с проверкой
// catalog_revision на каждом обращении — смена каталога другим процессом
// видна без перезапуска).
func (s *CatalogService) Snapshot(ctx context.Context) (*catalog.Snapshot, error) {
	return s.app.cache.Snapshot(ctx)
}

// Import применяет каталог upsert'ом по коду после валидации метасхемы
// (ApplyCatalog: вставка новой строки либо обновление полей; enum-значения,
// наборы условий и применимость параметров/атрибутов к классам замещаются
// целиком по строке; привязки систем и правил к классам — только
// добавление; непустой список проблем — отказ целиком). Изменение каталога
// инкрементирует catalog_revision атомарно в той же транзакции.
func (s *CatalogService) Import(ctx context.Context, in catalog.Input) error {
	cur, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return err
	}
	if _, problems := catalog.ApplyCatalog(cur, in); len(problems) > 0 {
		return problems[0].Err()
	}
	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	if err := tx.WriteCatalog(ctx, in); err != nil {
		return err
	}
	if err := tx.BumpCatalogRevision(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.app.cache.Invalidate()
	return nil
}

// Search выполняет поиск: подстрока обозначения, фильтр по набору id
// (пересечение с прочими фильтрами; без сортировки — порядок передачи
// id), фильтры полей/атрибутов/параметров (применимость к классу
// запроса — D7, ошибки валидации запроса), сортировка с финальным
// (kind, designation), колонки значений (пакетная проекция
// значений параметров/атрибутов в строки выдачи), пагинация
// (limit ≤ 200); вариантные параметрические фильтры применяются
// в пределах одного исполнения
// (docs/plan/04-module-functionality.md §1.1).
func (s *DeviceService) Search(ctx context.Context, q SearchQuery) (SearchPage, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return SearchPage{}, err
	}
	req, err := buildSearchRequest(snap, q)
	if err != nil {
		return SearchPage{}, err
	}
	rows, total, err := s.app.db.Search(ctx, req)
	if err != nil {
		return SearchPage{}, err
	}
	page := SearchPage{
		Items:  make([]SearchItem, 0, len(rows)),
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
	}
	for _, r := range rows {
		item := SearchItem{
			ID:          r.ID,
			Kind:        domain.Kind(r.Kind),
			System:      domain.System(r.System),
			Designation: r.Designation,
		}
		for _, f := range r.Fields {
			if f.Text != nil {
				item.Fields = append(item.Fields, domain.TextField(f.Name, *f.Text))
			} else if f.Num != nil {
				item.Fields = append(item.Fields, domain.NumField(f.Name, *f.Num))
			}
		}
		page.Items = append(page.Items, item)
	}
	if len(q.Values) > 0 && len(page.Items) > 0 {
		if err := s.attachSearchValues(ctx, snap, q.Values, page.Items); err != nil {
			return SearchPage{}, err
		}
	}
	return page, nil
}

// attachSearchValues — пакетная проекция колонок значений в строки
// страницы: одним чтением по id страницы × запрошенные коды (без сборки
// карточек целиком — N+1 внутри модуля не возникает), в транзакции чтения
// (снимок един для чтений значений и атрибутов проекции; сама страница
// ищется db.Search вне её — как у карточек, конкурентная запись меняет
// данные между поиском и проекцией не атомарно).
func (s *DeviceService) attachSearchValues(ctx context.Context, snap *catalog.Snapshot,
	cols []ValueColumn, items []SearchItem) error {
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	paramCodes := make([]string, 0, len(cols))
	attrCodes := make([]string, 0, len(cols))
	for _, col := range cols {
		if col.Attr {
			attrCodes = append(attrCodes, col.Code)
		} else {
			paramCodes = append(paramCodes, col.Code)
		}
	}
	tx, err := s.app.db.BeginReadOnly(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	var values map[int64][]storage.ValueRow
	if len(paramCodes) > 0 {
		if values, err = tx.LoadValuesByIDsCodes(ctx, ids, paramCodes); err != nil {
			return err
		}
	}
	var attrs map[int64][]storage.AttrRow
	if len(attrCodes) > 0 {
		if attrs, err = tx.LoadAttributesByIDsCodes(ctx, ids, attrCodes); err != nil {
			return err
		}
	}
	for i := range items {
		items[i].Values = projectSearchValues(snap, cols, values[items[i].ID], attrs[items[i].ID])
	}
	return nil
}

// projectSearchValues собирает значения запрошенных колонок одной записи
// в порядке запроса (нет значения — нет элемента). Правило выбора значения
// при нескольких строках у записи (данные типа + исполнения, повторные
// строки) — paramSearchValue; атрибут — text/exact-num по типу каталога.
func projectSearchValues(snap *catalog.Snapshot, cols []ValueColumn,
	values []storage.ValueRow, attrs []storage.AttrRow) []SearchValue {
	out := make([]SearchValue, 0, len(cols))
	for _, col := range cols {
		if col.Attr {
			if v, ok := attrSearchValue(col, snap, attrs); ok {
				out = append(out, v)
			}
			continue
		}
		def, ok := snap.Parameter(col.Code)
		if !ok {
			continue // неизвестные каталогу не проектируются (снимок един с валидацией)
		}
		var rows []storage.ValueRow
		for _, vr := range values {
			if vr.Value.Parameter == col.Code {
				rows = append(rows, vr)
			}
		}
		if len(rows) == 0 {
			continue
		}
		out = append(out, SearchValue{Column: col, Value: paramSearchValue(def, rows)})
	}
	return out
}

// paramSearchValue — значение параметра записи для колонки: единственная
// строка — как есть, с условиями измерения (тултип карточки); несколько —
// численные агрегируются границами MIN/MAX (семантика фасетов
// и сортировки: нижняя — MIN нижних границ строк, верхняя — MAX
// верхних, точное значение участвует в обеих), текстовые/enum — данные
// типа поверх исполнений (порядок строк: тип в целом, затем исполнения),
// среди уровня — первая строка; у агрегата смешанного происхождения
// условия не показываются.
func paramSearchValue(def *catalog.ParameterDef, rows []storage.ValueRow) CardValue {
	if !def.ValueType.Numeric() {
		for _, vr := range rows {
			if vr.VariantID == nil && vr.Value.Text != nil {
				return cardValueOf(def, vr)
			}
		}
		for _, vr := range rows {
			if vr.Value.Text != nil {
				return cardValueOf(def, vr)
			}
		}
		return CardValue{}
	}
	if len(rows) == 1 {
		return cardValueOf(def, rows[0])
	}
	v := CardValue{Parameter: def.Code, Unit: def.Unit}
	for _, vr := range rows {
		if lower := coalesceLower(vr.Value); lower != nil && (v.Min == nil || *lower < *v.Min) {
			v.Min = lower
		}
		if upper := coalesceUpper(vr.Value); upper != nil && (v.Max == nil || *upper > *v.Max) {
			v.Max = upper
		}
	}
	return v
}

// coalesceLower — нижняя граница строки значений: value_min, точное
// значение либо единственная верхняя (правило выбора значения).
func coalesceLower(v storage.ParamValue) *float64 {
	if v.Min != nil {
		return v.Min
	}
	if v.Exact != nil {
		return v.Exact
	}
	return v.Max
}

// coalesceUpper — верхняя граница: value_max, точное значение либо
// единственная нижняя.
func coalesceUpper(v storage.ParamValue) *float64 {
	if v.Max != nil {
		return v.Max
	}
	if v.Exact != nil {
		return v.Exact
	}
	return v.Min
}

// cardValueOf — значение строки параметра формы CardValue (единица —
// из определения параметра, условия — строки).
func cardValueOf(def *catalog.ParameterDef, vr storage.ValueRow) CardValue {
	return CardValue{
		Parameter:  vr.Value.Parameter,
		Unit:       def.Unit,
		Exact:      vr.Value.Exact,
		Min:        vr.Value.Min,
		Max:        vr.Value.Max,
		Text:       vr.Value.Text,
		Conditions: condsFromStorage(vr.Value.Conditions),
	}
}

// attrSearchValue — значение атрибута записи для колонки: text/enum —
// текст, int/number — точное число, name/unit — из определения (логические
// отвергаются валидацией запроса); у записи не более одной строки.
func attrSearchValue(col ValueColumn, snap *catalog.Snapshot, attrs []storage.AttrRow) (SearchValue, bool) {
	def, ok := snap.Attribute(col.Code)
	if !ok {
		return SearchValue{}, false
	}
	for _, a := range attrs {
		if a.Attribute != col.Code {
			continue
		}
		return SearchValue{Column: col, Value: CardValue{
			Parameter: col.Code, Unit: def.Unit, Text: a.Text, Exact: a.Num,
		}}, true
	}
	return SearchValue{}, false
}

// ValidateSearch проверяет поисковый запрос по каталогу (класс, система,
// фильтры атрибутов/параметров и их применимость) без обращения к БД:
// условное кэширование транспорта (304 по ETag) обязано отвечать
// невалидному запросу 400 до отсечения — валидация дешевле выполнения.
func (s *DeviceService) ValidateSearch(ctx context.Context, q SearchQuery) error {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return err
	}
	_, err = buildSearchRequest(snap, q)
	return err
}

// buildSearchRequest валидирует запрос по каталогу и собирает транспортный
// запрос хранилища.
func buildSearchRequest(snap *catalog.Snapshot, q SearchQuery) (storage.SearchRequest, error) {
	req := storage.SearchRequest{
		Kind:   string(q.Kind),
		System: string(q.System),
		Query:  normalizeQuery(q.Query),
		Limit:  q.Limit,
		Offset: q.Offset,
	}
	if req.Limit <= 0 {
		req.Limit = SearchDefaultLimit
	}
	if req.Limit > SearchLimitMax {
		req.Limit = SearchLimitMax
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	if len(q.IDs) > SearchIDsMax {
		return req, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgSvcSearchIDMax, SearchIDsMax)
	}
	seenIDs := make(map[int64]struct{}, len(q.IDs))
	for _, id := range q.IDs {
		if id < 1 {
			return req, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSearchIDPositive, id)
		}
		if _, dup := seenIDs[id]; dup {
			return req, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSearchIDDuplicate, id)
		}
		seenIDs[id] = struct{}{}
	}
	req.IDs = q.IDs
	if q.Kind != "" {
		if _, ok := snap.Kind(q.Kind); !ok {
			return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgKindUnknown, string(q.Kind))
		}
	}
	if q.System != "" {
		if _, ok := snap.System(q.System); !ok {
			return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSystemUnknown, string(q.System))
		}
	}
	for _, f := range q.Fields {
		if f.Field == "" {
			return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterFieldMissing)
		}
		if !domain.KnownDesignationField(f.Field) {
			return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSearchFieldUnknown, f.Field)
		}
		if err := knownFilterOp(f.Op); err != nil {
			return req, err
		}
		// Мультивыбор словарного поля: набор Texts — семантика
		// ИЛИ внутри одного фильтра; дубликат значения набора — ошибка
		// валидации (как у набора id). Транспорты канонизируют значения
		// к кодам словаря до сервисного слоя; набор приоритетнее
		// одиночного Text.
		seenTexts := make(map[string]struct{}, len(f.Texts))
		for _, v := range f.Texts {
			if _, dup := seenTexts[v]; dup {
				return req, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgSvcSearchValueDuplicate, f.Field, v)
			}
			seenTexts[v] = struct{}{}
		}
		req.Fields = append(req.Fields, storage.FieldCond{
			Field: f.Field, Text: f.Text, Texts: f.Texts, Num: f.Num,
			HasNum: f.HasNum, Op: storage.FilterOp(f.Op),
		})
	}
	for _, a := range q.Attributes {
		def, ok := snap.Attribute(a.Attribute)
		if !ok {
			return req, domain.NewErrorf(domain.CodeUnknownAttribute, domain.MsgEngineAttrUnknown, a.Attribute)
		}
		if q.Kind != "" && !def.AppliesTo(q.Kind) {
			return req, domain.NewErrorf(domain.CodeAttributeNotApplicable, domain.MsgEngineAttrNotApplicable, a.Attribute, string(q.Kind))
		}
		cond := storage.AttrCond{Code: a.Attribute, Text: a.Text, Num: a.Num, HasNum: a.HasNum, Op: storage.FilterOp(a.Op)}
		if err := knownFilterOp(a.Op); err != nil {
			return req, err
		}
		switch def.Type {
		case catalog.AttrText, catalog.AttrEnum:
			if a.HasNum {
				return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterAttrText, a.Attribute)
			}
		default:
			if !a.HasNum {
				return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterAttrNumber, a.Attribute)
			}
		}
		req.Attrs = append(req.Attrs, cond)
	}
	for _, p := range q.Parameters {
		def, ok := snap.Parameter(p.Parameter)
		if !ok {
			return req, domain.NewErrorf(domain.CodeUnknownParameter, domain.MsgEngineParamUnknown, p.Parameter)
		}
		if q.Kind != "" && !def.AppliesTo(q.Kind) {
			return req, domain.NewErrorf(domain.CodeParameterNotApplicable, domain.MsgEngineParamNotApplicable, p.Parameter, string(q.Kind))
		}
		hasNum := p.Min != nil || p.Max != nil || p.Exact != nil
		hasText := p.Text != ""
		if !hasNum && !hasText {
			return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterParamSet, p.Parameter)
		}
		cond := storage.ParamCond{Code: p.Parameter, Text: p.Text, HasText: hasText}
		if def.ValueType == catalog.ValueText || def.ValueType == catalog.ValueEnum {
			if hasNum {
				return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterParamText, p.Parameter)
			}
		} else if hasText {
			return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterParamNumber, p.Parameter)
		}
		if p.Min != nil {
			cond.Min, cond.HasMin = *p.Min, true
		}
		if p.Max != nil {
			cond.Max, cond.HasMax = *p.Max, true
		}
		if p.Exact != nil {
			cond.Exact, cond.HasExact = *p.Exact, true
		}
		req.Params = append(req.Params, cond)
	}
	for _, so := range q.Sort {
		// "designation"/"id" — колонки записи; коды полей разбора —
		// реестр домена (значение передаётся параметром, записи без
		// поля — в конце порядка); коды параметров каталога —
		// только численные и применимые к классу запроса (D7, по образцу
		// параметрических фильтров). Известность ключа проверяется по
		// единому реестру — неизвестный не «молча пустой порядок».
		switch {
		case so.Key == "designation" || so.Key == "id":
		case so.Param:
			def, ok := snap.Parameter(so.Key)
			if !ok {
				return req, domain.NewErrorf(domain.CodeUnknownParameter, domain.MsgEngineParamUnknown, so.Key)
			}
			if q.Kind != "" && !def.AppliesTo(q.Kind) {
				return req, domain.NewErrorf(domain.CodeParameterNotApplicable,
					domain.MsgEngineParamNotApplicable, so.Key, string(q.Kind))
			}
			if !def.ValueType.Numeric() {
				return req, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgSvcSortParamNumber, so.Key)
			}
		default:
			if !domain.KnownDesignationField(so.Key) {
				return req, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSearchFieldUnknown, so.Key)
			}
		}
		req.Sorts = append(req.Sorts, storage.SortKey{Key: so.Key, Numeric: so.Numeric, Param: so.Param, Desc: so.Desc})
	}
	// Колонки значений: известность и применимость кодов — по
	// снимку, по образцу фильтров (в единую точку валидации — их тексты
	// не дублируются); логические атрибуты формы CardValue не имеют —
	// 400, как у фильтров; дубликат кода и потолок набора — как у набора
	// id. В storage-запрос колонки не входят — это проекция ответа.
	if len(q.Values) > SearchValuesMax {
		return req, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgSvcSearchValuesMax, SearchValuesMax)
	}
	seenValues := make(map[string]struct{}, len(q.Values))
	for _, col := range q.Values {
		key := col.String()
		if _, dup := seenValues[key]; dup {
			return req, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSearchValuesDuplicate, key)
		}
		seenValues[key] = struct{}{}
		if col.Attr {
			def, ok := snap.Attribute(col.Code)
			if !ok {
				return req, domain.NewErrorf(domain.CodeUnknownAttribute, domain.MsgEngineAttrUnknown, col.Code)
			}
			if q.Kind != "" && !def.AppliesTo(q.Kind) {
				return req, domain.NewErrorf(domain.CodeAttributeNotApplicable,
					domain.MsgEngineAttrNotApplicable, col.Code, string(q.Kind))
			}
			if def.Type == catalog.AttrBool {
				return req, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgSvcValuesBoolAttr, col.Code)
			}
			continue
		}
		def, ok := snap.Parameter(col.Code)
		if !ok {
			return req, domain.NewErrorf(domain.CodeUnknownParameter, domain.MsgEngineParamUnknown, col.Code)
		}
		if q.Kind != "" && !def.AppliesTo(q.Kind) {
			return req, domain.NewErrorf(domain.CodeParameterNotApplicable,
				domain.MsgEngineParamNotApplicable, col.Code, string(q.Kind))
		}
	}
	return req, nil
}

// knownFilterOp — известность операции числового фильтра: пустая —
// «не задана» (трактуется как eq, как и раньше), неизвестная — не
// «молчит как =», а отвергается.
func knownFilterOp(op FilterOp) error {
	switch op {
	case "", OpEq, OpGte, OpLte:
		return nil
	}
	return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcFilterOpUnknown, string(op))
}

// normalizeQuery приводит подстроку поиска к каноническому регистру
// (хранятся канонические обозначения; символы подстановки экранирует
// хранилище — LIKE с ESCAPE переносим).
func normalizeQuery(q string) string {
	return strings.ToUpper(strings.TrimSpace(q))
}
