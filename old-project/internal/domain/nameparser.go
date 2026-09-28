package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseDesignation разбирает обозначение; ошибка — UserError (аналог FormatException).
func ParseDesignation(text string) (Transistor, error) {
	t, message, ok := TryParseDesignation(text)
	if !ok {
		return Transistor{}, &UserError{Message: message}
	}
	return t, nil
}

// TryParseDesignation — (транзистор, текст ошибки, корректно ли).
func TryParseDesignation(text string) (Transistor, string, bool) {
	if strings.TrimSpace(text) == "" {
		return Transistor{}, "обозначение пустое", false
	}

	s := []rune(normalizeDesignation(text))
	i := 0

	material := s[0]
	if !IsValidMaterialSymbol(material) {
		return Transistor{}, fmt.Sprintf("позиция 1: ожидался тип материала — Г или 1 (германий), К или 2 (кремний), А или 3 (арсенид галлия), И или 4 (индий), получено «%s»", showRune(s[0])), false
	}
	i = 1

	subclass := charAt(s, i)
	if subclass != 'Т' && subclass != 'П' {
		return Transistor{}, fmt.Sprintf("позиция %d: ожидался подкласс — Т (биполярный) или П (полевой), получено «%s»", i+1, showRune(subclass)), false
	}
	i++

	isAssembly := charAt(s, i) == 'С'
	if isAssembly {
		i++
	}

	featureChar := charAt(s, i)
	if !isASCIIDigit(featureChar) || featureChar == '0' {
		return Transistor{}, fmt.Sprintf("позиция %d: ожидался характерный эксплуатационный признак — цифра от 1 до 9, получено «%s»", i+1, showRune(featureChar)), false
	}
	feature := int(featureChar - '0')
	i++

	numberStart := i
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	numberLength := i - numberStart
	if numberLength < 2 || numberLength > 3 {
		got := "цифры отсутствуют"
		if numberLength > 0 {
			got = string(s[numberStart:i])
		}
		return Transistor{}, fmt.Sprintf("позиция %d: порядковый номер разработки — от 2 до 3 цифр (01–999), %s", numberStart+1, got), false
	}
	if numberLength == 3 && s[numberStart] == '0' {
		return Transistor{}, fmt.Sprintf("позиция %d: номер разработки из трёх цифр не может начинаться с нуля", numberStart+1), false
	}
	number, err := strconv.Atoi(string(s[numberStart:i]))
	if err != nil || number == 0 {
		return Transistor{}, fmt.Sprintf("позиция %d: номер разработки не может быть нулём", numberStart+1), false
	}

	lettersStart := i
	for i < len(s) && IsUpperLetter(s[i]) && i-lettersStart < 2 {
		i++
	}
	lettersLength := i - lettersStart
	if lettersLength == 0 {
		if i < len(s) {
			return Transistor{}, fmt.Sprintf("позиция %d: ожидалась буква классификации по параметрам (заглавная русская буква), получено «%s»", i+1, showRune(s[i])), false
		}
		return Transistor{}, "отсутствует буква классификации по параметрам (одна или две заглавные русские буквы после номера разработки)", false
	}
	letters := string(s[lettersStart:i])
	if i < len(s) && IsUpperLetter(s[i]) {
		return Transistor{}, fmt.Sprintf("позиция %d: не более двух букв классификации по параметрам", i+1), false
	}

	var modification *int
	if i < len(s) && isASCIIDigit(s[i]) {
		if s[i] == '0' {
			return Transistor{}, fmt.Sprintf("позиция %d: модификация — цифра от 1 до 9, получено «0»", i+1), false
		}
		modification = Ptr(int(s[i] - '0'))
		i++
	}

	var chipVariant *int
	if i < len(s) {
		if s[i] != '-' {
			return Transistor{}, fmt.Sprintf("позиция %d: неожидаемый символ «%s» — после букв допускаются только модификация (цифра 1–9) или бескорпусное исполнение (дефис и цифра 1–6)", i+1, showRune(s[i])), false
		}
		i++
		chipChar := charAt(s, i)
		if (chipChar < '1' || chipChar > '6') || chipChar == 0 {
			return Transistor{}, fmt.Sprintf("позиция %d: бескорпусное исполнение — дефис и цифра от 1 до 6, получено «%s»", i+1, showRune(chipChar)), false
		}
		chipVariant = Ptr(int(chipChar - '0'))
		i++
		if i < len(s) {
			return Transistor{}, fmt.Sprintf("позиция %d: лишние символы после бескорпусного исполнения: «%s»", i+1, string(s[i:])), false
		}
	}

	return Transistor{
		Material:          material,
		Subclass:          subclass,
		IsAssembly:        isAssembly,
		Feature:           feature,
		DevelopmentNumber: number,
		Letters:           letters,
		Modification:      modification,
		ChipVariant:       chipVariant,
	}, "", true
}

func normalizeDesignation(text string) string {
	replacer := strings.NewReplacer("−", "-", "–", "-")
	return replacer.Replace(strings.ToUpper(strings.TrimSpace(text)))
}

func charAt(s []rune, index int) rune {
	if index < len(s) {
		return s[index]
	}
	return 0
}

func isASCIIDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

func showRune(c rune) string {
	if c == 0 {
		return "конец обозначения"
	}
	return string(c)
}
