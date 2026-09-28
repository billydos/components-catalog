package domain

import "testing"

// TestConditionAccessors_CoverAllKeys — пин-тест пары доступов условий:
// ConditionWithValue обязан записать значение в поле своего ключа для каждого
// ключа каталога, ConditionValueOf — прочитать его. Ключ, добавленный
// в AllConditionKeys без пары доступов, падает здесь паникой; расходящиеся
// свитчи (значение записано не в то поле) — ошибкой assertions.
func TestConditionAccessors_CoverAllKeys(t *testing.T) {
	for _, key := range AllConditionKeys {
		parameter := ConditionWithValue(ElectricalParameter{}, key, 7)
		if value := ConditionValueOf(parameter, key); value == nil || *value != 7 {
			t.Errorf("ConditionWithValue/ConditionValueOf расходятся для ключа %s", key)
		}
		for _, other := range AllConditionKeys {
			if other != key && ConditionValueOf(parameter, other) != nil {
				t.Errorf("ConditionWithValue(%s) затронул чужое поле %s", key, other)
			}
		}
	}
}
