package domain_test

import (
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Перечисление кандидатов (features.log CC-1): все успешные строгие
// разборы в фиксированном порядке автодетекта, без дублей пары
// класс/система; первый кандидат — результат автодетекта. Грамматики
// строгих систем разделены алфавитами и структурой (кириллица gost/ост —
// латиница двухбуквенным номером pro — цифра-признак jedec/jis;
// подстроечные КТ/КП/КН конденсаторов требуют дефис после одной цифры
// группы — полупроводниковая грамматика таких не принимает), поэтому
// перечень сегодня одноэлементный; контракт допускает несколько —
// порядок обязан совпадать с автодетектом.
func TestParseDesignationCandidates(t *testing.T) {
	type pair struct {
		kind   domain.Kind
		system domain.System
	}
	cases := []struct {
		input      string
		kind       domain.Kind
		candidates []pair
	}{
		{"КТ315Б", "", []pair{{domain.KindTransistor, domain.SystemGost}}},
		{"кт315б", "", []pair{{domain.KindTransistor, domain.SystemGost}}},
		{"BC547B", "", []pair{{domain.KindTransistor, domain.SystemPro}}},
		{"2N2222A", "", []pair{{domain.KindTransistor, domain.SystemJedec}}},
		{"2SA1015", "", []pair{{domain.KindTransistor, domain.SystemJis}}},
		{"Р1-4", "", []pair{{domain.KindResistor, domain.SystemOst}}},
		{"К50-35", "", []pair{{domain.KindCapacitor, domain.SystemGost}}},
		// Сокращённая форма JIS: кандидат — восстановленная полная запись.
		{"C1815", "", []pair{{domain.KindTransistor, domain.SystemJis}}},
		// Подсказка kind разрешает неоднозначный класс Ф грамматики gost.
		{"КФ102А", domain.KindTransistor, []pair{{domain.KindTransistor, domain.SystemGost}}},
		{"КФ102А", domain.KindDiode, []pair{{domain.KindDiode, domain.SystemGost}}},
	}
	for _, tc := range cases {
		candidates, err := domain.ParseDesignationCandidates(tc.input, tc.kind)
		if err != nil {
			t.Errorf("«%s»: неожиданная ошибка: %v", tc.input, err)
			continue
		}
		if len(candidates) != len(tc.candidates) {
			t.Errorf("«%s»: кандидатов %d, ожидалось %d: %+v", tc.input, len(candidates), len(tc.candidates), candidates)
			continue
		}
		for i, want := range tc.candidates {
			if candidates[i].Kind != want.kind || candidates[i].System != want.system {
				t.Errorf("«%s»: кандидат %d: %s/%s, ожидался %s/%s",
					tc.input, i, candidates[i].Kind, candidates[i].System, want.kind, want.system)
			}
		}
	}

	// Кандидат сокращённой формы JIS — восстановленная полная запись.
	candidates, err := domain.ParseDesignationCandidates("C1815", "")
	if err != nil || len(candidates) != 1 {
		t.Fatalf("C1815: %+v (%v)", candidates, err)
	}
	if candidates[0].Designation != "2SC1815" {
		t.Errorf("C1815: обозначение %q, ожидалось «2SC1815»", candidates[0].Designation)
	}

	// Первый кандидат совпадает с результатом автодетекта (класс, система,
	// канонический ключ, поля) — включая пары с близкими грамматиками
	// (КТ4-25 конденсатор против полупроводниковой КТ, КС21-5 стабилитрон
	// против конденсаторной КС).
	for _, in := range []string{"КТ315Б", "BC547", "2N2222A", "С2-33Н", "КТ4-25", "КС21-5"} {
		p, err := domain.ParseDesignation(in)
		if err != nil {
			t.Fatalf("«%s»: автодетект: %v", in, err)
		}
		cs, err := domain.ParseDesignationCandidates(in, "")
		if err != nil || len(cs) == 0 {
			t.Fatalf("«%s»: кандидаты: %+v (%v)", in, cs, err)
		}
		first := cs[0]
		if first.Kind != p.Kind || first.System != p.System ||
			first.Designation != p.Designation || first.String() != p.String() {
			t.Errorf("«%s»: первый кандидат %+v не совпадает с автодетектом %+v", in, first, p)
		}
	}
}

// Контракт ошибок перечисления — общий с автодетектом: канонизация
// и вердикты как есть, неразбираемое — autodetect_failed, неизвестный
// класс — валидация.
func TestParseDesignationCandidatesErrors(t *testing.T) {
	cases := []struct {
		input string
		kind  domain.Kind
		code  domain.Code
		msgID domain.MsgID
	}{
		{"МЛТ-0.5", "", domain.CodeInvalidDesignation, domain.MsgAutodetectFailed},
		{"TIP120", "", domain.CodeInvalidDesignation, domain.MsgAutodetectFailed},
		{"", "", domain.CodeInvalidDesignation, domain.MsgEmptyDesignation},
		{"KT315Б", "", domain.CodeInvalidDesignation, domain.MsgCanonicalAlphabetMix},
		{"КУ101", "", domain.CodeKindNotSupported, domain.MsgKindNotSupported},
		{"КФ102А", "", domain.CodeKindAmbiguous, domain.MsgKindAmbiguous},
		{"С2-33Н-0.125", "", domain.CodePowerSuffix, domain.MsgPowerSuffix},
		{"КТ315Б", domain.Kind("x"), domain.CodeValidationFailed, domain.MsgKindUnknown},
	}
	for _, tc := range cases {
		_, err := domain.ParseDesignationCandidates(tc.input, tc.kind)
		de, ok := domain.AsError(err)
		if !ok {
			t.Errorf("«%s»: ожидалась ошибка домена, получено %v", tc.input, err)
			continue
		}
		if de.Code != tc.code || de.MsgID != tc.msgID {
			t.Errorf("«%s»: %s/%s, ожидалось %s/%s", tc.input, de.Code, de.MsgID, tc.code, tc.msgID)
		}
	}
}
