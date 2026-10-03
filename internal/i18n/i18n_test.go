package i18n

import "testing"

func TestParseLanguage(t *testing.T) {
	cases := []struct {
		tag  string
		want Language
		ok   bool
	}{
		{"en", En, true},
		{"EN", En, true},
		{"ru", Ru, true},
		{"ru-RU", Ru, true},
		{" en-US ", En, true},
		{"fr", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := ParseLanguage(c.tag)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseLanguage(%q) = %q,%v; want %q,%v", c.tag, got, ok, c.want, c.ok)
		}
	}
}

func TestNegotiate(t *testing.T) {
	cases := []struct {
		header string
		want   Language
	}{
		{"", En},
		{"ru", Ru},
		{"ru-RU,ru;q=0.9,en;q=0.8", Ru},
		{"fr-CA,fr;q=0.9,en;q=0.8", En},
		{"fr;q=1.0, ru;q=0.5", Ru},
		{"en-GB,en;q=0.9,ru;q=0.8", En},
		{"*", En},
		{"garbage", En},
	}
	for _, c := range cases {
		if got := Negotiate(c.header); got != c.want {
			t.Errorf("Negotiate(%q) = %q; want %q", c.header, got, c.want)
		}
	}
}

func TestResolveFallbacks(t *testing.T) {
	// en — канонический: все ключи сидов присутствуют.
	if got := UnitSymbol(En, "ohm"); got != "Ω" {
		t.Errorf("UnitSymbol(en, ohm) = %q", got)
	}
	// ru-строка локализована.
	if got := UnitSymbol(Ru, "ohm"); got != "Ом" {
		t.Errorf("UnitSymbol(ru, ohm) = %q", got)
	}
	// Ключа нет ни в ru, ни в en — отображается сам код сущности (D9:
	// расширения каталога данными).
	if got := ParameterName(En, "NewParam"); got != "NewParam" {
		t.Errorf("ParameterName(en, NewParam) = %q", got)
	}
	if got := FamilyName(Ru, "НОВОЕ"); got != "НОВОЕ" {
		t.Errorf("FamilyName(ru, НОВОЕ) = %q", got)
	}
	// Отсутствие строки в ru с наличием в en — fallback на en.
	if got := resolve(Ru, "field.material", "x"); got != "материал" {
		t.Errorf("resolve(ru, field.material) = %q", got)
	}
	if got := resolve(Ru, "system.gost.description", ""); got == "" {
		t.Error("resolve(ru, system.gost.description) пуст")
	}
}

func TestKeysMirror(t *testing.T) {
	// ru-бандл и en-бандл покрывают одинаковые ключи (ru — полная локаль).
	en, ru := Keys(En), Keys(Ru)
	if len(en) == 0 {
		t.Fatal("en-бандл пуст")
	}
	if len(en) != len(ru) {
		t.Fatalf("число ключей en (%d) ≠ ru (%d)", len(en), len(ru))
	}
	enSet := make(map[string]bool, len(en))
	for _, k := range en {
		enSet[k] = true
	}
	for _, k := range ru {
		if !enSet[k] {
			t.Errorf("ключ %q есть в ru, отсутствует в en (en — канонический)", k)
		}
		if !HasString(Ru, k) {
			t.Errorf("ключ %q ru-бандла с пустым значением", k)
		}
	}
}
