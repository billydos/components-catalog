package service

import (
	"cmp"
	"context"
	"maps"
	"math"
	"slices"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// Слитое состояние записи для Upsert: секции входа заменяют свои секции
// текущего состояния, отсутствующие — не трогают (docs/plan/02-database.md §6).
// Состояние же используется для канонического сравнения (исход Skipped)
// и для валидации движком (итоговое состояние записи).

// stateValue — значение параметра в состоянии (условия отсортированы
// по коду для канонического сравнения). Section — имя секции группы
// каталога, в которой значение стоит (у значений типа в целом и у
// значений исполнений; для валидации движком и экспорта не хранится в БД —
// восстанавливается по группе параметра).
type stateValue struct {
	Parameter  string
	Section    string
	Exact      *float64
	Min        *float64
	Max        *float64
	Text       *string
	Conditions []catalog.ConditionValue
}

// stateVariant — исполнение: метка, плоский упорядоченный список значений
// и атрибуты исполнения (отсортированы по коду). Значения следуют в
// каноническом порядке секций и параметров каталога; порядок секций
// и разных параметров входа значимостью не обладает, относительный
// порядок одноимённых строк одного параметра — сохраняется.
type stateVariant struct {
	Label      string
	Values     []stateValue
	Attributes []catalog.AttributeValue
}

// stateAnalog — исходящая ссылка-аналог в состоянии.
type stateAnalog struct {
	Designation string
	Note        string
}

// stateSynonym — историческое название записи в состоянии.
type stateSynonym struct {
	Designation string
	Note        string
	YearFrom    *int
	YearTo      *int
}

// stateSource — ссылка записи на источник в состоянии (код реестра
// и заметка места в источнике; сортировка по коду).
type stateSource struct {
	Code string
	Note string
}

// deviceState — слитое состояние записи.
type deviceState struct {
	fields        []domain.Field           // явные классификационные поля (по имени поля)
	attributes    []catalog.AttributeValue // отсортированы по коду атрибута
	groupValues   map[string][]stateValue  // код группы каталога → значения в порядке
	variants      []stateVariant           // исполнения в порядке
	manufacturers []string                 // отсортированы
	analogs       []stateAnalog            // отсортированы по обозначению
	synonyms      []stateSynonym           // отсортированы по обозначению
	sources       []stateSource            // отсортированы по коду источника
}

// loadState читает текущее состояние записи внутри транзакции записи
// (консистентность с последующим применением секций); значения типа
// в целом группируются по группам каталога (группа параметра — из снимка).
// Явные классификационные поля — хранимые поля минус продукты разбора
// обозначения записи (p).
func (s *stateMerger) loadState(ctx context.Context, tx *storage.Tx, deviceID int64,
	p domain.ParsedDesignation) (*deviceState, error) {
	attrs, err := tx.LoadAttributes(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	values, err := tx.LoadValues(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	variants, err := tx.LoadVariants(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	manufacturers, err := tx.LoadManufacturers(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	analogs, err := tx.LoadOutgoingAnalogs(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	synonyms, err := tx.LoadSynonyms(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	sourceLinks, err := tx.LoadDeviceSources(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	storedFields, err := tx.LoadFields(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	st := &deviceState{
		groupValues:   make(map[string][]stateValue),
		manufacturers: manufacturers,
		fields:        explicitFieldsOf(storedFields, p.Fields),
	}
	for _, a := range attrs {
		// Форма значения — по типу атрибута каталога: text_list — список
		// строк группы, прочие — скаляр первой строки.
		av := catalog.AttributeValue{Attribute: a.Attribute, Num: a.Num, Bool: a.Bool}
		if def, ok := s.snap.Attribute(a.Attribute); ok && def.Type == catalog.AttrTextList {
			av.Texts = a.Texts
		} else {
			av.Text = a.Text
		}
		st.attributes = append(st.attributes, av)
	}
	st.variants = make([]stateVariant, 0, len(variants))
	byVariant := make(map[int64]int, len(variants))
	for _, v := range variants {
		byVariant[v.ID] = len(st.variants)
		st.variants = append(st.variants, stateVariant{Label: v.Label})
	}
	// Атрибуты исполнений — форма значения по типу атрибута каталога,
	// как у атрибутов записи (варианты уже прочитаны — ids локальные).
	vattrRows, err := tx.LoadVariantAttributesByIDs(ctx, variantIDsOf(variants))
	if err != nil {
		return nil, err
	}
	for vid, rows := range vattrRows {
		idx, ok := byVariant[vid]
		if !ok {
			continue
		}
		for _, a := range rows {
			av := catalog.AttributeValue{Attribute: a.Attribute, Num: a.Num, Bool: a.Bool}
			if def, ok := s.snap.Attribute(a.Attribute); ok && def.Type == catalog.AttrTextList {
				av.Texts = a.Texts
			} else {
				av.Text = a.Text
			}
			st.variants[idx].Attributes = append(st.variants[idx].Attributes, av)
		}
	}
	for _, vr := range values {
		if vr.VariantID != nil {
			idx, ok := byVariant[*vr.VariantID]
			if !ok {
				continue
			}
			sv := toStateValue(vr.Value)
			sv.Section = s.sectionOf(vr.Value.Parameter)
			st.variants[idx].Values = append(st.variants[idx].Values, sv)
			continue
		}
		group := ""
		if p, ok := s.snap.Parameter(vr.Value.Parameter); ok {
			group = p.Group
		}
		st.groupValues[group] = append(st.groupValues[group], toStateValue(vr.Value))
	}
	st.analogs = make([]stateAnalog, 0, len(analogs))
	for _, a := range analogs {
		st.analogs = append(st.analogs, stateAnalog{Designation: a.Designation, Note: a.Note})
	}
	// Канонический порядок синонимов — по обозначению (сравнение
	// состояний не зависит от порядка чтения).
	st.synonyms = make([]stateSynonym, 0, len(synonyms))
	for _, s := range synonyms {
		st.synonyms = append(st.synonyms, stateSynonym{
			Designation: s.Designation, Note: s.Note, YearFrom: s.YearFrom, YearTo: s.YearTo,
		})
	}
	slices.SortStableFunc(st.synonyms, func(a, b stateSynonym) int {
		return cmp.Compare(a.Designation, b.Designation)
	})
	// Канонический порядок источников — по коду (LoadDeviceSources читает
	// уже отсортированным; копия — единообразно с прочими секциями).
	st.sources = make([]stateSource, 0, len(sourceLinks))
	for _, l := range sourceLinks {
		st.sources = append(st.sources, stateSource{Code: l.Code, Note: l.Note})
	}
	return st, nil
}

func toStateValue(v storage.ParamValue) stateValue {
	return stateValue{
		Parameter:  v.Parameter,
		Exact:      v.Exact,
		Min:        v.Min,
		Max:        v.Max,
		Text:       v.Text,
		Conditions: sortedConds(v.Conditions),
	}
}

// variantIDsOf — id исполнений одним срезом.
func variantIDsOf(variants []storage.VariantRow) []int64 {
	ids := make([]int64, 0, len(variants))
	for _, v := range variants {
		ids = append(ids, v.ID)
	}
	return ids
}

// sectionOf — имя секции группы параметра по снимку ("" — параметр
// неизвестен каталогу).
func (s *stateMerger) sectionOf(parameter string) string {
	p, ok := s.snap.Parameter(parameter)
	if !ok {
		return ""
	}
	g, ok := s.snap.Group(p.Group)
	if !ok {
		return ""
	}
	return g.SectionName
}

func sortedConds(conds []storage.Cond) []catalog.ConditionValue {
	out := make([]catalog.ConditionValue, 0, len(conds))
	for _, c := range conds {
		out = append(out, catalog.ConditionValue{Condition: c.Code, Value: c.Value})
	}
	slices.SortStableFunc(out, func(a, b catalog.ConditionValue) int { return cmp.Compare(a.Condition, b.Condition) })
	return out
}

// stateMerger — контекст слияния входа с текущим состоянием.
type stateMerger struct {
	snap *catalog.Snapshot
}

// merge строит слитое состояние: заданные секции входа заменяют текущие,
// отсутствующие — сохраняются. Вход предварительно нормализуется
// (сортировка/дедупликация — ошибки даёт validateInput до слияния).
// explicit — провалидированный набор явных классификационных полей
// (не nil только при заданной секции fields).
func (s *stateMerger) merge(cur *deviceState, in *DeviceInput, explicit []domain.Field,
	resolvedAnalogs []stateAnalog, resolvedSynonyms []stateSynonym, resolvedSources []stateSource) *deviceState {
	out := &deviceState{
		groupValues: make(map[string][]stateValue),
	}
	// Явные классификационные поля: nil — не менять, иначе замена целиком.
	if in.Fields != nil {
		out.fields = append([]domain.Field(nil), explicit...)
	} else {
		out.fields = cur.fields
	}
	// Атрибуты: nil — не менять, иначе замена целиком (включая очистку).
	if in.Attributes != nil {
		out.attributes = normalizeAttrs(in.Attributes)
	} else {
		out.attributes = cur.attributes
	}
	// Секции групп: заменяется только группа секции входа; значения
	// канонизируются по параметрам каталога (порядок разных параметров
	// значимостью не обладает — docs/fill-format.md §4).
	maps.Copy(out.groupValues, cur.groupValues)
	for _, sec := range in.Sections {
		g, _ := s.snap.GroupBySection(sec.Section)
		vals := make([]stateValue, 0, len(sec.Values))
		for _, v := range sec.Values {
			vals = append(vals, stateValue{
				Parameter: v.Parameter,
				Exact:     v.Exact, Min: v.Min, Max: v.Max, Text: v.Text,
				Conditions: sortedCondsInput(v.Conditions),
			})
		}
		out.groupValues[g.Code] = s.canonicalValues(vals)
	}
	// Исполнения: nil — не менять, иначе полная замена набора. Значения
	// несут имя секции группы, в которой стоят (проверка принадлежности
	// секции — движком, как у значений типа в целом); секции канонизируются
	// по группам каталога, значения внутри секции — по параметрам каталога:
	// порядок секций и разных параметров значимостью не обладает,
	// относительный порядок одноимённых строк одного параметра
	// сохраняется (docs/fill-format.md §4) и задаёт sort_order.
	if in.Variants != nil {
		out.variants = make([]stateVariant, 0, len(*in.Variants))
		for _, v := range *in.Variants {
			sv := stateVariant{Label: v.Label, Attributes: normalizeAttrs(v.Attributes)}
			for _, sec := range s.canonicalSections(v.Sections) {
				vals := make([]stateValue, 0, len(sec.Values))
				for _, val := range sec.Values {
					vals = append(vals, stateValue{
						Parameter: val.Parameter,
						Section:   sec.Section,
						Exact:     val.Exact, Min: val.Min, Max: val.Max, Text: val.Text,
						Conditions: sortedCondsInput(val.Conditions),
					})
				}
				sv.Values = append(sv.Values, s.canonicalValues(vals)...)
			}
			out.variants = append(out.variants, sv)
		}
	} else {
		out.variants = cur.variants
	}
	// Производители: nil — не менять, иначе замена списка.
	if in.Manufacturers != nil {
		names := append([]string(nil), *in.Manufacturers...)
		slices.Sort(names)
		out.manufacturers = names
	} else {
		out.manufacturers = cur.manufacturers
	}
	// Аналоги: nil — не менять, иначе замена только исходящих ссылок (D8).
	if in.Analogs != nil {
		out.analogs = resolvedAnalogs
	} else {
		out.analogs = cur.analogs
	}
	// Синонимы: nil — не менять, иначе замена целиком (включая очистку).
	if in.Synonyms != nil {
		out.synonyms = resolvedSynonyms
	} else {
		out.synonyms = cur.synonyms
	}
	// Источники: nil — не менять, иначе замена ссылок целиком (включая
	// очистку — сироты реестра вычищаются применением, состояние видит
	// только ссылки).
	if in.Sources != nil {
		out.sources = resolvedSources
	} else {
		out.sources = cur.sources
	}
	return out
}

func sortedCondsInput(conds []catalog.ConditionValue) []catalog.ConditionValue {
	out := append([]catalog.ConditionValue(nil), conds...)
	slices.SortStableFunc(out, func(a, b catalog.ConditionValue) int { return cmp.Compare(a.Condition, b.Condition) })
	return out
}

// canonicalSections — копия секций в каноническом порядке: SortOrder
// группы каталога, при равенстве — имя секции (детерминированный
// порядок). Порядок секций значений исполнений значимостью не обладает
// (план 03 §7): хранение канонично, экспорт пишет секции в порядке
// групп каталога — round-trip без изменений. Неизвестная секция
// невозможна (принадлежность проверена против каталога до слияния)
// и ставится последней.
func (s *stateMerger) canonicalSections(sections []SectionInput) []SectionInput {
	out := append([]SectionInput(nil), sections...)
	slices.SortStableFunc(out, func(a, b SectionInput) int {
		if ar, br := s.sectionRank(a.Section), s.sectionRank(b.Section); ar != br {
			return cmp.Compare(ar, br)
		}
		return cmp.Compare(a.Section, b.Section)
	})
	return out
}

func (s *stateMerger) sectionRank(section string) int {
	g, ok := s.snap.GroupBySection(section)
	if !ok {
		return math.MaxInt
	}
	return g.SortOrder
}

// canonicalValues — копия значений секции в каноническом порядке
// параметров каталога: SortOrder параметра, при равенстве — код
// (детерминированный порядок). Порядок разных параметров внутри секции
// значимостью не обладает — параметры внутри секции упорядочены каталогом
// (docs/fill-format.md §4); относительный порядок одноимённых строк
// одного параметра сохраняется (стабильная сортировка) — он значим и
// задаёт sort_order. Неизвестный каталогу параметр отвергается движком
// до записи (хранение защищено FK); до валидации он детерминированно
// ставится последним (MaxInt в compareParameterCodes).
func (s *stateMerger) canonicalValues(vals []stateValue) []stateValue {
	out := append([]stateValue(nil), vals...)
	slices.SortStableFunc(out, func(a, b stateValue) int {
		return compareParameterCodes(s.snap, a.Parameter, b.Parameter)
	})
	return out
}

// compareParameterCodes — канонический порядок параметров каталога:
// SortOrder параметра, при равенстве — код. Единая точка для слияния
// (canonicalValues — сохраняемый sort_order) и карточки (cardGroups —
// порядок отображения и экспорта): смена порядка в одном месте
// оставляет хранение, карточку и экспорт согласованными.
func compareParameterCodes(snap *catalog.Snapshot, a, b string) int {
	if ar, br := paramRank(snap, a), paramRank(snap, b); ar != br {
		return cmp.Compare(ar, br)
	}
	return cmp.Compare(a, b)
}

// paramRank — позиция параметра в каноническом порядке каталога
// (неизвестный каталогу — последний).
func paramRank(snap *catalog.Snapshot, code string) int {
	p, ok := snap.Parameter(code)
	if !ok {
		return math.MaxInt
	}
	return p.SortOrder
}

func normalizeAttrs(vals []catalog.AttributeValue) []catalog.AttributeValue {
	out := append([]catalog.AttributeValue(nil), vals...)
	slices.SortStableFunc(out, func(a, b catalog.AttributeValue) int { return cmp.Compare(a.Attribute, b.Attribute) })
	return out
}

// sameState сравнивает состояния канонически: значимы порядок исполнений
// и относительный порядок одноимённых строк одного параметра (они задают
// sort_order); порядок секций и разных параметров внутри секции
// значимостью не обладает — параметры внутри секции упорядочены каталогом
// (docs/fill-format.md §4); явные поля, атрибуты, условия, производители,
// аналоги, синонимы и источники сравниваются как множества
// (нормализуются сортировкой копий — равенство не зависит от порядка
// чтения из БД). Числа сравниваются бит-то-точно (round-trip REAL; NaN
// в значениях исключён валидацией).
func sameState(a, b *deviceState) bool {
	return fieldsEqual(a.fields, b.fields) &&
		attrsEqual(a.attributes, b.attributes) &&
		groupValuesEqual(a.groupValues, b.groupValues) &&
		variantsEqual(a.variants, b.variants) &&
		manufacturersEqual(a.manufacturers, b.manufacturers) &&
		analogsEqual(a.analogs, b.analogs) &&
		synonymsEqual(a.synonyms, b.synonyms) &&
		sourcesEqual(a.sources, b.sources)
}

// fieldsEqual — множества явных полей по имени.
func fieldsEqual(a, b []domain.Field) bool {
	byName := func(x, y domain.Field) int { return cmp.Compare(x.Name, y.Name) }
	return slices.Equal(
		slices.SortedFunc(slices.Values(a), byName),
		slices.SortedFunc(slices.Values(b), byName))
}

// attrsEqual — множества значений атрибутов по коду.
func attrsEqual(a, b []catalog.AttributeValue) bool {
	byCode := func(x, y catalog.AttributeValue) int { return cmp.Compare(x.Attribute, y.Attribute) }
	return slices.EqualFunc(
		slices.SortedFunc(slices.Values(a), byCode),
		slices.SortedFunc(slices.Values(b), byCode),
		attrEqual)
}

func attrEqual(a, b catalog.AttributeValue) bool {
	return a.Attribute == b.Attribute &&
		strPtrEqual(a.Text, b.Text) &&
		slices.Equal(a.Texts, b.Texts) &&
		numPtrEqual(a.Num, b.Num) &&
		boolPtrEqual(a.Bool, b.Bool)
}

// groupValuesEqual — сравнение по группам каталога; значения группы —
// по параметрам (valuesByKeyEqual).
func groupValuesEqual(a, b map[string][]stateValue) bool {
	if len(a) != len(b) {
		return false
	}
	for g, av := range a {
		bv, ok := b[g]
		if !ok || !valuesByKeyEqual(av, bv) {
			return false
		}
	}
	return true
}

// variantsEqual — исполнения сравниваются попарно по порядку (sort_order):
// метка, атрибуты (множества по коду) и значения по секциям и параметрам
// (порядок разных параметров и секций значимостью не обладает —
// docs/fill-format.md §4; исторические базы могут нести порядок файла
// наполнения, пришедшего до канонизации).
func variantsEqual(a, b []stateVariant) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Label != b[i].Label || !variantValuesEqual(a[i].Values, b[i].Values) ||
			!attrsEqual(a[i].Attributes, b[i].Attributes) {
			return false
		}
	}
	return true
}

// variantValuesEqual — значения исполнения по секциям, внутри секции —
// по параметрам: порядок секций, разных параметров и чередование — нет,
// порядок одноимённых строк одного параметра — значим.
func variantValuesEqual(a, b []stateValue) bool {
	if len(a) != len(b) {
		return false
	}
	ag, bg := valuesBySection(a), valuesBySection(b)
	if len(ag) != len(bg) {
		return false
	}
	for sec, av := range ag {
		bv, ok := bg[sec]
		if !ok || !valuesByKeyEqual(av, bv) {
			return false
		}
	}
	return true
}

// valuesByKeyEqual — значения секции/группы по параметрам: порядок
// разных параметров значимостью не обладает (каноничен по каталогу —
// docs/fill-format.md §4), порядок одноимённых строк одного параметра
// значим (он задаёт sort_order).
func valuesByKeyEqual(a, b []stateValue) bool {
	if len(a) != len(b) {
		return false
	}
	ag, bg := valuesByParameter(a), valuesByParameter(b)
	if len(ag) != len(bg) {
		return false
	}
	for p, av := range ag {
		bv, ok := bg[p]
		if !ok || !valuesEqual(av, bv) {
			return false
		}
	}
	return true
}

// valuesBySection — значения, сгруппированные по имени секции; порядок
// внутри секции — порядок следования в списке.
func valuesBySection(vals []stateValue) map[string][]stateValue {
	out := make(map[string][]stateValue)
	for _, v := range vals {
		out[v.Section] = append(out[v.Section], v)
	}
	return out
}

// valuesByParameter — значения, сгруппированные по коду параметра;
// порядок одноимённых строк — порядок следования в списке.
func valuesByParameter(vals []stateValue) map[string][]stateValue {
	out := make(map[string][]stateValue)
	for _, v := range vals {
		out[v.Parameter] = append(out[v.Parameter], v)
	}
	return out
}

// valuesEqual — значения одного параметра в порядке следования
// (относительный порядок одноимённых строк значим — sort_order).
func valuesEqual(a, b []stateValue) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !valueEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func valueEqual(a, b stateValue) bool {
	return a.Parameter == b.Parameter &&
		numPtrEqual(a.Exact, b.Exact) &&
		numPtrEqual(a.Min, b.Min) &&
		numPtrEqual(a.Max, b.Max) &&
		strPtrEqual(a.Text, b.Text) &&
		condsEqual(a.Conditions, b.Conditions)
}

// condsEqual — множества условий по коду.
func condsEqual(a, b []catalog.ConditionValue) bool {
	byCode := func(x, y catalog.ConditionValue) int { return cmp.Compare(x.Condition, y.Condition) }
	return slices.Equal(
		slices.SortedFunc(slices.Values(a), byCode),
		slices.SortedFunc(slices.Values(b), byCode))
}

// manufacturersEqual — множества имён производителей.
func manufacturersEqual(a, b []string) bool {
	return slices.Equal(
		slices.Sorted(slices.Values(a)),
		slices.Sorted(slices.Values(b)))
}

// analogsEqual — множества исходящих ссылок по обозначению.
func analogsEqual(a, b []stateAnalog) bool {
	byName := func(x, y stateAnalog) int { return cmp.Compare(x.Designation, y.Designation) }
	return slices.Equal(
		slices.SortedFunc(slices.Values(a), byName),
		slices.SortedFunc(slices.Values(b), byName))
}

// synonymsEqual — множества исторических названий по обозначению.
func synonymsEqual(a, b []stateSynonym) bool {
	byName := func(x, y stateSynonym) int { return cmp.Compare(x.Designation, y.Designation) }
	return slices.EqualFunc(
		slices.SortedFunc(slices.Values(a), byName),
		slices.SortedFunc(slices.Values(b), byName),
		func(x, y stateSynonym) bool {
			return x.Designation == y.Designation && x.Note == y.Note &&
				intPtrEqual(x.YearFrom, y.YearFrom) && intPtrEqual(x.YearTo, y.YearTo)
		})
}

// sourcesEqual — множества ссылок на источники по коду.
func sourcesEqual(a, b []stateSource) bool {
	byCode := func(x, y stateSource) int { return cmp.Compare(x.Code, y.Code) }
	return slices.EqualFunc(
		slices.SortedFunc(slices.Values(a), byCode),
		slices.SortedFunc(slices.Values(b), byCode),
		func(x, y stateSource) bool { return x.Code == y.Code && x.Note == y.Note })
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func numPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// toCatalogDevice собирает запись для валидации движком (Section значений —
// по группе каталога). Группы обходятся в детерминированном порядке
// (ключи группы сортируются): движок возвращает проблемы в порядке
// значений записи, REST 422 и тесты не зависят от порядка итерации map.
func (s *stateMerger) toCatalogDevice(p domain.ParsedDesignation, st *deviceState) catalog.Device {
	d := catalog.Device{
		Kind:        p.Kind,
		System:      p.System,
		Designation: p.Designation,
		Attributes:  st.attributes,
	}
	groups := slices.Sorted(maps.Keys(st.groupValues))
	for _, g := range groups {
		section := ""
		if grp, ok := s.snap.Group(g); ok {
			section = grp.SectionName
		}
		for _, v := range st.groupValues[g] {
			d.Values = append(d.Values, stateToParameterValue(section, v))
		}
	}
	for _, v := range st.variants {
		cv := catalog.Variant{Label: v.Label, Attributes: v.Attributes}
		for _, val := range v.Values {
			cv.Values = append(cv.Values, stateToParameterValue(val.Section, val))
		}
		d.Variants = append(d.Variants, cv)
	}
	return d
}

func stateToParameterValue(section string, v stateValue) catalog.ParameterValue {
	return catalog.ParameterValue{
		Parameter:  v.Parameter,
		Section:    section,
		Exact:      v.Exact,
		Min:        v.Min,
		Max:        v.Max,
		Text:       v.Text,
		Conditions: v.Conditions,
	}
}
