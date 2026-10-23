package service

import (
	"cmp"
	"context"
	"maps"
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
// и атрибуты исполнения (отсортированы по коду).
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

// deviceState — слитое состояние записи.
type deviceState struct {
	fields        []domain.Field           // явные классификационные поля (по имени поля)
	attributes    []catalog.AttributeValue // отсортированы по коду атрибута
	groupValues   map[string][]stateValue  // код группы каталога → значения в порядке
	variants      []stateVariant           // исполнения в порядке
	manufacturers []string                 // отсортированы
	analogs       []stateAnalog            // отсортированы по обозначению
	synonyms      []stateSynonym           // отсортированы по обозначению
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
	resolvedAnalogs []stateAnalog, resolvedSynonyms []stateSynonym) *deviceState {
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
	// Секции групп: заменяется только группа секции входа.
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
		out.groupValues[g.Code] = vals
	}
	// Исполнения: nil — не менять, иначе полная замена набора. Значения
	// несут имя секции группы, в которой стоят (проверка принадлежности
	// секции — движком, как у значений типа в целом).
	if in.Variants != nil {
		out.variants = make([]stateVariant, 0, len(*in.Variants))
		for _, v := range *in.Variants {
			sv := stateVariant{Label: v.Label, Attributes: normalizeAttrs(v.Attributes)}
			for _, sec := range v.Sections {
				for _, val := range sec.Values {
					sv.Values = append(sv.Values, stateValue{
						Parameter: val.Parameter,
						Section:   sec.Section,
						Exact:     val.Exact, Min: val.Min, Max: val.Max, Text: val.Text,
						Conditions: sortedCondsInput(val.Conditions),
					})
				}
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
	return out
}

func sortedCondsInput(conds []catalog.ConditionValue) []catalog.ConditionValue {
	out := append([]catalog.ConditionValue(nil), conds...)
	slices.SortStableFunc(out, func(a, b catalog.ConditionValue) int { return cmp.Compare(a.Condition, b.Condition) })
	return out
}

func normalizeAttrs(vals []catalog.AttributeValue) []catalog.AttributeValue {
	out := append([]catalog.AttributeValue(nil), vals...)
	slices.SortStableFunc(out, func(a, b catalog.AttributeValue) int { return cmp.Compare(a.Attribute, b.Attribute) })
	return out
}

// sameState сравнивает состояния канонически: порядок значений внутри
// секций и исполнений значим (он задаёт sort_order); явные поля,
// атрибуты, условия, производители, аналоги и синонимы сравниваются
// как множества (нормализуются сортировкой копий — равенство не зависит
// от порядка чтения из БД). Числа сравниваются бит-то-точно (round-trip
// REAL; NaN в значениях исключён валидацией).
func sameState(a, b *deviceState) bool {
	return fieldsEqual(a.fields, b.fields) &&
		attrsEqual(a.attributes, b.attributes) &&
		groupValuesEqual(a.groupValues, b.groupValues) &&
		variantsEqual(a.variants, b.variants) &&
		manufacturersEqual(a.manufacturers, b.manufacturers) &&
		analogsEqual(a.analogs, b.analogs) &&
		synonymsEqual(a.synonyms, b.synonyms)
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

// groupValuesEqual — сравнение по группам каталога; порядок значений
// внутри группы значим (sort_order).
func groupValuesEqual(a, b map[string][]stateValue) bool {
	if len(a) != len(b) {
		return false
	}
	for g, av := range a {
		bv, ok := b[g]
		if !ok || !valuesEqual(av, bv) {
			return false
		}
	}
	return true
}

// variantsEqual — порядок исполнений значим (sort_order); атрибуты
// сравниваются как множества по коду.
func variantsEqual(a, b []stateVariant) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Label != b[i].Label || !valuesEqual(a[i].Values, b[i].Values) ||
			!attrsEqual(a[i].Attributes, b[i].Attributes) {
			return false
		}
	}
	return true
}

// valuesEqual — порядок значений значим (sort_order).
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
