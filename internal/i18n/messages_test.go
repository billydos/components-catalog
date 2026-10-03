package i18n_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Полнота каталога сообщений (этап 8.3, D9): каждый MsgID реестра domain
// имеет непустой формат в en (канонический) и ru; ru-каталог не содержит
// ключей вне en. Расширение реестра — форматами обоих языков тем же
// изменением.
func TestMessageCatalogCompleteness(t *testing.T) {
	ids := domain.MsgIDs()
	if len(ids) < 200 {
		t.Fatalf("реестр сообщений подозрительно мал: %d", len(ids))
	}
	for _, id := range ids {
		if !i18n.HasMessage(i18n.En, string(id)) {
			t.Errorf("en-каталог: нет формата для %q", string(id))
		}
		if !i18n.HasMessage(i18n.Ru, string(id)) {
			t.Errorf("ru-каталог: нет формата для %q", string(id))
		}
	}
	enIDs := i18n.MessageIDs()
	for _, id := range enIDs {
		if !slices.ContainsFunc(ids, func(x domain.MsgID) bool { return string(x) == id }) {
			t.Errorf("en-каталог содержит ключ %q вне реестра domain", id)
		}
		if !i18n.HasMessage(i18n.Ru, id) {
			t.Errorf("ru-каталог: нет формата для %q", id)
		}
	}
}

// formatVerbs разбирает последовательность fmt-глаголов формата: флаги,
// ширина, точность и позиционные индексы %[1]s пропускаются, %% не
// считается глаголом.
func formatVerbs(format string) []string {
	var out []string
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(format) && strings.IndexByte("+-# []0123456789.", format[j]) >= 0 {
			j++
		}
		if j < len(format) {
			out = append(out, string(format[j]))
			i = j
		}
	}
	return out
}

// Паритет форматов en/ru: последовательность fmt-глаголов совпадает —
// рассинхрон не падает, а рендерится артефактами %!s(MISSING)/
// %!d(EXTRA …) при локализованном выводе.
func TestMessageVerbParity(t *testing.T) {
	for _, id := range domain.MsgIDs() {
		en := i18n.Message(i18n.En, string(id))
		ru := i18n.Message(i18n.Ru, string(id))
		ve, vr := formatVerbs(en), formatVerbs(ru)
		if !slices.Equal(ve, vr) {
			t.Errorf("MsgID %s: глаголы en %v ≠ ru %v\n  en: %s\n  ru: %s", string(id), ve, vr, en, ru)
		}
	}
}

// Канонический рендер и локализация: подстановка аргументов, вложенные
// аргументы-сообщения, fallbackи.
func TestMessageRender(t *testing.T) {
	got := i18n.Message(i18n.En, "scanner_expected", "КТ315Б", 3, i18n.Arg("expect_hyphen"), "-")
	want := "designation «КТ315Б»: position 3: expected: hyphen, got «-»"
	if got != want {
		t.Errorf("en: %q", got)
	}
	got = i18n.Message(i18n.Ru, "scanner_expected", "КТ315Б", 3, i18n.Arg("expect_hyphen"), "-")
	want = "обозначение «КТ315Б»: позиция 3: ожидалось: дефис, получено «-»"
	if got != want {
		t.Errorf("ru: %q", got)
	}
	// Неизвестный MsgID — защитный fallback на идентификатор.
	if got := i18n.Canonical("no_such_msg", "x"); got != "no_such_msg" {
		t.Errorf("fallback: %q", got)
	}
	// Порядок слов локали: value_key_required.
	if got := i18n.Message(i18n.En, "value_key_required", "Cnom", "range", "min"); got == "" {
		t.Error("пустой рендер")
	}
}

// Словарь материалов: полнота бандлов для всех кодов domain.Materials и
// канонизация входа (код либо название локали → код, D9).
func TestMaterialDictionary(t *testing.T) {
	for _, m := range domain.Materials() {
		for _, l := range i18n.Languages() {
			if !i18n.HasString(l, "material."+m.Code) {
				t.Errorf("бандл %s: нет материала %q", string(l), m.Code)
			}
		}
	}
	cases := map[string]string{
		"si":        "si",
		"кремний":   "si",
		"silicon":   "si",
		"германий":  "ge",
		"germanium": "ge",
		"gaas":      "gaas",
	}
	for in, want := range cases {
		got, ok := i18n.MaterialCode(in)
		if !ok || got != want {
			t.Errorf("MaterialCode(%q) = %q,%v; want %q", in, got, ok, want)
		}
	}
	if _, ok := i18n.MaterialCode("медь"); ok {
		t.Error("медь ошибочно канонизируется")
	}
	if _, ok := i18n.MaterialCode(""); ok {
		t.Error("пустая строка ошибочно канонизируется")
	}
}

// Словари подклассов и способов подстройки: полнота бандлов и канонизация
// входа фильтров (код либо название локали → код, D9).
func TestSubclassDictionary(t *testing.T) {
	for _, s := range domain.Subclasses() {
		for _, l := range i18n.Languages() {
			if !i18n.HasString(l, "subclass."+s.Code) {
				t.Errorf("бандл %s: нет подкласса %q", string(l), s.Code)
			}
		}
	}
	cases := map[string]string{
		"bjt": "bjt",
		"биполярный транзистор":           "bjt",
		"bipolar transistor":              "bjt",
		"стабилитрон":                     "zener",
		"zener (voltage-regulator) diode": "zener",
	}
	for in, want := range cases {
		got, ok := i18n.SubclassCode(in)
		if !ok || got != want {
			t.Errorf("SubclassCode(%q) = %q,%v; want %q", in, got, ok, want)
		}
	}
	if _, ok := i18n.SubclassCode("самодельный"); ok {
		t.Error("самодельный подкласс ошибочно канонизируется")
	}
}

func TestAdjustmentDictionary(t *testing.T) {
	for _, a := range domain.Adjustments() {
		for _, l := range i18n.Languages() {
			if !i18n.HasString(l, "adjustment."+a.Code) {
				t.Errorf("бандл %s: нет способа подстройки %q", string(l), a.Code)
			}
		}
	}
	cases := map[string]string{
		"fixed":        "fixed",
		"постоянный":   "fixed",
		"переменный":   "variable",
		"variable":     "variable",
		"подстроечный": "preset",
	}
	for in, want := range cases {
		got, ok := i18n.AdjustmentCode(in)
		if !ok || got != want {
			t.Errorf("AdjustmentCode(%q) = %q,%v; want %q", in, got, ok, want)
		}
	}
}

// Категории — каталожный словарь: канонизация по кодам каталога и
// названиям бандлов; расширения без бандла — только точным кодом.
func TestCategoryCode(t *testing.T) {
	codes := []string{"general_purpose", "power", "custom_ext"}
	cases := map[string]string{
		"general_purpose": "general_purpose",
		"универсальный":   "general_purpose",
		"general purpose": "general_purpose",
		"custom_ext":      "custom_ext",
	}
	for in, want := range cases {
		got, ok := i18n.CategoryCode(in, codes)
		if !ok || got != want {
			t.Errorf("CategoryCode(%q) = %q,%v; want %q", in, got, ok, want)
		}
	}
	// Название локали без кода в каталоге — не канонизируется.
	if _, ok := i18n.CategoryCode("precision", codes); ok {
		t.Error("код вне каталога ошибочно канонизируется названием")
	}
}

// Инженерное форматирование значений (этап 8.4): производные единицы
// отображения и разделитель локали.
func TestFormatValue(t *testing.T) {
	cases := []struct {
		lang i18n.Language
		unit string
		v    float64
		want string
	}{
		{i18n.En, "pF", 2200000, "2.2 µF"},
		{i18n.Ru, "pF", 2200000, "2,2 мкФ"},
		{i18n.En, "ohm", 4700, "4.7 kΩ"},
		{i18n.Ru, "ohm", 4700, "4,7 кОм"},
		{i18n.En, "ohm", 100, "100 Ω"},
		{i18n.Ru, "ohm", 100, "100 Ом"},
		{i18n.En, "MHz", 0.001, "1 kHz"},
		{i18n.Ru, "MHz", 0.001, "1 кГц"},
		{i18n.En, "mA", 0.5, "500 µA"},
		{i18n.Ru, "mA", 0.5, "500 мкА"},
		{i18n.En, "degC", 125, "125 °C"},
		{i18n.Ru, "degC", 125, "125 °C"},
		{i18n.En, "W", 0.125, "125 mW"},
		{i18n.Ru, "W", 0.125, "125 мВт"},
		{i18n.En, "", 42, "42"},
		{i18n.En, "pF", 0, "0 pF"},
	}
	for _, c := range cases {
		if got := i18n.FormatValue(c.lang, c.unit, c.v); got != c.want {
			t.Errorf("FormatValue(%s, %s, %v) = %q; want %q", c.lang, c.unit, c.v, got, c.want)
		}
	}
}
