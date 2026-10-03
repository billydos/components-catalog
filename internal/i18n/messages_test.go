package i18n_test

import (
	"slices"
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
