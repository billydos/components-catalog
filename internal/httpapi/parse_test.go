package httpapi

import (
	"net/http"
	"testing"
)

// GET /api/v1/parse: разбор с автодетектом/явной
// системой, кандидаты строгих систем, ошибки параметров — дословные
// тексты транспортных сообщений (контракт).

func TestParseAutodetect(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	// Канонизация кириллицы: нижний регистр входа → канонический ключ.
	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/parse?designation="+esc("кт315б"), "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp := decode[parseResponseJSON](t, body)
	if resp.Kind != "transistor" || resp.System != "gost" || resp.Designation != "КТ315Б" {
		t.Fatalf("основной разбор: %+v", resp)
	}
	fields := map[string]fieldJSON{}
	for _, f := range resp.Fields {
		fields[f.Name] = f
	}
	if fields["material"].Text == nil || *fields["material"].Text != "si" ||
		fields["dev_number"].Num == nil || *fields["dev_number"].Num != 315 ||
		fields["base"].Text == nil || *fields["base"].Text != "КТ315" {
		t.Fatalf("поля разбора: %+v", resp.Fields)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Kind != "transistor" ||
		resp.Candidates[0].System != "gost" {
		t.Fatalf("кандидаты: %+v", resp.Candidates)
	}
	// Первый кандидат при автодетекте совпадает с основным результатом.
	if resp.Candidates[0].Kind != resp.Kind || resp.Candidates[0].System != resp.System {
		t.Fatalf("первый кандидат ≠ основной результат: %+v", resp)
	}
}

func TestParseExplicitSystemKind(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodGet,
		apiPrefix+"/parse?designation=2N2222A&system=jedec&kind=transistor", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp := decode[parseResponseJSON](t, body)
	if resp.Kind != "transistor" || resp.System != "jedec" {
		t.Fatalf("явная система: %+v", resp)
	}

	// other — без грамматики: только канонизация, класс обязателен;
	// кандидаты строгих грамматик пусты.
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/parse?designation="+esc("МЛТ-0.5")+"&system=other&kind=resistor", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp = decode[parseResponseJSON](t, body)
	if resp.Kind != "resistor" || resp.System != "other" || resp.Designation != "МЛТ-0.5" {
		t.Fatalf("other: %+v", resp)
	}
	if len(resp.Fields) != 0 || len(resp.Candidates) != 0 {
		t.Fatalf("other: поля/кандидаты должны быть пусты: %+v", resp)
	}

	// Неоднозначный класс Ф разрешается явным kind.
	status, body, _ = do(t, srv, http.MethodGet,
		apiPrefix+"/parse?designation="+esc("КФ102А")+"&kind=transistor", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp = decode[parseResponseJSON](t, body)
	if resp.Kind != "transistor" || resp.System != "gost" || len(resp.Candidates) != 1 {
		t.Fatalf("КФ102А с kind: %+v", resp)
	}
}

// Кандидаты: грамматики строгих систем разделены алфавитами и структурой
// (кириллица gost/ост, латиница pro, цифра-признак jedec/jis; близкие
// префиксы КТ/КС расходятся позицией дефиса), поэтому перечень сегодня
// одноэлементный — контракт допускает несколько (фиксированный порядок
// автодетекта). Проверяем одноэлементность и совпадение с основным
// результатом, в том числе у близкой пары КТ4-25 (конденсатор).
func TestParseCandidates(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	status, body, _ := do(t, srv, http.MethodGet, apiPrefix+"/parse?designation=BC547", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp := decode[parseResponseJSON](t, body)
	if resp.Kind != "transistor" || resp.System != "pro" {
		t.Fatalf("основной разбор: %+v", resp)
	}
	want := []parseCandidateJSON{{Kind: "transistor", System: "pro"}}
	if len(resp.Candidates) != len(want) {
		t.Fatalf("кандидаты: %+v", resp.Candidates)
	}
	for i, w := range want {
		if resp.Candidates[i] != w {
			t.Fatalf("кандидат %d: %+v, ожидался %+v", i, resp.Candidates[i], w)
		}
	}
	if resp.Candidates[0].Kind != resp.Kind || resp.Candidates[0].System != resp.System {
		t.Fatalf("первый кандидат ≠ основной результат: %+v", resp)
	}

	// Сокращённая форма JIS: основной разбор — восстановленная полная
	// запись, кандидат — та же пара класс/система.
	status, body, _ = do(t, srv, http.MethodGet, apiPrefix+"/parse?designation=C1815", "")
	if status != http.StatusOK {
		t.Fatalf("статус: %d, тело %s", status, body)
	}
	resp = decode[parseResponseJSON](t, body)
	if resp.System != "jis" || resp.Designation != "2SC1815" ||
		len(resp.Candidates) != 1 || resp.Candidates[0].System != "jis" {
		t.Fatalf("C1815: %+v", resp)
	}
}

func TestParseErrors(t *testing.T) {
	_, srv := newTestAPI(t, Config{})
	cases := []struct {
		query   string
		code    string
		message string
	}{
		{"", "validation_failed", "query parameter designation is not set"},
		{"designation=", "validation_failed", "query parameter designation is not set"},
		{"system=other", "validation_failed", "query parameter designation is not set"},
		{"designation=" + esc("КТ315Б") + "&system=bogus", "validation_failed",
			"unknown designation system «bogus»"},
		{"designation=" + esc("КТ315Б") + "&kind=bogus", "validation_failed",
			"unknown device kind «bogus»"},
		{"designation=" + esc("КТ315Б") + "&designation=BC547B", "validation_failed",
			"query parameter «designation» is set more than once"},
		// Неразбираемое обозначение — отказ автодетекта (дословный текст).
		{"designation=" + esc("МЛТ-0.5"), "invalid_designation",
			"designation «МЛТ-0.5»: designation system not recognized (supported: gost, ost, pro, jedec, jis)"},
		// Ошибка канонизации — как есть.
		{"designation=" + esc("KT315Б"), "invalid_designation",
			"designation «KT315Б»: position 6: mixed alphabets (Cyrillic and Latin), got «Б»"},
		// other без класса — kind_required (400 в контексте query).
		{"designation=JANTX2N3055&system=other", "kind_ambiguous",
			"designation system other does not determine the device kind; specify the kind explicitly"},
		// Явный класс противоречит обозначению — designation_mismatch.
		{"designation=" + esc("КТ315Б") + "&kind=diode", "designation_mismatch",
			"designation «КТ315Б» belongs to kind transistor, kind diode was given"},
		// Вердикты грамматик — 400 в контексте query.
		{"designation=" + esc("КУ101"), "kind_not_supported",
			"designation belongs to a kind not supported by the module"},
		{"designation=" + esc("С2-33Н-0.125"), "power_suffix_not_supported",
			"designation «С2-33Н-0.125»: numeric tail after letters is a power suffix, an element of the full designation; use designation «С2-33Н» and the variants section"},
		// Неоднозначный класс Ф без подсказки.
		{"designation=" + esc("КФ102А"), "kind_ambiguous",
			"device kind is not determined unambiguously by the designation; specify the kind explicitly"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			path := apiPrefix + "/parse"
			if tc.query != "" {
				path += "?" + tc.query
			}
			status, _, eb := do(t, srv, http.MethodGet, path, "")
			wantError(t, status, eb, http.StatusBadRequest, tc.code, tc.message)
		})
	}
}
