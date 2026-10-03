package domain

import "slices"

// System — система условных обозначений. Реестр синхронизирован с сидами
// designation_systems (docs/plan/03-data-model.md §1.2): код ⇄ сиды — пин-тест.
// Отображаемые имя и пояснение — бандлы internal/i18n (D9): в реестре
// только коды.
type System string

// Реестр систем обозначений: gost покрывает полупроводники (ГОСТ 10862-64 и
// преемники до ГОСТ Р 71055-2023), резисторы (ГОСТ 3453-68) и конденсаторы
// (единая система, действующая кодификация — ГОСТ Р 57440-2017); грамматика
// каждой предметной области — свой парсер.
const (
	SystemGost   System = "gost"
	SystemOst    System = "ost"
	SystemPro    System = "pro"
	SystemJedec  System = "jedec"
	SystemJis    System = "jis"
	SystemSeries System = "series"
	SystemOther  System = "other"
)

// systemOrder — стабильный порядок реестра (порядок сидов и вывода).
var systemOrder = []System{SystemGost, SystemOst, SystemPro, SystemJedec, SystemJis, SystemSeries, SystemOther}

// Systems возвращает системы обозначений стартового реестра в стабильном порядке.
func Systems() []System {
	return slices.Clone(systemOrder)
}

// IsValid сообщает, входит ли система в реестр.
func (s System) IsValid() bool {
	return slices.Contains(systemOrder, s)
}

// systemKinds — применимость систем к классам (docs/plan/03-data-model.md §1.1);
// пин-тест фиксирует матрицу целиком.
var systemKinds = map[System][]Kind{
	SystemGost:   {KindTransistor, KindDiode, KindResistor, KindCapacitor},
	SystemOst:    {KindResistor},
	SystemPro:    {KindTransistor, KindDiode},
	SystemJedec:  {KindTransistor, KindDiode},
	SystemJis:    {KindTransistor, KindDiode},
	SystemSeries: {KindTransistor, KindDiode, KindResistor, KindCapacitor},
	SystemOther:  {KindTransistor, KindDiode, KindResistor, KindCapacitor},
}

// SystemsForKind возвращает системы, применимые к классу, в стабильном порядке.
func SystemsForKind(k Kind) []System {
	var out []System
	for _, s := range systemOrder {
		if slices.Contains(systemKinds[s], k) {
			out = append(out, s)
		}
	}
	return out
}

// SystemAppliesToKind сообщает, применима ли система к классу
// (проверяется метасхемой импорта и автодетектом).
func SystemAppliesToKind(s System, k Kind) bool {
	return slices.Contains(systemKinds[s], k)
}
