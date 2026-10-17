package components

import (
	"github.com/billydos/components-catalog/internal/service"
)

// Сервисы приложения (App.Services).

type (
	Services           = service.Services
	DeviceService      = service.DeviceService
	DesignationService = service.DesignationService
	CatalogService     = service.CatalogService
)

// Параметры пулов соединений (Config.Pool).

type PoolConfig = service.PoolConfig

// Записи наполнения: Upsert/Create сервисов устройств и DryRun импортёра.

type (
	DeviceInput         = service.DeviceInput
	SectionInput        = service.SectionInput
	VariantInput        = service.VariantInput
	AnalogInput         = service.AnalogInput
	PendingDesignations = service.PendingDesignations
)

// Исходы Upsert/Create.

type Outcome = service.Outcome

const (
	OutcomeAdded           = service.OutcomeAdded
	OutcomeUpdatedExisting = service.OutcomeUpdatedExisting
	OutcomeSkipped         = service.OutcomeSkipped
)

// Карточка записи (Get/GetByID/Find) и её блоки.

type (
	Card            = service.Card
	CardAttribute   = service.CardAttribute
	CardGroup       = service.CardGroup
	CardValue       = service.CardValue
	CardVariant     = service.CardVariant
	CardLink        = service.CardLink
	CardTypeGroup   = service.CardTypeGroup
	CardTypeSibling = service.CardTypeSibling
	CardTypeDiff    = service.CardTypeDiff
	CardTypeValue   = service.CardTypeValue
	FindResult      = service.FindResult
)

// Поиск: запрос, фильтры, сортировка, колонки значений, страница
// результатов.

type (
	SearchQuery     = service.SearchQuery
	SearchPage      = service.SearchPage
	SearchItem      = service.SearchItem
	SearchValue     = service.SearchValue
	SortField       = service.SortField
	FieldFilter     = service.FieldFilter
	AttributeFilter = service.AttributeFilter
	ParameterFilter = service.ParameterFilter
	ValueColumn     = service.ValueColumn
	FilterOp        = service.FilterOp
)

const (
	OpEq  = service.OpEq
	OpGte = service.OpGte
	OpLte = service.OpLte
)

// Лимиты пагинации и потолки фильтров (тот же контракт, что у REST).

const (
	SearchDefaultLimit  = service.SearchDefaultLimit
	SearchLimitMax      = service.SearchLimitMax
	SearchIDsMax        = service.SearchIDsMax
	SearchValuesMax     = service.SearchValuesMax
	SuggestDefaultLimit = service.SuggestDefaultLimit
	SuggestLimitMax     = service.SuggestLimitMax
	ChangesDefaultLimit = service.ChangesDefaultLimit
	ChangesLimitMax     = service.ChangesLimitMax
)

// Поток изменений записей (change feed).

type (
	ChangeEvent = service.ChangeEvent
	ChangeOp    = service.ChangeOp
	ChangesPage = service.ChangesPage
)

const (
	ChangeOpCreated = service.ChangeOpCreated
	ChangeOpUpdated = service.ChangeOpUpdated
	ChangeOpDeleted = service.ChangeOpDeleted
)

// Прочие типы сервисного слоя.

type (
	Stats      = service.Stats
	Suggestion = service.Suggestion
)

// Фасетные агрегаты значений параметров класса (ParameterStats).

type (
	ParameterStats     = service.ParameterStats
	ParameterStat      = service.ParameterStat
	ParameterValueFreq = service.ParameterValueFreq
)
