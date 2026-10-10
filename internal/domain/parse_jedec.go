package domain

// Грамматика JEDEC (система EIA-370, с 1982 — JESD370B; план 03 §2.1,
// выверка — 07 §3):
//
//	<число переходов>N<номер регистрации>[<суффиксные буквы>]
//
// Цифра — число p-n-переходов: 1 диоды, 2 транзисторы и тиристоры,
// 3 четырёхвыводные приборы (тетроды, двухзатворные полевые), 4 оптопары —
// вне классов модуля. По JESD370B N — корпусированное изделие, C — кристалл.
// Материал и подкласс/категория в обозначении не кодируются — задаются
// атрибутами записи; военные префиксы JAN/JANTX выводят обозначение из
// системы (класс other).

// parseJedec разбирает обозначения JEDEC.
func parseJedec(s *scanner, _ Kind) (ParsedDesignation, error) {
	canonical := s.text()

	// Число p-n-переходов.
	junc, has := s.peek()
	if !has || !isDigitRune(junc) {
		return ParsedDesignation{}, s.fail(MsgExpectJedecJunctions)
	}
	s.i++
	if junc < '1' || junc > '4' {
		return ParsedDesignation{}, s.failAt(s.i-1, MsgExpectJedecJunctions)
	}

	// Признак системы: N — корпусированное изделие, C — кристалл.
	mark, has := s.peek()
	if !has || (mark != 'N' && mark != 'C') {
		return ParsedDesignation{}, s.fail(MsgExpectJedecLetter)
	}
	s.i++

	// Номер регистрации EIA.
	start := s.i
	run, n := s.digits()
	if n == 0 {
		return ParsedDesignation{}, s.fail(MsgExpectJedecNumber)
	}
	if n > 4 || run[0] == '0' {
		return ParsedDesignation{}, s.failToken(start, MsgExpectJedecNumberNoZero, run)
	}
	devNumber := mustAtoi(run)

	// Суффиксные буквы улучшенных версий (2N2222A).
	letters := ""
	for len(letters) < 3 {
		r, has := s.peek()
		if !has || !isLatinUpper(r) {
			break
		}
		letters += string(r)
		s.i++
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	var kind Kind
	switch junc {
	case '1':
		kind = KindDiode
	case '2', '3':
		// Тетроды и двухзатворные полевые — транзисторы.
		kind = KindTransistor
	default:
		// 4N — оптопары.
		return ParsedDesignation{}, KindNotSupported()
	}

	fields := []Field{
		NumField("junctions", float64(junc-'0')),
		NumField("dev_number", float64(devNumber)),
	}
	if letters != "" {
		fields = append(fields, TextField("letters", letters))
	}
	return ParsedDesignation{Kind: kind, System: SystemJedec, Designation: canonical, Fields: fields}, nil
}
