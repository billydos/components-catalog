package domain

import "testing"

func TestParseProValid(t *testing.T) {
	cases := []struct {
		input  string
		kind   Kind
		fields string
	}{
		{"BC547B", KindTransistor, "material=кремний; subclass=C; dev_number=547; letters=B"},
		{"AD161", KindTransistor, "material=германий; subclass=D; dev_number=161"},
		{"BF245", KindTransistor, "material=кремний; subclass=F; dev_number=245"},
		{"BY133", KindDiode, "material=кремний; subclass=Y; dev_number=133"},
		{"AA112", KindDiode, "material=германий; subclass=A; dev_number=112"},
		{"ACY32", KindTransistor, "material=германий; subclass=C; dev_number=32"},
		// ASZ15 — индустриальная регистрация, поэтому НЕ входит в реестр
		// series (инвариант 03 §2.4).
		{"ASZ15", KindTransistor, "material=германий; subclass=S; dev_number=15"},
		{"BZX85C5V1", KindDiode, "material=кремний; subclass=Z; dev_number=85; letters=C"},
		{"BZY74-C6V3", KindDiode, "material=кремний; subclass=Z; dev_number=74"},
		{"BZW70-9V1", KindDiode, "material=кремний; subclass=Z; dev_number=70"},
		{"BZW10-15B", KindDiode, "material=кремний; subclass=Z; dev_number=10"},
		{"BLU80-24", KindTransistor, "material=кремний; subclass=L; dev_number=80"},
		{"BPW50-6", KindDiode, "material=кремний; subclass=P; dev_number=50"},
		{"CQY17", KindDiode, "material=арсенид галлия; subclass=Q; dev_number=17"},
		{"BU208A", KindTransistor, "material=кремний; subclass=U; dev_number=208; letters=A"},
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
			t.Errorf("«%s»: система/ключ %s/%s", tc.input, p.System, p.Designation)
		}
	}
}

func TestParseProMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"BC5", "обозначение «BC5»: позиция 3: ожидалось: номер регистрации 100–999 (до 9999), получено «5»"},
		{"BD23", "обозначение «BD23»: позиция 3: ожидалось: номер регистрации 100–999 (до 9999), получено «23»"},
		{"BZ", "обозначение «BZ»: позиция 3: ожидалось: номер регистрации (100–999 либо буква и 10–99), получено конец обозначения"},
		{"BX", "обозначение «BX»: позиция 3: ожидалось: номер регистрации (100–999 либо буква и 10–99), получено конец обозначения"},
		{"BX5", "обозначение «BX5»: позиция 3: ожидалось: номер регистрации 100–999 (до 9999), получено «5»"},
		{"BZX5", "обозначение «BZX5»: позиция 4: ожидалось: номер промышленной регистрации 10–99 (до 999), получено «5»"},
		{"2N2222A", "обозначение «2N2222A»: позиция 1: ожидалось: буква материала (A, B, C, R), получено «2»"},
		{"BI100", "обозначение «BI100»: позиция 2: ожидалось: буква класса прибора (A–Z по таблице классов), получено «I»"},
		{"BZY74-", "обозначение «BZY74-»: позиция 7: ожидалось: суффикс подклассификации (буквы и цифры), получено конец обозначения"},
		{"BC547BPX", "обозначение «BC547BPX»: позиция 8: ожидался конец обозначения, получено «X»"},
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
