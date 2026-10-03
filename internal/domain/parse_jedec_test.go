package domain

import "testing"

func TestParseJedecValid(t *testing.T) {
	cases := []struct {
		input  string
		kind   Kind
		fields string
	}{
		{"1N4148", KindDiode, "junctions=1; dev_number=4148"},
		{"1N4007", KindDiode, "junctions=1; dev_number=4007"},
		{"1N34A", KindDiode, "junctions=1; dev_number=34; letters=A"},
		{"1C4148", KindDiode, "junctions=1; dev_number=4148"},
		{"2N2222A", KindTransistor, "junctions=2; dev_number=2222; letters=A"},
		{"2N3055", KindTransistor, "junctions=2; dev_number=3055"},
		{"2N3904", KindTransistor, "junctions=2; dev_number=3904"},
		{"3N211", KindTransistor, "junctions=3; dev_number=211"},
	}
	for _, tc := range cases {
		p, err := parseJedec(newScanner(tc.input), "")
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
		if p.System != SystemJedec || p.Designation != tc.input {
			t.Errorf("«%s»: system/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseJedecMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"2X4148", "designation «2X4148»: position 2: expected: JEDEC registration letter (N or C), got «X»"},
		{"5N4148", "designation «5N4148»: position 1: expected: digit of the p-n junction count (1–4), got «5»"},
		{"0N4148", "designation «0N4148»: position 1: expected: digit of the p-n junction count (1–4), got «0»"},
		{"1N", "designation «1N»: position 3: expected: EIA registration number, got end of designation"},
		{"1N0448", "designation «1N0448»: position 3: expected: EIA registration number without a leading zero (up to four digits), got «0448»"},
		{"2N3055ABCD", "designation «2N3055ABCD»: position 10: expected end of designation, got «D»"},
	}
	for _, tc := range cases {
		_, err := parseJedec(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// 4N — оптопары: класс вне стартовых.
func TestParseJedecKind(t *testing.T) {
	for _, in := range []string{"4N35", "4C35"} {
		_, err := parseJedec(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}
}
