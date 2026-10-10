package domain

import "testing"

// Нулевая и отрицательная руна не «находит» материал: нулевые GostLetter/
// GostDigit (арсенид галлия без символов ГОСТ) не должны выпадать из
// поиска по невалидной руне.
func TestGostMaterialBySymbolZeroRune(t *testing.T) {
	for _, r := range []rune{0, -1} {
		if m, ok := GostMaterialBySymbol(r); ok {
			t.Errorf("GostMaterialBySymbol(%d) ошибочно находит материал %q", r, m.Code)
		}
	}
	letters := map[rune]string{
		'Г': "ge", 'К': "si", 'А': "ga", 'И': "in", 'Д': "sic", 'П': "other",
	}
	for letter, code := range letters {
		if m, ok := GostMaterialBySymbol(letter); !ok || m.Code != code {
			t.Errorf("GostMaterialBySymbol(%q) = %q,%v; want %q", letter, m.Code, ok, code)
		}
	}
	for digit := rune('1'); digit <= '6'; digit++ {
		if _, ok := GostMaterialBySymbol(digit); !ok {
			t.Errorf("GostMaterialBySymbol(%q) не находит материал", digit)
		}
	}
}

// Пустой сканер гост-грамматики не «спарсил» бы материал: конец
// обозначения даёт ту же ошибку ожидания символа материала.
func TestParseGostSemiconductorEmptyMaterial(t *testing.T) {
	want := "designation «»: position 1: expected: material symbol (Г, К, А, И, Д, П, or digit 1–6), got end of designation"
	if _, err := parseGostSemiconductor(newScanner(""), ""); err == nil {
		t.Fatal("пустая строка: ожидалась ошибка")
	} else if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}
}
