package domain

// Способ подстройки номинала: единый словарь значений для сквозного
// фильтра по полю adjustment между системами обозначений резисторов и
// конденсаторов (docs/plan/03-data-model.md §2.2–§2.3).
//
// Ключ словаря — стабильный код (латиница, D9): значение поля adjustment
// разбора обозначений, device_designation_fields.text_value и фильтры
// поиска оперируют кодом; отображаемые названия — бандлы internal/i18n
// (adjustment.<код>). Отображения задают парсеры: семейства резисторов
// ГОСТ 3453-68 (С постоянные, СП переменные) и ОСТ 11.074.009-78
// (Р постоянные, РП переменные; НР — наборы постоянных), подклассы
// конденсаторов (К постоянной ёмкости, КТ подстроечные, КП переменной
// ёмкости; КН нелинейные и КС сборки — не перестраиваемые вручную).
// Записи other задают поле явно секцией fields (нелинейные различия
// подстроечный/переменный у конкретных семейств — наполнением).
type Adjustment struct {
	Code string // стабильный код словаря: «fixed», «variable», «preset»
}

// adjustments — единый словарь способов подстройки.
var adjustments = []Adjustment{
	{Code: "fixed"},    // постоянный
	{Code: "variable"}, // переменный
	{Code: "preset"},   // подстроечный
}

// Adjustments возвращает словарь способов подстройки в стабильном порядке.
func Adjustments() []Adjustment {
	out := make([]Adjustment, len(adjustments))
	copy(out, adjustments)
	return out
}

// AdjustmentByCode ищет способ подстройки по стабильному коду словаря.
func AdjustmentByCode(code string) (Adjustment, bool) {
	for _, a := range adjustments {
		if a.Code == code {
			return a, true
		}
	}
	return Adjustment{}, false
}

// AdjustmentCodes перечисляет коды словаря в стабильном порядке
// (аргумент перечня допустимых в сообщениях об ошибках).
func AdjustmentCodes() []string {
	out := make([]string, 0, len(adjustments))
	for _, a := range adjustments {
		out = append(out, a.Code)
	}
	return out
}

// ResistorFamilyAdjustment отображает семейство резисторных грамматик
// (gost: С/СП; ost: Р/РП/НР) в код способа подстройки.
func ResistorFamilyAdjustment(family string) string {
	switch family {
	case "СП", "РП":
		return "variable"
	default:
		return "fixed"
	}
}

// CapacitorPrefixAdjustment отображает подкласс конденсаторной грамматики
// (К, КТ, КП, КН, КС) в код способа подстройки.
func CapacitorPrefixAdjustment(prefix string) string {
	switch prefix {
	case "КТ":
		return "preset"
	case "КП":
		return "variable"
	default:
		return "fixed"
	}
}
