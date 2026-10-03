package domain

import "testing"

func TestParseJisValid(t *testing.T) {
	cases := []struct {
		input      string
		designated string // канонический ключ (полная форма)
		kind       Kind
		fields     string
	}{
		{"2SA1015", "2SA1015", KindTransistor, "junctions=2; subclass=bjt; dev_number=1015"},
		{"2SC1815", "2SC1815", KindTransistor, "junctions=2; subclass=bjt; dev_number=1815"},
		{"2SK1058", "2SK1058", KindTransistor, "junctions=2; subclass=fet; dev_number=1058"},
		{"2SB75", "2SB75", KindTransistor, "junctions=2; subclass=bjt; dev_number=75"},
		{"2SJ50", "2SJ50", KindTransistor, "junctions=2; subclass=fet; dev_number=50"},
		{"2SD555GR", "2SD555GR", KindTransistor, "junctions=2; subclass=bjt; dev_number=555; letters=GR"},
		{"3SK122", "3SK122", KindTransistor, "junctions=3; subclass=fet; dev_number=122"},
		{"1S2076", "1S2076", KindDiode, "junctions=1; dev_number=2076"},
		{"1SS352", "1SS352", KindDiode, "junctions=1; subclass=signal; dev_number=352"},
		{"1SR154-400", "1SR154-400", KindDiode, "junctions=1; subclass=rectifier; dev_number=154"},
		{"1SZ10", "1SZ10", KindDiode, "junctions=1; subclass=zener; dev_number=10"},
		// Сокращённые формы: префикс 2S опущен, канонизация восстанавливает
		// полный формат; сокращённые — не ключи записей (03 §2.1).
		{"C1815", "2SC1815", KindTransistor, "junctions=2; subclass=bjt; dev_number=1815"},
		{"A1015", "2SA1015", KindTransistor, "junctions=2; subclass=bjt; dev_number=1015"},
		{"K1058", "2SK1058", KindTransistor, "junctions=2; subclass=fet; dev_number=1058"},
		{"D1047", "2SD1047", KindTransistor, "junctions=2; subclass=bjt; dev_number=1047"},
	}
	for _, tc := range cases {
		p, err := parseJis(newScanner(tc.input), "")
		if err != nil {
			t.Errorf("«%s»: неожиданная ошибка: %v", tc.input, err)
			continue
		}
		if p.Kind != tc.kind {
			t.Errorf("«%s»: класс %s, ожидался %s", tc.input, p.Kind, tc.kind)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
		if p.System != SystemJis || p.Designation != tc.designated {
			t.Errorf("«%s»: system/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseJisMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"2X1015", "designation «2X1015»: position 2: expected: registration letter S, got «X»"},
		{"4S1015", "designation «4S1015»: position 1: expected: digit of the p-n junction count (1–3), got «4»"},
		{"2SI1015", "designation «2SI1015»: position 3: expected: device class letter (A, B, C, D, E, F, G, H, J, K, M, Q, R, S, T, V, Z), got «I»"},
		{"2S1015", "designation «2S1015»: position 3: expected: device class letter (mandatory for 2S and 3S), got «1»"},
		{"2SC1", "designation «2SC1»: position 4: expected: registration number (2–4 digits, no leading zero), got «1»"},
		{"2SC015", "designation «2SC015»: position 4: expected: registration number (2–4 digits, no leading zero), got «015»"},
		{"2SC1815-", "designation «2SC1815-»: position 9: expected: suffix after the hyphen (letters and digits), got end of designation"},
		{"Q1234", "designation «Q1234»: position 1: expected: junction count digit and S, or transistor class letter (short form), got «Q»"},
		{"C015", "designation «C015»: position 2: expected: short-form registration number (3–4 digits), got «015»"},
		{"C12345", "designation «C12345»: position 2: expected: short-form registration number (3–4 digits), got «12345»"},
		{"2SC18X1", "designation «2SC18X1»: position 7: expected end of designation, got «1»"},
		{"C1815GR", "designation «C1815GR»: position 6: expected end of designation, got «G»"},
	}
	for _, tc := range cases {
		_, err := parseJis(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}
