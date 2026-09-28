package domain

// SemiconductorMaterial — материал полупроводника. Буква и цифра равнозначны
// как физический материал, но не как ключ записи: КТ312 и 2Т312 — разные записи
// с собственными данными.
type SemiconductorMaterial int

const (
	Germanium SemiconductorMaterial = iota
	Silicon
	GalliumArsenide
	Indium
)

var materialsBySymbol = map[rune]SemiconductorMaterial{
	'Г': Germanium, '1': Germanium,
	'К': Silicon, '2': Silicon,
	'А': GalliumArsenide, '3': GalliumArsenide,
	'И': Indium, '4': Indium,
}

var materialSymbolPairs = map[SemiconductorMaterial]struct{ Letter, Digit rune }{
	Germanium:       {'Г', '1'},
	Silicon:         {'К', '2'},
	GalliumArsenide: {'А', '3'},
	Indium:          {'И', '4'},
}

func IsValidMaterialSymbol(symbol rune) bool {
	_, ok := materialsBySymbol[symbol]
	return ok
}

func TryMaterialKind(symbol rune) (SemiconductorMaterial, bool) {
	kind, ok := materialsBySymbol[symbol]
	return kind, ok
}

func MaterialKindOf(symbol rune) (SemiconductorMaterial, error) {
	if kind, ok := materialsBySymbol[symbol]; ok {
		return kind, nil
	}
	return 0, NewUserError("«%c» не является обозначением материала (Г/1, К/2, А/3, И/4)", symbol)
}

// MaterialSymbolsOf — оба равнозначных символа материала, например Г и 1.
// Нужна фильтрам и отображению, не тождеству записи.
func MaterialSymbolsOf(kind SemiconductorMaterial) (letter, digit rune) {
	pair := materialSymbolPairs[kind]
	return pair.Letter, pair.Digit
}

func MaterialShortName(kind SemiconductorMaterial) string {
	switch kind {
	case Germanium:
		return "германий"
	case Silicon:
		return "кремний"
	case GalliumArsenide:
		return "арсенид галлия"
	case Indium:
		return "индий"
	}
	return "неизвестный материал"
}

func MaterialDisplayName(kind SemiconductorMaterial) string {
	switch kind {
	case Germanium:
		return "германий (Г/1)"
	case Silicon:
		return "кремний (К/2)"
	case GalliumArsenide:
		return "арсенид галлия (А/3)"
	case Indium:
		return "индий (И/4)"
	}
	return "неизвестный материал"
}
