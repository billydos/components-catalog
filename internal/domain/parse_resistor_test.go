package domain

import "testing"

func TestParseResistorGostValid(t *testing.T) {
	cases := []struct {
		input  string
		fields string
	}{
		{"С2-33Н", "family=С; adjustment=fixed; group=2; dev_number=33; letters=Н"},
		{"СП3-19А", "family=СП; adjustment=variable; group=3; dev_number=19; letters=А"},
		{"С5-16МВ", "family=С; adjustment=fixed; group=5; dev_number=16; letters=МВ"},
		{"С1-4", "family=С; adjustment=fixed; group=1; dev_number=4"},
		{"С4-2", "family=С; adjustment=fixed; group=4; dev_number=2"},
		{"С2-29В", "family=С; adjustment=fixed; group=2; dev_number=29; letters=В"},
		{"СП5-1", "family=СП; adjustment=variable; group=5; dev_number=1"},
		{"С6-6", "family=С; adjustment=fixed; group=6; dev_number=6"},
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
		{"С0-5", "designation «С0-5»: position 2: expected: material group digit 1–6, got «0»"},
		{"С7-1", "designation «С7-1»: position 2: expected: material group digit 1–6, got «7»"},
		{"С2", "designation «С2»: position 3: expected: hyphen, got end of designation"},
		{"С2-", "designation «С2-»: position 4: expected: development number, got end of designation"},
		{"С2-033", "designation «С2-033»: position 4: expected: development number without a leading zero (up to 3 digits), got «033»"},
		{"С2-33НАБВ", "designation «С2-33НАБВ»: position 9: expected end of designation, got «В»"},
		// Хвост мощности — строгий отказ с подсказкой (03 §2.2, D6).
		{"С2-33Н-0.125", "designation «С2-33Н-0.125»: numeric tail after letters is a power suffix, an element of the full designation; use designation «С2-33Н» and the variants section"},
		{"СП5-16-1", "designation «СП5-16-1»: numeric tail after letters is a power suffix, an element of the full designation; use designation «СП5-16» and the variants section"},
		{"С5-16МВ-0,25", "designation «С5-16МВ-0,25»: numeric tail after letters is a power suffix, an element of the full designation; use designation «С5-16МВ» and the variants section"},
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

// СТ/СН/СФ — терморезисторы/варисторы/фоторезисторы: иные классы deviceов.
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
		{"Р1-4", "family=Р; adjustment=fixed; group=1; dev_number=4"},
		{"РП1-46", "family=РП; adjustment=variable; group=1; dev_number=46"},
		{"Р2-12", "family=Р; adjustment=fixed; group=2; dev_number=12"},
		{"НР1-1", "family=НР; adjustment=fixed; group=1; dev_number=1"},
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
		{"Р3-1", "designation «Р3-1»: position 2: expected: material group digit 1–2, got «3»"},
		{"Р1", "designation «Р1»: position 3: expected: hyphen, got end of designation"},
		{"Р1-", "designation «Р1-»: position 4: expected: development number, got end of designation"},
		{"Р1-4В", "designation «Р1-4В»: position 5: expected end of designation, got «В»"},
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

// ВР/ВРП/ТР/ТРП — варисторы/терморезисторы: иные классы deviceов.
func TestParseResistorOstKind(t *testing.T) {
	for _, in := range []string{"ВР1-2", "ВРП1-2", "ТР1-2", "ТРП1-2"} {
		_, err := parseResistorOst(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}
}
