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
		// Пробел перед q и регистр префикса (RFC 9110 допускает «en; q=0.3»).
		{"en; q=0.3, ru", Ru},
		{"en;Q=0.3, ru", Ru},
		// q-параметр среди нескольких параметров кандидата.
		{"en; foo=bar; q=0.3, ru", Ru},
		{"en; q=0.3; foo=bar, ru", Ru},
		// q=0 — кандидат неприемлем и исключается из выбора.
		{"ru;q=0", En},
		{"ru, en; q=0", Ru},
		// Битое (не число/не конечное) и вне [0,1] q — кандидат неприемлем.
		{"en; q=nan, ru", Ru},
		{"en; q=inf, ru", Ru},
		{"en; q=2, ru", Ru},
		{"en; q=abc, ru", Ru},
		// Пробелы вокруг значения q и заголовка целиком.
		{"en; q = 0.3 , ru", Ru},
		{"  ru  ", Ru},
	}
	for _, c := range cases {
		if got := Negotiate(c.header); got != c.want {
			t.Errorf("Negotiate(%q) = %q; want %q", c.header, got, c.want)
		}
	}
}

// Невалидная локаль не паникует: все публичные функции отображения
// возвращают en-строку (fallback resolve к каноническому языку).
func TestInvalidLanguageFallback(t *testing.T) {
	bad := Language("fr")
	calls := []struct {
		name string
		call func(l Language) string
	}{
		{"KindName", func(l Language) string { return KindName(l, "transistor") }},
		{"SystemName", func(l Language) string { return SystemName(l, "gost") }},
		{"SystemDescription", func(l Language) string { return SystemDescription(l, "gost") }},
		{"FamilyName", func(l Language) string { return FamilyName(l, "МЛТ") }},
		{"UnitName", func(l Language) string { return UnitName(l, "ohm") }},
		{"UnitSymbol", func(l Language) string { return UnitSymbol(l, "ohm") }},
		{"ConditionName", func(l Language) string { return ConditionName(l, "temp") }},
		{"GroupName", func(l Language) string { return GroupName(l, "limiting") }},
		{"ParameterName", func(l Language) string { return ParameterName(l, "h21e") }},
		{"AttributeName", func(l Language) string { return AttributeName(l, "description") }},
		{"RuleDescription", func(l Language) string { return RuleDescription(l, "temp_pair") }},
		{"DesignationField", func(l Language) string { return DesignationField(l, "material") }},
		{"MaterialName", func(l Language) string { return MaterialName(l, "si") }},
		{"SubclassName", func(l Language) string { return SubclassName(l, "bjt") }},
		{"AdjustmentName", func(l Language) string { return AdjustmentName(l, "fixed") }},
		{"CategoryName", func(l Language) string { return CategoryName(l, "power") }},
		{"FormatValue", func(l Language) string { return FormatValue(l, "ohm", 4700) }},
	}
	for _, c := range calls {
		want := c.call(En)
		if want == "" {
			t.Fatalf("%s: пустой en-рендер — некорректный ключ теста", c.name)
		}
		if got := c.call(bad); got != want {
			t.Errorf("%s(невалидная локаль) = %q; want en %q", c.name, got, want)
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
