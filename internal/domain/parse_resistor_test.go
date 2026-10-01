package domain

import "testing"

func TestParseResistorGostValid(t *testing.T) {
	cases := []struct {
		input  string
		fields string
	}{
		{"С2-33Н", "family=С; group=2; dev_number=33; letters=Н"},
		{"СП3-19А", "family=СП; group=3; dev_number=19; letters=А"},
		{"С5-16МВ", "family=С; group=5; dev_number=16; letters=МВ"},
		{"С1-4", "family=С; group=1; dev_number=4"},
		{"С4-2", "family=С; group=4; dev_number=2"},
		{"С2-29В", "family=С; group=2; dev_number=29; letters=В"},
		{"СП5-1", "family=СП; group=5; dev_number=1"},
		{"С6-6", "family=С; group=6; dev_number=6"},
	}
	for _, tc := range cases {
		p, err := parseResistorGost(newScanner(tc.input), "")
		if err != nil {
			t.Errorf("«%s»: неожиданная ошибка: %v", tc.input, err)
			continue
		}
		if p.Kind != KindResistor || p.System != SystemGost || p.Designation != tc.input {
			t.Errorf("«%s»: %s/%s/%s", tc.input, p.Kind, p.System, p.Designation)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
	}
}

func TestParseResistorGostMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"С0-5", "обозначение «С0-5»: позиция 2: ожидалось: группа по материалу 1–6, получено «0»"},
		{"С7-1", "обозначение «С7-1»: позиция 2: ожидалось: группа по материалу 1–6, получено «7»"},
		{"С2", "обозначение «С2»: позиция 3: ожидалось: дефис, получено конец обозначения"},
		{"С2-", "обозначение «С2-»: позиция 4: ожидалось: номер разработки, получено конец обозначения"},
		{"С2-033", "обозначение «С2-033»: позиция 4: ожидалось: номер разработки без ведущего нуля (до 3 цифр), получено «033»"},
		{"С2-33НАБВ", "обозначение «С2-33НАБВ»: позиция 9: ожидался конец обозначения, получено «В»"},
		// Хвост мощности — строгий отказ с подсказкой (03 §2.2, D6).
		{"С2-33Н-0.125", "обозначение «С2-33Н-0.125»: числовой хвост после букв — суффикс мощности, элемент полного обозначения; используйте обозначение «С2-33Н» и секцию variants"},
		{"СП5-16-1", "обозначение «СП5-16-1»: числовой хвост после букв — суффикс мощности, элемент полного обозначения; используйте обозначение «СП5-16» и секцию variants"},
		{"С5-16МВ-0,25", "обозначение «С5-16МВ-0,25»: числовой хвост после букв — суффикс мощности, элемент полного обозначения; используйте обозначение «С5-16МВ» и секцию variants"},
	}
	for _, tc := range cases {
		_, err := parseResistorGost(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// СТ/СН/СФ — терморезисторы/варисторы/фоторезисторы: иные классы приборов.
func TestParseResistorGostKind(t *testing.T) {
	for _, in := range []string{"СТ1-17", "СН1-1", "СФ2-10"} {
		_, err := parseResistorGost(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}
}

func TestParseResistorOstValid(t *testing.T) {
	cases := []struct {
		input  string
		fields string
	}{
		{"Р1-4", "family=Р; group=1; dev_number=4"},
		{"РП1-46", "family=РП; group=1; dev_number=46"},
		{"Р2-12", "family=Р; group=2; dev_number=12"},
		{"НР1-1", "family=НР; group=1; dev_number=1"},
	}
	for _, tc := range cases {
		p, err := parseResistorOst(newScanner(tc.input), "")
		if err != nil {
			t.Errorf("«%s»: неожиданная ошибка: %v", tc.input, err)
			continue
		}
		if p.Kind != KindResistor || p.System != SystemOst || p.Designation != tc.input {
			t.Errorf("«%s»: %s/%s/%s", tc.input, p.Kind, p.System, p.Designation)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
	}
}

func TestParseResistorOstMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"Р3-1", "обозначение «Р3-1»: позиция 2: ожидалось: группа по материалу 1–2, получено «3»"},
		{"Р1", "обозначение «Р1»: позиция 3: ожидалось: дефис, получено конец обозначения"},
		{"Р1-", "обозначение «Р1-»: позиция 4: ожидалось: номер разработки, получено конец обозначения"},
		{"Р1-4В", "обозначение «Р1-4В»: позиция 5: ожидался конец обозначения, получено «В»"},
	}
	for _, tc := range cases {
		_, err := parseResistorOst(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// ВР/ВРП/ТР/ТРП — варисторы/терморезисторы: иные классы приборов.
func TestParseResistorOstKind(t *testing.T) {
	for _, in := range []string{"ВР1-2", "ВРП1-2", "ТР1-2", "ТРП1-2"} {
		_, err := parseResistorOst(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}
}
