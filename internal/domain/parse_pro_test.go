package domain

import "testing"

func TestParseProValid(t *testing.T) {
	cases := []struct {
		input  string
		kind   Kind
		fields string
	}{
		{"BC547B", KindTransistor, "material=si; subclass=C; dev_number=547; letters=B"},
		{"AD161", KindTransistor, "material=ge; subclass=D; dev_number=161"},
		{"BF245", KindTransistor, "material=si; subclass=F; dev_number=245"},
		{"BY133", KindDiode, "material=si; subclass=Y; dev_number=133"},
		{"AA112", KindDiode, "material=ge; subclass=A; dev_number=112"},
		{"ACY32", KindTransistor, "material=ge; subclass=C; dev_number=32"},
		// ASZ15 — индустриальная регистрация, поэтому НЕ входит в реестр
		// series (инвариант 03 §2.4).
		{"ASZ15", KindTransistor, "material=ge; subclass=S; dev_number=15"},
		{"BZX85C5V1", KindDiode, "material=si; subclass=Z; dev_number=85; letters=C"},
		{"BZY74-C6V3", KindDiode, "material=si; subclass=Z; dev_number=74"},
		{"BZW70-9V1", KindDiode, "material=si; subclass=Z; dev_number=70"},
		{"BZW10-15B", KindDiode, "material=si; subclass=Z; dev_number=10"},
		{"BLU80-24", KindTransistor, "material=si; subclass=L; dev_number=80"},
		{"BPW50-6", KindDiode, "material=si; subclass=P; dev_number=50"},
		{"CQY17", KindDiode, "material=gaas; subclass=Q; dev_number=17"},
		{"BU208A", KindTransistor, "material=si; subclass=U; dev_number=208; letters=A"},
	}
	for _, tc := range cases {
		p, err := parsePro(newScanner(tc.input), "")
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
		if p.System != SystemPro || p.Designation != tc.input {
			t.Errorf("«%s»: system/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseProMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"BC5", "designation «BC5»: position 3: expected: registration number 100–999 (up to 9999), got «5»"},
		{"BD23", "designation «BD23»: position 3: expected: registration number 100–999 (up to 9999), got «23»"},
		{"BZ", "designation «BZ»: position 3: expected: registration number (100–999, or a letter and 10–99), got end of designation"},
		{"BX", "designation «BX»: position 3: expected: registration number (100–999, or a letter and 10–99), got end of designation"},
		{"BX5", "designation «BX5»: position 3: expected: registration number 100–999 (up to 9999), got «5»"},
		{"BZX5", "designation «BZX5»: position 4: expected: consumer registration number 10–99 (up to 999), got «5»"},
		{"2N2222A", "designation «2N2222A»: position 1: expected: material letter (A, B, C, R), got «2»"},
		{"BI100", "designation «BI100»: position 2: expected: device class letter (A–Z per the class table), got «I»"},
		{"BZY74-", "designation «BZY74-»: position 7: expected: subclassification suffix (letters and digits), got end of designation"},
		{"BC547BPX", "designation «BC547BPX»: position 8: expected end of designation, got «X»"},
	}
	for _, tc := range cases {
		_, err := parsePro(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// Материал R и классы G/N/R/T/W — грамматически корректны, но класс вне
// модуля; грамматика проверяется раньше класса (RC05 не разбирается как
// PRO ELECTRON и доходит до реестра series — series_test.go).
func TestParseProKind(t *testing.T) {
	unsupported := []string{"RPY84", "BGN10", "BNN10", "BRN10", "BTN10", "BWN10"}
	for _, in := range unsupported {
		_, err := parsePro(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}
	// Грамматически некорректные R-обозначения — ошибка разбора, не класс.
	for _, in := range []string{"RC05", "RN55", "RL20", "RW74"} {
		_, err := parsePro(newScanner(in), "")
		if de, ok := AsError(err); !ok || de.Code != CodeInvalidDesignation {
			t.Errorf("«%s»: ожидалась ошибка разбора, получено %v", in, err)
		}
	}
}
