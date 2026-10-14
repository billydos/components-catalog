package domain_test

import (
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Golden-тест автодетекта: все примеры обозначений из плана —
// 03-data-model.md §2 (строки «Примеры:» всех систем) и 06-examples.md
// (записи наполнения по классам). Таблица фиксирует класс, систему,
// канонический ключ и поля разбора.
func TestGoldenAutodetect(t *testing.T) {
	cases := []struct {
		input  string
		kind   domain.Kind
		system domain.System
		key    string // канонический ключ (пуст — совпадает с вводом)
		fields string
	}{
		// Полупроводники gost (03 §2.1 + примеры ГОСТ Р 71055-2023).
		{"КТ315Б", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=bjt; assembly=0; dev_number=315; letters=Б; base=КТ315"},
		{"ГТ109Г", domain.KindTransistor, domain.SystemGost, "", "material=ge; subclass=bjt; assembly=0; dev_number=109; letters=Г; base=ГТ109"},
		{"2Т914А-1", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=bjt; assembly=0; feature=9; dev_number=14; letters=А; base=2Т914"},
		{"КДС111В", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=rectifier; assembly=1; dev_number=111; letters=В; base=КДС111"},
		{"КС168А", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=zener; assembly=0; dev_number=168; letters=А; base=КС168"},
		{"АЛ307Б", domain.KindDiode, domain.SystemGost, "", "material=ga; subclass=led; assembly=0; dev_number=307; letters=Б; base=АЛ307"},
		{"2Д627АС", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=rectifier; assembly=1; feature=6; dev_number=27; letters=А; base=2Д627"},
		{"КДШ289А", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=rectifier; assembly=0; feature=2; dev_number=89; letters=А; base=КДШ289"},
		{"2П798Г/ИФ", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=fet; assembly=0; feature=7; dev_number=98; letters=Г; base=2П798"},
		{"КТД735В", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=bjt; assembly=0; feature=7; dev_number=35; letters=В; base=КТД735"},
		{"2Т3148А", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=bjt; assembly=0; feature=3; dev_number=148; letters=А; base=2Т3148"},
		{"2В131А", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=varicap; assembly=0; feature=1; dev_number=31; letters=А; base=2В131"},
		{"3А538А", domain.KindDiode, domain.SystemGost, "", "material=ga; subclass=detector; assembly=0; feature=5; dev_number=38; letters=А; base=3А538"},

		// PRO ELECTRON (03 §2.1 + 06 §1–2).
		{"BC547B", domain.KindTransistor, domain.SystemPro, "", "material=si; subclass=bjt; dev_number=547; letters=B; base=BC547"},
		{"AD161", domain.KindTransistor, domain.SystemPro, "", "material=ge; subclass=bjt; dev_number=161; base=AD161"},
		{"BF245", domain.KindTransistor, domain.SystemPro, "", "material=si; subclass=bjt; dev_number=245; base=BF245"},
		{"AA119", domain.KindDiode, domain.SystemPro, "", "material=ge; subclass=signal; dev_number=119; base=AA119"},
		{"BY133", domain.KindDiode, domain.SystemPro, "", "material=si; subclass=rectifier; dev_number=133; base=BY133"},
		{"BZX85C5V1", domain.KindDiode, domain.SystemPro, "", "material=si; subclass=zener; dev_number=85; letters=C; base=BZX85"},

		// JEDEC (03 §2.1 + 06).
		{"1N4148", domain.KindDiode, domain.SystemJedec, "", "junctions=1; dev_number=4148; base=1N4148"},
		{"1N4007", domain.KindDiode, domain.SystemJedec, "", "junctions=1; dev_number=4007; base=1N4007"},
		{"1N5408", domain.KindDiode, domain.SystemJedec, "", "junctions=1; dev_number=5408; base=1N5408"},
		{"2N2222A", domain.KindTransistor, domain.SystemJedec, "", "junctions=2; dev_number=2222; letters=A; base=2N2222"},
		{"2N3055", domain.KindTransistor, domain.SystemJedec, "", "junctions=2; dev_number=3055; base=2N3055"},
		{"2N3904", domain.KindTransistor, domain.SystemJedec, "", "junctions=2; dev_number=3904; base=2N3904"},

		// JIS (03 §2.1 + 06), включая восстановление полной формы.
		{"2SA1015", domain.KindTransistor, domain.SystemJis, "", "junctions=2; subclass=bjt; dev_number=1015; base=2SA1015"},
		{"2SC1815", domain.KindTransistor, domain.SystemJis, "", "junctions=2; subclass=bjt; dev_number=1815; base=2SC1815"},
		{"2SK1058", domain.KindTransistor, domain.SystemJis, "", "junctions=2; subclass=fet; dev_number=1058; base=2SK1058"},
		{"1S2076", domain.KindDiode, domain.SystemJis, "", "junctions=1; dev_number=2076; base=1S2076"},
		{"1SS352", domain.KindDiode, domain.SystemJis, "", "junctions=1; subclass=signal; dev_number=352; base=1SS352"},
		{"1SR154-400", domain.KindDiode, domain.SystemJis, "", "junctions=1; subclass=rectifier; dev_number=154; base=1SR154"},
		{"C1815", domain.KindTransistor, domain.SystemJis, "2SC1815", "junctions=2; subclass=bjt; dev_number=1815; base=2SC1815"},

		// Резисторы (03 §2.2 + 06 §3).
		{"С2-33Н", domain.KindResistor, domain.SystemGost, "", "family=С; adjustment=fixed; group=2; dev_number=33; letters=Н; base=С2-33"},
		{"СП3-19А", domain.KindResistor, domain.SystemGost, "", "family=СП; adjustment=variable; group=3; dev_number=19; letters=А; base=СП3-19"},
		{"С5-16МВ", domain.KindResistor, domain.SystemGost, "", "family=С; adjustment=fixed; group=5; dev_number=16; letters=МВ; base=С5-16"},
		{"С1-4", domain.KindResistor, domain.SystemGost, "", "family=С; adjustment=fixed; group=1; dev_number=4; base=С1-4"},
		{"С4-2", domain.KindResistor, domain.SystemGost, "", "family=С; adjustment=fixed; group=4; dev_number=2; base=С4-2"},
		{"Р1-4", domain.KindResistor, domain.SystemOst, "", "family=Р; adjustment=fixed; group=1; dev_number=4; base=Р1-4"},
		{"РП1-46", domain.KindResistor, domain.SystemOst, "", "family=РП; adjustment=variable; group=1; dev_number=46; base=РП1-46"},

		// Конденсаторы (03 §2.3 + 06 §4).
		{"К10-17Б", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; adjustment=fixed; group=10; dev_number=17; letters=Б; base=К10-17"},
		{"К50-35", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; adjustment=fixed; group=50; dev_number=35; base=К50-35"},
		{"К73-17", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; adjustment=fixed; group=73; dev_number=17; base=К73-17"},
		{"К78-2", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; adjustment=fixed; group=78; dev_number=2; base=К78-2"},
		{"К42-19", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; adjustment=fixed; group=42; dev_number=19; base=К42-19"},
		{"КТ4-25", domain.KindCapacitor, domain.SystemGost, "", "prefix=КТ; adjustment=preset; group=4; dev_number=25; base=КТ4-25"},
		{"КН1-8", domain.KindCapacitor, domain.SystemGost, "", "prefix=КН; adjustment=fixed; group=1; dev_number=8; base=КН1-8"},
		{"К10-47в", domain.KindCapacitor, domain.SystemGost, "К10-47В", "prefix=К; adjustment=fixed; group=10; dev_number=47; letters=В; base=К10-47"},

		// Раздельность КТ312/2Т312 (критерий этапа 3): пара «буква/цифра»
		// материала физически равнозначна, записи раздельные.
		{"КТ312", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=bjt; assembly=0; dev_number=312; base=КТ312"},
		{"2Т312", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=bjt; assembly=0; feature=3; dev_number=12; base=2Т312"},
	}
	for _, tc := range cases {
		p, err := domain.ParseDesignation(tc.input)
		if err != nil {
			t.Errorf("«%s»: %v", tc.input, err)
			continue
		}
		key := tc.key
		if key == "" {
			key = tc.input
		}
		if p.Kind != tc.kind || p.System != tc.system || p.Designation != key {
			t.Errorf("«%s»: получили %s/%s/«%s», ожидалось %s/%s/«%s»",
				tc.input, p.Kind, p.System, p.Designation, tc.kind, tc.system, key)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
	}
}

// Негативные случаи автодетекта: классы вне модуля, неоднозначный Ф,
// хвост мощности, бессистемные имена.
func TestGoldenNegative(t *testing.T) {
	kindNotSupported := []string{
		"КУ101",   // триодные тиристоры
		"КУП101",  // полевые тиристоры
		"КН102",   // диодные тиристоры
		"КЕ716",   // IGBT
		"КР302А",  // ограничители напряжения
		"АОД158А", // оптопары
		"КОФ211А", // фотоприёмники
		"СТ1-17",  // терморезисторы
		"СН1-1",   // варисторы
		"ВР1-2",   // варисторы (ОСТ)
		"4N35",    // оптопары (JEDEC)
		"RPY84",   // CdS-фоторезисторы (PRO ELECTRON)
	}
	for _, in := range kindNotSupported {
		_, err := domain.ParseDesignation(in)
		if de, ok := domain.AsError(err); !ok || de.Code != domain.CodeKindNotSupported {
			t.Errorf("«%s»: ожидался kind_not_supported, получено %v", in, err)
		}
	}

	if _, err := domain.ParseDesignation("КФ102А"); err == nil {
		t.Error("КФ102А: ожидалась неоднозначность класса")
	} else if de, _ := domain.AsError(err); de.Code != domain.CodeKindAmbiguous {
		t.Errorf("КФ102А: %v", err)
	}

	_, err := domain.ParseDesignation("С2-33Н-0.125")
	if de, ok := domain.AsError(err); !ok || de.Code != domain.CodePowerSuffix {
		t.Fatalf("С2-33Н-0.125: %v", err)
	}
	want := "designation «С2-33Н-0.125»: numeric tail after letters is a power suffix, an element of the full designation; use designation «С2-33Н» and the variants section"
	if err.Error() != want {
		t.Errorf("\n got:  %s\n want: %s", err.Error(), want)
	}
}
