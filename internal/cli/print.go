package cli

// Вывод карточки и разбора обозначения — слой вывода. Отображаемые
// названия классов/систем/групп/параметров/атрибутов и символы единиц —
// бандлы internal/i18n по локали команды (--lang, D9); значения
// форматируются с инженерными приставками (i18n.FormatValue, этап 8.4).

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

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
	fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliParseHead),
		p.Designation, kindName, string(p.Kind), systemName, string(p.System)))
	for _, f := range p.Fields {
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliFieldLine),
			fieldDisplayName(lang, f.Name), fieldDisplayValue(lang, f)))
	}
}

// fieldDisplayValue — значение поля разбора: assembly — качественное
// (сборка/прибор), прочие числа — компактной записью локали.
func fieldDisplayValue(lang i18n.Language, f domain.Field) string {
	switch f.Name {
	case "assembly":
		if f.IsNum && f.Num != 0 {
			return i18n.Message(lang, string(domain.MsgCliAssemblyValue))
		}
		return i18n.Message(lang, string(domain.MsgCliDeviceValue))
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
	fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliSystemLine),
		i18n.SystemName(lang, string(c.System)), string(c.System)))
	for _, f := range c.Fields {
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliFieldLine),
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
	if len(c.Synonyms) > 0 {
		section()
		parts := make([]string, 0, len(c.Synonyms))
		for _, s := range c.Synonyms {
			parts = append(parts, synonymDisplay(lang, s))
		}
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliSynonyms), joinParts(parts)))
	}
	if len(c.Attributes) > 0 {
		section()
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliHAttrs)))
		printAttrLines(w, lang, c.Attributes, "")
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
		label := cmp.Or(v.Label, i18n.Message(lang, string(domain.MsgCliNoLabel)))
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliVariantHead), label))
		if len(v.Attributes) > 0 {
			fmt.Fprintln(w, "  "+i18n.Message(lang, string(domain.MsgCliHAttrs)))
			printAttrLines(w, lang, v.Attributes, "  ")
		}
		for _, g := range v.Groups {
			fmt.Fprintf(w, "  %s:\n", i18n.GroupName(lang, g.Code))
			for _, val := range g.Values {
				fmt.Fprintf(w, "    %s%s\n", i18n.ParameterName(lang, val.Parameter),
					valueWithConditions(lang, val, snap))
			}
		}
	}
	if tg := c.TypeGroup; tg != nil {
		section()
		names := make([]string, 0, len(tg.Siblings)+1)
		names = append(names, c.Designation)
		for _, s := range tg.Siblings {
			names = append(names, s.Designation)
		}
		slices.Sort(names)
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliTypeGroupHead),
			tg.Base, strings.Join(names, ", ")))
		for _, d := range tg.Diffs {
			cells := make([]string, 0, len(d.Values))
			for _, v := range d.Values {
				cells = append(cells, v.Designation+" — "+valueString(lang, v.Value, snap))
			}
			fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliFieldLine),
				i18n.ParameterName(lang, d.Parameter), strings.Join(cells, "; ")))
		}
	}
	if len(c.Manufacturers) > 0 {
		section()
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliManufacturers), joinParts(c.Manufacturers)))
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
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliAnalogs), joinParts(parts)))
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
		fmt.Fprintln(w, i18n.Message(lang, string(domain.MsgCliBacklinks), joinParts(parts)))
	}
}

// printAttrLines выводит значения атрибутов (записи либо исполнения)
// строками с заданным отступом: текст/enum — текст, text_list — заметки
// одной строкой через «; » (карточка — чтение, элемент за элементом —
// файл наполнения), числа — компактной записью локали. Код атрибута
// не повторяется: назначение — ключ файла наполнения, выводится
// catalog list.
func printAttrLines(w io.Writer, lang i18n.Language, attrs []service.CardAttribute, indent string) {
	for _, a := range attrs {
		value := ""
		switch {
		case a.Text != nil:
			value = *a.Text
		case a.Texts != nil:
			value = strings.Join(a.Texts, "; ")
		case a.Num != nil:
			value = i18n.FormatNumber(lang, *a.Num)
		case a.Bool != nil:
			value = strconv.FormatBool(*a.Bool)
		}
		fmt.Fprintf(w, "%s%s\n", indent, i18n.Message(lang, string(domain.MsgCliFieldLine),
			i18n.AttributeName(lang, a.Code), value))
	}
}

func joinParts(parts []string) string {
	return strings.Join(parts, "; ")
}

// synonymDisplay — историческое название карточки с происхождением
// и годами употребления имени: «ПЭВ (до 1968; обозначение до 1968 года)».
func synonymDisplay(lang i18n.Language, s service.CardSynonym) string {
	var extras []string
	switch {
	case s.YearFrom != nil && s.YearTo != nil:
		extras = append(extras, fmt.Sprintf("%d–%d", *s.YearFrom, *s.YearTo))
	case s.YearFrom != nil:
		extras = append(extras, i18n.Message(lang, string(domain.MsgCliYearSince), *s.YearFrom))
	case s.YearTo != nil:
		extras = append(extras, i18n.Message(lang, string(domain.MsgCliYearUntil), *s.YearTo))
	}
	if s.Note != "" {
		extras = append(extras, s.Note)
	}
	if len(extras) == 0 {
		return s.Designation
	}
	return s.Designation + " (" + joinParts(extras) + ")"
}

// valueWithConditions — значение параметра с единицей и условиями
// («: » перед значением — строка группы карточки); условия — в скобках,
// всё — по локали (этап 8.4 — инженерные приставки).
func valueWithConditions(lang i18n.Language, v service.CardValue, snap *catalog.Snapshot) string {
	return ": " + valueString(lang, v, snap)
}

// valueString — «голое» значение параметра с единицей и условиями
// (используется и ячейками мини-матрицы группы типа).
func valueString(lang i18n.Language, v service.CardValue, snap *catalog.Snapshot) string {
	value := ""
	switch {
	case v.Exact != nil:
		value = i18n.FormatValue(lang, v.Unit, *v.Exact)
	case v.Min != nil && v.Max != nil:
		value = i18n.FormatValue(lang, v.Unit, *v.Min) + "–" + i18n.FormatValue(lang, v.Unit, *v.Max)
	case v.Min != nil:
		value = i18n.Message(lang, string(domain.MsgCliAtLeast), i18n.FormatValue(lang, v.Unit, *v.Min))
	case v.Max != nil:
		value = i18n.Message(lang, string(domain.MsgCliAtMost), i18n.FormatValue(lang, v.Unit, *v.Max))
	case v.Text != nil:
		value = *v.Text
	}
	if len(v.Conditions) == 0 {
		return value
	}
	conds := make([]string, 0, len(v.Conditions))
	for _, c := range v.Conditions {
		unit := ""
		if def, ok := snap.Condition(c.Condition); ok && def.Unit != "" {
			unit = " " + i18n.UnitSymbol(lang, def.Unit)
		}
		conds = append(conds, fmt.Sprintf("%s=%s%s", c.Condition, i18n.FormatNumber(lang, c.Value), unit))
	}
	return value + i18n.Message(lang, string(domain.MsgCliCondAt), joinParts(conds))
}
