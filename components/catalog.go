package components

import (
	"github.com/billydos/components-catalog/internal/catalog"
)

// Снимок каталога (App.Snapshot / CatalogService.Snapshot) и вход
// расширения каталога (CatalogService.Import; штатный путь расширения —
// файлы наполнения через Importer.ImportCatalogFile).

type (
	Snapshot     = catalog.Snapshot
	CatalogInput = catalog.Input
)

// Значения записей: атрибуты и параметры с условиями измерения
// (DeviceInput.Attributes, SectionInput.Values; канонические единицы).

type (
	AttributeValue = catalog.AttributeValue
	ParameterValue = catalog.ParameterValue
	ConditionValue = catalog.ConditionValue
)

// Определения каталога (поля Snapshot и CatalogInput).

type (
	KindDef          = catalog.KindDef
	SystemDef        = catalog.SystemDef
	SystemKindRef    = catalog.SystemKindRef
	UnitDef          = catalog.UnitDef
	CategoryDef      = catalog.CategoryDef
	ConditionDef     = catalog.ConditionDef
	GroupDef         = catalog.GroupDef
	ConditionSet     = catalog.ConditionSet
	ConditionSetItem = catalog.ConditionSetItem
	ParameterDef     = catalog.ParameterDef
	AttributeDef     = catalog.AttributeDef
	RuleDef          = catalog.RuleDef
	KindRuleRef      = catalog.KindRuleRef
)

// Типы значений параметров (колонка value_type параметров каталога).

type ValueType = catalog.ValueType

const (
	ValueExact   = catalog.ValueExact
	ValueAtLeast = catalog.ValueAtLeast
	ValueAtMost  = catalog.ValueAtMost
	ValueRange   = catalog.ValueRange
	ValueText    = catalog.ValueText
	ValueEnum    = catalog.ValueEnum
)

// Типы значений атрибутов (docs/plan/03-data-model.md §10).

type AttrType = catalog.AttrType

const (
	AttrText   = catalog.AttrText
	AttrBool   = catalog.AttrBool
	AttrInt    = catalog.AttrInt
	AttrNumber = catalog.AttrNumber
	AttrEnum   = catalog.AttrEnum
)

// Роли условий в наборе условий параметра.

type ConditionMode = catalog.ConditionMode

const (
	ModeRequired = catalog.ModeRequired
	ModeOptional = catalog.ModeOptional
)
