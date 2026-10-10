package domain

import (
	"strings"
)

// Грамматики резисторных систем (план 03 §2.2, выверка — 07 §4).
//
// gost (ГОСТ 3453-68):
//
//	[С|СП]<группа 1–6>-<номер разработки>[<буквы>]
//
// Семейства С/СП — постоянные/переменные; группы — материал/технология:
// 1 непроволочные тонкослойные углеродистые и бороуглеродистые,
// 2 металлодиэлектрические/металлооксидные, 3 композиционные плёночные,
// 4 композиционные объёмные, 5 проволочные, 6 непроволочные тонкослойные
// металлизированные. СТ/СН/СФ — терморезисторы/варисторы/фоторезисторы:
// иные классы приборов, резисторному классу не принадлежат. Суффикс
// мощности (С2-33Н-0.125) — элемент полного обозначения, в ключ записи не
// входит: ошибка разбора с подсказкой задать исполнения-варианты (D6).
//
// ost (ОСТ 11.074.009-78):
//
//	[Р|РП|НР]<материал 1|2>-<регистрационный номер>
//
// Р постоянные, РП переменные, НР наборы; 1 непроволочные, 2 проволочные
// или металлофольговые. ВР/ВРП/ТР/ТРП — варисторы/терморезисторы — иные
// классы. Формы с gost не пересекаются (С/СП против Р/РП/НР) — автодетект
// однозначен.

// parseResistorGost разбирает обозначения ГОСТ 3453-68.
func parseResistorGost(s *scanner, _ Kind) (ParsedDesignation, error) {
	canonical := s.text()

	var family string
	switch {
	case strings.HasPrefix(canonical, "СП"):
		family = "СП"
		s.i = 2
	case strings.HasPrefix(canonical, "СТ"), strings.HasPrefix(canonical, "СН"), strings.HasPrefix(canonical, "СФ"):
		// Терморезисторы, варисторы, фоторезисторы — иные классы приборов.
		return ParsedDesignation{}, KindNotSupported()
	case strings.HasPrefix(canonical, "С"):
		family = "С"
		s.i = 1
	default:
		return ParsedDesignation{}, s.fail(MsgExpectResFamilyCS)
	}

	group, err := readGroupDigit(s, MsgExpectResMaterialGroup6, 1, 6)
	if err != nil {
		return ParsedDesignation{}, err
	}
	if err := readHyphen(s); err != nil {
		return ParsedDesignation{}, err
	}
	devNumber, err := readDevNumber(s, 3)
	if err != nil {
		return ParsedDesignation{}, err
	}

	// Буквы модификации (С2-33Н, С5-16МВ).
	letters := s.readCyrillicLetters(3)

	// Числовой хвост после букв — суффикс мощности полного обозначения:
	// в ключ не входит, мощность задаётся исполнениями-вариантами (D6).
	// Отдельный код: политический отказ всплывает из автодетекта, а не
	// затирается итоговым «не удалось распознать».
	if s.peekIs(s.i, '-') {
		if next, has := s.peekAt(s.i + 1); has && (isDigitRune(next) || next == '.') {
			base := string(s.src[:s.i])
			return ParsedDesignation{}, NewErrorf(CodePowerSuffix, MsgPowerSuffix, canonical, base)
		}
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	fields := []Field{
		TextField("family", family),
		TextField("adjustment", ResistorFamilyAdjustment(family)),
		NumField("group", float64(group)),
		NumField("dev_number", float64(devNumber)),
	}
	if letters != "" {
		fields = append(fields, TextField("letters", letters))
	}
	return ParsedDesignation{Kind: KindResistor, System: SystemGost, Designation: canonical, Fields: fields}, nil
}

// parseResistorOst разбирает обозначения ОСТ 11.074.009-78.
func parseResistorOst(s *scanner, _ Kind) (ParsedDesignation, error) {
	canonical := s.text()

	var family string
	switch {
	case strings.HasPrefix(canonical, "РП"):
		family = "РП"
		s.i = 2
	case strings.HasPrefix(canonical, "НР"):
		family = "НР"
		s.i = 2
	case strings.HasPrefix(canonical, "Р"):
		family = "Р"
		s.i = 1
	case strings.HasPrefix(canonical, "ВРП"), strings.HasPrefix(canonical, "ВР"),
		strings.HasPrefix(canonical, "ТРП"), strings.HasPrefix(canonical, "ТР"):
		// Варисторы и терморезисторы — иные классы приборов.
		return ParsedDesignation{}, KindNotSupported()
	default:
		return ParsedDesignation{}, s.fail(MsgExpectResFamilyR)
	}

	group, err := readGroupDigit(s, MsgExpectResMaterialGroup2, 1, 2)
	if err != nil {
		return ParsedDesignation{}, err
	}
	if err := readHyphen(s); err != nil {
		return ParsedDesignation{}, err
	}
	devNumber, err := readDevNumber(s, 3)
	if err != nil {
		return ParsedDesignation{}, err
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	return ParsedDesignation{
		Kind:        KindResistor,
		System:      SystemOst,
		Designation: canonical,
		Fields: []Field{
			TextField("family", family),
			TextField("adjustment", ResistorFamilyAdjustment(family)),
			NumField("group", float64(group)),
			NumField("dev_number", float64(devNumber)),
		},
	}, nil
}

// readGroupDigit читает цифру группы в диапазоне [lo, hi].
func readGroupDigit(s *scanner, expected MsgID, lo, hi int) (int, error) {
	r, has := s.peek()
	if !has || !isDigitRune(r) {
		return 0, s.fail(expected)
	}
	v := int(r - '0')
	if v < lo || v > hi {
		return 0, s.failAt(s.i, expected)
	}
	s.i++
	return v, nil
}

// readHyphen читает разделительный дефис.
func readHyphen(s *scanner) error {
	if !s.peekIs(s.i, '-') {
		return s.fail(MsgExpectHyphen)
	}
	s.i++
	return nil
}

// readDevNumber читает номер разработки — до max цифр, без ведущего нуля;
// ведущий ноль и превышение разрядности — отдельные сообщения.
func readDevNumber(s *scanner, max int) (int, error) {
	start := s.i
	run, n := s.digits()
	if n == 0 {
		return 0, s.fail(MsgExpectDevNumber)
	}
	if run[0] == '0' {
		return 0, NewErrorf(CodeInvalidDesignation, MsgDevNumberZeros, s.text(), start+1, max, run)
	}
	if n > max {
		return 0, NewErrorf(CodeInvalidDesignation, MsgDevNumberDigits, s.text(), start+1, max, run)
	}
	return mustAtoi(run), nil
}
