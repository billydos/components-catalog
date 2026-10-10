package i18n

// Форматирование значений с инженерными приставками (этап 8.4, D9):
// хранилище держит канонические единицы (латинские коды), слой вывода
// выбирает удобную производную единицу отображения (мкФ, МОм, кГц…)
// и записывает число с разделителем локали. Производные единицы в
// справочнике units не хранятся.

import (
	"math"
	"strconv"
	"strings"
)

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

// decExp — десятичный порядок значения (v = m × 10^e, m ∈ [1, 10)):
// порядок берётся из shortest-экспоненты strconv.FormatFloat — границы
// степеней десяти точны, в отличие от math.Log10 (округление которого у
// отдельных 1eN даёт ошибку на единицу порядка).
func decExp(v float64) int {
	s := strconv.FormatFloat(v, 'e', -1, 64)
	e, _ := strconv.Atoi(s[strings.IndexByte(s, 'e')+1:])
	return e
}

// floorDiv3 — пол деления на три (в сторону минус бесконечности):
// экспонента производной единицы кратна трём и для отрицательных порядков.
func floorDiv3(x int) int {
	q := x / 3
	if x%3 != 0 && x < 0 {
		q--
	}
	return q
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
	k := 3 * floorDiv3(decExp(v)+b)
	if k < -12 {
		k = -12
	}
	if k > 9 {
		k = 9
	}
	mantissa := v * math.Pow(10, float64(b)-float64(k))
	// Подавление шума двоичного представления (2.2e6 пФ → ровно 2.2):
	// порядок декады выбран до округления мантиссы — по значению,
	// округлённому до значащих цифр (shortest-порядок в decExp).
	mantissa = math.Round(mantissa*1e6) / 1e6
	if math.Abs(mantissa) >= 1000 && k < 9 {
		// Округление подняло мантиссу в следующую декаду («1000 мВт» →
		// «1 Вт»); у верхней границы приставок (k = 9) скачок невозможен —
		// мантисса 1000…9999 остаётся как есть.
		mantissa /= 1000
		k += 3
	}
	s := scriptFor(l)
	return FormatNumber(l, mantissa) + " " + s.prefixes[k] + s.bases[unit]
}
