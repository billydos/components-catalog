package domain

import "testing"

// ParseSeriesWithRegistry — разбор series над внешним реестром (данные
// каталога series_families): семейство, отсутствующее в стартовом реестре
// домена, разбирается той же грамматикой хвоста.
func TestParseSeriesWithRegistry(t *testing.T) {
	registry := []SeriesFamily{
		{Series: "ФГТ", Kind: KindTransistor},
		{Series: "МП", Kind: KindTransistor},
	}
	p, err := ParseSeriesWithRegistry("ФГТ-5", registry, "")
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if p.Kind != KindTransistor || p.System != SystemSeries || p.Designation != "ФГТ-5" {
		t.Fatalf("результат: %+v", p)
	}
	if f, ok := p.FieldByName("series"); !ok || f.Text != "ФГТ" {
		t.Fatalf("поле series: %+v", f)
	}
	if f, ok := p.FieldByName("dev_number"); !ok || f.Num != 5 {
		t.Fatalf("поле dev_number: %+v", f)
	}

	// Конфликт с явным классом — designation_mismatch.
	if _, err := ParseSeriesWithRegistry("ФГТ-5", registry, KindDiode); err == nil {
		t.Fatal("ожидался конфликт класса")
	}

	// Неизвестное семейство — ошибка с перечнем поддерживаемых.
	_, err = ParseSeriesWithRegistry("QQQ-1", registry, "")
	if de, ok := AsError(err); !ok || de.Code != CodeInvalidDesignation {
		t.Fatalf("ожидалась invalid_designation, получено %v", err)
	}

	// Самое длинное совпадение префикса: МП раньше отсутствующего «М».
	p, err = ParseSeriesWithRegistry("МП39", registry, "")
	if err != nil || p.Designation != "МП39" {
		t.Fatalf("МП39: %+v err=%v", p, err)
	}
}
