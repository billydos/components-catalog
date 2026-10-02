package service

import (
	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Outcome — исход Upsert записи (plan/04-module-functionality.md §1.1).
type Outcome string

// Реестр исходов Upsert.
const (
	// OutcomeAdded — записи не было, создана.
	OutcomeAdded Outcome = "Added"
	// OutcomeUpdatedExisting — запись была, секции применены.
	OutcomeUpdatedExisting Outcome = "UpdatedExisting"
	// OutcomeSkipped — слитое состояние совпадает с текущим: записи в БД
	// не выполнялось, ревизии не инкрементировались.
	OutcomeSkipped Outcome = "Skipped"
)

// SectionInput — секция группы параметров записи (имя — из группы каталога:
// parameters/ratings/dimensions); nil-семантики на уровне DeviceInput.
type SectionInput struct {
	Section string
	Values  []catalog.ParameterValue
}

// VariantInput — исполнение типа (D6): метка для карточки и собственные
// значения параметров по секциям групп.
type VariantInput struct {
	Label    string
	Sections []SectionInput
}

// AnalogInput — исходящая ссылка-аналог (D8): обозначение того же класса,
// опционально с заметкой характера замены в этом направлении.
type AnalogInput struct {
	Designation string
	Note        string
}

// DeviceInput — запись наполнения для Upsert. Семантика секций
// (plan/01-architecture.md §2.4): отсутствует (nil) — не менять; задана —
// заменить целиком; пустой непустой-nil срез/[] — очистить; ошибка в любом
// значении секции — запись не применяется вовсе. System/Kind необязательны
// (автодетект; Kind обязателен для other).
type DeviceInput struct {
	Name   string
	System domain.System
	Kind   domain.Kind

	Attributes    []catalog.AttributeValue
	Sections      []SectionInput
	Manufacturers *[]string
	Variants      *[]VariantInput
	Analogs       *[]AnalogInput
}

// FindResult — результат find: точное совпадение и подсказка равнозначной
// записи по материалу (полупроводники gost: Г/1, К/2 — раздельные записи).
type FindResult struct {
	Found      *Card
	Suggestion *Card
}

// CardAttribute — значение атрибута в карточке.
type CardAttribute struct {
	Code        string
	DisplayName string
	Text        *string
	Num         *float64
	Bool        *bool
}

// CardValue — значение параметра с условиями в карточке.
type CardValue struct {
	Parameter   string
	DisplayName string
	Unit        string
	Exact       *float64
	Min         *float64
	Max         *float64
	Text        *string
	Conditions  []catalog.ConditionValue
}

// CardGroup — группа каталога со значениями на карточке.
type CardGroup struct {
	Code        string
	Section     string
	DisplayName string
	Values      []CardValue
}

// CardVariant — исполнение на карточке (матрица «номинал × напряжение →
// габариты/масса» собирается группами каталога).
type CardVariant struct {
	Label  string
	Groups []CardGroup
}

// CardLink — ссылка-аналог: исходящая (аналоги записи) либо встречная
// (кто ссылается на запись).
type CardLink struct {
	Kind        domain.Kind
	System      domain.System
	Designation string
	Note        string
}

// Card — карточка записи: обозначение, система, поля разбора, атрибуты,
// значения параметров по группам, исполнения, производители, аналоги
// (исходящие и встречные — plan/02-database.md §6).
type Card struct {
	ID            int64
	Kind          domain.Kind
	KindName      string
	System        domain.System
	SystemName    string
	Designation   string
	Fields        []domain.Field
	Attributes    []CardAttribute
	Groups        []CardGroup
	Variants      []CardVariant
	Manufacturers []string
	Analogs       []CardLink
	Backlinks     []CardLink
}

// FieldByName ищет поле разбора обозначения в карточке по коду.
func (c *Card) FieldByName(name string) (domain.Field, bool) {
	for _, f := range c.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return domain.Field{}, false
}

// FilterOp — операция сравнения числового фильтра поиска.
type FilterOp string

// Операции фильтров поиска.
const (
	OpEq  FilterOp = "eq"
	OpGte FilterOp = "gte"
	OpLte FilterOp = "lte"
)

// FieldFilter — фильтр по полю разбора обозначения (material, subclass,
// junctions, group, dev_number, series…): текстовое равенство (Text) либо
// числовое со сравнением (Num + Op).
type FieldFilter struct {
	Field  string
	Text   string
	Num    float64
	HasNum bool
	Op     FilterOp
}

// AttributeFilter — фильтр по атрибуту (код op значение).
type AttributeFilter struct {
	Attribute string
	Text      string
	Num       float64
	HasNum    bool
	Op        FilterOp
}

// ParameterFilter — параметрический фильтр: границы (Min/Max), точное
// значение (Exact) либо текст/enum (Text); вариантные параметры применяются
// в пределах одного исполнения.
type ParameterFilter struct {
	Parameter string
	Min       *float64
	Max       *float64
	Exact     *float64
	Text      string
}

// SortField — ключ сортировки: "designation", "id" либо код поля разбора
// (Numeric — поле числовое).
type SortField struct {
	Key     string
	Numeric bool
	Desc    bool
}

// SearchQuery — запрос поиска: класс, система, подстрока обозначения,
// фильтры полей/атрибутов/параметров, сортировка, пагинация
// (plan/04-module-functionality.md §1.1).
type SearchQuery struct {
	Kind       domain.Kind
	System     domain.System
	Query      string
	Fields     []FieldFilter
	Attributes []AttributeFilter
	Parameters []ParameterFilter
	Sort       []SortField
	Limit      int
	Offset     int
}

// SearchItem — строка результатов поиска.
type SearchItem struct {
	ID          int64
	Kind        domain.Kind
	System      domain.System
	Designation string
	Fields      []domain.Field
}

// SearchPage — страница результатов с общим числом записей под фильтрами.
type SearchPage struct {
	Items  []SearchItem
	Total  int
	Limit  int
	Offset int
}

// Suggestion — подсказка автодополнения по префиксу обозначения.
type Suggestion struct {
	Kind        domain.Kind
	System      domain.System
	Designation string
}

// Лимиты пагинации (plan/04-module-functionality.md §1.1: limit ≤ 200);
// потолки нужны транспорту для валидации параметров запроса.
const (
	SearchDefaultLimit  = 50
	SearchLimitMax      = 200
	SuggestDefaultLimit = 10
	SuggestLimitMax     = 50
)
