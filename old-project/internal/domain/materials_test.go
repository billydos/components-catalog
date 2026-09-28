package domain

import (
	"testing"
)

func TestEquivalentSymbols_MapToSameKind(t *testing.T) {
	cases := []struct {
		symbol   rune
		expected SemiconductorMaterial
	}{
		{'Г', Germanium},
		{'1', Germanium},
		{'К', Silicon},
		{'2', Silicon},
		{'А', GalliumArsenide},
		{'3', GalliumArsenide},
		{'И', Indium},
		{'4', Indium},
	}
	for _, tc := range cases {
		t.Run(string(tc.symbol), func(t *testing.T) {
			if !IsValidMaterialSymbol(tc.symbol) {
				t.Error("IsValidMaterialSymbol = false")
			}
			kind, ok := TryMaterialKind(tc.symbol)
			if !ok || kind != tc.expected {
				t.Errorf("TryMaterialKind = (%v, %v), ожидалось (%v, true)", kind, ok, tc.expected)
			}
			resolved, err := MaterialKindOf(tc.symbol)
			if err != nil || resolved != tc.expected {
				t.Errorf("MaterialKindOf = (%v, %v), ожидалось (%v, nil)", resolved, err, tc.expected)
			}
		})
	}
}

func TestMaterialSymbolsOf_LetterAndDigitMapBackToSameKind(t *testing.T) {
	for _, kind := range []SemiconductorMaterial{Germanium, Silicon, GalliumArsenide, Indium} {
		letter, digit := MaterialSymbolsOf(kind)
		if got, err := MaterialKindOf(letter); err != nil || got != kind {
			t.Errorf("буква %c: (%v, %v), ожидалось (%v, nil)", letter, got, err, kind)
		}
		if got, err := MaterialKindOf(digit); err != nil || got != kind {
			t.Errorf("цифра %c: (%v, %v), ожидалось (%v, nil)", digit, got, err, kind)
		}
	}
}

func TestInvalidMaterialSymbols_AreRejected(t *testing.T) {
	for _, symbol := range []rune{'5', '0', 'В', 'K'} {
		if IsValidMaterialSymbol(symbol) {
			t.Errorf("%c: IsValidMaterialSymbol = true", symbol)
		}
		if _, ok := TryMaterialKind(symbol); ok {
			t.Errorf("%c: TryMaterialKind = true", symbol)
		}
		if _, err := MaterialKindOf(symbol); err == nil {
			t.Errorf("%c: MaterialKindOf не вернула ошибку", symbol)
		}
	}
}

func TestMaterialNames(t *testing.T) {
	if got := MaterialShortName(Germanium); got != "германий" {
		t.Errorf("ShortName = %q, ожидалось %q", got, "германий")
	}
	if got := MaterialDisplayName(Silicon); got != "кремний (К/2)" {
		t.Errorf("DisplayName = %q, ожидалось %q", got, "кремний (К/2)")
	}
}

func TestIsUpperLetter_AcceptsUppercaseRussian(t *testing.T) {
	for _, c := range []rune{'А', 'Я', 'Э', 'Ё'} {
		if !IsUpperLetter(c) {
			t.Errorf("%c: IsUpperLetter = false", c)
		}
	}
}

func TestIsUpperLetter_RejectsEverythingElse(t *testing.T) {
	for _, c := range []rune{'а', 'ё', 'я', 'A', '0', '-'} {
		if IsUpperLetter(c) {
			t.Errorf("%c: IsUpperLetter = true", c)
		}
	}
}

func TestIsUpperLetters_AcceptsOneOrTwo(t *testing.T) {
	for _, s := range []string{"Б", "БВ", "Ё"} {
		if !IsUpperLetters(s) {
			t.Errorf("%q: IsUpperLetters = false", s)
		}
	}
}

func TestIsUpperLetters_RejectsOtherLengthsAndAlphabets(t *testing.T) {
	for _, s := range []string{"", "БВГ", "б", "A"} {
		if IsUpperLetters(s) {
			t.Errorf("%q: IsUpperLetters = true", s)
		}
	}
}

func TestFmt_UsesInvariantFormatting(t *testing.T) {
	cases := []struct {
		value    float64
		expected string
	}{
		{5, "5"},
		{1.5, "1.5"},
		{0.001, "0.001"},
		{-60, "-60"},
	}
	for _, tc := range cases {
		if got := Fmt(tc.value); got != tc.expected {
			t.Errorf("Fmt(%v) = %q, ожидалось %q", tc.value, got, tc.expected)
		}
	}
}
