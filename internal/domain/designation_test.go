package domain_test

import (
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Канонизация (03 §2.4): trim, верхний регистр, удаление пробелов,
// типографские hyphenы → «-», запятая → точка.
func TestCanonicalize(t *testing.T) {
	cases := []struct{ input, want string }{
		{"КТ315Б", "КТ315Б"},
		{"кт315б", "КТ315Б"},
		{"  кт315б  ", "КТ315Б"},
		{"К Т 3 1 5 Б", "КТ315Б"},
		{"МЛТ–0,5", "МЛТ-0.5"}, // EN DASH
		{"МЛТ—0,5", "МЛТ-0.5"}, // EM DASH
		{"МЛТ−0.5", "МЛТ-0.5"}, // MINUS SIGN
		{"МЛТ‑0.5", "МЛТ-0.5"}, // NON-BREAKING HYPHEN
		{"С2-33Н-0,125", "С2-33Н-0.125"},
		{"M39003/04-0003", "M39003/04-0003"},
	}
	for _, tc := range cases {
		got, err := domain.Canonicalize(tc.input)
		if err != nil {
			t.Errorf("«%s»: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("«%s»: получено «%s», ожидалось «%s»", tc.input, got, tc.want)
		}
	}
}

func TestCanonicalizeMessages(t *testing.T) {
	cases := []struct {
		input string
		code  domain.Code
		want  string
	}{
		{"", domain.CodeInvalidDesignation, "empty designation"},
		{"   ", domain.CodeInvalidDesignation, "empty designation"},
		{"1234", domain.CodeInvalidDesignation, "designation «1234»: no letters in the designation"},
		{"КТ3#5", domain.CodeInvalidDesignation, "designation «КТ3#5»: position 4: invalid character «#»"},
		{"KT315Б", domain.CodeInvalidDesignation, "designation «KT315Б»: position 6: mixed alphabets (Cyrillic and Latin), got «Б»"},
		{"BC547В", domain.CodeInvalidDesignation, "designation «BC547В»: position 6: mixed alphabets (Cyrillic and Latin), got «В»"},
		{"КТ315B", domain.CodeInvalidDesignation, "designation «КТ315B»: position 6: mixed alphabets (Cyrillic and Latin), got «B»"},
		// Русский алфавит, а не весь блок Unicode «кириллица».
		{"КТ3102Њ", domain.CodeInvalidDesignation, "designation «КТ3102Њ»: position 7: invalid character «Њ»"},
		{"Д226Ў", domain.CodeInvalidDesignation, "designation «Д226Ў»: position 5: invalid character «Ў»"},
	}
	for _, tc := range cases {
		_, err := domain.Canonicalize(tc.input)
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		de, ok := domain.AsError(err)
		if !ok || de.Code != tc.code {
			t.Errorf("«%s»: код %v", tc.input, err)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// Автодетект: отказ с перечнем поддерживаемых систем (03 §2.4, риск R6).
func TestAutodetectFail(t *testing.T) {
	for _, in := range []string{"XYZ123", "JANTX2N3055", "ЧТОТО", "ФОБОС"} {
		_, err := domain.ParseDesignation(in)
		if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeInvalidDesignation {
			t.Errorf("«%s»: ожидался invalid_designation, получено %v", in, err)
			continue
		}
		want := "designation «" + in + "»: designation system not recognized (supported: gost, ost, pro, jedec, jis, series)"
		if err.Error() != want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", in, err.Error(), want)
		}
	}
}

// Явная system и класс переопределяют автодетект (03 §2.4).
func TestParseForSystem(t *testing.T) {
	// other — без разбора, только канонизация; класс обязателен.
	p, err := domain.ParseDesignationForSystem("JANTX2N3055", domain.SystemOther, domain.KindTransistor)
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	if p.Kind != domain.KindTransistor || p.System != domain.SystemOther ||
		p.Designation != "JANTX2N3055" || len(p.Fields) != 0 {
		t.Errorf("other: %+v", p)
	}
	if _, err := domain.ParseDesignationForSystem("JANTX2N3055", domain.SystemOther, ""); err == nil {
		t.Error("other без класса должен требовать явный класс")
	} else if de, _ := domain.AsError(err); de.Code != domain.CodeKindAmbiguous {
		t.Errorf("other без класса: %v", err)
	} else if want := "designation system other does not determine the device kind; specify the kind explicitly"; err.Error() != want {
		t.Errorf("other без класса:\n got:  %s\n want: %s", err.Error(), want)
	}

	// Несоответствие системы.
	_, err = domain.ParseDesignationForSystem("КТ315Б", domain.SystemJedec, "")
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDesignationMismatch {
		t.Fatalf("ожидался designation_mismatch, получено %v", err)
	}
	want := "designation «КТ315Б» does not match designation system jedec"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// Несоответствие класса при опознанной системе.
	_, err = domain.ParseDesignationForSystem("КТ315Б", domain.SystemGost, domain.KindDiode)
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDesignationMismatch {
		t.Fatalf("ожидался designation_mismatch, получено %v", err)
	}
	want = "designation «КТ315Б» belongs to kind transistor, kind diode was given"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// gost объединяет три грамматики: без класса — перебор в порядке
	// автодетекта; КТ4-25 — конденсатор, КТ315Б — транзистор, С2-33Н — резистор.
	for _, tc := range []struct {
		in   string
		kind domain.Kind
	}{
		{"КТ4-25", domain.KindCapacitor},
		{"КТ315Б", domain.KindTransistor},
		{"С2-33Н", domain.KindResistor},
	} {
		p, err := domain.ParseDesignationForSystem(tc.in, domain.SystemGost, "")
		if err != nil {
			t.Errorf("«%s» gost: %v", tc.in, err)
			continue
		}
		if p.Kind != tc.kind {
			t.Errorf("«%s» gost: класс %s, ожидался %s", tc.in, p.Kind, tc.kind)
		}
	}

	// Ни одна из трёх гост-грамматик не подходит — несоответствие системе.
	_, err = domain.ParseDesignationForSystem("КЯ123", domain.SystemGost, "")
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDesignationMismatch {
		t.Fatalf("КЯ123 gost: %v", err)
	}
	if want := "designation «КЯ123» does not match designation system gost"; err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// Канонизация внутри разбора: empty designation всплывает из Parse.
	if _, err := domain.ParseDesignation("   "); err == nil {
		t.Error("empty designation должно давать ошибку разбора")
	} else if de, _ := domain.AsError(err); de == nil || de.Code != domain.CodeInvalidDesignation {
		t.Errorf("empty designation: %v", err)
	}

	// Класс при gost выбирает предметную грамматику напрямую.
	for _, tc := range []struct {
		in   string
		kind domain.Kind
	}{
		{"КС168А", domain.KindDiode},
		{"2Т3148А", domain.KindTransistor},
		{"С2-33Н", domain.KindResistor},
		{"К50-35", domain.KindCapacitor},
	} {
		if _, err := domain.ParseDesignationForSystem(tc.in, domain.SystemGost, tc.kind); err != nil {
			t.Errorf("«%s» gost/%s: %v", tc.in, tc.kind, err)
		}
	}

	// Остальные строгие системы с явным указанием.
	for _, tc := range []struct {
		in     string
		system domain.System
	}{
		{"Р1-4", domain.SystemOst},
		{"BC547B", domain.SystemPro},
		{"2N2222A", domain.SystemJedec},
		{"2SA1015", domain.SystemJis},
		{"МЛТ-0.5", domain.SystemSeries},
	} {
		if _, err := domain.ParseDesignationForSystem(tc.in, tc.system, ""); err != nil {
			t.Errorf("«%s» %s: %v", tc.in, tc.system, err)
		}
		if _, err := domain.ParseDesignationForSystem("КТ315Б", tc.system, ""); err == nil {
			t.Errorf("КТ315Б не должен разбираться системой %s", tc.system)
		}
	}

	// Явный класс при автодетекте системы.
	if _, err := domain.ParseDesignationForSystem("2N2222A", "", domain.KindDiode); err == nil {
		t.Error("явный класс при автодетекте должен проверяться")
	} else if de, _ := domain.AsError(err); de.Code != domain.CodeDesignationMismatch {
		t.Errorf("2N2222A/diode: %v", err)
	}
	if _, err := domain.ParseDesignationForSystem("2N2222A", "", domain.KindTransistor); err != nil {
		t.Errorf("2N2222A/transistor: %v", err)
	}

	// Конфликт класса у семейства — симметрично строгим systemм.
	for _, tc := range []struct {
		in         string
		kind       domain.Kind
		actualKind string
	}{
		{"МЛТ-0.5", domain.KindCapacitor, "resistor"},
		{"П214", domain.KindDiode, "transistor"},
	} {
		_, err := domain.ParseDesignationForSystem(tc.in, "", tc.kind)
		if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDesignationMismatch {
			t.Errorf("«%s»/%s: ожидался designation_mismatch, получено %v", tc.in, tc.kind, err)
			continue
		}
		want := "designation «" + tc.in + "» belongs to kind " + tc.actualKind +
			", kind " + string(tc.kind) + " was given"
		if err.Error() != want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.in, err.Error(), want)
		}
	}

	// Явная system series: неизвестное семейство — перечень поддерживаемых
	// (план 03 §2.1–2.2), а не generic-несоответствие системе. Область
	// перечня без класса — аргумент-сообщение families_all_scopes
	// (канонический en-рендер — «all classes», ru — «все классы»).
	_, err = domain.ParseDesignationForSystem("ЧТОТО", domain.SystemSeries, "")
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeInvalidDesignation {
		t.Fatalf("ЧТОТО/series: %v", err)
	}
	want = "designation «ЧТОТО»: unknown family «ЧТОТО»; supported families (all classes): " +
		"CFR, KNP, M55342, MPSA, OC, RB, RC, RL, RN, RW, TIP, " +
		"БМ, ВК, ВС, Д, ДГ, КБГИ, КД, КИМ, КЛС, КМ, КПК, КСО, КЭГ, МБГО, МБГЧ, МБМ, " +
		"МГТ, МЛТ, МП, МТ, ОМЛТ, П, ПЭ, ПЭВ, СГМ, СПО, УЛИ, ЭМ, ЭТО"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}
	if de, ok := domain.AsError(err); ok {
		ru := i18n.Message(i18n.Ru, string(de.MsgID), de.Args...)
		if !strings.Contains(ru, "(все классы)") {
			t.Errorf("ru-рендер family_unknown без «(все классы)»: %s", ru)
		}
	}

	// С явным классом перечень ограничен классом.
	_, err = domain.ParseDesignationForSystem("ЧТОТО", domain.SystemSeries, domain.KindResistor)
	if err == nil {
		t.Fatal("ЧТОТО/series/resistor: ожидалась ошибка")
	}
	want = "designation «ЧТОТО»: unknown family «ЧТОТО»; supported families (resistor): " +
		"CFR, KNP, M55342, RB, RC, RL, RN, RW, ВК, ВС, КИМ, МГТ, МЛТ, МТ, ОМЛТ, ПЭ, ПЭВ, СПО, УЛИ"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// По-позиционные ошибки хвоста опознанного семейства всплывают из
	// автодетекта, а не затираются итоговым отказом.
	for _, tc := range []struct{ in, want string }{
		{"МЛТ-А", "designation «МЛТ-А»: position 5: expected: positive nominal power number, got «А»"},
		{"КСО-", "designation «КСО-»: position 5: expected: family tail (digits and letters), got end of designation"},
		{"Д99999999999999999999", "designation «Д99999999999999999999»: position 2: expected: number in the family tail (up to 5 digits), got «99999999999999999999»"},
	} {
		_, err := domain.ParseDesignation(tc.in)
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.in)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.in, err.Error(), tc.want)
		}
	}

	// Ошибки класса и политики хвоста сохраняют код при явной системе.
	if _, err := domain.ParseDesignationForSystem("КУ101", domain.SystemGost, ""); err == nil {
		t.Error("КУ101: ожидался kind_not_supported")
	} else if de, _ := domain.AsError(err); de.Code != domain.CodeKindNotSupported {
		t.Errorf("КУ101 gost: %v", err)
	}
	if _, err := domain.ParseDesignationForSystem("С2-33Н-0.125", domain.SystemGost, domain.KindResistor); err == nil {
		t.Error("суффикс мощности: ожидалась ошибка")
	} else if de, _ := domain.AsError(err); de.Code != domain.CodePowerSuffix {
		t.Errorf("суффикс мощности gost/resistor: %v", err)
	}

	// Неизвестные коды системы/класса — ожидаемые ошибки валидации
	// (validation_failed, контракт CLI/REST для пользовательского ввода).
	_, err = domain.ParseDesignationForSystem("КТ315Б", domain.System("x"), "")
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeValidationFailed {
		t.Errorf("неизвестная system: %v", err)
	}
	if want := "unknown designation system «x»"; err.Error() != want {
		t.Errorf("неизвестная system:\n got:  %s\n want: %s", err.Error(), want)
	}
	_, err = domain.ParseDesignationForSystem("КТ315Б", "", domain.Kind("x"))
	de, ok = domain.AsError(err)
	if !ok || de.Code != domain.CodeValidationFailed {
		t.Errorf("неизвестный класс: %v", err)
	}
	if want := "unknown device kind «x»"; err.Error() != want {
		t.Errorf("неизвестный класс:\n got:  %s\n want: %s", err.Error(), want)
	}
}
