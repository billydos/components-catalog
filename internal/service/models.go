package service

import (
	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Outcome — исход Upsert записи (docs/plan/04-module-functionality.md §1.1).
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

// SynonymInput — историческое название записи (синоним, план 03 §9):
// обозначение, опционально заметка происхождения и годы употребления
// имени. Синоним — не запись: обозначение разрешается в запись-владельца
// (find, поиск), своей карточки у него нет.
type SynonymInput struct {
	Designation string
	Note        string
	YearFrom    *int
	YearTo      *int
}

// DeviceInput — запись наполнения для Upsert. Семантика секций
// (docs/plan/01-architecture.md §2.4): отсутствует (nil) — не менять; задана —
// заменить целиком; пустой непустой-nil срез/[] — очистить; ошибка в любом
// значении секции — запись не применяется вовсе. System/Kind необязательны
// (автодетект; Kind обязателен для other). Fields — явные классификацион-
// ные поля (material/subclass/adjustment/category/assembly): допустимы
// только те, что парсер системы записи не устанавливает сам, и только
// применимые к классу (проверка — сервисным слоем после разбора).
type DeviceInput struct {
	Name   string
	System domain.System
	Kind   domain.Kind

	Fields        *[]domain.Field
	Attributes    []catalog.AttributeValue
	Sections      []SectionInput
	Manufacturers *[]string
	Variants      *[]VariantInput
	Analogs       *[]AnalogInput
	Synonyms      *[]SynonymInput
}

// FindResult — результат find: точное совпадение (MatchedBySynonym —
// обозначение синонима, когда совпадение нашлось по историческому
// названию, а не по ключу записи), подсказка равнозначной по материалу
// записи (полупроводники gost: Г/1, К/2 — раздельные записи) и подсказка
// соседом по типу (запись того же базового обозначения с другой буквой
// классификации либо хвостовым элементом — КТ315А → КТ315Б).
type FindResult struct {
	Found            *Card
	MatchedBySynonym string
	Suggestion       *Card
	TypeSuggestion   *Card
}

// CardAttribute — значение атрибута в карточке (отображаемое название —
// бандлы internal/i18n: attr.<код> — D9).
type CardAttribute struct {
	Code string
	Text *string
	Num  *float64
	Bool *bool
}

// CardValue — значение параметра с условиями в карточке (отображаемые
// название и символ единицы — бандлы internal/i18n: param.<код>,
// unit.<код>.symbol — D9; Unit — канонический код).
type CardValue struct {
	Parameter  string
	Unit       string
	Exact      *float64
	Min        *float64
	Max        *float64
	Text       *string
	Conditions []catalog.ConditionValue
}

// CardGroup — группа каталога со значениями на карточке (отображаемое
// название — бандлы internal/i18n: group.<код> — D9).
type CardGroup struct {
	Code    string
	Section string
	Values  []CardValue
}

// CardVariant — исполнение на карточке (матрица «номинал × напряжение →
// габариты/масса» собирается группами каталога).
type CardVariant struct {
	Label  string
	Groups []CardGroup
}

// CardTypeSibling — запись группы типа: запись того же класса с тем же
// базовым обозначением (поле base), кроме самой записи карточки.
type CardTypeSibling struct {
	ID          int64
	System      domain.System
	Designation string
}

// CardTypeValue — значение параметра одной записи группы типа (строка
// мини-матрицы различий: параметр × записи).
type CardTypeValue struct {
	Designation string
	Value       CardValue
}

// CardTypeDiff — параметр, значения которого различаются между записями
// группы типа (в матрицу входят только различия; порядок значений — по
// обозначению записи).
type CardTypeDiff struct {
	Parameter string
	Unit      string
	Values    []CardTypeValue
}

// CardTypeGroup — группа типа: записи одного базового обозначения
// (base) и мини-матрица различий их данных типа в целом (план 03 §7).
type CardTypeGroup struct {
	Base     string
	Siblings []CardTypeSibling
	Diffs    []CardTypeDiff
}

// CardLink — ссылка-аналог: исходящая (аналоги записи) либо встречная
// (кто ссылается на запись).
type CardLink struct {
	Kind        domain.Kind
	System      domain.System
	Designation string
	Note        string
}

// CardSynonym — историческое название записи на карточке: заметка —
// происхождение имени, годы — период его употребления (nil — не задано).
type CardSynonym struct {
	Designation string
	Note        string
	YearFrom    *int
	YearTo      *int
}

// Card — карточка записи: обозначение, система, поля разбора, атрибуты,
// значения параметров по группам, исполнения, группа типа (одно базовое
// обозначение — соседние записи и различия), производители, аналоги
// (исходящие и встречные — docs/plan/02-database.md §6) и исторические
// названия (синонимы — план 03 §9).
type Card struct {
	ID            int64
	Kind          domain.Kind
	System        domain.System
	Designation   string
	Fields        []domain.Field
	Attributes    []CardAttribute
	Groups        []CardGroup
	Variants      []CardVariant
	TypeGroup     *CardTypeGroup
	Manufacturers []string
	Analogs       []CardLink
	Backlinks     []CardLink
	Synonyms      []CardSynonym
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
// junctions, group, dev_number, series…): текстовое равенство (Text),
// набор текстовых значений с семантикой ИЛИ (Texts — словарные поля
// мультивыбора material/subclass/adjustment/category; повторные
// фильтры того же поля в запросе не сливаются — каждое значение отдельно
// остаётся семантикой И; потолка размера набора нет — REST ограничен
// мощностью словаря канонизацией значений, прямые Go-вызовы держат
// набор разумным) либо числовое со сравнением (Num + Op).
type FieldFilter struct {
	Field  string
	Text   string
	Texts  []string
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

// SortField — ключ сортировки: "designation", "id", код поля разбора
// (Numeric — поле числовое) либо код параметра каталога (Param — всегда
// численная сортировка по значению параметра; допустимость — сервисный
// слой по снимку каталога).
type SortField struct {
	Key     string
	Numeric bool
	Param   bool
	Desc    bool
}

// ValueColumn — колонка значений строки поиска: код параметра
// каталога либо код атрибута (Attr — форма запроса attr.<код>). Коды
// передаются явно: модуль не знает о курируемом наборе колонок сайта.
type ValueColumn struct {
	Code string
	Attr bool
}

// String — форма колонки запроса (attr.<код> для атрибутов) для
// сообщений об ошибках.
func (c ValueColumn) String() string {
	if c.Attr {
		return "attr." + c.Code
	}
	return c.Code
}

// SearchValue — значение колонки значений строки поиска:
// запрошенная колонка и её значение у записи формы CardValue (у атрибутов
// Parameter — код атрибута, заполнены Text либо Exact).
type SearchValue struct {
	Column ValueColumn
	Value  CardValue
}

// SearchQuery — запрос поиска: класс, система, подстрока обозначения,
// фильтр по набору стабильных id (пересечение с прочими фильтрами; без
// сортировки порядок выдачи — порядок передачи id), фильтры полей/
// атрибутов/параметров, сортировка, колонки значений (пакетная
// проекция в строки выдачи), пагинация
// (docs/plan/04-module-functionality.md §1.1).
type SearchQuery struct {
	Kind       domain.Kind
	System     domain.System
	Query      string
	IDs        []int64
	Fields     []FieldFilter
	Attributes []AttributeFilter
	Parameters []ParameterFilter
	Sort       []SortField
	Values     []ValueColumn
	Limit      int
	Offset     int
}

// SearchItem — строка результатов поиска; Values — значения запрошенных
// колонок в порядке запроса, присутствующие у записи (нет
// значения — нет элемента).
type SearchItem struct {
	ID          int64
	Kind        domain.Kind
	System      domain.System
	Designation string
	Fields      []domain.Field
	Values      []SearchValue
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

// Лимиты пагинации (docs/plan/04-module-functionality.md §1.1: limit ≤ 200);
// потолки нужны транспорту для валидации параметров запроса.
const (
	SearchDefaultLimit  = 50
	SearchLimitMax      = 200
	SuggestDefaultLimit = 10
	SuggestLimitMax     = 50
	// Фильтр по набору id (docs/plan/04-module-functionality.md §1.1):
	// до 200 положительных значений повторяемого параметра id.
	SearchIDsMax = 200
	// Колонки значений строк поиска: до 16 кодов повторяемого
	// параметра values (параметр либо attr.<атрибут>).
	SearchValuesMax = 16
	// Поток изменений (change feed): страница по умолчанию и потолок
	// GET /api/v1/changes.
	ChangesDefaultLimit = 100
	ChangesLimitMax     = 1000
)
