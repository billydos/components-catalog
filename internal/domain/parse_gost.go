package domain

import "strings"

// Грамматика полупроводниковой системы gost — ГОСТ 10862-64 и преемники
// (ОСТ 11.336.038-77/919-81, ГОСТ Р 71055-2023) как одна система
// (план 03 §2.1; выверка по тексту ГОСТ Р 71055-2023 — 07 §1):
//
//	<материал><подкласс>[уточнение Д|Ш][сборка С]<признак><номер разработки>
//	[буква группы][сборка С][подобранные группы][модернизация/9][-бескорпусное][/код]
//
// Материал: Г/К/А/И/Д/П (общегражданское применение) или 1–6 (категории
// качества ВП/ОС/ОСМ). Признак (цифра 1–9) обязателен в современных
// обозначениях: материал-цифра, современные подклассы (Е, Ж, Э, Х, М, Р,
// УП, оптоэлектронные) и уточнения Дарлингтона/Шоттки; в ГОСТ 10862-64
// признак отсутствует — номер трёхзначный (101–999). Номер разработки —
// 1–99 либо 101–999 (стандарт допускает ведущий ноль: КР302А = признак 3,
// номер 02). Буква группы — прописная русская, кроме З О Ч Ы Ш Щ Э Ю Я Ь Ъ.
// Хвост (порядок фиксирован): сборка С — после подкласса в старых
// обозначениях (КДС111В) и после буквы группы в современных (2Д627АС);
// подобранные группы Р/Т/Г/К/Н; цифра 1–8 — модернизация, 9 — корпус для
// поверхностного монтажа; -1…-6 — бескорпусное исполнение; /ИФ — код
// предприятия-изготовителя воспроизводимого прибора.

// gostSubclassState — классификация подкласса по классам модуля.
type gostSubclassState int

const (
	gostSubclassFixed       gostSubclassState = iota // класс определён подклассом
	gostSubclassAmbiguous                            // Ф: фотодиод или фототранзистор
	gostSubclassUnsupported                          // класс вне стартовых (тиристоры, модули, …)
)

type gostSubclass struct {
	state gostSubclassState
	kind  Kind // для state == gostSubclassFixed
}

// gostSubclasses — таблица однобуквенных подклассов (ГОСТ Р 71055-2023,
// таблица элемента 2, объединённая с историческим набором ГОСТ 10862-64).
var gostSubclasses = map[rune]gostSubclass{
	'Т': {gostSubclassFixed, KindTransistor},
	'П': {gostSubclassFixed, KindTransistor},
	'Д': {gostSubclassFixed, KindDiode},
	'С': {gostSubclassFixed, KindDiode},
	'В': {gostSubclassFixed, KindDiode},
	'А': {gostSubclassFixed, KindDiode},
	'И': {gostSubclassFixed, KindDiode},
	'Г': {gostSubclassFixed, KindDiode},
	'Л': {gostSubclassFixed, KindDiode},
	'Ц': {gostSubclassFixed, KindDiode},
	'Ф': {state: gostSubclassAmbiguous},
	'Н': {state: gostSubclassUnsupported},
	'У': {state: gostSubclassUnsupported},
	'Е': {state: gostSubclassUnsupported},
	'Р': {state: gostSubclassUnsupported},
	'Ж': {state: gostSubclassUnsupported},
	'Э': {state: gostSubclassUnsupported},
	'Х': {state: gostSubclassUnsupported},
	'М': {state: gostSubclassUnsupported},
}

// gostModernSubclasses — подклассы, существующие только в современных
// стандартах: признак обязателен.
var gostModernSubclasses = map[rune]bool{
	'Е': true, 'Ж': true, 'Э': true, 'Х': true, 'Р': true, 'М': true,
}

// gostOptoFunctions — буквы функции оптоэлектронных приборов (элемент 3
// для подкласса О: излучатели, приёмники, оптопары, октроны, …).
const gostOptoFunctions = "ИФДТРУКЛМП"

// gostGroupLetterExcluded — буквы, не используемые в обозначении группы
// (ГОСТ Р 71055-2023, п. 4.5).
const gostGroupLetterExcluded = "ЗОЧЫШЩЭЮЯЬЪ"

// gostMatchedGroups — подобранные группы: пары/тройки/четвёрки/шестёрки/восьмёрки.
const gostMatchedGroups = "РТГКН"

func isGostGroupLetter(r rune) bool {
	return isCyrillicUpper(r) && !strings.ContainsRune(gostGroupLetterExcluded, r)
}

// gostReadSubclass читает элемент 2: двухбуквенные формы (УП; О с буквой
// функции; М с буквой состава модуля) имеют приоритет над однобуквенными.
func gostReadSubclass(s *scanner) (subclass string, modern, opto bool, class gostSubclass, err error) {
	const expectedSubclass = MsgExpectGostSubclass
	r, ok := s.peek()
	if !ok {
		return "", false, false, gostSubclass{}, s.fail(expectedSubclass)
	}
	switch {
	case r == 'О':
		fn, has := s.peekAt(s.i + 1)
		if !has || !strings.ContainsRune(gostOptoFunctions, fn) {
			return "", false, false, gostSubclass{}, s.failAt(s.i+1,
				MsgExpectGostOptoFunction)
		}
		s.i += 2
		return "О" + string(fn), true, true, gostSubclass{state: gostSubclassUnsupported}, nil
	case r == 'У' && s.peekIs(s.i+1, 'П'):
		s.i += 2
		return "УП", true, false, gostSubclass{state: gostSubclassUnsupported}, nil
	}
	info, known := gostSubclasses[r]
	if !known {
		return "", false, false, gostSubclass{}, s.fail(expectedSubclass)
	}
	s.i++
	if r == 'М' {
		// Модули: после М допускается буква состава комплектующих (4.13).
		if next, has := s.peek(); has && isCyrillicUpper(next) {
			s.i++
			return "М" + string(next), true, false, info, nil
		}
		return "М", true, false, info, nil
	}
	return string(r), gostModernSubclasses[r], false, info, nil
}

// checkGostDevNumber проверяет номер разработки: значение 1–99 либо
// 101–999; allowLeadingZero допускает написание с ведущим нулём (КР302А).
func checkGostDevNumber(s *scanner, digits string, allowLeadingZero bool, pos int) (int, error) {
	v := mustAtoi(digits)
	if !allowLeadingZero && len(digits) > 1 && digits[0] == '0' {
		return 0, s.failToken(pos, MsgExpectGostDevNumberFull, digits)
	}
	if (v >= 1 && v <= 99) || (v >= 101 && v <= 999) {
		return v, nil
	}
	return 0, s.failToken(pos, MsgExpectGostDevNumber, digits)
}

// parseGostSemiconductor разбирает полупроводниковые обозначения gost;
// kindHint переопределяет неоднозначный класс Ф (фотодиод/фототранзистор).
func parseGostSemiconductor(s *scanner, kindHint Kind) (ParsedDesignation, error) {
	canonical := s.text()

	// Элемент 1 — материал (пустая строка даёт ту же ошибку «конец
	// обозначения» — символ материала обязателен).
	mat, _ := s.peek()
	material, ok := GostMaterialBySymbol(mat)
	if !ok {
		return ParsedDesignation{}, s.fail(MsgExpectGostMaterial)
	}
	s.i++
	matIsDigit := isDigitRune(mat)

	// Элемент 2 — подкласс.
	subclass, modern, opto, class, err := gostReadSubclass(s)
	if err != nil {
		return ParsedDesignation{}, err
	}

	// Уточнения: Д — Дарлингтон (после Т, КТД735В), Ш — Шоттки (после Д,
	// КДШ289А); уточнение означает современную форму — признак обязателен.
	refined := false
	if r, has := s.peek(); has {
		if r == 'Д' && subclass == "Т" || r == 'Ш' && subclass == "Д" {
			refined = true
			s.i++
		}
	}

	// Сборка С после подкласса — старые обозначения (КДС111В, КСС393А).
	assembly := 0
	if s.peekIs(s.i, 'С') {
		if next, has := s.peekAt(s.i + 1); has && isDigitRune(next) {
			assembly = 1
			s.i++
		}
	}

	// Элементы 3–4 — признак и номер разработки.
	digitStart := s.i
	run, n := s.digits()
	var (
		feature    int
		hasFeature bool
		devNumber  int
	)
	featureRequired := matIsDigit || modern || refined
	switch {
	case opto:
		// Элемент 3 — буква функции (уже прочитана), номер следует напрямую.
		if n == 0 {
			return ParsedDesignation{}, s.fail(MsgExpectDevNumber)
		}
		devNumber, err = checkGostDevNumber(s, run, true, digitStart)
		if err != nil {
			return ParsedDesignation{}, err
		}
	case featureRequired:
		if n < 2 {
			return ParsedDesignation{}, s.fail(MsgExpectGostFeatureAndDev)
		}
		if run[0] == '0' {
			return ParsedDesignation{}, s.failToken(digitStart, MsgExpectGostFeatureDigit, run[:1])
		}
		feature = int(run[0] - '0')
		hasFeature = true
		devNumber, err = checkGostDevNumber(s, run[1:], true, digitStart+1)
		if err != nil {
			return ParsedDesignation{}, err
		}
	default:
		// Материал-буква, исторический подкласс, без уточнения: признак
		// опционален — три цифры читаются как номер (КТ315), четыре — как
		// признак и номер (КТ3102).
		switch n {
		case 0:
			return ParsedDesignation{}, s.fail(MsgExpectDevNumber)
		case 1:
			return ParsedDesignation{}, s.failToken(digitStart, MsgExpectGostDevOneDigit, run)
		case 2:
			if run[0] == '0' {
				return ParsedDesignation{}, s.failToken(digitStart, MsgExpectGostFeatureDigit, run[:1])
			}
			feature = int(run[0] - '0')
			hasFeature = true
			if run[1] == '0' {
				return ParsedDesignation{}, s.failToken(digitStart+1, MsgExpectGostDevOne, run[1:])
			}
			devNumber = int(run[1] - '0')
		case 3:
			devNumber, err = checkGostDevNumber(s, run, false, digitStart)
			if err != nil {
				return ParsedDesignation{}, err
			}
		case 4:
			if run[0] == '0' {
				return ParsedDesignation{}, s.failToken(digitStart, MsgExpectGostFeatureDigit, run[:1])
			}
			feature = int(run[0] - '0')
			hasFeature = true
			devNumber, err = checkGostDevNumber(s, run[1:], false, digitStart+1)
			if err != nil {
				return ParsedDesignation{}, err
			}
		default:
			return ParsedDesignation{}, s.failToken(digitStart, MsgExpectGostFeatureDevRun, run)
		}
	}

	// Элемент 5 — буква группы.
	letters := ""
	if r, has := s.peek(); has && isGostGroupLetter(r) {
		letters = string(r)
		s.i++
	}

	// Хвост: сборка С (современная позиция) → подобранные группы →
	// модернизация/9 (SMD) → бескорпусное → код изготовителя.
	if s.peekIs(s.i, 'С') && assembly == 0 {
		assembly = 1
		s.i++
	}
	if r, has := s.peek(); has && strings.ContainsRune(gostMatchedGroups, r) {
		s.i++
	}
	modification, chip := 0, 0
	if r, has := s.peek(); has && isDigitRune(r) {
		switch r {
		case '0':
			return ParsedDesignation{}, s.fail(MsgExpectGostModification)
		case '9':
			chip = 9
		default:
			modification = int(r - '0')
		}
		s.i++
	}
	if s.peekIs(s.i, '-') {
		next, has := s.peekAt(s.i + 1)
		if !has || next < '1' || next > '6' {
			return ParsedDesignation{}, s.failAt(s.i+1, MsgExpectGostChip)
		}
		s.i += 2
	}
	if s.peekIs(s.i, '/') {
		s.i++
		count := 0
		for count < 4 {
			r, has := s.peek()
			if !has || !(isCyrillicUpper(r) || isDigitRune(r)) {
				break
			}
			if count == 0 && isDigitRune(r) {
				break
			}
			s.i++
			count++
		}
		if count == 0 {
			return ParsedDesignation{}, s.fail(MsgExpectGostMakerCode)
		}
	}
	if !s.atEnd() {
		return ParsedDesignation{}, s.eofErr()
	}

	// Класс прибора по подклассу (план 03 §2.4).
	var kind Kind
	switch class.state {
	case gostSubclassFixed:
		kind = class.kind
	case gostSubclassAmbiguous:
		if kindHint == KindDiode || kindHint == KindTransistor {
			kind = kindHint
		} else {
			return ParsedDesignation{}, KindAmbiguous()
		}
	case gostSubclassUnsupported:
		return ParsedDesignation{}, KindNotSupported()
	}

	fields := []Field{
		TextField("material", material.Name),
		TextField("subclass", subclass),
		NumField("assembly", float64(assembly)),
	}
	if hasFeature {
		fields = append(fields, NumField("feature", float64(feature)))
	}
	fields = append(fields, NumField("dev_number", float64(devNumber)))
	if letters != "" {
		fields = append(fields, TextField("letters", letters))
	}
	if modification > 0 {
		fields = append(fields, NumField("modification", float64(modification)))
	}
	if chip > 0 {
		fields = append(fields, NumField("chip", float64(chip)))
	}
	return ParsedDesignation{Kind: kind, System: SystemGost, Designation: canonical, Fields: fields}, nil
}
