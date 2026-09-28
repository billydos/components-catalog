package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestTryParseDesignation_ValidDesignation_ParsesFields(t *testing.T) {
	cases := []struct {
		text         string
		material     rune
		subclass     rune
		isAssembly   bool
		feature      int
		number       int
		letters      string
		modification *int
		chipVariant  *int
	}{
		{"КТ315Б", 'К', 'Т', false, 3, 15, "Б", nil, nil},
		{"2Т914А-1", '2', 'Т', false, 9, 14, "А", nil, Ptr(1)},
		{"ГТ402Ж", 'Г', 'Т', false, 4, 2, "Ж", nil, nil},
		{"1Т402Ж", '1', 'Т', false, 4, 2, "Ж", nil, nil},
		{"КП303А", 'К', 'П', false, 3, 3, "А", nil, nil},
		{"КТ3102АМ", 'К', 'Т', false, 3, 102, "АМ", nil, nil},
		{"КТ815Г1", 'К', 'Т', false, 8, 15, "Г", Ptr(1), nil},
		{"ГТС398А", 'Г', 'Т', true, 3, 98, "А", nil, nil},
		{"АТ312А-5", 'А', 'Т', false, 3, 12, "А", nil, Ptr(5)},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			transistor, message, ok := TryParseDesignation(tc.text)
			if !ok {
				t.Fatalf("ожидался успешный разбор, получена ошибка: %s", message)
			}
			if transistor.Material != tc.material {
				t.Errorf("Material = %c, ожидалось %c", transistor.Material, tc.material)
			}
			if transistor.Subclass != tc.subclass {
				t.Errorf("Subclass = %c, ожидалось %c", transistor.Subclass, tc.subclass)
			}
			if transistor.IsAssembly != tc.isAssembly {
				t.Errorf("IsAssembly = %v, ожидалось %v", transistor.IsAssembly, tc.isAssembly)
			}
			if transistor.Feature != tc.feature {
				t.Errorf("Feature = %d, ожидалось %d", transistor.Feature, tc.feature)
			}
			if transistor.DevelopmentNumber != tc.number {
				t.Errorf("DevelopmentNumber = %d, ожидалось %d", transistor.DevelopmentNumber, tc.number)
			}
			if transistor.Letters != tc.letters {
				t.Errorf("Letters = %q, ожидалось %q", transistor.Letters, tc.letters)
			}
			assertOptionalInt(t, "Modification", transistor.Modification, tc.modification)
			assertOptionalInt(t, "ChipVariant", transistor.ChipVariant, tc.chipVariant)
		})
	}
}

func TestParseDesignation_RoundTrip_NameEqualsNormalizedDesignation(t *testing.T) {
	cases := [][2]string{
		{"КТ315Б", "КТ315Б"},
		{"кт315б", "КТ315Б"},
		{"  КТ315Б  ", "КТ315Б"},
		{"2Т914А–1", "2Т914А-1"},
		{"2Т914А−1", "2Т914А-1"},
		{"КП303А", "КП303А"},
		{"ГТ402Ж", "ГТ402Ж"},
		{"ГТС398А", "ГТС398А"},
		{"КТ815Г1", "КТ815Г1"},
	}
	for _, tc := range cases {
		t.Run(tc[0], func(t *testing.T) {
			transistor, err := ParseDesignation(tc[0])
			if err != nil {
				t.Fatalf("ожидался успешный разбор, получена ошибка: %s", err)
			}
			if got := transistor.Name(); got != tc[1] {
				t.Errorf("Name() = %q, ожидалось %q", got, tc[1])
			}
		})
	}
}

func TestTryParseDesignation_InvalidDesignation_ReturnsFalseWithMessage(t *testing.T) {
	cases := [][2]string{
		{"", "обозначение пустое"},
		{"   ", "обозначение пустое"},
		{"5Т315Б", "позиция 1: ожидался тип материала"},
		{"ХТ315Б", "позиция 1: ожидался тип материала"},
		{"KT315Б", "позиция 1: ожидался тип материала"},
		{"КМ315Б", "позиция 2: ожидался подкласс"},
		{"КТ015Б", "позиция 3: ожидался характерный эксплуатационный признак"},
		{"КТ3АБ", "порядковый номер разработки — от 2 до 3 цифр"},
		{"КТ12345Б", "от 2 до 3 цифр"},
		{"КТ1015Б", "номер разработки из трёх цифр не может начинаться с нуля"},
		{"КТ100Б", "номер разработки не может быть нулём"},
		{"КТ315", "отсутствует буква классификации по параметрам"},
		{"КТ315-2", "ожидалась буква классификации по параметрам"},
		{"КТ315БВГ", "не более двух букв классификации по параметрам"},
		{"КТ315Б0", "модификация — цифра от 1 до 9"},
		{"КТ315Б:", "неожидаемый символ"},
		{"КТ315Б-0", "бескорпусное исполнение — дефис и цифра от 1 до 6"},
		{"КТ315Б-7", "бескорпусное исполнение — дефис и цифра от 1 до 6"},
		{"КТ315Б-", "бескорпусное исполнение — дефис и цифра от 1 до 6"},
		{"КТ315Б-1В", "лишние символы после бескорпусного исполнения"},
	}
	for _, tc := range cases {
		t.Run(tc[0], func(t *testing.T) {
			_, message, ok := TryParseDesignation(tc[0])
			if ok {
				t.Fatal("ожидалась ошибка разбора")
			}
			if message == "" {
				t.Error("текст ошибки пуст")
			}
			if !containsSubstring(message, tc[1]) {
				t.Errorf("текст ошибки %q не содержит %q", message, tc[1])
			}
		})
	}
}

func TestTryParseDesignation_Empty_ReturnsEmptyDesignationError(t *testing.T) {
	_, message, ok := TryParseDesignation("")
	if ok {
		t.Fatal("ожидалась ошибка разбора")
	}
	if message != "обозначение пустое" {
		t.Errorf("текст ошибки = %q, ожидалось %q", message, "обозначение пустое")
	}
}

func TestParseDesignation_InvalidDesignation_ReturnsUserError(t *testing.T) {
	_, err := ParseDesignation("ХТ315Б")
	var userError *UserError
	if !errors.As(err, &userError) {
		t.Fatalf("ожидался *UserError, получено %T: %v", err, err)
	}
}

func assertOptionalInt(t *testing.T, name string, got, want *int) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("%s = %v, ожидалось %v", name, got, want)
	case *got != *want:
		t.Errorf("%s = %d, ожидалось %d", name, *got, *want)
	}
}

func containsSubstring(s, fragment string) bool {
	return strings.Contains(s, fragment)
}
