package domain

import (
	"strings"
	"testing"
)

// Инвариант реестра series (03 §2.4): обозначения семейств не разбираются
// строгими systemми; автодетект приводит их к series с классом семейства.
func TestSeriesRegistryInvariant(t *testing.T) {
	strict := map[string]func(*scanner, Kind) (ParsedDesignation, error){
		"gost (полупроводники)": parseGostSemiconductor,
		"pro":                   parsePro,
		"jedec":                 parseJedec,
		"jis":                   parseJis,
		"gost (резисторы)":      parseResistorGost,
		"ost":                   parseResistorOst,
		"gost (конденсаторы)":   parseCapacitorGost,
	}
	for _, family := range SeriesFamilies() {
		for _, example := range family.Examples {
			for name, parse := range strict {
				if _, err := parse(newScanner(example), ""); err == nil {
					t.Errorf("инвариант нарушен: «%s» (семейство %s) разбирается системой %s",
						example, family.Series, name)
				}
			}
			p, err := ParseDesignation(example)
			if err != nil {
				t.Errorf("«%s»: автодетект: %v", example, err)
				continue
			}
			if p.System != SystemSeries || p.Kind != family.Kind {
				t.Errorf("«%s»: автодетект дал %s/%s, ожидалось series/%s",
					example, p.System, p.Kind, family.Kind)
			}
			if f, ok := p.FieldByName("series"); !ok || f.Text != family.Series {
				t.Errorf("«%s»: семейство поля разбора %q, ожидалось %s", example, f.Text, family.Series)
			}
		}
	}
}

// Коды семейств уникальны в пределах класса; хвостовые семантики заданы.
func TestSeriesRegistryShape(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range SeriesFamilies() {
		key := string(f.Kind) + "\x00" + f.Series
		if seen[key] {
			t.Errorf("дубликат семейства %s для класса %s", f.Series, f.Kind)
		}
		seen[key] = true
		if !f.Kind.IsValid() {
			t.Errorf("семейство %s: неизвестный класс %s", f.Series, f.Kind)
		}
		if f.Series != strings.ToUpper(f.Series) {
			t.Errorf("семейство %s не в верхнем регистре", f.Series)
		}
	}
	// Самое длинное совпадение префикса: ПЭВ раньше ПЭ, ОМЛТ раньше МЛТ.
	f, ok := matchSeriesFamily("ПЭВ-10")
	if !ok {
		t.Fatal("ПЭВ-10 не сопоставлен")
	}
	if f.Series != "ПЭВ" {
		t.Errorf("ПЭВ-10 сопоставлен с %s", f.Series)
	}
	f, _ = matchSeriesFamily("ПЭ-10")
	if f.Series != "ПЭ" {
		t.Errorf("ПЭ-10 сопоставлен с %s", f.Series)
	}
	f, _ = matchSeriesFamily("ОМЛТ-0.5")
	if f.Series != "ОМЛТ" {
		t.Errorf("ОМЛТ-0.5 сопоставлен с %s", f.Series)
	}
}

func TestParseSeriesPowerTail(t *testing.T) {
	cases := []struct {
		input string
		power string
	}{
		{"МЛТ-0.5", "series=МЛТ; power=0.5"},
		{"МЛТ-0.125", "series=МЛТ; power=0.125"},
		{"МЛТ-1", "series=МЛТ; power=1"},
		{"МЛТ-2", "series=МЛТ; power=2"},
		{"ВС-0.5", "series=ВС; power=0.5"},
		{"ПЭВ-10", "series=ПЭВ; power=10"},
		{"ПЭВ-25", "series=ПЭВ; power=25"},
		{"КИМ-0.05", "series=КИМ; power=0.05"},
	}
	for _, tc := range cases {
		p, err := ParseDesignation(tc.input)
		if err != nil {
			t.Errorf("«%s»: %v", tc.input, err)
			continue
		}
		if got := p.String(); got != tc.power {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.power)
		}
		if p.Kind != KindResistor || p.System != SystemSeries {
			t.Errorf("«%s»: %s/%s", tc.input, p.Kind, p.System)
		}
	}
}

func TestParseSeriesWeakTail(t *testing.T) {
	cases := []struct {
		input  string
		kind   Kind
		fields string
	}{
		{"МП39", KindTransistor, "series=МП; dev_number=39"},
		{"МП41А", KindTransistor, "series=МП; dev_number=41; letters=А"},
		{"П13", KindTransistor, "series=П; dev_number=13"},
		{"П214", KindTransistor, "series=П; dev_number=214"},
		{"Д2Б", KindDiode, "series=Д; dev_number=2; letters=Б"},
		{"Д7Ж", KindDiode, "series=Д; dev_number=7; letters=Ж"},
		{"Д814А", KindDiode, "series=Д; dev_number=814; letters=А"},
		{"ДГ-Ц8", KindDiode, "series=ДГ; dev_number=8; letters=Ц"},
		{"OC44", KindTransistor, "series=OC; dev_number=44"},
		{"TIP120", KindTransistor, "series=TIP; dev_number=120"},
		{"MPSA42", KindTransistor, "series=MPSA; dev_number=42"},
		{"RC05", KindResistor, "series=RC; dev_number=5"},
		{"RN55", KindResistor, "series=RN; dev_number=55"},
		{"M55342K06B", KindResistor, "series=M55342; dev_number=6; letters=KB"},
		{"KNP-100", KindResistor, "series=KNP; dev_number=100"},
		{"CFR-25", KindResistor, "series=CFR; dev_number=25"},
		{"КСО-2", KindCapacitor, "series=КСО; dev_number=2"},
		{"КМ-4", KindCapacitor, "series=КМ; dev_number=4"},
		{"ЭТО-1", KindCapacitor, "series=ЭТО; dev_number=1"},
		{"БМ-2", KindCapacitor, "series=БМ; dev_number=2"},
		{"МБГЧ-1", KindCapacitor, "series=МБГЧ; dev_number=1"},
		{"МБМ", KindCapacitor, "series=МБМ"},
		{"КПК-МН", KindCapacitor, "series=КПК; letters=МН"},
	}
	for _, tc := range cases {
		p, err := ParseDesignation(tc.input)
		if err != nil {
			t.Errorf("«%s»: %v", tc.input, err)
			continue
		}
		if p.Kind != tc.kind || p.System != SystemSeries {
			t.Errorf("«%s»: %s/%s", tc.input, p.Kind, p.System)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
	}
}

func TestParseSeriesMessages(t *testing.T) {
	cases := []struct {
		input string
		kind  Kind
		want  string
	}{
		{"МЛТ", KindResistor,
			"designation «МЛТ»: position 4: expected: power tail after the hyphen (e.g. -0.5), got end of designation"},
		{"МЛТ-А", KindResistor,
			"designation «МЛТ-А»: position 5: expected: positive nominal power number, got «А»"},
		{"КМ4А5", KindCapacitor,
			"designation «КМ4А5»: position 5: expected: family tail (digits and letters), got «5»"},
		// Конфликт класса — designation_mismatch (симметрично строгим
		// systemм), а не «неизвестное семейство».
		{"МЛТ-0.5", KindCapacitor,
			"designation «МЛТ-0.5» belongs to kind resistor, kind capacitor was given"},
		{"КМ-4А5", KindCapacitor,
			"designation «КМ-4А5»: position 6: expected: family tail (digits and letters), got «5»"},
		{"Д99999999999999999999", KindDiode,
			"designation «Д99999999999999999999»: position 2: expected: number in the family tail (up to 5 digits), got «99999999999999999999»"},
		{"МЛТ-0", KindResistor,
			"designation «МЛТ-0»: position 5: expected: positive nominal power number, got «0»"},
		{"МЛТ-1А", KindResistor,
			"designation «МЛТ-1А»: position 6: expected end of designation, got «А»"},
		{"МЛТ-0.", KindResistor,
			"designation «МЛТ-0.»: position 7: expected: digits of the fractional part of the power, got end of designation"},
		{"КСО-", KindCapacitor,
			"designation «КСО-»: position 5: expected: family tail (digits and letters), got end of designation"},
	}
	for _, tc := range cases {
		_, err := parseSeries(mustCanonical(t, tc.input), tc.kind)
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

func mustCanonical(t *testing.T, s string) string {
	t.Helper()
	c, err := Canonicalize(s)
	if err != nil {
		t.Fatalf("канонизация «%s»: %v", s, err)
	}
	return c
}
