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
		{"КТ315Б", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=Т; assembly=0; dev_number=315; letters=Б"},
		{"ГТ109Г", domain.KindTransistor, domain.SystemGost, "", "material=ge; subclass=Т; assembly=0; dev_number=109; letters=Г"},
		{"2Т914А-1", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=Т; assembly=0; feature=9; dev_number=14; letters=А"},
		{"КДС111В", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=Д; assembly=1; dev_number=111; letters=В"},
		{"КС168А", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=С; assembly=0; dev_number=168; letters=А"},
		{"АЛ307Б", domain.KindDiode, domain.SystemGost, "", "material=ga; subclass=Л; assembly=0; dev_number=307; letters=Б"},
		{"2Д627АС", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=Д; assembly=1; feature=6; dev_number=27; letters=А"},
		{"КДШ289А", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=Д; assembly=0; feature=2; dev_number=89; letters=А"},
		{"2П798Г/ИФ", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=П; assembly=0; feature=7; dev_number=98; letters=Г"},
		{"КТД735В", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=Т; assembly=0; feature=7; dev_number=35; letters=В"},
		{"2Т3148А", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=Т; assembly=0; feature=3; dev_number=148; letters=А"},
		{"2В131А", domain.KindDiode, domain.SystemGost, "", "material=si; subclass=В; assembly=0; feature=1; dev_number=31; letters=А"},
		{"3А538А", domain.KindDiode, domain.SystemGost, "", "material=ga; subclass=А; assembly=0; feature=5; dev_number=38; letters=А"},

		// PRO ELECTRON (03 §2.1 + 06 §1–2).
		{"BC547B", domain.KindTransistor, domain.SystemPro, "", "material=si; subclass=C; dev_number=547; letters=B"},
		{"AD161", domain.KindTransistor, domain.SystemPro, "", "material=ge; subclass=D; dev_number=161"},
		{"BF245", domain.KindTransistor, domain.SystemPro, "", "material=si; subclass=F; dev_number=245"},
		{"AA119", domain.KindDiode, domain.SystemPro, "", "material=ge; subclass=A; dev_number=119"},
		{"BY133", domain.KindDiode, domain.SystemPro, "", "material=si; subclass=Y; dev_number=133"},
		{"BZX85C5V1", domain.KindDiode, domain.SystemPro, "", "material=si; subclass=Z; dev_number=85; letters=C"},

		// JEDEC (03 §2.1 + 06).
		{"1N4148", domain.KindDiode, domain.SystemJedec, "", "junctions=1; dev_number=4148"},
		{"1N4007", domain.KindDiode, domain.SystemJedec, "", "junctions=1; dev_number=4007"},
		{"1N5408", domain.KindDiode, domain.SystemJedec, "", "junctions=1; dev_number=5408"},
		{"2N2222A", domain.KindTransistor, domain.SystemJedec, "", "junctions=2; dev_number=2222; letters=A"},
		{"2N3055", domain.KindTransistor, domain.SystemJedec, "", "junctions=2; dev_number=3055"},
		{"2N3904", domain.KindTransistor, domain.SystemJedec, "", "junctions=2; dev_number=3904"},

		// JIS (03 §2.1 + 06), включая восстановление полной формы.
		{"2SA1015", domain.KindTransistor, domain.SystemJis, "", "junctions=2; subclass=A; dev_number=1015"},
		{"2SC1815", domain.KindTransistor, domain.SystemJis, "", "junctions=2; subclass=C; dev_number=1815"},
		{"2SK1058", domain.KindTransistor, domain.SystemJis, "", "junctions=2; subclass=K; dev_number=1058"},
		{"1S2076", domain.KindDiode, domain.SystemJis, "", "junctions=1; dev_number=2076"},
		{"1SS352", domain.KindDiode, domain.SystemJis, "", "junctions=1; subclass=S; dev_number=352"},
		{"1SR154-400", domain.KindDiode, domain.SystemJis, "", "junctions=1; subclass=R; dev_number=154"},
		{"C1815", domain.KindTransistor, domain.SystemJis, "2SC1815", "junctions=2; subclass=C; dev_number=1815"},

		// Резисторы (03 §2.2 + 06 §3).
		{"С2-33Н", domain.KindResistor, domain.SystemGost, "", "family=С; group=2; dev_number=33; letters=Н"},
		{"СП3-19А", domain.KindResistor, domain.SystemGost, "", "family=СП; group=3; dev_number=19; letters=А"},
		{"С5-16МВ", domain.KindResistor, domain.SystemGost, "", "family=С; group=5; dev_number=16; letters=МВ"},
		{"С1-4", domain.KindResistor, domain.SystemGost, "", "family=С; group=1; dev_number=4"},
		{"С4-2", domain.KindResistor, domain.SystemGost, "", "family=С; group=4; dev_number=2"},
		{"Р1-4", domain.KindResistor, domain.SystemOst, "", "family=Р; group=1; dev_number=4"},
		{"РП1-46", domain.KindResistor, domain.SystemOst, "", "family=РП; group=1; dev_number=46"},
		{"МЛТ-0.5", domain.KindResistor, domain.SystemSeries, "", "series=МЛТ; power=0.5"},
		{"ВС-0.5", domain.KindResistor, domain.SystemSeries, "", "series=ВС; power=0.5"},
		{"ПЭВ-10", domain.KindResistor, domain.SystemSeries, "", "series=ПЭВ; power=10"},

		// Конденсаторы (03 §2.3 + 06 §4).
		{"К10-17Б", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; group=10; dev_number=17; letters=Б"},
		{"К50-35", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; group=50; dev_number=35"},
		{"К73-17", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; group=73; dev_number=17"},
		{"К78-2", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; group=78; dev_number=2"},
		{"К42-19", domain.KindCapacitor, domain.SystemGost, "", "prefix=К; group=42; dev_number=19"},
		{"КТ4-25", domain.KindCapacitor, domain.SystemGost, "", "prefix=КТ; group=4; dev_number=25"},
		{"КН1-8", domain.KindCapacitor, domain.SystemGost, "", "prefix=КН; group=1; dev_number=8"},
		{"К10-47в", domain.KindCapacitor, domain.SystemGost, "К10-47В", "prefix=К; group=10; dev_number=47; letters=В"},
		{"КСО-2", domain.KindCapacitor, domain.SystemSeries, "", "series=КСО; dev_number=2"},
		{"МБМ", domain.KindCapacitor, domain.SystemSeries, "", "series=МБМ"},
		{"МБГЧ-1", domain.KindCapacitor, domain.SystemSeries, "", "series=МБГЧ; dev_number=1"},

		// Семейства series (03 §2.1–2.3, реестр 07 §7).
		{"МП39", domain.KindTransistor, domain.SystemSeries, "", "series=МП; dev_number=39"},
		{"МП41А", domain.KindTransistor, domain.SystemSeries, "", "series=МП; dev_number=41; letters=А"},
		{"П13", domain.KindTransistor, domain.SystemSeries, "", "series=П; dev_number=13"},
		{"П214", domain.KindTransistor, domain.SystemSeries, "", "series=П; dev_number=214"},
		{"OC44", domain.KindTransistor, domain.SystemSeries, "", "series=OC; dev_number=44"},
		{"TIP120", domain.KindTransistor, domain.SystemSeries, "", "series=TIP; dev_number=120"},
		{"MPSA42", domain.KindTransistor, domain.SystemSeries, "", "series=MPSA; dev_number=42"},
		{"Д2Б", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=2; letters=Б"},
		{"Д7А", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=7; letters=А"},
		{"Д104", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=104"},
		{"Д226", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=226"},
		{"Д242", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=242"},
		{"Д808", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=808"},
		{"Д814А", domain.KindDiode, domain.SystemSeries, "", "series=Д; dev_number=814; letters=А"},
		{"ДГ-Ц8", domain.KindDiode, domain.SystemSeries, "", "series=ДГ; dev_number=8; letters=Ц"},
		{"КМ-4", domain.KindCapacitor, domain.SystemSeries, "", "series=КМ; dev_number=4"},
		{"ЭТО-1", domain.KindCapacitor, domain.SystemSeries, "", "series=ЭТО; dev_number=1"},
		{"БМ-2", domain.KindCapacitor, domain.SystemSeries, "", "series=БМ; dev_number=2"},
		{"RC05", domain.KindResistor, domain.SystemSeries, "", "series=RC; dev_number=5"},
		{"RN55", domain.KindResistor, domain.SystemSeries, "", "series=RN; dev_number=55"},
		{"KNP-100", domain.KindResistor, domain.SystemSeries, "", "series=KNP; dev_number=100"},
		{"CFR-25", domain.KindResistor, domain.SystemSeries, "", "series=CFR; dev_number=25"},

		// Раздельность КТ312/2Т312 (критерий этапа 3): пара «буква/цифра»
		// материала физически равнозначна, записи раздельные.
		{"КТ312", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=Т; assembly=0; dev_number=312"},
		{"2Т312", domain.KindTransistor, domain.SystemGost, "", "material=si; subclass=Т; assembly=0; feature=3; dev_number=12"},
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
