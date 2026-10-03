package domain

// Подклассы приборов: единый словарь значений для сквозного фильтра по
// полю subclass между системами обозначений (docs/plan/03-data-model.md §2).
//
// Ключ словаря — стабильный код (латиница, D9): значение поля subclass
// разбора обозначений, device_designation_fields.text_value и фильтры
// поиска оперируют кодом; отображаемые названия — бандлы internal/i18n
// (subclass.<код>). Коды — наименьший общий знаменатель систем: более
// тонкие различия (p-n-p/n-p-n, ВЧ/НЧ, маломощный/мощный) кодируются
// атрибутами (structure) и параметрами, словарь их не дублирует.
// Отображения букв — многие-к-одному (jis C и gost Т → bjt), обратных
// расщеплений нет. Гранулярность и источники отображений — таблицы
// парсеров (parse_gost.go, parse_pro.go, parse_jis.go; выверка —
// docs/plan/07-r1-verification.md §4).
//
// Подкласс Ф ГОСТ (фотодиод/фототранзистор) неоднозначен по классу:
// код photo един для обоих — класс записи разрешается явным ключом kind,
// как и в грамматике парсера.
type Subclass struct {
	Code string // стабильный код словаря: «bjt», «zener», …

	// GostLetter — буква подкласса ГОСТ 10862-64 / ГОСТ Р 71055-2023
	// (элемент 2, поддерживаемые классами модуля однобуквенные формы;
	// 0 — буква не используется).
	GostLetter rune

	// ProLetter — вторая буква PRO ELECTRON (0 — не используется).
	ProLetter rune

	// JisLetter — буква класса JIS C 7012 (0 — не используется).
	JisLetter rune
}

// subclasses — единый словарь подклассов; порядок строк — стабильный
// порядок словаря (тесты полноты бандлов, вывод справочников).
var subclasses = []Subclass{
	{Code: "bjt", GostLetter: 'Т', ProLetter: 'C', JisLetter: 'A'},
	{Code: "fet", GostLetter: 'П', JisLetter: 'J'},
	{Code: "ujt", JisLetter: 'H'},
	{Code: "avalanche", JisLetter: 'T'},
	{Code: "thyristor", JisLetter: 'F'},
	{Code: "triac", JisLetter: 'M'},
	{Code: "rectifier", GostLetter: 'Д', ProLetter: 'Y', JisLetter: 'E'},
	{Code: "zener", GostLetter: 'С', ProLetter: 'Z', JisLetter: 'Z'},
	{Code: "varicap", GostLetter: 'В', ProLetter: 'B', JisLetter: 'V'},
	{Code: "tunnel", GostLetter: 'И', ProLetter: 'E'},
	{Code: "gunn", JisLetter: 'G'},
	{Code: "generator", GostLetter: 'Г'},
	{Code: "led", GostLetter: 'Л', ProLetter: 'Q', JisLetter: 'Q'},
	{Code: "detector", GostLetter: 'А'},
	{Code: "signal", ProLetter: 'A', JisLetter: 'S'},
	{Code: "multiplier", ProLetter: 'X'},
	{Code: "magnetic", ProLetter: 'H'},
	{Code: "photo", GostLetter: 'Ф', ProLetter: 'P'},
}

// Отображения многих-к-одному: дополнительные буквы систем, сходящиеся
// к коду основной строки словаря (полная таблица классов — в парсерах).
var (
	// gost: Ц — выпрямительные столбы и блоки.
	gostSubclassAliases = map[rune]string{'Ц': "rectifier"}
	// pro: D/F/L/S/U — мощные/ВЧ/переключательные биполярные транзисторы.
	proSubclassAliases = map[rune]string{
		'D': "bjt", 'F': "bjt", 'L': "bjt", 'S': "bjt", 'U': "bjt",
	}
	// jis: B/C/D — p-n-p НЧ, n-p-n ВЧ/НЧ; K — n-канал ПТ; R — выпрямительный.
	jisSubclassAliases = map[rune]string{
		'B': "bjt", 'C': "bjt", 'D': "bjt", 'K': "fet", 'R': "rectifier",
	}
)

// Subclasses возвращает словарь подклассов в стабильном порядке.
func Subclasses() []Subclass {
	out := make([]Subclass, len(subclasses))
	copy(out, subclasses)
	return out
}

// SubclassByCode ищет подкласс по стабильному коду словаря.
func SubclassByCode(code string) (Subclass, bool) {
	for _, s := range subclasses {
		if s.Code == code {
			return s, true
		}
	}
	return Subclass{}, false
}

// SubclassCodes перечисляет коды словаря подклассов в стабильном порядке
// (аргумент перечня допустимых в сообщениях об ошибках).
func SubclassCodes() []string {
	out := make([]string, 0, len(subclasses))
	for _, s := range subclasses {
		out = append(out, s.Code)
	}
	return out
}

// GostSubclassBySymbol ищет код подкласса по букве ГОСТ (поддерживаемые
// классами модуля однобуквенные формы: Т, П, Д, С, В, А, И, Г, Л, Ц, Ф).
func GostSubclassBySymbol(r rune) (string, bool) {
	if code, ok := gostSubclassAliases[r]; ok {
		return code, true
	}
	for _, s := range subclasses {
		if s.GostLetter == r {
			return s.Code, true
		}
	}
	return "", false
}

// ProSubclassByLetter ищет код подкласса по второй букве PRO ELECTRON.
func ProSubclassByLetter(r rune) (string, bool) {
	if code, ok := proSubclassAliases[r]; ok {
		return code, true
	}
	for _, s := range subclasses {
		if s.ProLetter == r {
			return s.Code, true
		}
	}
	return "", false
}

// JisSubclassByLetter ищет код подкласса по букве класса JIS.
func JisSubclassByLetter(r rune) (string, bool) {
	if code, ok := jisSubclassAliases[r]; ok {
		return code, true
	}
	for _, s := range subclasses {
		if s.JisLetter == r {
			return s.Code, true
		}
	}
	return "", false
}
