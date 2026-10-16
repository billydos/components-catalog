package components

import (
	"github.com/billydos/components-catalog/internal/domain"
)

// Класс приборов (реестр синхронизирован с сидами kinds).

type Kind = domain.Kind

const (
	KindTransistor = domain.KindTransistor
	KindDiode      = domain.KindDiode
	KindResistor   = domain.KindResistor
	KindCapacitor  = domain.KindCapacitor
	KindRelay      = domain.KindRelay
)

// Система обозначений (реестр синхронизирован с сидами designation_systems).

type System = domain.System

const (
	SystemGost  = domain.SystemGost
	SystemOst   = domain.SystemOst
	SystemPro   = domain.SystemPro
	SystemJedec = domain.SystemJedec
	SystemJis   = domain.SystemJis
	SystemOther = domain.SystemOther
)

// Поле разбора обозначения (текстовое либо числовое) и результат разбора
// Designations.Parse: класс, система, каноническое обозначение, поля.

type (
	Field             = domain.Field
	ParsedDesignation = domain.ParsedDesignation
)

// Ошибки-значения: машиночитаемый код и сообщение каталога (D9). Слои выше
// возвращают *Error; AsError извлекает его для классификации на стороне
// потребителя (канонический текст — Error.Message).

type (
	Code  = domain.Code
	MsgID = domain.MsgID
	Error = domain.Error
)

// AsError извлекает *Error из цепочки ошибок.
func AsError(err error) (*Error, bool) { return domain.AsError(err) }
