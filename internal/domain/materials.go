package domain

// Материалы полупроводников: единый словарь значений для сквозного фильтра
// по полю material между системами gost и pro (docs/plan/03-data-model.md §2.1,
// §2.4; выверка — docs/plan/07-r1-verification.md §1–2).
//
// Ключ словаря — стабильный код (латиница, D9): значение поля material
// разбора обозначений, device_designation_fields.text_value и фильтры
// поиска оперируют кодом; отображаемые названия — бандлы internal/i18n
// (material.<код>). Равнозначные символы ГОСТ (буква — общегражданское
// применение, цифра — категории качества ВП/ОС/ОСМ) физически равнозначны:
// КТ312 и 2Т312 — раздельные записи, подсказка find использует пару
// символов материала.
type Material struct {
	Code string // стабильный код словаря: «ge», «si», …

	// GostLetter и GostDigit — пара равнозначных символов ГОСТ
	// (0 — символ не используется, например у арсенида галлия).
	GostLetter rune
	GostDigit  rune

	// ProLetter — буква материала PRO ELECTRON (0 — нет).
	ProLetter rune
}

// materials — единый словарь: gost и pro отображаются в одни коды,
// где символы совпадают физически (германий, кремний).
var materials = []Material{
	{Code: "ge", GostLetter: 'Г', GostDigit: '1', ProLetter: 'A'},
	{Code: "si", GostLetter: 'К', GostDigit: '2', ProLetter: 'B'},
	{Code: "ga", GostLetter: 'А', GostDigit: '3'},
	{Code: "in", GostLetter: 'И', GostDigit: '4'},
	{Code: "sic", GostLetter: 'Д', GostDigit: '5'},
	{Code: "other", GostLetter: 'П', GostDigit: '6'},
	{Code: "gaas", ProLetter: 'C'},
}

// Materials возвращает словарь материалов в стабильном порядке.
func Materials() []Material {
	out := make([]Material, len(materials))
	copy(out, materials)
	return out
}

// MaterialByCode ищет материал по стабильному коду словаря.
func MaterialByCode(code string) (Material, bool) {
	for _, m := range materials {
		if m.Code == code {
			return m, true
		}
	}
	return Material{}, false
}

// GostMaterialBySymbol ищет материал по символу ГОСТ — букве (Г, К, А, И, Д, П)
// или цифре (1–6); используется парсером gost и подсказкой find.
func GostMaterialBySymbol(r rune) (Material, bool) {
	for _, m := range materials {
		if m.GostLetter == r || m.GostDigit == r {
			return m, true
		}
	}
	return Material{}, false
}

// GostEquivalentSymbol возвращает парный равнозначный символ материала ГОСТ
// (Г ↔ 1, К ↔ 2, А ↔ 3, И ↔ 4, Д ↔ 5, П ↔ 6) — только для подсказки find
// у полупроводников gost; записи с обоими символами раздельны (03 §2.1).
func GostEquivalentSymbol(r rune) (rune, bool) {
	for _, m := range materials {
		switch r {
		case m.GostLetter:
			return m.GostDigit, true
		case m.GostDigit:
			return m.GostLetter, true
		}
	}
	return 0, false
}

// ProMaterialByLetter ищет материал по первой букве PRO ELECTRON (A, B, C).
func ProMaterialByLetter(r rune) (Material, bool) {
	for _, m := range materials {
		if m.ProLetter == r {
			return m, true
		}
	}
	return Material{}, false
}
