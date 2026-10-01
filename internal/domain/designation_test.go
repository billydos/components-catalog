package domain_test

import (
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Канонизация (03 §2.4): trim, верхний регистр, удаление пробелов,
// типографские дефисы → «-», запятая → точка.
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
		{"", domain.CodeInvalidDesignation, "пустое обозначение"},
		{"   ", domain.CodeInvalidDesignation, "пустое обозначение"},
		{"1234", domain.CodeInvalidDesignation, "обозначение «1234»: в обозначении нет букв"},
		{"КТ3#5", domain.CodeInvalidDesignation, "обозначение «КТ3#5»: позиция 4: недопустимый символ «#»"},
		{"KT315Б", domain.CodeInvalidDesignation, "обозначение «KT315Б»: позиция 6: смешение алфавитов (кириллица и латиница), получено «Б»"},
		{"BC547В", domain.CodeInvalidDesignation, "обозначение «BC547В»: позиция 6: смешение алфавитов (кириллица и латиница), получено «В»"},
		{"КТ315B", domain.CodeInvalidDesignation, "обозначение «КТ315B»: позиция 6: смешение алфавитов (кириллица и латиница), получено «B»"},
		// Русский алфавит, а не весь блок Unicode «кириллица».
		{"КТ3102Њ", domain.CodeInvalidDesignation, "обозначение «КТ3102Њ»: позиция 7: недопустимый символ «Њ»"},
		{"Д226Ў", domain.CodeInvalidDesignation, "обозначение «Д226Ў»: позиция 5: недопустимый символ «Ў»"},
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
		want := "обозначение «" + in + "»: не удалось распознать систему обозначений (поддерживаемые: gost, ost, pro, jedec, jis, series)"
		if err.Error() != want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", in, err.Error(), want)
		}
	}
}

// Явная система и класс переопределяют автодетект (03 §2.4).
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
	}

	// Несоответствие системы.
	_, err = domain.ParseDesignationForSystem("КТ315Б", domain.SystemJedec, "")
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDesignationMismatch {
		t.Fatalf("ожидался designation_mismatch, получено %v", err)
	}
	want := "обозначение «КТ315Б» не соответствует системе обозначений jedec"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// Несоответствие класса при опознанной системе.
	_, err = domain.ParseDesignationForSystem("КТ315Б", domain.SystemGost, domain.KindDiode)
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeDesignationMismatch {
		t.Fatalf("ожидался designation_mismatch, получено %v", err)
	}
	want = "обозначение «КТ315Б» принадлежит классу transistor, указан класс diode"
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
	if want := "обозначение «КЯ123» не соответствует системе обозначений gost"; err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// Канонизация внутри разбора: пустое обозначение всплывает из Parse.
	if _, err := domain.ParseDesignation("   "); err == nil {
		t.Error("пустое обозначение должно давать ошибку разбора")
	} else if de, _ := domain.AsError(err); de == nil || de.Code != domain.CodeInvalidDesignation {
		t.Errorf("пустое обозначение: %v", err)
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

	// Конфликт класса у семейства — симметрично строгим системам.
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
		want := "обозначение «" + tc.in + "» принадлежит классу " + tc.actualKind +
			", указан класс " + string(tc.kind)
		if err.Error() != want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.in, err.Error(), want)
		}
	}

	// Явная система series: неизвестное семейство — перечень поддерживаемых
	// (план 03 §2.1–2.2), а не generic-несоответствие системе.
	_, err = domain.ParseDesignationForSystem("ЧТОТО", domain.SystemSeries, "")
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeInvalidDesignation {
		t.Fatalf("ЧТОТО/series: %v", err)
	}
	want = "обозначение «ЧТОТО»: неизвестное семейство «ЧТОТО»; поддерживаемые семейства (все классы): " +
		"CFR, KNP, M55342, MPSA, OC, RB, RC, RL, RN, RW, TIP, " +
		"БМ, ВК, ВС, Д, ДГ, КБГИ, КД, КИМ, КЛС, КМ, КПК, КСО, КЭГ, МБГО, МБГЧ, МБМ, " +
		"МГТ, МЛТ, МП, МТ, ОМЛТ, П, ПЭ, ПЭВ, СГМ, СПО, УЛИ, ЭМ, ЭТО"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// С явным классом перечень ограничен классом.
	_, err = domain.ParseDesignationForSystem("ЧТОТО", domain.SystemSeries, domain.KindResistor)
	if err == nil {
		t.Fatal("ЧТОТО/series/resistor: ожидалась ошибка")
	}
	want = "обозначение «ЧТОТО»: неизвестное семейство «ЧТОТО»; поддерживаемые семейства (resistor): " +
		"CFR, KNP, M55342, RB, RC, RL, RN, RW, ВК, ВС, КИМ, МГТ, МЛТ, МТ, ОМЛТ, ПЭ, ПЭВ, СПО, УЛИ"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}

	// По-позиционные ошибки хвоста опознанного семейства всплывают из
	// автодетекта, а не затираются итоговым отказом.
	for _, tc := range []struct{ in, want string }{
		{"МЛТ-А", "обозначение «МЛТ-А»: позиция 5: ожидалось: положительное число номинальной мощности, получено «А»"},
		{"КСО-", "обозначение «КСО-»: позиция 5: ожидалось: хвост семейства (число и буквы), получено конец обозначения"},
		{"Д99999999999999999999", "обозначение «Д99999999999999999999»: позиция 2: ожидалось: число в хвосте семейства (до 5 цифр), получено «99999999999999999999»"},
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

	// Неизвестные коды системы/класса — непредвиденные ошибки значений.
	_, err = domain.ParseDesignationForSystem("КТ315Б", domain.System("x"), "")
	if _, ok := domain.AsError(err); ok {
		t.Errorf("неизвестная система: %v", err)
	}
	_, err = domain.ParseDesignationForSystem("КТ315Б", "", domain.Kind("x"))
	if _, ok := domain.AsError(err); ok {
		t.Errorf("неизвестный класс: %v", err)
	}
}
