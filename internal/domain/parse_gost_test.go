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
	if isVerdictError(NewError(CodeInvalidDesignation, "x")) {
		t.Error("ошибка разбора — не вердикт")
	}
	for _, err := range []error{
		KindNotSupported(),
		KindAmbiguous(),
		NewError(CodePowerSuffix, "x"),
		NewError(CodeDesignationMismatch, "x"),
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
		{"КТ315Б", KindTransistor, "material=кремний; subclass=Т; assembly=0; dev_number=315; letters=Б"},
		{"ГТ109Г", KindTransistor, "material=германий; subclass=Т; assembly=0; dev_number=109; letters=Г"},
		{"2Т914А-1", KindTransistor, "material=кремний; subclass=Т; assembly=0; feature=9; dev_number=14; letters=А"},
		{"КТ3102", KindTransistor, "material=кремний; subclass=Т; assembly=0; feature=3; dev_number=102"},
		{"КТ31", KindTransistor, "material=кремний; subclass=Т; assembly=0; feature=3; dev_number=1"},
		{"2Т312", KindTransistor, "material=кремний; subclass=Т; assembly=0; feature=3; dev_number=12"},
		// Раздельность КТ312/2Т312: буква — трёхзначный номер (старая
		// форма), цифра — признак и номер (современная).
		{"КТ312", KindTransistor, "material=кремний; subclass=Т; assembly=0; dev_number=312"},
		{"2Т3148А", KindTransistor, "material=кремний; subclass=Т; assembly=0; feature=3; dev_number=148; letters=А"},
		{"2П798Г/ИФ", KindTransistor, "material=кремний; subclass=П; assembly=0; feature=7; dev_number=98; letters=Г"},
		{"КТД735В", KindTransistor, "material=кремний; subclass=Т; assembly=0; feature=7; dev_number=35; letters=В"},
		{"АП362А9", KindTransistor, "material=соединения галлия; subclass=П; assembly=0; dev_number=362; letters=А; chip=9"},
		{"КТ315А5", KindTransistor, "material=кремний; subclass=Т; assembly=0; dev_number=315; letters=А; modification=5"},
		{"КП303А", KindTransistor, "material=кремний; subclass=П; assembly=0; dev_number=303; letters=А"},
		{"КДС111В", KindDiode, "material=кремний; subclass=Д; assembly=1; dev_number=111; letters=В"},
		{"КСС393А", KindDiode, "material=кремний; subclass=С; assembly=1; dev_number=393; letters=А"},
		{"КС168А", KindDiode, "material=кремний; subclass=С; assembly=0; dev_number=168; letters=А"},
		{"2Д627АС", KindDiode, "material=кремний; subclass=Д; assembly=1; feature=6; dev_number=27; letters=А"},
		{"КДШ289А", KindDiode, "material=кремний; subclass=Д; assembly=0; feature=2; dev_number=89; letters=А"},
		{"АЛ307Б", KindDiode, "material=соединения галлия; subclass=Л; assembly=0; dev_number=307; letters=Б"},
		{"2В131А", KindDiode, "material=кремний; subclass=В; assembly=0; feature=1; dev_number=31; letters=А"},
		{"3А538А", KindDiode, "material=соединения галлия; subclass=А; assembly=0; feature=5; dev_number=38; letters=А"},
		{"ГТ402В", KindTransistor, "material=германий; subclass=Т; assembly=0; dev_number=402; letters=В"},
		{"АИ101А", KindDiode, "material=соединения галлия; subclass=И; assembly=0; dev_number=101; letters=А"},
		{"КВ102А", KindDiode, "material=кремний; subclass=В; assembly=0; dev_number=102; letters=А"},
		{"КГ508А", KindDiode, "material=кремний; subclass=Г; assembly=0; dev_number=508; letters=А"},
		{"КЦ405А", KindDiode, "material=кремний; subclass=Ц; assembly=0; dev_number=405; letters=А"},
		// Подобранная группа Р после буквы группы — часть ключа, поля не меняет.
		{"КТ805АР", KindTransistor, "material=кремний; subclass=Т; assembly=0; dev_number=805; letters=А"},
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
			t.Errorf("«%s»: система/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseGostSemiconductorMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"КТ", "обозначение «КТ»: позиция 3: ожидалось: номер разработки, получено конец обозначения"},
		{"КТ3", "обозначение «КТ3»: позиция 3: ожидалось: номер разработки 101–999 либо цифра признака и номер 1–99, получено «3»"},
		{"КТ3100", "обозначение «КТ3100»: позиция 4: ожидалось: номер разработки 1–99 или 101–999, получено «100»"},
		{"КТ0215", "обозначение «КТ0215»: позиция 3: ожидалось: цифра признака 1–9, получено «0»"},
		{"КТ3015", "обозначение «КТ3015»: позиция 4: ожидалось: номер разработки 1–99 или 101–999 без ведущего нуля, получено «015»"},
		{"КТ015", "обозначение «КТ015»: позиция 3: ожидалось: номер разработки 1–99 или 101–999 без ведущего нуля, получено «015»"},
		{"К", "обозначение «К»: позиция 2: ожидалось: буква подкласса (Т, П, Д, С, В, А, И, Г, Л, Ф, Ц, Н, У, Е, Р, Ж, Э, Х, М, УП либо О с буквой функции), получено конец обозначения"},
		{"АОД", "обозначение «АОД»: позиция 4: ожидалось: номер разработки, получено конец обозначения"},
		{"КОФ2111", "обозначение «КОФ2111»: позиция 4: ожидалось: номер разработки 1–99 или 101–999, получено «2111»"},
		{"2Т31485", "обозначение «2Т31485»: позиция 4: ожидалось: номер разработки 1–99 или 101–999, получено «1485»"},
		{"КТ30", "обозначение «КТ30»: позиция 4: ожидалось: номер разработки 1–9, получено «0»"},
		{"КТ05", "обозначение «КТ05»: позиция 3: ожидалось: цифра признака 1–9, получено «0»"},
		{"КТ315А0", "обозначение «КТ315А0»: позиция 7: ожидалось: цифра модернизации 1–8 либо 9 (поверхностный монтаж), получено «0»"},
		{"КТ31485", "обозначение «КТ31485»: позиция 3: ожидалось: признак и номер разработки (не более четырёх цифр), получено «31485»"},
		{"2Т3", "обозначение «2Т3»: позиция 4: ожидалось: цифра признака (1–9) и номер разработки, получено конец обозначения"},
		{"2Т014", "обозначение «2Т014»: позиция 3: ожидалось: цифра признака 1–9, получено «0»"},
		{"ХТ315Б", "обозначение «ХТ315Б»: позиция 1: ожидалось: символ материала (Г, К, А, И, Д, П либо цифра 1–6), получено «Х»"},
		{"КЯ123", "обозначение «КЯ123»: позиция 2: ожидалось: буква подкласса (Т, П, Д, С, В, А, И, Г, Л, Ф, Ц, Н, У, Е, Р, Ж, Э, Х, М, УП либо О с буквой функции), получено «Я»"},
		{"КОХ123", "обозначение «КОХ123»: позиция 3: ожидалось: буква функции оптоэлектронного прибора (И, Ф, Д, Т, Р, У, К, Л, М, П), получено «Х»"},
		{"КТ315З", "обозначение «КТ315З»: позиция 6: ожидался конец обозначения, получено «З»"},
		{"КТ315Б-7", "обозначение «КТ315Б-7»: позиция 8: ожидалось: цифра бескорпусного исполнения 1–6, получено «7»"},
		{"КТ315Б/9", "обозначение «КТ315Б/9»: позиция 8: ожидалось: код предприятия-изготовителя (буквы), получено «9»"},
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
	want := "material=кремний; subclass=Д; assembly=0; feature=6; dev_number=2; letters=А"
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
