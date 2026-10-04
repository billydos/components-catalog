package service

import (
	"context"
	"slices"
	"sort"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// Слитое состояние записи для Upsert: секции входа заменяют свои секции
// текущего состояния, отсутствующие — не трогают (docs/plan/02-database.md §6).
// Состояние же используется для канонического сравнения (исход Skipped)
// и для валидации движком (итоговое состояние записи).

// stateValue — значение параметра в состоянии (условия отсортированы
// по коду для канонического сравнения).
type stateValue struct {
	Parameter  string
	Exact      *float64
	Min        *float64
	Max        *float64
	Text       *string
	Conditions []catalog.ConditionValue
}

// stateVariant — исполнение: метка и плоский упорядоченный список значений.
type stateVariant struct {
	Label  string
	Values []stateValue
}

// stateAnalog — исходящая ссылка-аналог в состоянии.
type stateAnalog struct {
	Designation string
	Note        string
}

// deviceState — слитое состояние записи.
type deviceState struct {
	fields        []domain.Field           // явные классификационные поля (по имени поля)
	attributes    []catalog.AttributeValue // отсортированы по коду атрибута
	groupValues   map[string][]stateValue  // код группы каталога → значения в порядке
	variants      []stateVariant           // исполнения в порядке
	manufacturers []string                 // отсортированы
	analogs       []stateAnalog            // отсортированы по обозначению
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
		st.attributes = append(st.attributes, catalog.AttributeValue{
			Attribute: a.Attribute, Text: a.Text, Num: a.Num, Bool: a.Bool,
		})
	}
	st.variants = make([]stateVariant, 0, len(variants))
	byVariant := make(map[int64]int, len(variants))
	for _, v := range variants {
		byVariant[v.ID] = len(st.variants)
		st.variants = append(st.variants, stateVariant{Label: v.Label})
	}
	for _, vr := range values {
		if vr.VariantID != nil {
			idx, ok := byVariant[*vr.VariantID]
			if !ok {
				continue
			}
			st.variants[idx].Values = append(st.variants[idx].Values, toStateValue(vr.Value))
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

func sortedConds(conds []storage.Cond) []catalog.ConditionValue {
	out := make([]catalog.ConditionValue, 0, len(conds))
	for _, c := range conds {
		out = append(out, catalog.ConditionValue{Condition: c.Code, Value: c.Value})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Condition < out[j].Condition })
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
	resolvedAnalogs []stateAnalog) *deviceState {
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
	for g, vals := range cur.groupValues {
		out.groupValues[g] = vals
	}
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
	// Исполнения: nil — не менять, иначе полная замена набора.
	if in.Variants != nil {
		out.variants = make([]stateVariant, 0, len(*in.Variants))
		for _, v := range *in.Variants {
			sv := stateVariant{Label: v.Label}
			for _, sec := range v.Sections {
				for _, val := range sec.Values {
					sv.Values = append(sv.Values, stateValue{
						Parameter: val.Parameter,
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
		sort.Strings(names)
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
	return out
}

func sortedCondsInput(conds []catalog.ConditionValue) []catalog.ConditionValue {
	out := append([]catalog.ConditionValue(nil), conds...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Condition < out[j].Condition })
	return out
}

func normalizeAttrs(vals []catalog.AttributeValue) []catalog.AttributeValue {
	out := append([]catalog.AttributeValue(nil), vals...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Attribute < out[j].Attribute })
	return out
}

// sameState сравнивает состояния канонически: порядок значений внутри
// секций и исполнений значим (он задаёт sort_order); явные поля,
// атрибуты, условия, производители и аналоги сравниваются как множества
// (нормализуются сортировкой копий — равенство не зависит от порядка
// чтения из БД). Числа сравниваются бит-точно (round-trip REAL; NaN
// в значениях исключён валидацией).
func sameState(a, b *deviceState) bool {
	return fieldsEqual(a.fields, b.fields) &&
		attrsEqual(a.attributes, b.attributes) &&
		groupValuesEqual(a.groupValues, b.groupValues) &&
		variantsEqual(a.variants, b.variants) &&
		manufacturersEqual(a.manufacturers, b.manufacturers) &&
		analogsEqual(a.analogs, b.analogs)
}

// fieldsEqual — множества явных полей по имени.
func fieldsEqual(a, b []domain.Field) bool {
	if len(a) != len(b) {
		return false
	}
	ac, bc := append([]domain.Field(nil), a...), append([]domain.Field(nil), b...)
	sort.SliceStable(ac, func(i, j int) bool { return ac[i].Name < ac[j].Name })
	sort.SliceStable(bc, func(i, j int) bool { return bc[i].Name < bc[j].Name })
	return slices.Equal(ac, bc)
}

// attrsEqual — множества значений атрибутов по коду.
func attrsEqual(a, b []catalog.AttributeValue) bool {
	if len(a) != len(b) {
		return false
	}
	ac, bc := append([]catalog.AttributeValue(nil), a...), append([]catalog.AttributeValue(nil), b...)
	sort.SliceStable(ac, func(i, j int) bool { return ac[i].Attribute < ac[j].Attribute })
	sort.SliceStable(bc, func(i, j int) bool { return bc[i].Attribute < bc[j].Attribute })
	for i := range ac {
		if !attrEqual(ac[i], bc[i]) {
			return false
		}
	}
	return true
}

func attrEqual(a, b catalog.AttributeValue) bool {
	return a.Attribute == b.Attribute &&
		strPtrEqual(a.Text, b.Text) &&
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

// variantsEqual — порядок исполнений значим (sort_order).
func variantsEqual(a, b []stateVariant) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Label != b[i].Label || !valuesEqual(a[i].Values, b[i].Values) {
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
	if len(a) != len(b) {
		return false
	}
	ac, bc := append([]catalog.ConditionValue(nil), a...), append([]catalog.ConditionValue(nil), b...)
	sort.SliceStable(ac, func(i, j int) bool { return ac[i].Condition < ac[j].Condition })
	sort.SliceStable(bc, func(i, j int) bool { return bc[i].Condition < bc[j].Condition })
	return slices.Equal(ac, bc)
}

// manufacturersEqual — множества имён производителей.
func manufacturersEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac, bc := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	return slices.Equal(ac, bc)
}

// analogsEqual — множества исходящих ссылок по обозначению.
func analogsEqual(a, b []stateAnalog) bool {
	if len(a) != len(b) {
		return false
	}
	ac, bc := append([]stateAnalog(nil), a...), append([]stateAnalog(nil), b...)
	sort.SliceStable(ac, func(i, j int) bool { return ac[i].Designation < ac[j].Designation })
	sort.SliceStable(bc, func(i, j int) bool { return bc[i].Designation < bc[j].Designation })
	return slices.Equal(ac, bc)
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
// по группе каталога).
func (s *stateMerger) toCatalogDevice(p domain.ParsedDesignation, st *deviceState) catalog.Device {
	d := catalog.Device{
		Kind:        p.Kind,
		System:      p.System,
		Designation: p.Designation,
		Attributes:  st.attributes,
	}
	for g, vals := range st.groupValues {
		section := ""
		if grp, ok := s.snap.Group(g); ok {
			section = grp.SectionName
		}
		for _, v := range vals {
			d.Values = append(d.Values, stateToParameterValue(section, v))
		}
	}
	for _, v := range st.variants {
		cv := catalog.Variant{Label: v.Label}
		for _, val := range v.Values {
			cv.Values = append(cv.Values, stateToParameterValue("", val))
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
