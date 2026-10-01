package domain

import "testing"

func TestParseJisValid(t *testing.T) {
	cases := []struct {
		input      string
		designated string // канонический ключ (полная форма)
		kind       Kind
		fields     string
	}{
		{"2SA1015", "2SA1015", KindTransistor, "junctions=2; subclass=A; dev_number=1015"},
		{"2SC1815", "2SC1815", KindTransistor, "junctions=2; subclass=C; dev_number=1815"},
		{"2SK1058", "2SK1058", KindTransistor, "junctions=2; subclass=K; dev_number=1058"},
		{"2SB75", "2SB75", KindTransistor, "junctions=2; subclass=B; dev_number=75"},
		{"2SJ50", "2SJ50", KindTransistor, "junctions=2; subclass=J; dev_number=50"},
		{"2SD555GR", "2SD555GR", KindTransistor, "junctions=2; subclass=D; dev_number=555; letters=GR"},
		{"3SK122", "3SK122", KindTransistor, "junctions=3; subclass=K; dev_number=122"},
		{"1S2076", "1S2076", KindDiode, "junctions=1; dev_number=2076"},
		{"1SS352", "1SS352", KindDiode, "junctions=1; subclass=S; dev_number=352"},
		{"1SR154-400", "1SR154-400", KindDiode, "junctions=1; subclass=R; dev_number=154"},
		{"1SZ10", "1SZ10", KindDiode, "junctions=1; subclass=Z; dev_number=10"},
		// Сокращённые формы: префикс 2S опущен, канонизация восстанавливает
		// полный формат; сокращённые — не ключи записей (03 §2.1).
		{"C1815", "2SC1815", KindTransistor, "junctions=2; subclass=C; dev_number=1815"},
		{"A1015", "2SA1015", KindTransistor, "junctions=2; subclass=A; dev_number=1015"},
		{"K1058", "2SK1058", KindTransistor, "junctions=2; subclass=K; dev_number=1058"},
		{"D1047", "2SD1047", KindTransistor, "junctions=2; subclass=D; dev_number=1047"},
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
			t.Errorf("«%s»: система/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseJisMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"2X1015", "обозначение «2X1015»: позиция 2: ожидалось: признак регистрации S, получено «X»"},
		{"4S1015", "обозначение «4S1015»: позиция 1: ожидалось: цифра числа p-n-переходов (1–3), получено «4»"},
		{"2SI1015", "обозначение «2SI1015»: позиция 3: ожидалось: буква класса прибора (A, B, C, D, E, F, G, H, J, K, M, Q, R, S, T, V, Z), получено «I»"},
		{"2S1015", "обозначение «2S1015»: позиция 3: ожидалось: буква класса прибора (обязательна для 2S и 3S), получено «1»"},
		{"2SC1", "обозначение «2SC1»: позиция 4: ожидалось: номер регистрации (2–4 цифры, без ведущего нуля), получено «1»"},
		{"2SC015", "обозначение «2SC015»: позиция 4: ожидалось: номер регистрации (2–4 цифры, без ведущего нуля), получено «015»"},
		{"2SC1815-", "обозначение «2SC1815-»: позиция 9: ожидалось: суффикс после дефиса (буквы и цифры), получено конец обозначения"},
		{"Q1234", "обозначение «Q1234»: позиция 1: ожидалось: цифра числа переходов и S либо буква транзисторного класса (сокращённая форма), получено «Q»"},
		{"C015", "обозначение «C015»: позиция 2: ожидалось: номер регистрации сокращённой формы (3–4 цифры), получено «015»"},
		{"C12345", "обозначение «C12345»: позиция 2: ожидалось: номер регистрации сокращённой формы (3–4 цифры), получено «12345»"},
		{"2SC18X1", "обозначение «2SC18X1»: позиция 7: ожидался конец обозначения, получено «1»"},
		{"C1815GR", "обозначение «C1815GR»: позиция 6: ожидался конец обозначения, получено «G»"},
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
