package i18n

// Форматирование значений с инженерными приставками (этап 8.4, D9):
// хранилище держит канонические единицы (латинские коды), слой вывода
// выбирает удобную производную единицу отображения (мкФ, МОм, кГц…)
// и записывает число с разделителем локали. Производные единицы в
// справочнике units не хранятся.

import "math"

// unitExps — базовые величины канонических единиц: экспонента кода
// (pF = 1e-12 F). Только единицы с осмысленными приставками; прочие (dB,
// pct, degC и производные температур, mcd, g) выводятся как есть символом
// локали.
var unitExps = map[string]int{
	"V":   0,
	"mV":  -3,
	"mA":  -3,
	"uA":  -6,
	"A":   0,
	"MHz": 6,
	"W":   0,
	"mW":  -3,
	"ohm": 0,
	"pF":  -12,
	"ps":  -12,
	"ns":  -9,
	"us":  -6,
	"mm":  -3,
	"nm":  -9,
}

// unitScript — отображение производных единиц языка: базы канонических
// единиц и десятичные приставки (F/Ф, Ω/Ом, Гц/Hz…; п/н/мк…), кратные
// трём экспонентам.
type unitScript struct {
	bases    map[string]string
	prefixes map[int]string
}

// unitScripts — наборы отображения по языкам; язык без собственного
// набора — канонический en.
var unitScripts = map[Language]unitScript{
	En: {
		bases: map[string]string{
			"V":   "V",
			"mV":  "V",
			"mA":  "A",
			"uA":  "A",
			"A":   "A",
			"MHz": "Hz",
			"W":   "W",
			"mW":  "W",
			"ohm": "Ω",
			"pF":  "F",
			"ps":  "s",
			"ns":  "s",
			"us":  "s",
			"mm":  "m",
			"nm":  "m",
		},
		prefixes: map[int]string{
			-12: "p",
			-9:  "n",
			-6:  "µ",
			-3:  "m",
			0:   "",
			3:   "k",
			6:   "M",
			9:   "G",
		},
	},
	Ru: {
		bases: map[string]string{
			"V":   "В",
			"mV":  "В",
			"mA":  "А",
			"uA":  "А",
			"A":   "А",
			"MHz": "Гц",
			"W":   "Вт",
			"mW":  "Вт",
			"ohm": "Ом",
			"pF":  "Ф",
			"ps":  "с",
			"ns":  "с",
			"us":  "с",
			"mm":  "м",
			"nm":  "м",
		},
		prefixes: map[int]string{
			-12: "п",
			-9:  "н",
			-6:  "мк",
			-3:  "м",
			0:   "",
			3:   "к",
			6:   "М",
			9:   "Г",
		},
	},
}

// scriptFor — набор отображения языка; язык без собственного набора —
// канонический en.
func scriptFor(l Language) unitScript {
	if s, ok := unitScripts[l]; ok {
		return s
	}
	return unitScripts[En]
}

// FormatValue — значение с единицей по локали: удобная производная
// единица отображения (мантисса в [1, 1000)) либо символ канонической
// единицы, если приставки не определены. unit "" — безразмерное значение.
func FormatValue(l Language, unit string, v float64) string {
	num := FormatNumber(l, v)
	if unit == "" {
		return num
	}
	b, ok := unitExps[unit]
	if !ok {
		return num + " " + UnitSymbol(l, unit)
	}
	// Экспонента значения в базовых единицах; ноль выводится символом
	// канонической единицы.
	if v == 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return num + " " + UnitSymbol(l, unit)
	}
	exp := math.Floor(math.Log10(math.Abs(v))) + float64(b)
	k := 3 * math.Floor(exp/3)
	if k < -12 {
		k = -12
	}
	if k > 9 {
		k = 9
	}
	mantissa := v * math.Pow(10, float64(b)-k)
	// Подавление шума двоичного представления (2.2e6 пФ → ровно 2.2).
	mantissa = math.Round(mantissa*1e6) / 1e6
	s := scriptFor(l)
	return FormatNumber(l, mantissa) + " " + s.prefixes[int(k)] + s.bases[unit]
}
