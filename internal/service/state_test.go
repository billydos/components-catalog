package service

import (
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// TestSameState — каноническое сравнение состояний: произвольные строки
// (имена производителей, метки исполнений, заметки аналогов, тексты
// атрибутов и значений) не должны склеивать разные состояния — исход
// Skipped возможен только для полностью равных состояний. Множества
// (поля, атрибуты, условия, производители, аналоги) нечувствительны
// к порядку; порядок значений в секциях и исполнениях значим.
func TestSameState(t *testing.T) {
	group := func(vals ...stateValue) map[string][]stateValue {
		return map[string][]stateValue{"g": vals}
	}
	exact := func(p string, v float64, conds ...catalog.ConditionValue) stateValue {
		return stateValue{Parameter: p, Exact: &v, Conditions: conds}
	}
	text := func(p, v string) stateValue {
		return stateValue{Parameter: p, Text: &v}
	}
	cases := []struct {
		name string
		a, b *deviceState
		want bool
	}{
		// Коллизии crafted-строк: разные состояния.
		{
			name: "производители: имя содержит разделитель записей",
			a:    &deviceState{manufacturers: []string{"A", "B"}},
			b:    &deviceState{manufacturers: []string{"A;M:B"}},
			want: false,
		},
		{
			name: "исполнения: метка содержит скобки секции",
			a:    &deviceState{variants: []stateVariant{{Label: "x"}, {Label: "y"}}},
			b:    &deviceState{variants: []stateVariant{{Label: "x{}V1:y"}}},
			want: false,
		},
		{
			name: "аналоги: заметка содержит фрагмент второй ссылки",
			a: &deviceState{analogs: []stateAnalog{
				{Designation: "A", Note: "B"}, {Designation: "C", Note: "D"},
			}},
			b:    &deviceState{analogs: []stateAnalog{{Designation: "A", Note: "B;N:C|D"}}},
			want: false,
		},
		{
			name: "атрибуты: текст содержит фрагмент второго атрибута",
			a: &deviceState{attributes: []catalog.AttributeValue{
				attrText("x", "y"), attrText("z", "w"),
			}},
			b:    &deviceState{attributes: []catalog.AttributeValue{attrText("x", "y;A:z=t:w")}},
			want: false,
		},
		{
			name: "значения: текст содержит фрагмент второго значения",
			a:    &deviceState{groupValues: group(text("P", "a"), exact("Q", 1))},
			b:    &deviceState{groupValues: group(text("P", "a,)Q(x=1"))},
			want: false,
		},
		{
			name: "значение: текст и число не равны",
			a:    &deviceState{groupValues: group(text("P", "1"))},
			b:    &deviceState{groupValues: group(exact("P", 1))},
			want: false,
		},
		{
			name: "атрибут: пустой текст и отсутствие значения различаются",
			a:    &deviceState{attributes: []catalog.AttributeValue{attrText("x", "")}},
			b:    &deviceState{attributes: []catalog.AttributeValue{{Attribute: "x"}}},
			want: false,
		},
		{
			name: "подмножество производителей не равно множеству",
			a:    &deviceState{manufacturers: []string{"A"}},
			b:    &deviceState{manufacturers: []string{"A", "B"}},
			want: false,
		},
		// Множества: порядок не значим.
		{
			name: "поля: порядок не значим",
			a: &deviceState{fields: []domain.Field{
				domain.TextField("material", "si"), domain.TextField("subclass", "bjt"),
			}},
			b: &deviceState{fields: []domain.Field{
				domain.TextField("subclass", "bjt"), domain.TextField("material", "si"),
			}},
			want: true,
		},
		{
			name: "атрибуты: порядок не значим",
			a:    &deviceState{attributes: []catalog.AttributeValue{attrText("x", "1"), attrNum("y", 2)}},
			b:    &deviceState{attributes: []catalog.AttributeValue{attrNum("y", 2), attrText("x", "1")}},
			want: true,
		},
		{
			name: "условия: порядок не значим",
			a:    &deviceState{groupValues: group(exact("P", 5, cond("Uke", 10), cond("Ik", 1)))},
			b:    &deviceState{groupValues: group(exact("P", 5, cond("Ik", 1), cond("Uke", 10)))},
			want: true,
		},
		{
			name: "производители: порядок не значим",
			a:    &deviceState{manufacturers: []string{"B", "A"}},
			b:    &deviceState{manufacturers: []string{"A", "B"}},
			want: true,
		},
		{
			name: "аналоги: порядок не значим",
			a: &deviceState{analogs: []stateAnalog{
				{Designation: "КТ315Б", Note: "полный"}, {Designation: "МП39", Note: "частичный"},
			}},
			b: &deviceState{analogs: []stateAnalog{
				{Designation: "МП39", Note: "частичный"}, {Designation: "КТ315Б", Note: "полный"},
			}},
			want: true,
		},
		{
			name: "пустые секции равны отсутствующим",
			a:    &deviceState{groupValues: map[string][]stateValue{}, variants: []stateVariant{}},
			b:    &deviceState{},
			want: true,
		},
		// Порядок значений и исполнений значим (sort_order).
		{
			name: "порядок значений в группе значим",
			a:    &deviceState{groupValues: group(exact("P", 1), exact("Q", 2))},
			b:    &deviceState{groupValues: group(exact("Q", 2), exact("P", 1))},
			want: false,
		},
		{
			name: "порядок исполнений значим",
			a:    &deviceState{variants: []stateVariant{{Label: "0.125 Вт"}, {Label: "1 Вт"}}},
			b:    &deviceState{variants: []stateVariant{{Label: "1 Вт"}, {Label: "0.125 Вт"}}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameState(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameState = %v, ожидалось %v", got, tc.want)
			}
		})
	}
}
