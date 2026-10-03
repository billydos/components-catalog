package domain

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/billydos/components-catalog/internal/i18n"
)

// Field — поле разбора обозначения: единые имена для одинаковой семантики
// между системами (material, subclass, junctions, dev_number, letters,
// series, …) — план 03-data-model.md §1.2, §2; хранится в
// device_designation_fields как текстовое либо числовое значение.
type Field struct {
	Name string

	// Text — текстовое значение; Num и IsNum задают числовое:
	// поле либо текстовое, либо числовое, одновременно — никогда.
	Text  string
	Num   float64
	IsNum bool
}

// TextField создаёт текстовое поле разбора.
func TextField(name, text string) Field {
	return Field{Name: name, Text: text}
}

// NumField создаёт числовое поле разбора.
func NumField(name string, num float64) Field {
	return Field{Name: name, Num: num, IsNum: true}
}

// String возвращает значение поля в текстовом виде (для вывода и тестов).
func (f Field) String() string {
	if f.IsNum {
		return trimFloat(f.Num)
	}
	return f.Text
}

// trimFloat форматирует число без лишней точки: 315, а не 315.0.
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ParsedDesignation — результат разбора: класс, система, каноническая
// строка (точный ключ записи внутри класса) и поля разбора.
type ParsedDesignation struct {
	Kind        Kind
	System      System
	Designation string
	Fields      []Field
}

// FieldByName ищет поле разбора по коду.
func (p ParsedDesignation) FieldByName(name string) (Field, bool) {
	for _, f := range p.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// String — компактная сериализация полей разбора для вывода и
// golden-тестов: поле=значение; …
func (p ParsedDesignation) String() string {
	parts := make([]string, 0, len(p.Fields))
	for _, f := range p.Fields {
		parts = append(parts, f.Name+"="+f.String())
	}
	return strings.Join(parts, "; ")
}

// Реестр полей разбора по разрядам: текстовые и числовые. Единый источник
// для фильтров и сортировки поиска (REST/CLI) — транспорты собственных
// списков полей не ведут: новое поле парсера попадает в реестр и сразу
// доступно фильтрам. Состав синхронен полям, которые создают парсеры
// строгих систем и series (закреплён тестом).
var (
	textDesignationFields = map[string]bool{
		"material": true, "subclass": true, "letters": true,
		"prefix": true, "family": true, "series": true,
	}
	numericDesignationFields = map[string]bool{
		"assembly": true, "feature": true, "dev_number": true,
		"modification": true, "chip": true, "junctions": true,
		"group": true, "power": true,
	}
)

// KnownDesignationField сообщает, существует ли поле разбора с кодом name.
func KnownDesignationField(name string) bool {
	return textDesignationFields[name] || numericDesignationFields[name]
}

// NumericDesignationField возвращает разряд поля разбора: числовое либо
// текстовое (второе значение — существование поля; числовые поля допускают
// eq-фильтр и числовую сортировку).
func NumericDesignationField(name string) (numeric, known bool) {
	if numericDesignationFields[name] {
		return true, true
	}
	if textDesignationFields[name] {
		return false, true
	}
	return false, false
}

// scanner — по-позиционный разбор канонической строки; все сообщения
// об ошибках несут позицию (1-based) и форму «ожидалось/получено» —
// дословный контракт (план 03 §2.4). Каноническая строка кэшируется,
// чтобы построение отброшенных ошибок автодетекта не перекодировала руны.
type scanner struct {
	src       []rune
	i         int
	canonical string
}

func newScanner(canonical string) *scanner {
	return &scanner{src: []rune(canonical), canonical: canonical}
}

// newScannerRunes создаёт сканер над уже построенными рунами канонической
// строки — без повторного декодирования на горячем пути автодетекта.
func newScannerRunes(src []rune, canonical string) *scanner {
	return &scanner{src: src, canonical: canonical}
}

func (s *scanner) atEnd() bool {
	return s.i >= len(s.src)
}

func (s *scanner) peek() (rune, bool) {
	if s.atEnd() {
		return 0, false
	}
	return s.src[s.i], true
}

// peekAt заглядывает в произвольную позицию (без сдвига).
func (s *scanner) peekAt(i int) (rune, bool) {
	if i < 0 || i >= len(s.src) {
		return 0, false
	}
	return s.src[i], true
}

// peekIs сравнивает символ в позиции с ожидаемым.
func (s *scanner) peekIs(i int, want rune) bool {
	r, ok := s.peekAt(i)
	return ok && r == want
}

// fail строит по-позиционную ошибку «ожидалось/получено» в текущей позиции;
// expected — MsgID фрагмента грамматики (expect_*), локализуемый при рендере.
func (s *scanner) fail(expected MsgID) *Error {
	return s.failAt(s.i, expected)
}

// failAt строит по-позиционную ошибку «ожидалось/получено» в заданной позиции.
func (s *scanner) failAt(pos int, expected MsgID) *Error {
	arg := i18n.Arg(string(expected))
	if r, ok := s.peekAt(pos); ok {
		return NewErrorf(CodeInvalidDesignation, MsgScannerExpected,
			s.text(), pos+1, arg, string(r))
	}
	return NewErrorf(CodeInvalidDesignation, MsgScannerUnexpected,
		s.text(), pos+1, arg)
}

// failToken — ошибка значения токена (вне позиции одного символа):
// «ожидалось: …, получено «токен»».
func (s *scanner) failToken(pos int, expected MsgID, token string) *Error {
	return NewErrorf(CodeInvalidDesignation, MsgScannerToken,
		s.text(), pos+1, i18n.Arg(string(expected)), token)
}

func (s *scanner) text() string {
	return s.canonical
}

// eofErr — ошибка непотреблённого хвоста: «ожидался конец обозначения».
func (s *scanner) eofErr() *Error {
	return NewErrorf(CodeInvalidDesignation, MsgScannerEof,
		s.text(), s.i+1, string(s.src[s.i]))
}

func isDigitRune(r rune) bool {
	return r >= '0' && r <= '9'
}

// mustAtoi — atoi для проверенной непустой последовательности цифр.
func mustAtoi(digits string) int {
	v, _ := strconv.Atoi(digits)
	return v
}

func isLatinUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

// isCyrillicUpper — прописная буква русского алфавита (А–Я, Ё).
// Именно русский алфавит: блок Unicode «кириллица» шире (болгарская,
// македонская, расширенная) — посторонние прописные должны отвергаться
// канонизацией как недопустимые символы, а не попадать в ключи записей.
func isCyrillicUpper(r rune) bool {
	return (r >= 'А' && r <= 'Я') || r == 'Ё'
}

// digits — максимальная последовательность цифр с текущей позиции.
func (s *scanner) digits() (string, int) {
	start := s.i
	for s.i < len(s.src) && isDigitRune(s.src[s.i]) {
		s.i++
	}
	return string(s.src[start:s.i]), s.i - start
}

// readAlnumRun потребляет последовательность [0–9A-Z] до max символов;
// возвращает число потреблённых символов.
func (s *scanner) readAlnumRun(max int) int {
	count := 0
	for count < max {
		r, has := s.peek()
		if !has || !(isDigitRune(r) || isLatinUpper(r)) {
			break
		}
		s.i++
		count++
	}
	return count
}

// readCyrillicLetters потребляет до max прописных русских букв.
func (s *scanner) readCyrillicLetters(max int) string {
	var b strings.Builder
	count := 0
	for count < max {
		r, has := s.peek()
		if !has || !isCyrillicUpper(r) {
			break
		}
		b.WriteRune(r)
		s.i++
		count++
	}
	return b.String()
}

// typographicHyphens — дефисы типографики, приводимые к «-»
// (план 03 §2.4: нормализация на входе).
var typographicHyphens = map[rune]struct{}{
	'‐': {}, // U+2010 hyphen
	'‑': {}, // U+2011 non-breaking hyphen
	'‒': {}, // U+2012 figure dash
	'–': {}, // U+2013 en dash
	'—': {}, // U+2014 em dash
	'―': {}, // U+2015 horizontal bar
	'−': {}, // U+2212 minus sign
	'﹘': {}, // U+FE58 small em dash
	'﹣': {}, // U+FE63 small hyphen-minus
	'－': {}, // U+FF0D fullwidth hyphen-minus
}

// Canonicalize нормализует обозначение на входе (план 03 §2.4):
// trim, верхний регистр, удаление пробелов, типографские дефисы → «-»,
// запятая → точка (десятичный разделитель хвостов-мощностей).
// Кириллица и латиница не смешиваются в одном обозначении — смешение
// алфавитов диагностируется ошибкой с указанием позиции.
func Canonicalize(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", NewErrorf(CodeInvalidDesignation, MsgEmptyDesignation)
	}
	upper := []rune(strings.ToUpper(trimmed))
	var b strings.Builder
	pos := 0            // позиция руны в канонической строке (0-based)
	alphabet := byte(0) // 0 — не определён; 1 — кириллица; 2 — латиница
	letters := false
	for _, r := range upper {
		if unicode.IsSpace(r) {
			continue
		}
		if _, ok := typographicHyphens[r]; ok {
			r = '-'
		}
		if r == ',' {
			r = '.'
		}
		switch {
		case isCyrillicUpper(r):
			letters = true
			if alphabet == 0 {
				alphabet = 1
			} else if alphabet == 2 {
				return "", NewErrorf(CodeInvalidDesignation, MsgCanonicalAlphabetMix,
					string(upper), pos+1, string(r))
			}
		case isLatinUpper(r):
			letters = true
			if alphabet == 0 {
				alphabet = 2
			} else if alphabet == 1 {
				return "", NewErrorf(CodeInvalidDesignation, MsgCanonicalAlphabetMix,
					string(upper), pos+1, string(r))
			}
		case isDigitRune(r), r == '-', r == '.', r == '/':
			// нейтральные символы
		default:
			return "", NewErrorf(CodeInvalidDesignation, MsgCanonicalBadRune,
				string(upper), pos+1, string(r))
		}
		b.WriteRune(r)
		pos++
	}
	if !letters {
		return "", NewErrorf(CodeInvalidDesignation, MsgCanonicalNoLetters, string(upper))
	}
	return b.String(), nil
}

// ParseDesignation разбирает обозначение с автодетектом класса и системы
// (порядок попыток — план 03 §2.4): полупроводник gost → pro → jedec → jis →
// резистор gost/ost → конденсатор gost → series (класс — из реестра семейств).
// Система other автодетекта не имеет — требует явного указания.
func ParseDesignation(text string) (ParsedDesignation, error) {
	return parseDesignation(text, "", "")
}

// ParseDesignationForSystem разбирает обозначение с явной системой
// (и опционально классом — переопределяет автодетект, обязателен для other).
// Пустая система — автодетект; несоответствие обозначения системе —
// ошибка designation_mismatch.
func ParseDesignationForSystem(text string, system System, kind Kind) (ParsedDesignation, error) {
	return parseDesignation(text, system, kind)
}

// strictParser — парсер строгой системы: получает сканер над каноническими
// рунами (одно декодирование строки на весь автодетект).
type strictParser func(s *scanner, kind Kind) (ParsedDesignation, error)

// parseDesignation — единая точка разбора: канонизация, явная система
// либо автодетект; явный класс проверяется против результата.
func parseDesignation(text string, system System, kind Kind) (ParsedDesignation, error) {
	if kind != "" && !kind.IsValid() {
		return ParsedDesignation{}, fmt.Errorf("unknown device kind %q", string(kind))
	}
	canonical, err := Canonicalize(text)
	if err != nil {
		return ParsedDesignation{}, err
	}
	runes := []rune(canonical)
	scan := func() *scanner { return newScannerRunes(runes, canonical) }

	if system == "" {
		p, err := autodetect(scan, canonical, kind)
		if err != nil {
			return ParsedDesignation{}, err
		}
		if kind != "" && p.Kind != kind {
			return ParsedDesignation{}, kindMismatch(canonical, p.Kind, kind)
		}
		return p, nil
	}
	p, err := parseWithSystem(scan, canonical, system, kind)
	if err != nil {
		return ParsedDesignation{}, err
	}
	if kind != "" && p.Kind != kind {
		return ParsedDesignation{}, kindMismatch(canonical, p.Kind, kind)
	}
	return p, nil
}

// kindMismatch — явный класс противоречит классу обозначения.
func kindMismatch(canonical string, got, want Kind) *Error {
	return NewErrorf(CodeDesignationMismatch, MsgKindMismatch,
		canonical, string(got), string(want))
}

// systemMismatch — обозначение синтаксически не соответствует системе.
func systemMismatch(canonical string, system System) *Error {
	return NewErrorf(CodeDesignationMismatch, MsgSystemMismatch,
		canonical, string(system))
}

// isVerdictError — семантическая ошибка разбора: обозначение грамматически
// принадлежит опознанной системе, но его класс вне модуля
// (kind_not_supported), класс неоднозначен (kind_ambiguous), нарушена
// политика хвоста мощности (power_suffix_not_supported — подсказка важнее
// перебора следующих систем) либо класс противоречит явному указанию
// (designation_mismatch). Такая ошибка — вердикт: она сохраняет код и текст
// при любом способе разбора (автодетект и явная система).
func isVerdictError(err error) bool {
	de, ok := AsError(err)
	if !ok {
		return false
	}
	switch de.Code {
	case CodeKindNotSupported, CodeKindAmbiguous, CodePowerSuffix, CodeDesignationMismatch:
		return true
	}
	return false
}

// parseWithSystem — разбор в явно заданной системе; синтаксическое
// несоответствие — designation_mismatch, вердикты сохраняют свой код.
func parseWithSystem(scan func() *scanner, canonical string, system System, kind Kind) (ParsedDesignation, error) {
	mismatch := func(err error) (ParsedDesignation, error) {
		if isVerdictError(err) {
			return ParsedDesignation{}, err
		}
		return ParsedDesignation{}, systemMismatch(canonical, system)
	}
	switch system {
	case SystemGost:
		// gost — три предметные грамматики; выбор по классу либо перебор
		// в порядке автодетекта.
		run := func(parse strictParser) (ParsedDesignation, error) {
			p, err := parse(scan(), kind)
			if err != nil {
				return mismatch(err)
			}
			return p, nil
		}
		switch kind {
		case KindTransistor, KindDiode:
			return run(parseGostSemiconductor)
		case KindResistor:
			return run(parseResistorGost)
		case KindCapacitor:
			return run(parseCapacitorGost)
		}
		for _, parse := range []strictParser{
			parseGostSemiconductor, parseResistorGost, parseCapacitorGost,
		} {
			p, err := parse(scan(), kind)
			if err == nil {
				return p, nil
			}
			if isVerdictError(err) {
				return ParsedDesignation{}, err
			}
		}
		return ParsedDesignation{}, systemMismatch(canonical, SystemGost)
	case SystemOst:
		return runStrict(scan, kind, parseResistorOst, mismatch)
	case SystemPro:
		return runStrict(scan, kind, parsePro, mismatch)
	case SystemJedec:
		return runStrict(scan, kind, parseJedec, mismatch)
	case SystemJis:
		return runStrict(scan, kind, parseJis, mismatch)
	case SystemSeries:
		// Семейство опознано или нет — в обоих случаях серия даёт
		// собственную диагностику: неизвестное семейство с перечнем
		// поддерживаемых, конфликт класса, ошибка хвоста.
		return parseSeries(canonical, kind)
	case SystemOther:
		if kind == "" {
			return ParsedDesignation{}, KindAmbiguous()
		}
		return ParsedDesignation{Kind: kind, System: SystemOther, Designation: canonical}, nil
	}
	return ParsedDesignation{}, fmt.Errorf("unknown designation system %q", string(system))
}

// runStrict прогоняет парсер явной системы, преобразуя синтаксические
// ошибки в designation_mismatch (вердикты сохраняют код).
func runStrict(scan func() *scanner, kind Kind, parse strictParser,
	mismatch func(error) (ParsedDesignation, error)) (ParsedDesignation, error) {
	p, err := parse(scan(), kind)
	if err != nil {
		return mismatch(err)
	}
	return p, nil
}

// autodetect — порядок попыток фиксирован планом 03 §2.4: строгие системы
// в порядке плана, затем series. Вердикты прерывают перебор; ошибка
// опознанного семейства (включая по-позиционные ошибки хвоста) всплывает
// как есть, а не затирается итоговым отказом.
func autodetect(scan func() *scanner, canonical string, kind Kind) (ParsedDesignation, error) {
	for _, parse := range []strictParser{
		parseGostSemiconductor, parsePro, parseJedec, parseJis,
		parseResistorGost, parseResistorOst, parseCapacitorGost,
	} {
		p, err := parse(scan(), kind)
		if err == nil {
			return p, nil
		}
		if isVerdictError(err) {
			return ParsedDesignation{}, err
		}
	}
	if _, ok := matchSeriesFamily(canonical); ok {
		// Префикс семейства опознан: parseSeries даёт либо результат,
		// либо вердикт/по-позиционную ошибку — все сохраняются.
		return parseSeries(canonical, kind)
	}
	return ParsedDesignation{}, NewErrorf(CodeInvalidDesignation, MsgAutodetectFailed, canonical)
}
