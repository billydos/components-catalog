package catalog

import (
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/billydos/components-catalog/internal/domain"
)

// ValueType — тип значения параметра каталога (docs/plan/03-data-model.md §6):
// exact (точное), at_least (не менее, с необязательной верхней границей),
// at_most (не более), range (диапазон), text, enum.
type ValueType string

// Реестр типов значений параметров (DB-значения колонки value_type).
const (
	ValueExact   ValueType = "exact"
	ValueAtLeast ValueType = "at_least"
	ValueAtMost  ValueType = "at_most"
	ValueRange   ValueType = "range"
	ValueText    ValueType = "text"
	ValueEnum    ValueType = "enum"
)

// Valid сообщает, входит ли тип значения в реестр.
func (t ValueType) Valid() bool {
	switch t {
	case ValueExact, ValueAtLeast, ValueAtMost, ValueRange, ValueText, ValueEnum:
		return true
	}
	return false
}

// Numeric сообщает, численный ли тип (для проверок единиц и enum-значений).
func (t ValueType) Numeric() bool {
	switch t {
	case ValueExact, ValueAtLeast, ValueAtMost, ValueRange:
		return true
	}
	return false
}

// AttrType — тип значения атрибута (docs/plan/03-data-model.md §9):
// text | bool | int | number | enum.
type AttrType string

// Реестр типов значений атрибутов (DB-значения колонки value_type).
const (
	AttrText   AttrType = "text"
	AttrBool   AttrType = "bool"
	AttrInt    AttrType = "int"
	AttrNumber AttrType = "number"
	AttrEnum   AttrType = "enum"
)

// Valid сообщает, входит ли тип атрибута в реестр.
func (t AttrType) Valid() bool {
	switch t {
	case AttrText, AttrBool, AttrInt, AttrNumber, AttrEnum:
		return true
	}
	return false
}

// ConditionMode — роль условия в наборе условий параметра: required
// (обязательно) либо optional (допустимо; остальные условия набора
// запрещены — docs/plan/02-database.md §2.2).
type ConditionMode string

// Реестр режимов условий набора.
const (
	ModeRequired ConditionMode = "required"
	ModeOptional ConditionMode = "optional"
)

// Valid сообщает, входит ли режим в реестр.
func (m ConditionMode) Valid() bool {
	return m == ModeRequired || m == ModeOptional
}

// KindDef — строка таблицы kinds: класс приборов. Отображаемое название —
// бандлы internal/i18n: kind.<код> (D9).
type KindDef struct {
	Code domain.Kind
}

// SystemDef — строка таблицы designation_systems: система обозначений.
// Отображаемые имя и пояснение — бандлы internal/i18n (D9).
type SystemDef struct {
	Code domain.System
}

// SystemKindRef — строка таблицы designation_system_kinds: применимость
// системы обозначений к классу (docs/plan/03-data-model.md §1.1).
type SystemKindRef struct {
	System domain.System
	Kind   domain.Kind
}

// TailSemanticPower — значение tail_semantic «мощность, Вт»
// (series_families, docs/plan/02-database.md §2.2).
const TailSemanticPower = "power"

// SeriesFamilyDef — строка таблицы series_families: реестр семейств
// системы series. TailSemantic — "" (общий слабый разбор хвоста) либо
// "power" (хвост-число есть номинальная мощность, Вт). Расшифровка —
// бандлы internal/i18n: family.<семейство> (D9).
type SeriesFamilyDef struct {
	Series       string
	Kind         domain.Kind
	TailSemantic string
}

// UnitDef — строка таблицы units: каноническая единица измерения (код —
// латиница). Название и символ — бандлы internal/i18n (D9).
type UnitDef struct {
	Code string
}

// CategoryDef — строка таблицы categories: код классификационного поля
// category (применение/характеристика прибора — переключательный,
// импульсный, малошумящий…; ортогонален подклассу — функции перехода).
// Словарь — данные каталога (расширяется секцией catalog без правки
// кода); отображаемое название — бандлы internal/i18n: category.<код>,
// расширения без записи в бандле отображаются кодом (D9).
type CategoryDef struct {
	Code string
}

// ConditionDef — строка таблицы conditions: условие измерения/контекста
// значения. Unit "" — безразмерное условие; AllowNegative разрешает
// неположительные значения (temp), по умолчанию условия положительны.
// Отображаемое название — бандлы internal/i18n (D9).
type ConditionDef struct {
	Code          string
	Unit          string
	AllowNegative bool
}

// GroupDef — строка таблицы parameter_groups: группа параметров с именем
// секции файла наполнения и REST (SectionName — ключ формата, ASCII).
// Отображаемое название группы — бандлы internal/i18n (D9).
type GroupDef struct {
	Code        string
	SectionName string
	SortOrder   int
}

// ConditionSetItem — элемент набора условий параметра: условие, режим
// (required/optional) и необязательная константа fixed_value (допустима
// только при mode = required: значение в файле можно опустить либо задать
// равным fixed_value — docs/plan/02-database.md §2.2).
type ConditionSetItem struct {
	Condition  string
	Mode       ConditionMode
	FixedValue *float64
}

// ConditionSet — альтернативный набор условий параметра: значение корректно,
// если его условия подходят хотя бы к одному набору.
type ConditionSet struct {
	No    int
	Items []ConditionSetItem
}

// ParameterDef — строка таблицы parameters вместе с применимостью к классам
// (parameter_kinds), enum-значениями и наборами условий. Kinds пуст —
// параметр применим ко всем классам, включая добавленные в будущем (D7).
// DisplayName параметра — бандлы internal/i18n: param.<код> (D9).
type ParameterDef struct {
	Code           string
	Group          string
	Unit           string
	ValueType      ValueType
	Ceiling        *float64
	AllowNegative  bool
	ValidationRule string
	SortOrder      int
	Active         bool
	Kinds          []domain.Kind
	EnumValues     []string
	ConditionSets  []ConditionSet
}

// AppliesTo сообщает, применим ли параметр к классу записи (D7).
func (p *ParameterDef) AppliesTo(kind domain.Kind) bool {
	return len(p.Kinds) == 0 || containsKind(p.Kinds, kind)
}

// AttributeDef — строка таблицы attributes вместе с применимостью к классам
// (attribute_kinds) и enum-значениями. Kinds пуст — все классы (D7).
// DisplayName атрибута — бандлы internal/i18n: attr.<код> (D9).
type AttributeDef struct {
	Code           string
	GroupName      string
	Type           AttrType
	Unit           string
	ValidationRule string
	SortOrder      int
	Active         bool
	Kinds          []domain.Kind
	EnumValues     []string
}

// AppliesTo сообщает, применим ли атрибут к классу записи (D7).
func (a *AttributeDef) AppliesTo(kind domain.Kind) bool {
	return len(a.Kinds) == 0 || containsKind(a.Kinds, kind)
}

// RuleDef — строка таблицы validation_rules: именованный код-валидатор;
// реализации — в реестре этого пакета, привязка — данными. Строка — якорь
// FK; описание правила — бандлы internal/i18n: rule.<код> (D9).
type RuleDef struct {
	Code string
}

// KindRuleRef — строка таблицы kind_validation_rules: правило, проверяемое
// для записи класса в целом (когда привязка к отдельному параметру
// бессмысленна — матрицы исполнений).
type KindRuleRef struct {
	Kind domain.Kind
	Rule string
}

func containsKind(kinds []domain.Kind, kind domain.Kind) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Snapshot — иммутабельный снимок каталога: все определения (классы,
// системы обозначений и применимость, семейства, единицы, условия, группы,
// параметры с применимостью/enum/наборами условий, атрибуты, именованные
// правила и их привязка к классам). Заполняется транспортом (storage,
// этап 3) либо применением каталога (ApplyCatalog); после заполнения
// не мутирует — кэш заменяет снимок целиком по указателю
// (docs/plan/01-architecture.md §2.2).
type Snapshot struct {
	Revision       int64
	Kinds          []KindDef
	Systems        []SystemDef
	SystemKinds    []SystemKindRef
	SeriesFamilies []SeriesFamilyDef
	Units          []UnitDef
	Categories     []CategoryDef
	Conditions     []ConditionDef
	Groups         []GroupDef
	Parameters     []ParameterDef
	Attributes     []AttributeDef
	Rules          []RuleDef
	KindRules      []KindRuleRef

	once sync.Once
	idx  *snapshotIndex
}

type seriesKey struct {
	series string
	kind   domain.Kind
}

type systemKindKey struct {
	system domain.System
	kind   domain.Kind
}

type snapshotIndex struct {
	kinds         map[domain.Kind]int
	systems       map[domain.System]int
	units         map[string]int
	categories    map[string]int
	conditions    map[string]int
	groups        map[string]int
	groupSections map[string]int
	parameters    map[string]int
	attributes    map[string]int
	families      map[seriesKey]int
	systemKinds   map[systemKindKey]bool
	kindRules     map[domain.Kind][]string
}

// index строит карты поиска один раз при первом обращении; снимок
// считается заполненным до начала чтения.
func (s *Snapshot) index() *snapshotIndex {
	s.once.Do(func() {
		idx := &snapshotIndex{
			kinds:         make(map[domain.Kind]int, len(s.Kinds)),
			systems:       make(map[domain.System]int, len(s.Systems)),
			units:         make(map[string]int, len(s.Units)),
			categories:    make(map[string]int, len(s.Categories)),
			conditions:    make(map[string]int, len(s.Conditions)),
			groups:        make(map[string]int, len(s.Groups)),
			groupSections: make(map[string]int, len(s.Groups)),
			parameters:    make(map[string]int, len(s.Parameters)),
			attributes:    make(map[string]int, len(s.Attributes)),
			families:      make(map[seriesKey]int, len(s.SeriesFamilies)),
			systemKinds:   make(map[systemKindKey]bool, len(s.SystemKinds)),
			kindRules:     make(map[domain.Kind][]string),
		}
		for i := range s.Kinds {
			idx.kinds[s.Kinds[i].Code] = i
		}
		for i := range s.Systems {
			idx.systems[s.Systems[i].Code] = i
		}
		for i := range s.Units {
			idx.units[s.Units[i].Code] = i
		}
		for i := range s.Categories {
			idx.categories[s.Categories[i].Code] = i
		}
		for i := range s.Conditions {
			idx.conditions[s.Conditions[i].Code] = i
		}
		for i := range s.Groups {
			idx.groups[s.Groups[i].Code] = i
			if _, busy := idx.groupSections[s.Groups[i].SectionName]; !busy {
				idx.groupSections[s.Groups[i].SectionName] = i
			}
		}
		for i := range s.Parameters {
			idx.parameters[s.Parameters[i].Code] = i
		}
		for i := range s.Attributes {
			idx.attributes[s.Attributes[i].Code] = i
		}
		for i := range s.SeriesFamilies {
			f := &s.SeriesFamilies[i]
			if _, busy := idx.families[seriesKey{f.Series, f.Kind}]; !busy {
				idx.families[seriesKey{f.Series, f.Kind}] = i
			}
		}
		for i := range s.SystemKinds {
			r := &s.SystemKinds[i]
			idx.systemKinds[systemKindKey{r.System, r.Kind}] = true
		}
		for i := range s.KindRules {
			r := &s.KindRules[i]
			idx.kindRules[r.Kind] = append(idx.kindRules[r.Kind], r.Rule)
		}
		s.idx = idx
	})
	return s.idx
}

// Kind ищет класс по коду.
func (s *Snapshot) Kind(code domain.Kind) (KindDef, bool) {
	if s == nil {
		return KindDef{}, false
	}
	i, ok := s.index().kinds[code]
	if !ok {
		return KindDef{}, false
	}
	return s.Kinds[i], true
}

// System ищет систему обозначений по коду.
func (s *Snapshot) System(code domain.System) (SystemDef, bool) {
	if s == nil {
		return SystemDef{}, false
	}
	i, ok := s.index().systems[code]
	if !ok {
		return SystemDef{}, false
	}
	return s.Systems[i], true
}

// SystemAppliesTo сообщает, применима ли система обозначений к классу.
func (s *Snapshot) SystemAppliesTo(system domain.System, kind domain.Kind) bool {
	if s == nil {
		return false
	}
	return s.index().systemKinds[systemKindKey{system, kind}]
}

// Family ищет семейство системы series по коду и классу.
func (s *Snapshot) Family(series string, kind domain.Kind) (SeriesFamilyDef, bool) {
	if s == nil {
		return SeriesFamilyDef{}, false
	}
	i, ok := s.index().families[seriesKey{series, kind}]
	if !ok {
		return SeriesFamilyDef{}, false
	}
	return s.SeriesFamilies[i], true
}

// MatchSeriesFamily ищет семейство класса kind по самому длинному
// совпадению префикса обозначения (ПЭВ раньше ПЭ, ОМЛТ раньше МЛТ).
func (s *Snapshot) MatchSeriesFamily(designation string, kind domain.Kind) (SeriesFamilyDef, bool) {
	if s == nil {
		return SeriesFamilyDef{}, false
	}
	best, bestLen := -1, 0
	for i := range s.SeriesFamilies {
		f := &s.SeriesFamilies[i]
		if f.Kind != kind || !strings.HasPrefix(designation, f.Series) {
			continue
		}
		if n := utf8.RuneCountInString(f.Series); n > bestLen {
			best, bestLen = i, n
		}
	}
	if best < 0 {
		return SeriesFamilyDef{}, false
	}
	return s.SeriesFamilies[best], true
}

// Unit ищет единицу по коду.
func (s *Snapshot) Unit(code string) (UnitDef, bool) {
	if s == nil {
		return UnitDef{}, false
	}
	i, ok := s.index().units[code]
	if !ok {
		return UnitDef{}, false
	}
	return s.Units[i], true
}

// Category ищет код словаря категорий (поле category).
func (s *Snapshot) Category(code string) (CategoryDef, bool) {
	if s == nil {
		return CategoryDef{}, false
	}
	i, ok := s.index().categories[code]
	if !ok {
		return CategoryDef{}, false
	}
	return s.Categories[i], true
}

// CategoryCodes перечисляет коды словаря категорий в порядке строк снимка
// (аргумент перечня допустимых в сообщениях об ошибках).
func (s *Snapshot) CategoryCodes() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.Categories))
	for _, c := range s.Categories {
		out = append(out, c.Code)
	}
	return out
}

// Condition ищет условие по коду.
func (s *Snapshot) Condition(code string) (ConditionDef, bool) {
	if s == nil {
		return ConditionDef{}, false
	}
	i, ok := s.index().conditions[code]
	if !ok {
		return ConditionDef{}, false
	}
	return s.Conditions[i], true
}

// Group ищет группу параметров по коду.
func (s *Snapshot) Group(code string) (GroupDef, bool) {
	if s == nil {
		return GroupDef{}, false
	}
	i, ok := s.index().groups[code]
	if !ok {
		return GroupDef{}, false
	}
	return s.Groups[i], true
}

// GroupBySection ищет группу по имени секции файла наполнения.
func (s *Snapshot) GroupBySection(section string) (GroupDef, bool) {
	if s == nil {
		return GroupDef{}, false
	}
	i, ok := s.index().groupSections[section]
	if !ok {
		return GroupDef{}, false
	}
	return s.Groups[i], true
}

// Parameter ищет параметр по коду; указатель стабилен, пока жив снимок.
func (s *Snapshot) Parameter(code string) (*ParameterDef, bool) {
	if s == nil {
		return nil, false
	}
	i, ok := s.index().parameters[code]
	if !ok {
		return nil, false
	}
	return &s.Parameters[i], true
}

// Attribute ищет атрибут по коду; указатель стабилен, пока жив снимок.
func (s *Snapshot) Attribute(code string) (*AttributeDef, bool) {
	if s == nil {
		return nil, false
	}
	i, ok := s.index().attributes[code]
	if !ok {
		return nil, false
	}
	return &s.Attributes[i], true
}

// RulesForKind возвращает коды именованных правил, привязанных к классу
// записи (kind_validation_rules), в порядке строк снимка.
func (s *Snapshot) RulesForKind(kind domain.Kind) []string {
	if s == nil {
		return nil
	}
	return s.index().kindRules[kind]
}

// Clone глубоко копирует снимок (основа для применения каталога).
func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return &Snapshot{}
	}
	out := &Snapshot{
		Revision:       s.Revision,
		Kinds:          slices.Clone(s.Kinds),
		Systems:        slices.Clone(s.Systems),
		SystemKinds:    slices.Clone(s.SystemKinds),
		SeriesFamilies: slices.Clone(s.SeriesFamilies),
		Units:          slices.Clone(s.Units),
		Categories:     slices.Clone(s.Categories),
		Conditions:     slices.Clone(s.Conditions),
		Groups:         slices.Clone(s.Groups),
		Parameters:     slices.Clone(s.Parameters),
		Attributes:     slices.Clone(s.Attributes),
		Rules:          slices.Clone(s.Rules),
		KindRules:      slices.Clone(s.KindRules),
	}
	for i := range out.Parameters {
		p := &out.Parameters[i]
		p.Kinds = slices.Clone(p.Kinds)
		p.EnumValues = slices.Clone(p.EnumValues)
		if p.Ceiling != nil {
			c := *p.Ceiling
			p.Ceiling = &c
		}
		sets := make([]ConditionSet, len(p.ConditionSets))
		for j, set := range p.ConditionSets {
			items := make([]ConditionSetItem, len(set.Items))
			for k, it := range set.Items {
				if it.FixedValue != nil {
					fv := *it.FixedValue
					items[k] = ConditionSetItem{Condition: it.Condition, Mode: it.Mode, FixedValue: &fv}
					continue
				}
				items[k] = it
			}
			sets[j] = ConditionSet{No: set.No, Items: items}
		}
		p.ConditionSets = sets
	}
	for i := range out.Attributes {
		a := &out.Attributes[i]
		a.Kinds = slices.Clone(a.Kinds)
		a.EnumValues = slices.Clone(a.EnumValues)
	}
	return out
}
