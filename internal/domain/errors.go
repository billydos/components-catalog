package domain

import (
	"errors"
	"fmt"
	"slices"
)

// Code — машиночитаемый код ошибки (латиница, snake_case).
type Code string

// Реестр кодов ошибок — контракт CLI, REST и импорта; коды стабильны,
// изменение кода — несовместимое изменение. Исходный перечень —
// docs/plan/01-architecture.md §2.1, расширяется по мере этапов плана.
const (
	CodeAlreadyExists          Code = "already_exists"
	CodeAttributeNotApplicable Code = "attribute_not_applicable"
	CodeDatabaseNotInitialized Code = "database_not_initialized"
	CodeDesignationMismatch    Code = "designation_mismatch"
	CodeInvalidDesignation     Code = "invalid_designation"
	CodeInvalidImportFile      Code = "invalid_import_file"
	CodeInternal               Code = "internal_error"
	CodeKindAmbiguous          Code = "kind_ambiguous"
	CodeKindNotSupported       Code = "kind_not_supported"
	CodeMethodNotAllowed       Code = "method_not_allowed"
	CodeNotFound               Code = "not_found"
	CodeParameterNotApplicable Code = "parameter_not_applicable"
	CodePowerSuffix            Code = "power_suffix_not_supported"
	CodeSchemaVersionMismatch  Code = "schema_version_mismatch"
	CodeUnknownAttribute       Code = "unknown_attribute"
	CodeUnknownCondition       Code = "unknown_condition"
	CodeUnknownParameter       Code = "unknown_parameter"
	CodeValidationFailed       Code = "validation_failed"
)

var codeRegistry = map[Code]struct{}{
	CodeAlreadyExists:          {},
	CodeAttributeNotApplicable: {},
	CodeDatabaseNotInitialized: {},
	CodeDesignationMismatch:    {},
	CodeInvalidDesignation:     {},
	CodeInvalidImportFile:      {},
	CodeInternal:               {},
	CodeKindAmbiguous:          {},
	CodeKindNotSupported:       {},
	CodeMethodNotAllowed:       {},
	CodeNotFound:               {},
	CodeParameterNotApplicable: {},
	CodePowerSuffix:            {},
	CodeSchemaVersionMismatch:  {},
	CodeUnknownAttribute:       {},
	CodeUnknownCondition:       {},
	CodeUnknownParameter:       {},
	CodeValidationFailed:       {},
}

// IsKnown сообщает, зарегистрирован ли код в реестре.
func (c Code) IsKnown() bool {
	_, ok := codeRegistry[c]
	return ok
}

// KnownCodes возвращает все зарегистрированные коды, отсортированные.
func KnownCodes() []Code {
	codes := make([]Code, 0, len(codeRegistry))
	for c := range codeRegistry {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	return codes
}

// Error — ожидаемая ошибка модуля: машиночитаемый код и дословный русский
// текст сообщения. Тексты — контракт, закреплённый тестами дословно.
type Error struct {
	Code    Code
	Message string
}

// Error возвращает дословный текст сообщения (без кода: код transport
// передаёт отдельно).
func (e *Error) Error() string {
	return e.Message
}

// NewError создаёт ожидаемую ошибку по коду и тексту сообщения.
func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// AsError извлекает *Error из цепочки ошибок; классификация
// «ожидаемая/непредвиденная» для вывода CLI и статусов REST.
func AsError(err error) (*Error, bool) {
	var de *Error
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

// Дословные тексты сообщений — контракт (docs/plan/02-database.md §5,
// docs/plan/03-data-model.md §2.4); закреплены тестами дословно.
const (
	msgSchemaVersionMismatch  = "база данных создана другой версией модуля (%d ≠ %d); пересоздайте её: удалите файл/базу и выполните import"
	msgDatabaseNotInitialized = "база данных не инициализирована или не является базой модуля; выполните init (CLI) или EnsureCreated"
	msgKindNotSupported       = "обозначение принадлежит классу, не поддерживаемому модулем"
	msgKindAmbiguous          = "класс прибора не определяется по обозначению однозначно; укажите класс явно"
)

// SchemaVersionMismatch — версия схемы базы (dbVersion) не совпадает
// с версией модуля (moduleVersion); продолжение работы запрещено.
func SchemaVersionMismatch(dbVersion, moduleVersion int) *Error {
	return &Error{
		Code:    CodeSchemaVersionMismatch,
		Message: fmt.Sprintf(msgSchemaVersionMismatch, dbVersion, moduleVersion),
	}
}

// DatabaseNotInitialized — в базе нет таблицы schema_meta: база не
// инициализирована, принадлежит другой программе или повреждена.
func DatabaseNotInitialized() *Error {
	return &Error{Code: CodeDatabaseNotInitialized, Message: msgDatabaseNotInitialized}
}

// KindNotSupported — обозначение принадлежит классу приборов,
// не поддерживаемому модулем.
func KindNotSupported() *Error {
	return &Error{Code: CodeKindNotSupported, Message: msgKindNotSupported}
}

// KindAmbiguous — класс не определяется по обозначению однозначно
// (фотоприборы: фотодиоды и фототранзисторы) — требуется явное указание
// класса (ключ kind / --kind), которое переопределяет автодетект.
func KindAmbiguous() *Error {
	return &Error{Code: CodeKindAmbiguous, Message: msgKindAmbiguous}
}
