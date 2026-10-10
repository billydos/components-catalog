package domain

import "strings"

// Грамматика JIS C 7012 / EIAJ (номер регистрации присваивает JEITA-EDEREC;
// план 03 §2.1, выверка — 07 §3):
//
//	<число переходов>S<класс><номер регистрации>[<суффикс>]
//
// Цифра — число p-n-переходов: 1 диоды, 2 транзисторы, 3 четырёхвыводные
// (двухзатворные). S — признак регистрации. Буква класса опциональна только
// у старых диодных регистраций (1S2076); полный перечень классов: A p-n-p
// ВЧ, B p-n-p НЧ, C n-p-n ВЧ, D n-p-n НЧ, E диод, F тиристор, G диод Ганна,
// H однопереходный, J p-канал ПТ, K n-канал ПТ, M симистор, Q светодиод,
// R выпрямительный, S сигнальный диод, T лавинный, V варикап, Z стабилитрон.
// На корпусах префикс 2S часто опускается (C1815) — канонизация восстанав-
// ливает полный формат: сокращённые формы не являются ключами записей.

// jisClassLetters — полный перечень букв класса.
const jisClassLetters = "ABCDEFGHJKMQRSTVZ"

// jisShortFormClasses — классы, для которых распространено сокращение без
// префикса 2S (транзисторные); восстановление — только для них.
const jisShortFormClasses = "ABCDJK"

// parseJis разбирает обозначения JIS, включая сокращённые формы
// (C1815 → 2SC1815).
func parseJis(s *scanner, _ Kind) (ParsedDesignation, error) {
	canonical := s.text()
	if r, has := s.peek(); !has || !isDigitRune(r) {
		// Сокращённая форма: буква класса + номер, префикс 2S опущен.
		return parseJisShortForm(s)
	}

	// Число p-n-переходов.
	junc := s.src[s.i]
	s.i++
	if junc < '1' || junc > '3' {
		return ParsedDesignation{}, s.failAt(s.i-1, MsgExpectJisJunctions)
	}

	// Признак регистрации S.
	if !s.peekIs(s.i, 'S') {
		return ParsedDesignation{}, s.fail(MsgExpectJisS)
	}
	s.i++

	// Буква класса — опциональна только у диодов (старые регистрации).
	var subclass string
	if r, has := s.peek(); has && isLatinUpper(r) {
		if !strings.ContainsRune(jisClassLetters, r) {
			return ParsedDesignation{}, s.fail(MsgExpectJisClassLetter)
		}
		subclass = string(r)
		s.i++
	}
	if subclass == "" && junc != '1' {
		return ParsedDesignation{}, s.fail(MsgExpectJisClassRequired)
	}

	// Номер регистрации JEITA-EDEREC.
	start := s.i
	run, n := s.digits()
	if n < 2 || n > 4 || run[0] == '0' {
		return ParsedDesignation{}, s.failToken(start, MsgExpectJisNumber, run)
	}
	devNumber := mustAtoi(run)

	// Суффикс: группы hFE (A/B/C), цвет светодиодов; после дефиса —
	// параметры (1SR154-400).
	letters := ""
	for len(letters) < 3 {
		r, has := s.peek()
		if !has || !isLatinUpper(r) {
			break
		}
		letters += string(r)
		s.i++
	}
	if s.peekIs(s.i, '-') {
		s.i++
		if s.readAlnumRun(8) == 0 {
			return ParsedDesignation{}, s.fail(MsgExpectJisSuffix)
		}
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	kind := KindDiode
	if junc != '1' {
		kind = KindTransistor
	}
	designation := canonical
	fields := []Field{NumField("junctions", float64(junc-'0'))}
	if subclass != "" {
		// Подкласс — код единого словаря (A/B/C/D → bjt, J/K → fet, …).
		code, _ := JisSubclassByLetter([]rune(subclass)[0])
		fields = append(fields, TextField("subclass", code))
	}
	fields = append(fields, NumField("dev_number", float64(devNumber)))
	if letters != "" {
		fields = append(fields, TextField("letters", letters))
	}
	return ParsedDesignation{Kind: kind, System: SystemJis, Designation: designation, Fields: fields}, nil
}

// parseJisShortForm восстанавливает полную форму 2S<класс><номер>;
// каноническое обозначение — полная форма (сокращённые — не ключи записей).
func parseJisShortForm(s *scanner) (ParsedDesignation, error) {
	class, has := s.peek()
	if !has || !strings.ContainsRune(jisShortFormClasses, class) {
		return ParsedDesignation{}, s.fail(MsgExpectJisShortForm)
	}
	s.i++
	start := s.i
	run, n := s.digits()
	if n < 3 || n > 4 || run[0] == '0' {
		return ParsedDesignation{}, s.failToken(start, MsgExpectJisShortNumber, run)
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}
	full := "2S" + string(class) + run
	subclassCode, _ := JisSubclassByLetter(class)
	return ParsedDesignation{
		Kind:        KindTransistor,
		System:      SystemJis,
		Designation: full,
		Fields: []Field{
			NumField("junctions", 2),
			TextField("subclass", subclassCode),
			NumField("dev_number", float64(mustAtoi(run))),
		},
	}, nil
}
