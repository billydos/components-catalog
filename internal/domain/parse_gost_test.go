package domain

import (
	"errors"
	"slices"
	"testing"
)

// Охрана вердиктов: семантические ошибки разбора сохраняют код и в
// автодетекте, и при явной системе; прочие ошибки не прерывают перебор.
func TestVerdictErrorGuard(t *testing.T) {
	if isVerdictError(errors.New("прочая")) {
		t.Error("прочая ошибка — не вердикт")
	}
	if isVerdictError(NewErrorf(CodeInvalidDesignation, MsgScannerEof)) {
		t.Error("ошибка разбора — не вердикт")
	}
	for _, err := range []error{
		KindNotSupported(),
		KindAmbiguous(),
		NewErrorf(CodePowerSuffix, MsgPowerSuffix),
		NewErrorf(CodeDesignationMismatch, MsgKindMismatch),
	} {
		if !isVerdictError(err) {
			t.Errorf("ожидался вердикт: %v", err)
		}
	}
}

// Таблицы тестов парсера gost (полупроводники): валид — значения полей,
// невалид — дословные по-позиционные сообщения (контракт, 03 §2.4).
func TestParseGostSemiconductorValid(t *testing.T) {
	cases := []struct {
		input  string
		kind   Kind
		fields string
	}{
		{"КТ315Б", KindTransistor, "material=si; subclass=Т; assembly=0; dev_number=315; letters=Б"},
		{"ГТ109Г", KindTransistor, "material=ge; subclass=Т; assembly=0; dev_number=109; letters=Г"},
		{"2Т914А-1", KindTransistor, "material=si; subclass=Т; assembly=0; feature=9; dev_number=14; letters=А"},
		{"КТ3102", KindTransistor, "material=si; subclass=Т; assembly=0; feature=3; dev_number=102"},
		{"КТ31", KindTransistor, "material=si; subclass=Т; assembly=0; feature=3; dev_number=1"},
		{"2Т312", KindTransistor, "material=si; subclass=Т; assembly=0; feature=3; dev_number=12"},
		// Раздельность КТ312/2Т312: буква — трёхзначный номер (старая
		// форма), цифра — признак и номер (современная).
		{"КТ312", KindTransistor, "material=si; subclass=Т; assembly=0; dev_number=312"},
		{"2Т3148А", KindTransistor, "material=si; subclass=Т; assembly=0; feature=3; dev_number=148; letters=А"},
		{"2П798Г/ИФ", KindTransistor, "material=si; subclass=П; assembly=0; feature=7; dev_number=98; letters=Г"},
		{"КТД735В", KindTransistor, "material=si; subclass=Т; assembly=0; feature=7; dev_number=35; letters=В"},
		{"АП362А9", KindTransistor, "material=ga; subclass=П; assembly=0; dev_number=362; letters=А; chip=9"},
		{"КТ315А5", KindTransistor, "material=si; subclass=Т; assembly=0; dev_number=315; letters=А; modification=5"},
		{"КП303А", KindTransistor, "material=si; subclass=П; assembly=0; dev_number=303; letters=А"},
		{"КДС111В", KindDiode, "material=si; subclass=Д; assembly=1; dev_number=111; letters=В"},
		{"КСС393А", KindDiode, "material=si; subclass=С; assembly=1; dev_number=393; letters=А"},
		{"КС168А", KindDiode, "material=si; subclass=С; assembly=0; dev_number=168; letters=А"},
		{"2Д627АС", KindDiode, "material=si; subclass=Д; assembly=1; feature=6; dev_number=27; letters=А"},
		{"КДШ289А", KindDiode, "material=si; subclass=Д; assembly=0; feature=2; dev_number=89; letters=А"},
		{"АЛ307Б", KindDiode, "material=ga; subclass=Л; assembly=0; dev_number=307; letters=Б"},
		{"2В131А", KindDiode, "material=si; subclass=В; assembly=0; feature=1; dev_number=31; letters=А"},
		{"3А538А", KindDiode, "material=ga; subclass=А; assembly=0; feature=5; dev_number=38; letters=А"},
		{"ГТ402В", KindTransistor, "material=ge; subclass=Т; assembly=0; dev_number=402; letters=В"},
		{"АИ101А", KindDiode, "material=ga; subclass=И; assembly=0; dev_number=101; letters=А"},
		{"КВ102А", KindDiode, "material=si; subclass=В; assembly=0; dev_number=102; letters=А"},
		{"КГ508А", KindDiode, "material=si; subclass=Г; assembly=0; dev_number=508; letters=А"},
		{"КЦ405А", KindDiode, "material=si; subclass=Ц; assembly=0; dev_number=405; letters=А"},
		// Подобранная группа Р после буквы группы — часть ключа, поля не меняет.
		{"КТ805АР", KindTransistor, "material=si; subclass=Т; assembly=0; dev_number=805; letters=А"},
	}
	for _, tc := range cases {
		p, err := parseGostSemiconductor(newScanner(tc.input), "")
		if err != nil {
			t.Errorf("«%s»: неожиданная ошибка: %v", tc.input, err)
			continue
		}
		if p.Kind != tc.kind {
			t.Errorf("«%s»: класс %s, ожидался %s", tc.input, p.Kind, tc.kind)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»: поля:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
		if p.System != SystemGost || p.Designation != tc.input {
			t.Errorf("«%s»: system/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseGostSemiconductorMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"КТ", "designation «КТ»: position 3: expected: development number, got end of designation"},
		{"КТ3", "designation «КТ3»: position 3: expected: development number 101–999, or a feature digit and number 1–99, got «3»"},
		{"КТ3100", "designation «КТ3100»: position 4: expected: development number 1–99 or 101–999, got «100»"},
		{"КТ0215", "designation «КТ0215»: position 3: expected: feature digit 1–9, got «0»"},
		{"КТ3015", "designation «КТ3015»: position 4: expected: development number 1–99 or 101–999 without a leading zero, got «015»"},
		{"КТ015", "designation «КТ015»: position 3: expected: development number 1–99 or 101–999 without a leading zero, got «015»"},
		{"К", "designation «К»: position 2: expected: subclass letter (Т, П, Д, С, В, А, И, Г, Л, Ф, Ц, Н, У, Е, Р, Ж, Э, Х, М, УП, or О with a function letter), got end of designation"},
		{"АОД", "designation «АОД»: position 4: expected: development number, got end of designation"},
		{"КОФ2111", "designation «КОФ2111»: position 4: expected: development number 1–99 or 101–999, got «2111»"},
		{"2Т31485", "designation «2Т31485»: position 4: expected: development number 1–99 or 101–999, got «1485»"},
		{"КТ30", "designation «КТ30»: position 4: expected: development number 1–9, got «0»"},
		{"КТ05", "designation «КТ05»: position 3: expected: feature digit 1–9, got «0»"},
		{"КТ315А0", "designation «КТ315А0»: position 7: expected: modification digit 1–8 or 9 (surface mount), got «0»"},
		{"КТ31485", "designation «КТ31485»: position 3: expected: feature and development number (at most four digits), got «31485»"},
		{"2Т3", "designation «2Т3»: position 4: expected: feature digit (1–9) and development number, got end of designation"},
		{"2Т014", "designation «2Т014»: position 3: expected: feature digit 1–9, got «0»"},
		{"ХТ315Б", "designation «ХТ315Б»: position 1: expected: material symbol (Г, К, А, И, Д, П, or digit 1–6), got «Х»"},
		{"КЯ123", "designation «КЯ123»: position 2: expected: subclass letter (Т, П, Д, С, В, А, И, Г, Л, Ф, Ц, Н, У, Е, Р, Ж, Э, Х, М, УП, or О with a function letter), got «Я»"},
		{"КОХ123", "designation «КОХ123»: position 3: expected: optoelectronic device function letter (И, Ф, Д, Т, Р, У, К, Л, М, П), got «Х»"},
		{"КТ315З", "designation «КТ315З»: position 6: expected end of designation, got «З»"},
		{"КТ315Б-7", "designation «КТ315Б-7»: position 8: expected: chip (leadless) variant digit 1–6, got «7»"},
		{"КТ315Б/9", "designation «КТ315Б/9»: position 8: expected: manufacturer code (letters), got «9»"},
	}
	for _, tc := range cases {
		_, err := parseGostSemiconductor(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// Подклассы вне стартовых классов и неоднозначный Ф (03 §2.4).
func TestParseGostSemiconductorKind(t *testing.T) {
	unsupported := []string{
		"КУ101", "КУП101", "КН102", "КЕ716", "КР302А", "КЖ101", "КЭ101", "КХ101",
		"КМД12", "АОД158А", "АОИ113А", "КОФ211А",
	}
	for _, in := range unsupported {
		_, err := parseGostSemiconductor(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}

	_, err := parseGostSemiconductor(newScanner("КФ102А"), "")
	if de, ok := AsError(err); !ok || de.Code != CodeKindAmbiguous {
		t.Fatalf("«КФ102А»: ожидался kind_ambiguous, получено %v", err)
	}
	// Явный класс переопределяет неоднозначность Ф.
	for _, kind := range []Kind{KindDiode, KindTransistor} {
		p, err := parseGostSemiconductor(newScanner("КФ102А"), kind)
		if err != nil || p.Kind != kind {
			t.Errorf("«КФ102А» с классом %s: %v", kind, err)
		}
	}
	if _, err := parseGostSemiconductor(newScanner("КФ102А"), KindResistor); err == nil {
		t.Error("«КФ102А» с классом resistor: ожидалась ошибка")
	}
}

// Ведущий ноль номера в современных формах: КР302А = признак 3, номер 02
// (пример ГОСТ Р 71055-2023, п. 5.9); Р — класс вне модуля, проверяем
// грамматику через 2Д602А (признак 6, номер 02).
func TestParseGostSemiconductorLeadingZero(t *testing.T) {
	p, err := parseGostSemiconductor(newScanner("2Д602А"), "")
	if err != nil {
		t.Fatalf("«2Д602А»: %v", err)
	}
	want := "material=si; subclass=Д; assembly=0; feature=6; dev_number=2; letters=А"
	if got := p.String(); got != want {
		t.Fatalf("«2Д602А»:\n got:  %s\n want: %s", got, want)
	}
}

func TestGostEquivalentSymbols(t *testing.T) {
	pairs := map[rune]rune{'Г': '1', 'К': '2', 'А': '3', 'И': '4', 'Д': '5', 'П': '6'}
	for letter, digit := range pairs {
		if got, ok := GostEquivalentSymbol(letter); !ok || got != digit {
			t.Errorf("пара %c: получено %c", letter, got)
		}
		if got, ok := GostEquivalentSymbol(digit); !ok || got != letter {
			t.Errorf("пара %c: получено %c", digit, got)
		}
	}
	if _, ok := GostEquivalentSymbol('Т'); ok {
		t.Error("Т не является символом материала")
	}
}

func TestGostSubclassCoverage(t *testing.T) {
	// Все однобуквенные подклассы реестра покрыты классификацией.
	want := []rune{'А', 'В', 'Г', 'Д', 'Е', 'Ж', 'И', 'Л', 'М', 'Н', 'П', 'Р', 'С', 'Т', 'У', 'Ф', 'Х', 'Ц', 'Э'}
	got := make([]rune, 0, len(gostSubclasses))
	for r := range gostSubclasses {
		got = append(got, r)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("таблица подклассов изменилась: %c", got)
	}
}
