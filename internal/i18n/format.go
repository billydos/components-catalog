package i18n

// Форматирование значений с инженерными приставками (этап 8.4, D9):
// хранилище держит канонические единицы (латинские коды), слой вывода
// выбирает удобную производную единицу отображения (мкФ, МОм, кГц…)
// и записывает число с разделителем локали. Производные единицы в
// справочнике units не хранятся.

import "math"

// unitBase — базовая величина канонической единицы: экспонента кода
// (pF = 1e-12 F) и символы базы по локалям (F/Ф, Ω/Ом, Гц/Hz …).
type unitBase struct {
	exp    int
	enBase string
	ruBase string
}

// unitBases — только единицы с осмысленными приставками; прочие (dB, pct,
// degC и производные температур, mcd, g) выводятся как есть символом
// локали.
var unitBases = map[string]unitBase{
	"V":   {0, "V", "В"},
	"mV":  {-3, "V", "В"},
	"mA":  {-3, "A", "А"},
	"uA":  {-6, "A", "А"},
	"A":   {0, "A", "А"},
	"MHz": {6, "Hz", "Гц"},
	"W":   {0, "W", "Вт"},
	"mW":  {-3, "W", "Вт"},
	"ohm": {0, "Ω", "Ом"},
	"pF":  {-12, "F", "Ф"},
	"ps":  {-12, "s", "с"},
	"ns":  {-9, "s", "с"},
	"us":  {-6, "s", "с"},
	"mm":  {-3, "m", "м"},
	"nm":  {-9, "m", "м"},
}

// prefixes — десятичные приставки по экспонентам, кратным трём.
var prefixes = map[int]struct{ en, ru string }{
	-12: {"p", "п"},
	-9:  {"n", "н"},
	-6:  {"µ", "мк"},
	-3:  {"m", "м"},
	0:   {"", ""},
	3:   {"k", "к"},
	6:   {"M", "М"},
	9:   {"G", "Г"},
}

// FormatValue — значение с единицей по локали: удобная производная
// единица отображения (мантисса в [1, 1000)) либо символ канонической
// единицы, если приставки не определены. unit "" — безразмерное значение.
func FormatValue(l Language, unit string, v float64) string {
	num := FormatNumber(l, v)
	if unit == "" {
		return num
	}
	b, ok := unitBases[unit]
	if !ok {
		return num + " " + UnitSymbol(l, unit)
	}
	// Экспонента значения в базовых единицах; ноль выводится символом
	// канонической единицы.
	if v == 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return num + " " + UnitSymbol(l, unit)
	}
	exp := math.Floor(math.Log10(math.Abs(v))) + float64(b.exp)
	k := 3 * math.Floor(exp/3)
	if k < -12 {
		k = -12
	}
	if k > 9 {
		k = 9
	}
	mantissa := v * math.Pow(10, float64(b.exp)-k)
	// Подавление шума двоичного представления (2.2e6 пФ → ровно 2.2).
	mantissa = math.Round(mantissa*1e6) / 1e6
	p := prefixes[int(k)]
	if l == Ru {
		return FormatNumber(l, mantissa) + " " + p.ru + b.ruBase
	}
	return FormatNumber(l, mantissa) + " " + p.en + b.enBase
}
