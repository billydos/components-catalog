package cli

// Вывод карточки и разбора обозначения — слой вывода. Отображаемые
// названия классов/систем/групп/параметров/атрибутов и символы единиц —
// бандлы internal/i18n по локали команды (--lang, D9); значения
// форматируются с инженерными приставками (i18n.FormatValue, этап 8.4).

import (
	"fmt"
	"io"
	"strconv"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/service"
)

// fieldDisplayName — отображаемое имя поля разбора по локали.
func fieldDisplayName(lang i18n.Language, name string) string {
	return i18n.DesignationField(lang, name)
}

// printParsed выводит результат разбора обозначения.
func printParsed(w io.Writer, lang i18n.Language, p domain.ParsedDesignation) {
	kindName, systemName := string(p.Kind), string(p.System)
	if k := p.Kind; k.IsValid() {
		kindName = i18n.KindName(lang, string(k))
	}
	if s := p.System; s.IsValid() {
		systemName = i18n.SystemName(lang, string(s))
	}
	fmt.Fprintln(w, i18n.Message(lang, "cli_parse_head",
		p.Designation, kindName, string(p.Kind), systemName, string(p.System)))
	for _, f := range p.Fields {
		fmt.Fprintln(w, i18n.Message(lang, "cli_field_line",
			fieldDisplayName(lang, f.Name), fieldDisplayValue(lang, f)))
	}
}

// fieldDisplayValue — значение поля разбора: assembly — качественное
// (сборка/прибор), прочие числа — компактной записью локали.
func fieldDisplayValue(lang i18n.Language, f domain.Field) string {
	switch f.Name {
	case "assembly":
		if f.IsNum && f.Num != 0 {
			return i18n.Message(lang, "cli_assembly_value")
		}
		return i18n.Message(lang, "cli_device_value")
	case "material":
		// Значение словаря материалов — стабильный код (D9); отображается
		// названием локали, расширения без записи в бандле — кодом.
		return i18n.MaterialName(lang, f.Text)
	case "subclass":
		return i18n.SubclassName(lang, f.Text)
	case "adjustment":
		return i18n.AdjustmentName(lang, f.Text)
	case "category":
		return i18n.CategoryName(lang, f.Text)
	}
	if f.IsNum {
		return i18n.FormatNumber(lang, f.Num)
	}
	return f.Text
}

// printCard выводит карточку записи сгруппированными секциями: заголовочный
// блок (обозначение, система, поля обозначения — значения с отступом), далее
// самостоятельные секции (атрибуты, группы параметров, исполнения,
// производители, аналоги, встречные ссылки), разделённые пустой строкой.
func printCard(w io.Writer, lang i18n.Language, c *service.Card, snap *catalog.Snapshot) {
	fmt.Fprintf(w, "%s — %s; id %d\n",
		c.Designation, i18n.KindName(lang, string(c.Kind)), c.ID)
	fmt.Fprintln(w, i18n.Message(lang, "cli_system_line",
		i18n.SystemName(lang, string(c.System)), string(c.System)))
	for _, f := range c.Fields {
		fmt.Fprintln(w, i18n.Message(lang, "cli_field_line",
			fieldDisplayName(lang, f.Name), fieldDisplayValue(lang, f)))
	}

	// Заголовочный блок — первая секция: разделитель нужен уже перед
	// следующей за ним.
	sep := true
	// section — пустая строка между секциями карточки.
	section := func() {
		if sep {
			fmt.Fprintln(w)
		}
		sep = true
	}
	if len(c.Attributes) > 0 {
		section()
		fmt.Fprintln(w, i18n.Message(lang, "cli_h_attrs"))
		for _, a := range c.Attributes {
			value := ""
			switch {
			case a.Text != nil:
				value = *a.Text
			case a.Num != nil:
				value = i18n.FormatNumber(lang, *a.Num)
			case a.Bool != nil:
				value = strconv.FormatBool(*a.Bool)
			}
			fmt.Fprintln(w, i18n.Message(lang, "cli_attr_line",
				i18n.AttributeName(lang, a.Code), a.Code, value))
		}
	}
	for _, g := range c.Groups {
		section()
		fmt.Fprintf(w, "%s:\n", i18n.GroupName(lang, g.Code))
		for _, v := range g.Values {
			fmt.Fprintf(w, "  %s%s\n", i18n.ParameterName(lang, v.Parameter),
				valueWithConditions(lang, v, snap))
		}
	}
	for _, v := range c.Variants {
		section()
		label := v.Label
		if label == "" {
			label = i18n.Message(lang, "cli_no_label")
		}
		fmt.Fprintln(w, i18n.Message(lang, "cli_variant_head", label))
		for _, g := range v.Groups {
			fmt.Fprintf(w, "  %s:\n", i18n.GroupName(lang, g.Code))
			for _, val := range g.Values {
				fmt.Fprintf(w, "    %s%s\n", i18n.ParameterName(lang, val.Parameter),
					valueWithConditions(lang, val, snap))
			}
		}
	}
	if len(c.Manufacturers) > 0 {
		section()
		fmt.Fprintln(w, i18n.Message(lang, "cli_manufacturers", joinParts(c.Manufacturers)))
	}
	if len(c.Analogs) > 0 {
		section()
		parts := make([]string, 0, len(c.Analogs))
		for _, a := range c.Analogs {
			if a.Note != "" {
				parts = append(parts, a.Designation+" ("+a.Note+")")
				continue
			}
			parts = append(parts, a.Designation)
		}
		fmt.Fprintln(w, i18n.Message(lang, "cli_analogs", joinParts(parts)))
	}
	if len(c.Backlinks) > 0 {
		section()
		parts := make([]string, 0, len(c.Backlinks))
		for _, b := range c.Backlinks {
			if b.Note != "" {
				parts = append(parts, b.Designation+" ("+b.Note+")")
				continue
			}
			parts = append(parts, b.Designation)
		}
		fmt.Fprintln(w, i18n.Message(lang, "cli_backlinks", joinParts(parts)))
	}
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

// valueWithConditions — значение параметра с единицей и условиями:
// точное — число, диапазон — «мин–макс», «не менее», «не более»;
// условия — в скобках, всё — по локали (этап 8.4 — инженерные
// приставки).
func valueWithConditions(lang i18n.Language, v service.CardValue, snap *catalog.Snapshot) string {
	value := ""
	switch {
	case v.Exact != nil:
		value = i18n.FormatValue(lang, v.Unit, *v.Exact)
	case v.Min != nil && v.Max != nil:
		value = i18n.FormatValue(lang, v.Unit, *v.Min) + "–" + i18n.FormatValue(lang, v.Unit, *v.Max)
	case v.Min != nil:
		value = i18n.Message(lang, "cli_at_least", i18n.FormatValue(lang, v.Unit, *v.Min))
	case v.Max != nil:
		value = i18n.Message(lang, "cli_at_most", i18n.FormatValue(lang, v.Unit, *v.Max))
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
			unit = " " + i18n.UnitSymbol(lang, def.Unit)
		}
		conds = append(conds, fmt.Sprintf("%s=%s%s", c.Condition, i18n.FormatNumber(lang, c.Value), unit))
	}
	return ": " + value + i18n.Message(lang, "cli_cond_at", joinParts(conds))
}
