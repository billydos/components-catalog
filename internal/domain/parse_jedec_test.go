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
			t.Errorf("«%s»: система/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseJedecMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"2X4148", "обозначение «2X4148»: позиция 2: ожидалось: признак регистрации JEDEC (N либо C), получено «X»"},
		{"5N4148", "обозначение «5N4148»: позиция 1: ожидалось: цифра числа p-n-переходов (1–4), получено «5»"},
		{"0N4148", "обозначение «0N4148»: позиция 1: ожидалось: цифра числа p-n-переходов (1–4), получено «0»"},
		{"1N", "обозначение «1N»: позиция 3: ожидалось: номер регистрации EIA, получено конец обозначения"},
		{"1N0448", "обозначение «1N0448»: позиция 3: ожидалось: номер регистрации EIA без ведущего нуля (до четырёх цифр), получено «0448»"},
		{"2N3055ABCD", "обозначение «2N3055ABCD»: позиция 10: ожидался конец обозначения, получено «D»"},
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
