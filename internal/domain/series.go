package domain

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Система series — слабые системы обозначений: семейство из реестра
// series_families (советские досистемные семейства и мировые дом-номера
// производителей) плюс единая слабая грамматика хвоста (план 03 §2.1–2.3;
// стартовые перечни выверены — 07 §7).
//
// Инвариант реестра (03 §2.4): в реестр включаются только семейства,
// обозначения которых не разбираются строгими системами (порядок
// «строгие грамматики → series» фиксирован; пример: ASZ15 корректно
// разбирается как PRO ELECTRON и в реестр не входит). Инвариант проверяется
// тестом по контрольным примерам семейств (series_test.go).
//
// Power (tail_semantic = power) — хвост через дефис есть номинальная
// мощность, Вт (МЛТ-0.5, ПЭВ-10); без него — общий слабый разбор хвоста
// (числа и буквы). Реестр — данные: расширение без правки кода.

// SeriesFamily — запись реестра семейств (синхронизируется с сидами
// series_families на этапе 2).
type SeriesFamily struct {
	Series string // код семейства (канонический, верхний регистр)
	Kind   Kind   // класс, который задаёт семейство при автодетекте
	Name   string // расшифровка

	// Power — tail_semantic = power: хвост-число есть мощность, Вт.
	Power bool

	// Examples — контрольные обозначения семейства: проверка инварианта
	// реестра (не разбираются строгими системами) и golden-тесты.
	Examples []string
}

// seriesFamilies — стартовый реестр (07 §7). Трёхбуквенные производные
// мил-рядов (RCR, RLR, RNC, RNR, RBR, RWR, RER) в стартовый реестр не
// входят: их написания без дефиса разбираются промышленной формой
// PRO ELECTRON (RCR20 = R+C+R20), что нарушает инвариант; базовые RC/RL/RN
// безопасны (двузначные номера PRO ELECTRON не принимает).
var seriesFamilies = []SeriesFamily{
	// Транзисторы: досистемные семейства и мировые дом-номера.
	{Series: "П", Kind: KindTransistor, Name: "германиевые транзисторы до ГОСТ 10862-64",
		Examples: []string{"П13", "П214", "П402А"}},
	{Series: "МП", Kind: KindTransistor, Name: "германиевые транзисторы до ГОСТ 10862-64",
		Examples: []string{"МП39", "МП41А", "МП42Б"}},
	{Series: "OC", Kind: KindTransistor, Name: "старая система Mullard–Philips",
		Examples: []string{"OC44", "OC71"}},
	{Series: "TIP", Kind: KindTransistor, Name: "дом-номер Texas Instruments",
		Examples: []string{"TIP120", "TIP121"}},
	{Series: "MPSA", Kind: KindTransistor, Name: "дом-номер Motorola",
		Examples: []string{"MPSA42", "MPSA92"}},

	// Диоды: досистемные семейства.
	{Series: "Д", Kind: KindDiode, Name: "диоды до ГОСТ 10862-64",
		Examples: []string{"Д2Б", "Д7А", "Д7Ж", "Д104", "Д226", "Д242", "Д808", "Д814А"}},
	{Series: "ДГ", Kind: KindDiode, Name: "точечные германиевые диоды",
		Examples: []string{"ДГ-Ц8"}},

	// Резисторы: советские досистемные семейства.
	{Series: "ВС", Kind: KindResistor, Name: "высокостабильные углеродистые", Power: true,
		Examples: []string{"ВС-0.125", "ВС-0.25", "ВС-0.5", "ВС-1", "ВС-2"}},
	{Series: "ВК", Kind: KindResistor, Name: "досистемное семейство", Power: true,
		Examples: []string{"ВК-2"}},
	{Series: "КИМ", Kind: KindResistor, Name: "композиционные изолированные малогабаритные", Power: true,
		Examples: []string{"КИМ-0.05", "КИМ-0.125"}},
	{Series: "МГТ", Kind: KindResistor, Name: "металлоплёночные герметизированные", Power: true,
		Examples: []string{"МГТ-0.5", "МГТ-1"}},
	{Series: "МЛТ", Kind: KindResistor, Name: "металлоплёночные лакированные теплостойкие", Power: true,
		Examples: []string{"МЛТ-0.125", "МЛТ-0.5", "МЛТ-1", "МЛТ-2"}},
	{Series: "МТ", Kind: KindResistor, Name: "металлоплёночные теплостойкие", Power: true,
		Examples: []string{"МТ-0.5", "МТ-1"}},
	{Series: "ОМЛТ", Kind: KindResistor, Name: "огнестойкие металлоплёночные", Power: true,
		Examples: []string{"ОМЛТ-0.5", "ОМЛТ-1"}},
	{Series: "ПЭ", Kind: KindResistor, Name: "проволочные эмалированные", Power: true,
		Examples: []string{"ПЭ-10", "ПЭ-25"}},
	{Series: "ПЭВ", Kind: KindResistor, Name: "проволочные эмалированные влагостойкие", Power: true,
		Examples: []string{"ПЭВ-5", "ПЭВ-10", "ПЭВ-25"}},
	{Series: "СПО", Kind: KindResistor, Name: "сопротивления переменные объёмные", Power: true,
		Examples: []string{"СПО-0.5", "СПО-1"}},
	{Series: "УЛИ", Kind: KindResistor, Name: "углеродистые лакированные измерительные", Power: true,
		Examples: []string{"УЛИ-0.5", "УЛИ-1"}},

	// Резисторы: мил-ряды и фирменные семейства.
	{Series: "RC", Kind: KindResistor, Name: "углеродистые композиционные (MIL-R-11/39008)",
		Examples: []string{"RC05", "RC20"}},
	{Series: "RB", Kind: KindResistor, Name: "проволочные прецизионные (MIL-R-93/39005)",
		Examples: []string{"RB52"}},
	{Series: "RL", Kind: KindResistor, Name: "металлоплёночные (MIL-R-22684/39017)",
		Examples: []string{"RL07", "RL20"}},
	{Series: "RN", Kind: KindResistor, Name: "металлоплёночные прецизионные (MIL-R-10509/55182)",
		Examples: []string{"RN55", "RN65"}},
	{Series: "RW", Kind: KindResistor, Name: "проволочные мощные (MIL-R-26)",
		Examples: []string{"RW67", "RW74"}},
	{Series: "M55342", Kind: KindResistor, Name: "чип (MIL-PRF-55342)",
		Examples: []string{"M55342K06B"}},
	{Series: "KNP", Kind: KindResistor, Name: "фирменное семейство (цементные)",
		Examples: []string{"KNP-100"}},
	{Series: "CFR", Kind: KindResistor, Name: "фирменное семейство (углеродистые)",
		Examples: []string{"CFR-25"}},

	// Конденсаторы: старые системы вне единой.
	{Series: "БМ", Kind: KindCapacitor, Name: "бумажные малогабаритные",
		Examples: []string{"БМ-2"}},
	{Series: "КБГИ", Kind: KindCapacitor, Name: "бумажные герметизированные изолированные"},
	{Series: "КД", Kind: KindCapacitor, Name: "керамические дисковые"},
	{Series: "КЛС", Kind: KindCapacitor, Name: "керамические литые секционные"},
	{Series: "КМ", Kind: KindCapacitor, Name: "керамические монолитные",
		Examples: []string{"КМ-4", "КМ-5", "КМ-6"}},
	{Series: "КПК", Kind: KindCapacitor, Name: "подстроечные керамические"},
	{Series: "КСО", Kind: KindCapacitor, Name: "слюдяные спрессованные",
		Examples: []string{"КСО-1", "КСО-2", "КСО-5"}},
	{Series: "КЭГ", Kind: KindCapacitor, Name: "электролитические герметизированные"},
	{Series: "МБГЧ", Kind: KindCapacitor, Name: "металлобумажные герметизированные частотные",
		Examples: []string{"МБГЧ-1", "МБГЧ-2"}},
	{Series: "МБГО", Kind: KindCapacitor, Name: "металлобумажные герметизированные"},
	{Series: "МБМ", Kind: KindCapacitor, Name: "металлобумажные малогабаритные",
		Examples: []string{"МБМ"}},
	{Series: "СГМ", Kind: KindCapacitor, Name: "слюдяные герметизированные малогабаритные"},
	{Series: "ЭМ", Kind: KindCapacitor, Name: "электролитические"},
	{Series: "ЭТО", Kind: KindCapacitor, Name: "электролитические танталовые объёмно-пористые",
		Examples: []string{"ЭТО-1", "ЭТО-2"}},
}

// seriesByLength — реестр по убыванию длины кода семейства: сопоставление
// по самому длинному совпадению префикса (ПЭВ раньше ПЭ, ОМЛТ раньше МЛТ).
var seriesByLength = func() []SeriesFamily {
	out := slices.Clone(seriesFamilies)
	slices.SortStableFunc(out, func(a, b SeriesFamily) int {
		return utf8.RuneCountInString(b.Series) - utf8.RuneCountInString(a.Series)
	})
	return out
}()

// seriesByCode — реестр, отсортированный по коду семейства; строится один
// раз при инициализации пакета: перечень поддерживаемых собирается в
// сообщениях об ошибках, в том числе на пути автодетекта (suggest,
// импорт), и не должен сортировать реестр на каждый вызов.
var seriesByCode = func() []SeriesFamily {
	out := slices.Clone(seriesFamilies)
	slices.SortFunc(out, func(a, b SeriesFamily) int {
		return strings.Compare(a.Series, b.Series)
	})
	return out
}()

// SeriesFamilies возвращает реестр семейств, отсортированный по коду.
func SeriesFamilies() []SeriesFamily {
	return slices.Clone(seriesByCode)
}

// SeriesFamiliesForKind возвращает семейства класса, отсортированные по коду.
func SeriesFamiliesForKind(k Kind) []SeriesFamily {
	var out []SeriesFamily
	for _, f := range seriesByCode {
		if f.Kind == k {
			out = append(out, f)
		}
	}
	return out
}

// matchSeriesFamily ищет семейство по префиксу обозначения (самое длинное
// совпадение, без фильтра классом: семейство однозначно определяет класс,
// а конфликт с явным классом разбирает parseSeries).
func matchSeriesFamily(canonical string) (SeriesFamily, bool) {
	for _, f := range seriesByLength {
		if strings.HasPrefix(canonical, f.Series) {
			return f, true
		}
	}
	return SeriesFamily{}, false
}

// parseSeries разбирает обозначения системы series: семейство задаёт класс
// (автодетект); конфликт с kindHint — designation_mismatch (симметрично
// строгим системам), неизвестное семейство — ошибка разбора с перечнем
// поддерживаемых (план 03 §2.1–2.2), ошибки хвоста — по-позиционные.
func parseSeries(canonical string, kindHint Kind) (ParsedDesignation, error) {
	family, ok := matchSeriesFamily(canonical)
	if !ok {
		return ParsedDesignation{}, seriesUnknownFamily(canonical, kindHint)
	}
	return parseSeriesFamily(canonical, &family, kindHint)
}

// ParseSeriesWithRegistry разбирает обозначение системы series над внешним
// реестром семейств — данными каталога (series_families расширяем без правки
// кода): стартовый реестр домена синхронизирован с сидами пин-тестом, каталог
// может быть расширен импортом. Сопоставление — по самому длинному префиксу
// (как у стартового реестра); грамматика хвоста — единая. Каноническая строка
// передаётся уже канонизированной (Canonicalize).
func ParseSeriesWithRegistry(canonical string, registry []SeriesFamily, kindHint Kind) (ParsedDesignation, error) {
	var best *SeriesFamily
	bestLen := 0
	for i := range registry {
		f := &registry[i]
		if !strings.HasPrefix(canonical, f.Series) {
			continue
		}
		if n := utf8.RuneCountInString(f.Series); n > bestLen {
			best, bestLen = f, n
		}
	}
	if best == nil {
		return ParsedDesignation{}, seriesUnknownFamily(canonical, kindHint)
	}
	return parseSeriesFamily(canonical, best, kindHint)
}

// parseSeriesFamily разбирает хвост обозначения в опознанном семействе.
func parseSeriesFamily(canonical string, family *SeriesFamily, kindHint Kind) (ParsedDesignation, error) {
	if kindHint != "" && family.Kind != kindHint {
		return ParsedDesignation{}, kindMismatch(canonical, family.Kind, kindHint)
	}
	s := newScanner(canonical)
	s.i = utf8.RuneCountInString(family.Series)

	fields := []Field{TextField("series", family.Series)}
	if family.Power {
		power, err := parseSeriesPowerTail(s)
		if err != nil {
			return ParsedDesignation{}, err
		}
		fields = append(fields, NumField("power", power))
	} else {
		devNumber, letters, err := parseSeriesWeakTail(s)
		if err != nil {
			return ParsedDesignation{}, err
		}
		if devNumber != nil {
			fields = append(fields, NumField("dev_number", float64(*devNumber)))
		}
		if letters != "" {
			fields = append(fields, TextField("letters", letters))
		}
	}
	return ParsedDesignation{Kind: family.Kind, System: SystemSeries, Designation: canonical, Fields: fields}, nil
}

// parseSeriesPowerTail разбирает хвост мощности: дефис и положительное
// число (МЛТ-0.5, ПЭВ-10); десятичная запятая нормализована канонизацией.
func parseSeriesPowerTail(s *scanner) (float64, error) {
	if !s.peekIs(s.i, '-') {
		return 0, s.fail("хвост мощности через дефис (например, -0.5)")
	}
	s.i++
	start := s.i
	run, _ := s.digits()
	if s.peekIs(s.i, '.') {
		s.i++
		frac, _ := s.digits()
		if frac == "" {
			return 0, s.failAt(s.i, "цифры дробной части мощности")
		}
		run += "." + frac
	}
	if run == "" {
		return 0, s.fail("положительное число номинальной мощности")
	}
	if !s.atEnd() {
		return 0, s.eofErr()
	}
	v, err := strconv.ParseFloat(run, 64)
	if err != nil || v <= 0 {
		return 0, s.failToken(start, "положительное число номинальной мощности", run)
	}
	return v, nil
}

// parseSeriesWeakTail разбирает слабый хвост: необязательный дефис и до трёх
// чередующихся групп — буквы (до 4), цифры (до 5), буквы (до 3):
// Д7А, ДГ-Ц8, КПК-МН, M55342K06B; числовая группа одна — это dev_number.
func parseSeriesWeakTail(s *scanner) (*int, string, error) {
	tailStart := s.i
	if s.peekIs(s.i, '-') {
		s.i++
	}
	readLetters := func(max int) string {
		var b strings.Builder
		count := 0
		for count < max {
			r, has := s.peek()
			if !has || !(isCyrillicUpper(r) || isLatinUpper(r)) {
				break
			}
			b.WriteRune(r)
			s.i++
			count++
		}
		return b.String()
	}

	letters := readLetters(4)
	start := s.i
	s.digits()
	digits := string(s.src[start:s.i])
	if len(digits) > 5 {
		return nil, "", s.failToken(start, "число в хвосте семейства (до 5 цифр)", digits)
	}
	letters2 := readLetters(3)

	if !s.atEnd() {
		return nil, "", s.fail("хвост семейства (число и буквы)")
	}
	if digits == "" && letters == "" && letters2 == "" && s.i > tailStart {
		// Хвост из одного дефиса без содержимого.
		return nil, "", s.fail("хвост семейства (число и буквы)")
	}
	var number *int
	if digits != "" {
		v := mustAtoi(digits)
		number = &v
	}
	return number, letters + letters2, nil
}

// seriesUnknownFamily — ошибка неизвестного семейства с перечнем
// поддерживаемых (класса — при явном классе).
func seriesUnknownFamily(canonical string, kindHint Kind) *Error {
	known := seriesByCode
	scope := "все классы"
	if kindHint != "" {
		known = SeriesFamiliesForKind(kindHint)
		scope = string(kindHint)
	}
	codes := make([]string, 0, len(known))
	for _, f := range known {
		codes = append(codes, f.Series)
	}
	run := []rune(canonical)
	end := 0
	for end < len(run) && (isCyrillicUpper(run[end]) || isLatinUpper(run[end])) {
		end++
	}
	return NewError(CodeInvalidDesignation, fmt.Sprintf(
		"обозначение «%s»: неизвестное семейство «%s»; поддерживаемые семейства (%s): %s",
		canonical, string(run[:end]), scope, strings.Join(codes, ", ")))
}
