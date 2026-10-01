package domain

import (
	"fmt"
	"strings"
)

// Грамматика единой системы обозначений конденсаторов (действующая
// кодификация — ГОСТ Р 57440-2017, историческая семантика групп —
// советские справочники; план 03 §2.3, выверка — 07 §5):
//
//	<подкласс><группа>-<номер разработки>[буква]
//
// Подклассы: К постоянной ёмкости, КТ подстроечные, КП переменной ёмкости,
// КН нелинейные (ГОСТ Р 57440-2017); исторически также КС — конденсаторные
// сборки. Группы — две цифры у К (таблица перспективных и исторических),
// одна цифра у КТ/КП/КН (и одна-две у КС). Обозначения подстроечных вида
// КТ4-25 принадлежат этой системе, а не реестру семейств.

// capGroups — таблица групп конденсаторов постоянной ёмкости (подкласс К):
// перспективные по ГОСТ Р 57440-2017 (таблица 1) и исторические
// (приложение А; расхождения семантики — 61 вакуумные/газообразные,
// 70/71 полиэтиленнафталатные/полистирольные, 73/74/76 — обе версии
// хранятся данными с атрибуцией периода).
var capGroups = map[int]string{
	10: "керамические на номинальное напряжение ниже 1600 В",
	15: "керамические на номинальное напряжение 1600 В и выше",
	21: "стеклянные (историческая)",
	22: "стеклокерамические (историческая)",
	23: "стеклоэмалевые (историческая)",
	26: "тонкоплёночные",
	31: "слюдяные малой мощности (историческая)",
	32: "слюдяные большой мощности (историческая)",
	40: "бумажные фольговые ниже 2 кВ (историческая)",
	41: "бумажные фольговые 2 кВ и выше (историческая)",
	42: "бумажные металлизированные (историческая)",
	50: "оксидно-электролитические алюминиевые",
	51: "оксидно-электролитические танталовые фольговые",
	52: "оксидно-электролитические танталовые объёмно-пористые",
	53: "оксидно-полупроводниковые",
	58: "с двойным электрическим слоем (ионисторы)",
	60: "воздушные (историческая)",
	61: "вакуумные / газообразные (историческая)",
	70: "полиэтиленнафталатные / полистирольные фольговые (историческая)",
	71: "полистирольные металлизированные (историческая)",
	72: "фторопластовые (историческая)",
	73: "полиэтилентерефталатные",
	74: "полиэтилентерефталатные фольговые (историческая)",
	75: "комбинированные",
	76: "полифениленсульфидные / лакоплёночные (историческая)",
	77: "поликарбонатные (историческая)",
	78: "полипропиленовые",
	79: "полиимидные (историческая)",
}

// parseCapacitorGost разбирает обозначения единой системы конденсаторов.
func parseCapacitorGost(s *scanner, _ Kind) (ParsedDesignation, error) {
	canonical := s.text()

	var prefix string
	switch {
	case strings.HasPrefix(canonical, "КТ"):
		prefix = "КТ"
		s.i = 2
	case strings.HasPrefix(canonical, "КП"):
		prefix = "КП"
		s.i = 2
	case strings.HasPrefix(canonical, "КН"):
		prefix = "КН"
		s.i = 2
	case strings.HasPrefix(canonical, "КС"):
		prefix = "КС"
		s.i = 2
	case strings.HasPrefix(canonical, "К"):
		prefix = "К"
		s.i = 1
	default:
		return ParsedDesignation{}, s.fail("префикс подкласса (К, КТ, КП, КН либо КС)")
	}

	// Группа: таблица по подклассу.
	var group int
	switch prefix {
	case "К":
		start := s.i
		run, n := s.digits()
		if n != 2 {
			return ParsedDesignation{}, s.failToken(start, "двузначная группа по таблице групп конденсаторов", run)
		}
		if _, known := capGroups[mustAtoi(run)]; !known {
			return ParsedDesignation{}, NewError(CodeInvalidDesignation, fmt.Sprintf(
				"обозначение «%s»: неизвестная группа конденсаторов «%s» (префикс К)", canonical, run))
		}
		group = mustAtoi(run)
	default:
		hi := 4 // КТ/КП: подстроечные и переменные группы 1–4
		if prefix == "КН" || prefix == "КС" {
			hi = 2
		}
		var err error
		group, err = readGroupDigit(s, "группа по таблице подкласса", 1, hi)
		if err != nil {
			return ParsedDesignation{}, err
		}
	}

	if err := readHyphen(s); err != nil {
		return ParsedDesignation{}, err
	}
	devNumber, err := readDevNumber(s, 3)
	if err != nil {
		return ParsedDesignation{}, err
	}

	// Буква варианта конструкции/ТКЕ — одна буква (К10-17Б, К10-47В).
	letters := s.readCyrillicLetters(1)
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	fields := []Field{
		TextField("prefix", prefix),
		NumField("group", float64(group)),
		NumField("dev_number", float64(devNumber)),
	}
	if letters != "" {
		fields = append(fields, TextField("letters", letters))
	}
	return ParsedDesignation{Kind: KindCapacitor, System: SystemGost, Designation: canonical, Fields: fields}, nil
}

// CapGroupKnown сообщает, входит ли группа подкласса К в таблицу
// (перспективные и исторические); используется тестами и будущими сидами.
func CapGroupKnown(group int) bool {
	_, ok := capGroups[group]
	return ok
}
