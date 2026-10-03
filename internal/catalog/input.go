package catalog

import (
	"github.com/billydos/components-catalog/internal/domain"
)

// Input — каталог для применения: секция catalog файла наполнения либо
// сиды (seed/). Применяется поверх существующего снимка upsert'ом по коду
// (docs/plan/02-database.md §5.4) через ApplyCatalog; разделы опциональны —
// заданные применяются, отсутствующие не трогают.
type Input struct {
	Kinds          []KindDef
	Systems        []SystemDef
	SystemKinds    []SystemKindRef
	SeriesFamilies []SeriesFamilyDef
	Units          []UnitDef
	Conditions     []ConditionDef
	Groups         []GroupDef
	Parameters     []ParameterDef
	Attributes     []AttributeDef
	Rules          []RuleDef
	KindRules      []KindRuleRef
}

// Device — запись наполнения для валидации движком: класс, система
// обозначений и каноническое обозначение (результат разбора — сервисный
// слой, этап 3), атрибуты, значения параметров типа в целом и исполнения.
// Форма значений повторяет файл наполнения (docs/plan/06-examples.md):
// значение задаётся ключом value (exact) | min/max | text, условия —
// соседними ключами по коду условия.
type Device struct {
	Kind        domain.Kind
	System      domain.System
	Designation string

	Attributes []AttributeValue
	Values     []ParameterValue
	Variants   []Variant
}

// AttributeValue — значение атрибута: ровно одно из Text/Num/Bool задано
// по типу атрибута каталога.
type AttributeValue struct {
	Attribute string
	Text      *string
	Num       *float64
	Bool      *bool
}

// ParameterValue — значение параметра с условиями измерения. Section —
// имя секции файла наполнения, в которой задано значение (группа
// каталога: parameters/ratings/dimensions); пустая секция пропускает
// проверку принадлежности (программатическое задание).
type ParameterValue struct {
	Parameter  string
	Section    string
	Exact      *float64
	Min        *float64
	Max        *float64
	Text       *string
	Conditions []ConditionValue
}

// ConditionValue — заданное условие измерения со значением
// (в канонических единицах условия).
type ConditionValue struct {
	Condition string
	Value     float64
}

// Variant — исполнение типа (D6): метка для карточки («160 В», «0.125 Вт»)
// и собственные значения параметров (включая массогабаритные).
type Variant struct {
	Label  string
	Values []ParameterValue
}

// Problem — выявленное нарушение: машиночитаемый код и сообщение каталога
// (MsgID + аргументы рендера, этап 8.3). Message — канонический en-рендер;
// локализованный рендер — транспорты (как у domain.Error). Коды и тексты —
// контракт CLI/REST; проблемы накапливаются за один прогон.
type Problem struct {
	Code    domain.Code
	MsgID   domain.MsgID
	Args    []any
	Message string
}

// Problemf строит проблему по коду и сообщению каталога.
func Problemf(code domain.Code, id domain.MsgID, args ...any) Problem {
	return Problem{Code: code, MsgID: id, Args: args, Message: domain.Msgf(id, args...)}
}

// Err преобразует проблему в ошибку домена.
func (p Problem) Err() *domain.Error {
	return domain.NewErrorf(p.Code, p.MsgID, p.Args...)
}
