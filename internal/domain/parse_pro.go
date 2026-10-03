package domain

// Грамматика PRO ELECTRON (первоисточник — справочник Philips Semiconductors
// «Pro Electron type numbering», сентябрь 1994, docs/sources/PRO-ELECTRON.pdf;
// план 03 §2.1, выверка — 07 §2):
//
//	<материал><класс><номер>[<версионные буквы>][-<суффикс подклассификации>]
//
// Материал: A германий, B кремний, C арсенид галлия (и соединения с зоной
// ≥ 1,3 эВ), R соединения без p-n-перехода — класс вне модуля. Класс —
// вторая буква (полная таблица). Номер: 100–999 (бытовая аппаратура,
// расширение до четырёх цифр) либо буква + 10–99 (промышленная/специальная
// регистрация, расширение до трёх цифр); буква номера не имеет фиксирован-
// ного значения, кроме специальных: A триаки (после R/T), F волоконная
// оптика (после G/P/Q), L лазеры (после G/Q), O оптотриаки (после R),
// T трёхвыводные двухцветные светодиоды (после Q), W TVS (после Z).
// Версионные буквы — одна-две (R — обратная полярность, W — SMD).
// Суффиксы-подклассификации распознаются, но в поля не раскладываются.
//
// Порядок проверок: грамматика раньше класса — материал R или класс
// G/N/R/T/W дают kind_not_supported только для грамматически корректного
// обозначения (RC05 не разбирается как PRO ELECTRON и доходит до реестра
// series, RPY84 разбирается и отклоняется по классу).

// proClassKinds — вторая буква (класс прибора); граница «маломощный/
// мощный» — Rth j-mb > / ≤ 15 К/Вт.
var proClassKinds = map[rune]Kind{
	'A': KindDiode,      // маломощный сигнальный диод
	'B': KindDiode,      // варикап
	'C': KindTransistor, // маломощный НЧ транзистор
	'D': KindTransistor, // мощный НЧ транзистор
	'E': KindDiode,      // туннельный диод
	'F': KindTransistor, // маломощный ВЧ транзистор
	'L': KindTransistor, // мощный ВЧ транзистор
	'H': KindDiode,      // магниточувствительный диод
	'P': KindDiode,      // детектор излучения
	'Q': KindDiode,      // излучатель (светодиод, лазер)
	'S': KindTransistor, // маломощный переключательный транзистор
	'U': KindTransistor, // мощный переключательный транзистор
	'X': KindDiode,      // диод-умножитель (варактор, step-recovery)
	'Y': KindDiode,      // мощный выпрямитель
	'Z': KindDiode,      // стабилитрон/стабистор/TVS
}

// proUnsupportedClasses — классы вне стартовых: G наборы разнородных
// приборов, N оптопары, R/T управляющие приборы (тиристоры, триаки),
// W устройства на ПАВ.
const proUnsupportedClasses = "GNRTW"

// parsePro разбирает обозначения PRO ELECTRON.
func parsePro(s *scanner, _ Kind) (ParsedDesignation, error) {
	canonical := s.text()

	// Первая буква — материал.
	mat, has := s.peek()
	if !has || (mat != 'A' && mat != 'B' && mat != 'C' && mat != 'R') {
		return ParsedDesignation{}, s.fail(MsgExpectProMaterial)
	}
	s.i++
	material, hasMaterial := ProMaterialByLetter(mat)

	// Вторая буква — класс прибора.
	class, has := s.peek()
	if !has || !isLatinUpper(class) {
		return ParsedDesignation{}, s.fail(MsgExpectProClassLetter)
	}
	s.i++
	kind, classKnown := proClassKinds[class]
	classUnsupported := !classKnown && runeIn(class, proUnsupportedClasses)
	if !classKnown && !classUnsupported {
		return ParsedDesignation{}, s.failAt(s.i-1, MsgExpectProClassTable)
	}

	// Номер: бытовая (100–999/9999) либо промышленная (буква + 10–99/999)
	// регистрация.
	var devNumber int
	if r, has := s.peek(); has && isDigitRune(r) {
		start := s.i
		run, n := s.digits()
		if n < 3 || n > 4 || run[0] == '0' {
			return ParsedDesignation{}, s.failToken(start, MsgExpectProNumber, run)
		}
		devNumber = mustAtoi(run)
	} else if r, has := s.peek(); has && isLatinUpper(r) {
		// Буква промышленной регистрации (без фиксированного значения).
		s.i++
		start := s.i
		run, n := s.digits()
		if n == 0 {
			return ParsedDesignation{}, s.fail(MsgExpectProConsumerNumber)
		}
		if n < 2 || n > 3 || run[0] == '0' {
			return ParsedDesignation{}, s.failToken(start, MsgExpectProConsumerNumber, run)
		}
		devNumber = mustAtoi(run)
	} else {
		return ParsedDesignation{}, s.fail(MsgExpectProAnyNumber)
	}

	// Версионные буквы (одна-две, без фиксированного значения).
	letters := ""
	for len(letters) < 2 {
		r, has := s.peek()
		if !has || !isLatinUpper(r) {
			break
		}
		letters += string(r)
		s.i++
	}

	// Суффикс-подклассификация: через дефис либо слитно (у стабилитронов
	// BZX85C5V1); распознаётся, в полях не раскладывается.
	if s.peekIs(s.i, '-') {
		s.i++
		if s.readAlnumRun(12) == 0 {
			return ParsedDesignation{}, s.fail(MsgExpectProSuffix)
		}
	} else if r, has := s.peek(); has && isDigitRune(r) {
		s.readAlnumRun(12)
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	// Класс прибора — после грамматики (см. комментарий пакета).
	if !hasMaterial || classUnsupported {
		return ParsedDesignation{}, KindNotSupported()
	}

	fields := []Field{
		TextField("material", material.Name),
		TextField("subclass", string(class)),
		NumField("dev_number", float64(devNumber)),
	}
	if letters != "" {
		fields = append(fields, TextField("letters", letters))
	}
	return ParsedDesignation{Kind: kind, System: SystemPro, Designation: canonical, Fields: fields}, nil
}

// runeIn сообщает, входит ли руна в строку.
func runeIn(r rune, s string) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
