package cli

// Вывод карточки и разбора обозначения — слой вывода; производные
// единицы отображения (мкФ, МОм, кГц) здесь не переводятся: вывод
// канонических единиц хранилища.

import (
	"fmt"
	"io"
	"strconv"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// fieldDisplayNames — названия полей разбора для вывода.
var fieldDisplayNames = map[string]string{
	"material":     "материал",
	"subclass":     "подкласс",
	"junctions":    "переходы",
	"assembly":     "сборка",
	"feature":      "признак",
	"group":        "группа",
	"dev_number":   "номер разработки",
	"letters":      "буквы",
	"modification": "модификатор",
	"chip":         "вариант кристалла",
	"prefix":       "префикс",
	"series":       "семейство",
	"power":        "мощность",
	"family":       "семейство (буква)",
}

func fieldDisplayName(name string) string {
	if display, ok := fieldDisplayNames[name]; ok {
		return display
	}
	return name
}

// printParsed выводит результат разбора обозначения.
func printParsed(w io.Writer, p domain.ParsedDesignation) {
	kindName, systemName := string(p.Kind), string(p.System)
	if k := p.Kind; k.IsValid() {
		kindName = k.Name()
	}
	if s := p.System; s.IsValid() {
		systemName = s.Name()
	}
	fmt.Fprintf(w, "%s: %s (%s), система %s (%s)\n", p.Designation, kindName, string(p.Kind), systemName, string(p.System))
	for _, f := range p.Fields {
		fmt.Fprintf(w, "  %s: %s\n", fieldDisplayName(f.Name), fieldDisplayValue(f))
	}
}

// fieldDisplayValue — значение поля разбора: assembly — качественное
// (сборка/прибор), прочие числа — компактной записью.
func fieldDisplayValue(f domain.Field) string {
	if f.Name == "assembly" {
		if f.IsNum && f.Num != 0 {
			return "сборка"
		}
		return "прибор"
	}
	if f.IsNum {
		return trimFloatDisplay(f.Num)
	}
	return f.Text
}

// printCard выводит карточку записи: заголовок, поля обозначения,
// атрибуты, значения по группам, исполнения, производители, аналоги.
func printCard(w io.Writer, c *service.Card, snap *catalog.Snapshot) {
	fmt.Fprintf(w, "%s — %s; система %s (%s); id %d\n",
		c.Designation, kindTitle(c), systemTitle(c), string(c.System), c.ID)
	if len(c.Fields) > 0 {
		parts := make([]string, 0, len(c.Fields))
		for _, f := range c.Fields {
			parts = append(parts, fieldDisplayName(f.Name)+": "+fieldDisplayValue(f))
		}
		fmt.Fprintf(w, "Поля обозначения: %s\n", joinParts(parts))
	}
	for _, a := range c.Attributes {
		value := ""
		switch {
		case a.Text != nil:
			value = *a.Text
		case a.Num != nil:
			value = trimFloatDisplay(*a.Num)
		case a.Bool != nil:
			value = strconv.FormatBool(*a.Bool)
		}
		fmt.Fprintf(w, "Атрибут %s (%s): %s\n", a.DisplayName, a.Code, value)
	}
	for _, g := range c.Groups {
		fmt.Fprintf(w, "%s:\n", g.DisplayName)
		for _, v := range g.Values {
			fmt.Fprintf(w, "  %s%s%s\n", v.DisplayName, unitSuffix(v.Unit),
				valueWithConditions(v, snap))
		}
	}
	for _, v := range c.Variants {
		label := v.Label
		if label == "" {
			label = "без метки"
		}
		fmt.Fprintf(w, "Исполнение «%s»:\n", label)
		for _, g := range v.Groups {
			fmt.Fprintf(w, "  %s:\n", g.DisplayName)
			for _, val := range g.Values {
				fmt.Fprintf(w, "    %s%s%s\n", val.DisplayName, unitSuffix(val.Unit),
					valueWithConditions(val, snap))
			}
		}
	}
	if len(c.Manufacturers) > 0 {
		fmt.Fprintf(w, "Производители: %s\n", joinParts(c.Manufacturers))
	}
	if len(c.Analogs) > 0 {
		parts := make([]string, 0, len(c.Analogs))
		for _, a := range c.Analogs {
			if a.Note != "" {
				parts = append(parts, a.Designation+" ("+a.Note+")")
				continue
			}
			parts = append(parts, a.Designation)
		}
		fmt.Fprintf(w, "Аналоги: %s\n", joinParts(parts))
	}
	if len(c.Backlinks) > 0 {
		parts := make([]string, 0, len(c.Backlinks))
		for _, b := range c.Backlinks {
			if b.Note != "" {
				parts = append(parts, b.Designation+" ("+b.Note+")")
				continue
			}
			parts = append(parts, b.Designation)
		}
		fmt.Fprintf(w, "Встречные ссылки: %s\n", joinParts(parts))
	}
}

func kindTitle(c *service.Card) string {
	if c.KindName != "" {
		return c.KindName
	}
	return string(c.Kind)
}

func systemTitle(c *service.Card) string {
	if c.SystemName != "" {
		return c.SystemName
	}
	return string(c.System)
}

func joinParts(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

func unitSuffix(unit string) string {
	if unit == "" {
		return ""
	}
	return ", " + unit
}

// valueWithConditions — значение параметра с условиями измерения:
// точное — число, диапазон — «мин–макс», «не менее», «не более»;
// условия — в скобках с единицами из каталога.
func valueWithConditions(v service.CardValue, snap *catalog.Snapshot) string {
	value := ""
	switch {
	case v.Exact != nil:
		value = trimFloatDisplay(*v.Exact)
	case v.Min != nil && v.Max != nil:
		value = trimFloatDisplay(*v.Min) + "–" + trimFloatDisplay(*v.Max)
	case v.Min != nil:
		value = "не менее " + trimFloatDisplay(*v.Min)
	case v.Max != nil:
		value = "не более " + trimFloatDisplay(*v.Max)
	case v.Text != nil:
		value = *v.Text
	}
	if len(v.Conditions) == 0 {
		return ": " + value
	}
	conds := make([]string, 0, len(v.Conditions))
	for _, c := range v.Conditions {
		unit := ""
		if def, ok := snap.Condition(c.Condition); ok && def.Unit != "" {
			unit = " " + def.Unit
		}
		conds = append(conds, fmt.Sprintf("%s=%s%s", c.Condition, trimFloatDisplay(c.Value), unit))
	}
	return ": " + value + " (при " + joinParts(conds) + ")"
}

// trimFloatDisplay — компактная запись числа (целое — без точки).
func trimFloatDisplay(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
