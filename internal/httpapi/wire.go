package httpapi

import (
	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/service"
)

// Модели провода (wire): JSON-представление ответов /api/v1. Ключи —
// snake_case, как в формате наполнения (коды и колонки каталога);
// контракт закреплён api/openapi.yaml и тестами. Отображаемые поля
// (name, kind_name, system_name, unit name/symbol, description) —
// локализуемые строки бандлов internal/i18n по локали запроса (D9);
// кодовые поля всегда канонические.

// fieldJSON — поле разбора обозначения: текстовое либо числовое.
type fieldJSON struct {
	Name string   `json:"name"`
	Text *string  `json:"text,omitempty"`
	Num  *float64 `json:"num,omitempty"`
}

func fieldToJSON(f domain.Field) fieldJSON {
	out := fieldJSON{Name: f.Name}
	if f.IsNum {
		num := f.Num
		out.Num = &num
		return out
	}
	text := f.Text
	out.Text = &text
	return out
}

func fieldsToJSON(fields []domain.Field) []fieldJSON {
	out := make([]fieldJSON, 0, len(fields))
	for _, f := range fields {
		out = append(out, fieldToJSON(f))
	}
	return out
}

// cardAttrJSON — значение атрибута карточки.
type cardAttrJSON struct {
	Code string   `json:"code"`
	Name string   `json:"name"`
	Text *string  `json:"text,omitempty"`
	Num  *float64 `json:"num,omitempty"`
	Bool *bool    `json:"bool,omitempty"`
}

// conditionJSON — условие измерения значения.
type conditionJSON struct {
	Condition string  `json:"condition"`
	Value     float64 `json:"value"`
}

// cardValueJSON — значение параметра с условиями.
type cardValueJSON struct {
	Parameter  string          `json:"parameter"`
	Name       string          `json:"name"`
	Unit       string          `json:"unit,omitempty"`
	Exact      *float64        `json:"exact,omitempty"`
	Min        *float64        `json:"min,omitempty"`
	Max        *float64        `json:"max,omitempty"`
	Text       *string         `json:"text,omitempty"`
	Conditions []conditionJSON `json:"conditions,omitempty"`
}

// cardGroupJSON — группа каталога со значениями.
type cardGroupJSON struct {
	Code    string          `json:"code"`
	Section string          `json:"section"`
	Name    string          `json:"name"`
	Values  []cardValueJSON `json:"values"`
}

// cardVariantJSON — исполнение (матрица «номинал × напряжение → габариты»
// собирается группами каталога).
type cardVariantJSON struct {
	Label  string          `json:"label"`
	Groups []cardGroupJSON `json:"groups"`
}

// linkJSON — ссылка-аналог: исходящая либо встречная.
type linkJSON struct {
	Kind        string `json:"kind"`
	System      string `json:"system"`
	Designation string `json:"designation"`
	Note        string `json:"note,omitempty"`
}

// cardJSON — карточка записи (docs/plan/04-module-functionality.md §2).
type cardJSON struct {
	ID            int64             `json:"id"`
	Kind          string            `json:"kind"`
	KindName      string            `json:"kind_name"`
	System        string            `json:"system"`
	SystemName    string            `json:"system_name"`
	Designation   string            `json:"designation"`
	Fields        []fieldJSON       `json:"fields"`
	Attributes    []cardAttrJSON    `json:"attributes"`
	Groups        []cardGroupJSON   `json:"groups"`
	Variants      []cardVariantJSON `json:"variants"`
	Manufacturers []string          `json:"manufacturers"`
	Analogs       []linkJSON        `json:"analogs"`
	Backlinks     []linkJSON        `json:"backlinks"`
}

func cardToJSON(lang i18n.Language, c *service.Card) cardJSON {
	out := cardJSON{
		ID:            c.ID,
		Kind:          string(c.Kind),
		KindName:      i18n.KindName(lang, string(c.Kind)),
		System:        string(c.System),
		SystemName:    i18n.SystemName(lang, string(c.System)),
		Designation:   c.Designation,
		Fields:        fieldsToJSON(c.Fields),
		Attributes:    make([]cardAttrJSON, 0, len(c.Attributes)),
		Variants:      make([]cardVariantJSON, 0, len(c.Variants)),
		Manufacturers: make([]string, 0, len(c.Manufacturers)),
		Analogs:       make([]linkJSON, 0, len(c.Analogs)),
		Backlinks:     make([]linkJSON, 0, len(c.Backlinks)),
	}
	for _, a := range c.Attributes {
		out.Attributes = append(out.Attributes, cardAttrJSON{
			Code: a.Code, Name: i18n.AttributeName(lang, a.Code),
			Text: a.Text, Num: a.Num, Bool: a.Bool,
		})
	}
	out.Groups = groupsToJSON(lang, c.Groups)
	for _, v := range c.Variants {
		out.Variants = append(out.Variants, cardVariantJSON{Label: v.Label, Groups: groupsToJSON(lang, v.Groups)})
	}
	out.Manufacturers = append(out.Manufacturers, c.Manufacturers...)
	for _, l := range c.Analogs {
		out.Analogs = append(out.Analogs, linkToJSON(l))
	}
	for _, l := range c.Backlinks {
		out.Backlinks = append(out.Backlinks, linkToJSON(l))
	}
	return out
}

func groupsToJSON(lang i18n.Language, groups []service.CardGroup) []cardGroupJSON {
	out := make([]cardGroupJSON, 0, len(groups))
	for _, g := range groups {
		gj := cardGroupJSON{
			Code: g.Code, Section: g.Section, Name: i18n.GroupName(lang, g.Code),
			Values: make([]cardValueJSON, 0, len(g.Values)),
		}
		for _, v := range g.Values {
			vj := cardValueJSON{
				Parameter: v.Parameter, Name: i18n.ParameterName(lang, v.Parameter),
				Unit:  v.Unit,
				Exact: v.Exact, Min: v.Min, Max: v.Max, Text: v.Text,
			}
			for _, c := range v.Conditions {
				vj.Conditions = append(vj.Conditions, conditionJSON{Condition: c.Condition, Value: c.Value})
			}
			gj.Values = append(gj.Values, vj)
		}
		out = append(out, gj)
	}
	return out
}

func linkToJSON(l service.CardLink) linkJSON {
	return linkJSON{
		Kind: string(l.Kind), System: string(l.System),
		Designation: l.Designation, Note: l.Note,
	}
}

// upsertResponseJSON — итог записи POST/PUT: исход и карточка.
type upsertResponseJSON struct {
	Outcome string   `json:"outcome"`
	Card    cardJSON `json:"card"`
}

// searchItemJSON — строка результатов поиска.
type searchItemJSON struct {
	ID          int64       `json:"id"`
	Kind        string      `json:"kind"`
	System      string      `json:"system"`
	Designation string      `json:"designation"`
	Fields      []fieldJSON `json:"fields"`
}

// searchPageJSON — страница поиска с общим числом под фильтрами.
type searchPageJSON struct {
	Items  []searchItemJSON `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

func searchPageToJSON(p service.SearchPage) searchPageJSON {
	out := searchPageJSON{
		Items: make([]searchItemJSON, 0, len(p.Items)),
		Total: p.Total, Limit: p.Limit, Offset: p.Offset,
	}
	for _, it := range p.Items {
		out.Items = append(out.Items, searchItemJSON{
			ID: it.ID, Kind: string(it.Kind), System: string(it.System),
			Designation: it.Designation, Fields: fieldsToJSON(it.Fields),
		})
	}
	return out
}

// kindsResponseJSON — GET /kinds.
type kindsResponseJSON struct {
	Kinds []kindDefJSON `json:"kinds"`
}

type kindDefJSON struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// suggestResponseJSON — GET /suggest.
type suggestResponseJSON struct {
	Items []suggestionJSON `json:"items"`
}

type suggestionJSON struct {
	Kind        string `json:"kind"`
	System      string `json:"system"`
	Designation string `json:"designation"`
}

// statsJSON — GET /stats: количества по классам, версия схемы, ревизии.
type statsJSON struct {
	SchemaVersion   int            `json:"schema_version"`
	CatalogRevision int64          `json:"catalog_revision"`
	DataRevision    int64          `json:"data_revision"`
	Total           int            `json:"total"`
	Kinds           map[string]int `json:"kinds"`
}

// Снимок каталога (GET /catalog): ключи повторяют формат наполнения
// (snake_case по колонкам каталожных таблиц — docs/plan/04 §4).

type catalogSnapshotJSON struct {
	Revision    int64               `json:"revision"`
	Kinds       []kindDefJSON       `json:"kinds"`
	Systems     []systemDefJSON     `json:"systems"`
	SystemKinds []systemKindRefJSON `json:"system_kinds"`
	Units       []unitDefJSON       `json:"units"`
	Conditions  []conditionDefJSON  `json:"conditions"`
	Groups      []groupDefJSON      `json:"groups"`
	Parameters  []parameterDefJSON  `json:"parameters"`
	Attributes  []attributeDefJSON  `json:"attributes"`
	Rules       []ruleDefJSON       `json:"rules"`
	KindRules   []kindRuleRefJSON   `json:"kind_rules"`
}

type systemDefJSON struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type systemKindRefJSON struct {
	System string `json:"system"`
	Kind   string `json:"kind"`
}

type unitDefJSON struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

type conditionDefJSON struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Unit          string `json:"unit,omitempty"`
	AllowNegative bool   `json:"allow_negative"`
}

type groupDefJSON struct {
	Code      string `json:"code"`
	Section   string `json:"section"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type parameterDefJSON struct {
	Code           string             `json:"code"`
	Group          string             `json:"group"`
	Name           string             `json:"name"`
	Unit           string             `json:"unit,omitempty"`
	ValueType      string             `json:"value_type"`
	Ceiling        *float64           `json:"ceiling,omitempty"`
	AllowNegative  bool               `json:"allow_negative"`
	ValidationRule string             `json:"validation_rule,omitempty"`
	SortOrder      int                `json:"sort_order"`
	IsActive       bool               `json:"is_active"`
	Kinds          []string           `json:"kinds"`
	EnumValues     []string           `json:"enum_values"`
	ConditionSets  []conditionSetJSON `json:"condition_sets"`
}

type conditionSetJSON struct {
	No    int                    `json:"no"`
	Items []conditionSetItemJSON `json:"items"`
}

type conditionSetItemJSON struct {
	Condition  string   `json:"condition"`
	Mode       string   `json:"mode"`
	FixedValue *float64 `json:"fixed_value,omitempty"`
}

type attributeDefJSON struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Group          string   `json:"group,omitempty"`
	Type           string   `json:"type"`
	Unit           string   `json:"unit,omitempty"`
	ValidationRule string   `json:"validation_rule,omitempty"`
	SortOrder      int      `json:"sort_order"`
	IsActive       bool     `json:"is_active"`
	Kinds          []string `json:"kinds"`
	EnumValues     []string `json:"enum_values"`
}

type ruleDefJSON struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type kindRuleRefJSON struct {
	Kind string `json:"kind"`
	Rule string `json:"rule"`
}

func catalogToJSON(lang i18n.Language, s *catalog.Snapshot) catalogSnapshotJSON {
	out := catalogSnapshotJSON{
		Revision: s.Revision,
		Kinds:    make([]kindDefJSON, 0, len(s.Kinds)),
	}
	for _, k := range s.Kinds {
		out.Kinds = append(out.Kinds, kindDefJSON{
			Code: string(k.Code), Name: i18n.KindName(lang, string(k.Code)),
		})
	}
	for _, sys := range s.Systems {
		out.Systems = append(out.Systems, systemDefJSON{
			Code:        string(sys.Code),
			Name:        i18n.SystemName(lang, string(sys.Code)),
			Description: i18n.SystemDescription(lang, string(sys.Code)),
		})
	}
	for _, ref := range s.SystemKinds {
		out.SystemKinds = append(out.SystemKinds, systemKindRefJSON{
			System: string(ref.System), Kind: string(ref.Kind),
		})
	}
	for _, u := range s.Units {
		out.Units = append(out.Units, unitDefJSON{
			Code:   u.Code,
			Name:   i18n.UnitName(lang, u.Code),
			Symbol: i18n.UnitSymbol(lang, u.Code),
		})
	}
	for _, c := range s.Conditions {
		out.Conditions = append(out.Conditions, conditionDefJSON{
			Code: c.Code, Name: i18n.ConditionName(lang, c.Code),
			Unit: c.Unit, AllowNegative: c.AllowNegative,
		})
	}
	for _, g := range s.Groups {
		out.Groups = append(out.Groups, groupDefJSON{
			Code: g.Code, Section: g.SectionName,
			Name:      i18n.GroupName(lang, g.Code),
			SortOrder: g.SortOrder,
		})
	}
	for _, p := range s.Parameters {
		pj := parameterDefJSON{
			Code: p.Code, Group: p.Group, Name: i18n.ParameterName(lang, p.Code), Unit: p.Unit,
			ValueType: string(p.ValueType), Ceiling: p.Ceiling, AllowNegative: p.AllowNegative,
			ValidationRule: p.ValidationRule, SortOrder: p.SortOrder, IsActive: p.Active,
			Kinds:      kindsToJSON(p.Kinds),
			EnumValues: make([]string, 0, len(p.EnumValues)),
		}
		pj.EnumValues = append(pj.EnumValues, p.EnumValues...)
		for _, set := range p.ConditionSets {
			sj := conditionSetJSON{No: set.No, Items: make([]conditionSetItemJSON, 0, len(set.Items))}
			for _, it := range set.Items {
				sj.Items = append(sj.Items, conditionSetItemJSON{
					Condition: it.Condition, Mode: string(it.Mode), FixedValue: it.FixedValue,
				})
			}
			pj.ConditionSets = append(pj.ConditionSets, sj)
		}
		out.Parameters = append(out.Parameters, pj)
	}
	for _, at := range s.Attributes {
		aj := attributeDefJSON{
			Code: at.Code, Name: i18n.AttributeName(lang, at.Code), Group: at.GroupName, Type: string(at.Type),
			Unit: at.Unit, ValidationRule: at.ValidationRule, SortOrder: at.SortOrder,
			IsActive: at.Active, Kinds: kindsToJSON(at.Kinds),
			EnumValues: make([]string, 0, len(at.EnumValues)),
		}
		aj.EnumValues = append(aj.EnumValues, at.EnumValues...)
		out.Attributes = append(out.Attributes, aj)
	}
	for _, rl := range s.Rules {
		out.Rules = append(out.Rules, ruleDefJSON{
			Code: rl.Code, Description: i18n.RuleDescription(lang, rl.Code),
		})
	}
	for _, kr := range s.KindRules {
		out.KindRules = append(out.KindRules, kindRuleRefJSON{Kind: string(kr.Kind), Rule: kr.Rule})
	}
	return out
}

func kindsToJSON(kinds []domain.Kind) []string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, string(k))
	}
	return out
}
