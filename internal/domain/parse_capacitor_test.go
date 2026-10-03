package domain

import "testing"

func TestParseCapacitorGostValid(t *testing.T) {
	cases := []struct {
		input  string
		fields string
	}{
		{"К10-17Б", "prefix=К; adjustment=fixed; group=10; dev_number=17; letters=Б"},
		{"К50-35", "prefix=К; adjustment=fixed; group=50; dev_number=35"},
		{"К73-17", "prefix=К; adjustment=fixed; group=73; dev_number=17"},
		{"К78-2", "prefix=К; adjustment=fixed; group=78; dev_number=2"},
		{"К42-19", "prefix=К; adjustment=fixed; group=42; dev_number=19"},
		{"К10-47В", "prefix=К; adjustment=fixed; group=10; dev_number=47; letters=В"},
		{"К31-10", "prefix=К; adjustment=fixed; group=31; dev_number=10"},
		{"К52-1", "prefix=К; adjustment=fixed; group=52; dev_number=1"},
		{"К76-2", "prefix=К; adjustment=fixed; group=76; dev_number=2"},
		{"КТ4-25", "prefix=КТ; adjustment=preset; group=4; dev_number=25"},
		{"КТ1-5", "prefix=КТ; adjustment=preset; group=1; dev_number=5"},
		{"КП1-3", "prefix=КП; adjustment=variable; group=1; dev_number=3"},
		{"КН1-8", "prefix=КН; adjustment=fixed; group=1; dev_number=8"},
	}
	for _, tc := range cases {
		p, err := parseCapacitorGost(newScanner(tc.input), "")
		if err != nil {
			t.Errorf("«%s»: неожиданная ошибка: %v", tc.input, err)
			continue
		}
		if p.Kind != KindCapacitor || p.System != SystemGost || p.Designation != tc.input {
			t.Errorf("«%s»: %s/%s/%s", tc.input, p.Kind, p.System, p.Designation)
		}
		if got := p.String(); got != tc.fields {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, got, tc.fields)
		}
	}
}

func TestParseCapacitorGostMessages(t *testing.T) {
	cases := []struct{ input, want string }{
		{"К55-1", "designation «К55-1»: unknown capacitor group «55» (prefix К)"},
		{"К100-1", "designation «К100-1»: position 2: expected: two-digit group per the capacitor group table, got «100»"},
		{"КТ9-9", "designation «КТ9-9»: position 3: expected: группа по таблице подкласса, got «9»"},
		{"КН3-8", "designation «КН3-8»: position 3: expected: группа по таблице подкласса, got «3»"},
		{"К10", "designation «К10»: position 4: expected: hyphen, got end of designation"},
		{"К10-017", "designation «К10-017»: position 5: expected: development number without a leading zero (up to 3 digits), got «017»"},
		{"К10-17БМ", "designation «К10-17БМ»: position 8: expected end of designation, got «М»"},
	}
	for _, tc := range cases {
		_, err := parseCapacitorGost(newScanner(tc.input), "")
		if err == nil {
			t.Errorf("«%s»: ожидалась ошибка", tc.input)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("«%s»:\n got:  %s\n want: %s", tc.input, err.Error(), tc.want)
		}
	}
}

// Таблица групп подкласса К — перспективные и исторические (07 §5);
// пин-фиксация состава.
func TestCapGroupsPin(t *testing.T) {
	want := []int{10, 15, 21, 22, 23, 26, 31, 32, 40, 41, 42, 50, 51, 52, 53,
		58, 60, 61, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79}
	for _, g := range want {
		if !CapGroupKnown(g) {
			t.Errorf("группа %d отсутствует в таблице", g)
		}
	}
	for _, g := range []int{11, 20, 24, 25, 33, 54, 56, 57, 59, 62, 80, 99} {
		if CapGroupKnown(g) {
			t.Errorf("группа %d ошибочно известна", g)
		}
	}
}
