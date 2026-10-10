package importer

import "github.com/billydos/components-catalog/internal/domain"

// Упорядоченное дерево значений — общее промежуточное представление всех
// форматов реестра (jsonc/json, yaml/yml, ndjson): объект хранит элементы
// в порядке появления в файле (порядок ключей значим для семантики секций
// и сообщений об ошибках), число — исходным текстом (в float64 превращается
// при чтении полей, чтобы сообщения об ошибках были едиными для всех
// форматов). Имена ключей объекта уникальны: повторяющиеся ключи все
// фронтенды отвергают при разборе (тихая потеря данных недопустима —
// docs/plan/04-module-functionality.md §4).

type valueKind int

const (
	kindNull valueKind = iota
	kindBool
	kindNumber
	kindString
	kindArray
	kindObject
)

type member struct {
	name  string
	value value
}

// value — значение файла наполнения.
type value struct {
	kind    valueKind
	str     string
	num     string
	boolean bool
	members []member
	items   []value
}

// has возвращает элемент объекта по имени (второе значение — наличие ключа).
func (v value) has(name string) (value, bool) {
	for _, m := range v.members {
		if m.name == name {
			return m.value, true
		}
	}
	return value{}, false
}

// Конструкторы дерева для экспорта (writer): значения собираются в том же
// представлении, из которого читает reader, — round-trip один код пути.

func str(s string) value { return value{kind: kindString, str: s} }

func boolean(b bool) value { return value{kind: kindBool, boolean: b} }

// num — число с каноническим текстом (без экспоненты, без потери точности;
// парсер форматов читает такой текст без round-trip-искажений).
func num(f float64) value {
	return value{kind: kindNumber, num: formatExportNum(f)}
}

func object(pairs ...member) value {
	return value{kind: kindObject, members: pairs}
}

func array(items ...value) value {
	return value{kind: kindArray, items: items}
}

func nullValue() value { return value{kind: kindNull} }

// pair — сокращение для элемента объекта.
func pair(name string, v value) member { return member{name: name, value: v} }

// describeKindArg — аргумент-сообщение с человекочитаемым именем типа
// (локализуется каталогом сообщений).
func describeKindArg(v value) any {
	switch v.kind {
	case kindNull:
		return "null"
	case kindBool:
		return domain.MsgArg(domain.MsgImportValueKindBool)
	case kindNumber:
		return domain.MsgArg(domain.MsgImportValueKindNumber)
	case kindString:
		return domain.MsgArg(domain.MsgImportValueKindString)
	case kindArray:
		return domain.MsgArg(domain.MsgImportValueKindArray)
	case kindObject:
		return domain.MsgArg(domain.MsgImportValueKindObject)
	}
	return domain.MsgArg(domain.MsgImportValueKindOther)
}
