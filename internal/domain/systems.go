package domain

import "slices"

// System — система условных обозначений. Реестр синхронизирован с сидами
// designation_systems (docs/plan/03-data-model.md §1.2): код ⇄ сиды — пин-тест.
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

type systemInfo struct {
	name        string
	description string
}

var systems = map[System]systemInfo{
	SystemGost: {
		name:        "ГОСТ",
		description: "единые системы обозначений: полупроводники (ГОСТ 10862-64 и преемники до ГОСТ Р 71055-2023), резисторы (ГОСТ 3453-68), конденсаторы (ГОСТ Р 57440-2017)",
	},
	SystemOst: {
		name:        "ОСТ 11.074.009-78",
		description: "действующая система обозначений резисторов",
	},
	SystemPro: {
		name:        "PRO ELECTRON",
		description: "европейская система регистрации полупроводниковых приборов",
	},
	SystemJedec: {
		name:        "JEDEC",
		description: "система EIA-370 (с 1982 — JESD370B), США",
	},
	SystemJis: {
		name:        "JIS C 7012",
		description: "система EIAJ (JEITA-EDEREC), Япония",
	},
	SystemSeries: {
		name:        "семейства",
		description: "слабые системы: советские досистемные семейства и мировые дом-номера производителей (реестр series_families)",
	},
	SystemOther: {
		name:        "вне строгых систем",
		description: "бессистемные имена: фирменные варианты, военные префиксы, полные заказные коды; разбора нет, только канонизация",
	},
}

// systemOrder — стабильный порядок реестра (порядок сидов и вывода).
var systemOrder = []System{SystemGost, SystemOst, SystemPro, SystemJedec, SystemJis, SystemSeries, SystemOther}

// Systems возвращает системы обозначений стартового реестра в стабильном порядке.
func Systems() []System {
	return slices.Clone(systemOrder)
}

// IsValid сообщает, входит ли система в реестр.
func (s System) IsValid() bool {
	_, ok := systems[s]
	return ok
}

// Name возвращает короткое имя системы; неизвестная система — пустая строка.
func (s System) Name() string {
	return systems[s].name
}

// Description возвращает пояснение системы; неизвестная система — пустая строка.
func (s System) Description() string {
	return systems[s].description
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
