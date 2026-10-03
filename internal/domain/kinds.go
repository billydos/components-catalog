package domain

import "slices"

// Kind — класс приборов. Реестр синхронизирован с сидами kinds
// (docs/plan/03-data-model.md §1.1): код ⇄ сиды проверяется пин-тестом.
// Отображаемое название класса — бандлы internal/i18n (D9): в реестре
// только коды.
type Kind string

// Реестр классов: расширение (тиристоры, оптопары) — новыми кодами
// здесь и строками сидов; код стабилен (ключ фильтров и REST).
const (
	KindTransistor Kind = "transistor"
	KindDiode      Kind = "diode"
	KindResistor   Kind = "resistor"
	KindCapacitor  Kind = "capacitor"
)

// kindOrder — стабильный порядок реестра (порядок сидов и вывода).
var kindOrder = []Kind{KindTransistor, KindDiode, KindResistor, KindCapacitor}

// Kinds возвращает классы стартового реестра в стабильном порядке.
func Kinds() []Kind {
	return slices.Clone(kindOrder)
}

// IsValid сообщает, входит ли класс в реестр.
func (k Kind) IsValid() bool {
	return slices.Contains(kindOrder, k)
}
