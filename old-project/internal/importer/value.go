// Package importer читает справочник из файлов нескольких форматов (jsonc,
// yaml). Каждый формат разбирается в общее упорядоченное дерево значений —
// порядок ключей нужен для дословных сообщений об ошибках; семантика секций
// и валидация записей не зависят от формата.
package importer

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

// value — значение файла импорта: объект хранит элементы в порядке появления
// в файле, число — исходным текстом (в int/float превращается при валидации
// полей, чтобы сообщения об ошибках были едиными для всех форматов). Имена
// ключей объекта уникальны — повторяющиеся ключи оба фронтенда отвергают
// при разборе (jsonc — decodeValue, yaml — goccy/go-yaml).
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
