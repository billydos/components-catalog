package catalog

import (
	"github.com/billydos/components-catalog/internal/domain"
)

// Ключи формата наполнения, зарезервированные служебными секциями
// (docs/fill-format.md). Реестр — единственный источник для читателя
// формата (internal/importer/reader.go) и инвариантов метасхемы:
// коды условий и имена секций групп не могут совпадать с зарезервиро-
// ванными ключами — такие определения каталога невыразимы в файле
// наполнения, round-trip их искажает.

// KindSection — ключ секции корня формата наполнения для класса:
// код класса с «s» (transistors, diodes, resistors, capacitors;
// docs/fill-format.md §2).
func KindSection(k domain.Kind) string { return string(k) + "s" }

// ValueKeyReserved сообщает, зарезервирован ли ключ объекта значения
// секции группы: parameter, value, min, max, text (docs/fill-format.md §4).
func ValueKeyReserved(key string) bool {
	switch key {
	case "parameter", "value", "min", "max", "text":
		return true
	}
	return false
}

// RecordKeyReserved сообщает, служебный ли ключ записи формата наполнения:
// name, system, fields, attributes, manufacturers, variants, analogs
// (docs/fill-format.md §3).
func RecordKeyReserved(key string) bool {
	switch key {
	case "name", "system", "fields", "attributes", "manufacturers",
		"variants", "analogs":
		return true
	}
	return false
}

// reservedSectionName сообщает, недопустимо ли имя для секции группы:
// совпадает со служебным ключом записи (RecordKeyReserved), ключом label
// исполнения (docs/fill-format.md §6), ключом корня catalog либо секцией
// класса корня (§2).
func reservedSectionName(kinds []KindDef, name string) bool {
	if RecordKeyReserved(name) || name == "label" || name == "catalog" {
		return true
	}
	for _, k := range kinds {
		if KindSection(k.Code) == name {
			return true
		}
	}
	return false
}
