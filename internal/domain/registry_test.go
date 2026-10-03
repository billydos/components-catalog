package domain_test

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Реестры классов и систем — код ⇄ будущие сиды (kinds, designation_systems,
// designation_system_kinds); пин-тест фиксирует состав и применимость
// (docs/plan/03-data-model.md §1.1–1.2). Этап 2 синхронизирует сиды с этими
// таблицами.
func TestKindsPin(t *testing.T) {
	want := []domain.Kind{"transistor", "diode", "resistor", "capacitor"}
	if !slices.Equal(domain.Kinds(), want) {
		t.Errorf("реестр классов изменился: %v", domain.Kinds())
	}
	for _, k := range want {
		if !k.IsValid() {
			t.Errorf("класс %s не валиден", k)
		}
	}
	if domain.Kind("thyristor").IsValid() {
		t.Error("тиристоры отсутствуют в стартовом реестре")
	}
}

func TestSystemsPin(t *testing.T) {
	want := []domain.System{"gost", "ost", "pro", "jedec", "jis", "series", "other"}
	if !slices.Equal(domain.Systems(), want) {
		t.Errorf("реестр систем изменился: %v", domain.Systems())
	}
	for _, s := range want {
		if !s.IsValid() {
			t.Errorf("system %s не валидна", s)
		}
	}
	if domain.System("din").IsValid() {
		t.Error("посторонняя system ошибочно валидна")
	}
}

// Применимость систем к классам — матрица 03 §1.1.
func TestSystemsForKind(t *testing.T) {
	cases := map[domain.Kind][]domain.System{
		domain.KindTransistor: {"gost", "pro", "jedec", "jis", "series", "other"},
		domain.KindDiode:      {"gost", "pro", "jedec", "jis", "series", "other"},
		domain.KindResistor:   {"gost", "ost", "series", "other"},
		domain.KindCapacitor:  {"gost", "series", "other"},
	}
	for kind, want := range cases {
		got := domain.SystemsForKind(kind)
		if !slices.Equal(got, want) {
			t.Errorf("класс %s:\n got:  %v\n want: %v", kind, got, want)
		}
	}
	if domain.SystemAppliesToKind(domain.SystemOst, domain.KindCapacitor) {
		t.Error("ost неприменим к конденсаторам")
	}
	if !domain.SystemAppliesToKind(domain.SystemGost, domain.KindCapacitor) {
		t.Error("gost применим к конденсаторам")
	}
}

// Единый словарь материалов gost/pro (03 §2.1; выверка — 07 §1–2).
func TestMaterials(t *testing.T) {
	gost := map[rune]string{
		'Г': "ge", '1': "ge",
		'К': "si", '2': "si",
		'А': "ga", '3': "ga",
		'И': "in", '4': "in",
		'Д': "sic", '5': "sic",
		'П': "other", '6': "other",
	}
	for sym, name := range gost {
		m, ok := domain.GostMaterialBySymbol(sym)
		if !ok || m.Code != name {
			t.Errorf("символ %c: %+v", sym, m)
		}
	}
	if _, ok := domain.GostMaterialBySymbol('Т'); ok {
		t.Error("Т не символ материала")
	}
	if _, ok := domain.GostMaterialBySymbol('7'); ok {
		t.Error("7 не символ материала")
	}
	pro := map[rune]string{
		'A': "ge",
		'B': "si",
		'C': "gaas",
	}
	for sym, name := range pro {
		m, ok := domain.ProMaterialByLetter(sym)
		if !ok || m.Code != name {
			t.Errorf("буква pro %c: %+v", sym, m)
		}
	}
	if _, ok := domain.ProMaterialByLetter('R'); ok {
		t.Error("R — соединения без p-n-перехода, вне словаря классов модуля")
	}
	if _, ok := domain.MaterialByCode("si"); !ok {
		t.Error("si отсутствует в словаре")
	}
	if _, ok := domain.MaterialByCode("copper"); ok {
		t.Error("copper отсутствует в словаре материалов модуля")
	}
	if got := len(domain.Materials()); got != 7 {
		t.Errorf("словарь материалов: %d записей", got)
	}
}

// Поля разбора: поиск по коду и текстовое представление.
func TestParsedDesignationFields(t *testing.T) {
	p, err := domain.ParseDesignation("КТ315Б")
	if err != nil {
		t.Fatalf("КТ315Б: %v", err)
	}
	f, ok := p.FieldByName("material")
	if !ok || f.Text != "si" || f.String() != "si" {
		t.Errorf("поле material: %+v", f)
	}
	if _, ok := p.FieldByName("series"); ok {
		t.Error("поля series нет в gost-разборе")
	}
	f, ok = p.FieldByName("dev_number")
	if !ok || !f.IsNum || f.String() != "315" {
		t.Errorf("поле dev_number: %+v", f)
	}
	if got := domain.TextField("x", "y").String(); got != "y" {
		t.Errorf("текстовое поле: %s", got)
	}
}

// Реестр полей разбора: разряды и полнота. Фильтры и сортировка поиска
// (REST/CLI) используют реестр домена — состав обязан быть синхронен
// полям, которые создают парсеры.
func TestDesignationFieldRegistry(t *testing.T) {
	for code, numeric := range map[string]bool{
		"material": false, "subclass": false, "letters": false,
		"prefix": false, "family": false, "series": false,
		"assembly": true, "feature": true, "dev_number": true,
		"modification": true, "chip": true, "junctions": true,
		"group": true, "power": true,
	} {
		n, known := domain.NumericDesignationField(code)
		if !domain.KnownDesignationField(code) || !known || n != numeric {
			t.Errorf("поле %s: known=%v numeric=%v, ожидалось numeric=%v", code, known, n, numeric)
		}
	}
	if _, known := domain.NumericDesignationField("bogus"); known {
		t.Error("bogus не должно быть полем разбора")
	}
	// Поля реальных разборов всех систем — только из реестра и с тем же
	// разрядом.
	for _, d := range []string{
		"КТ315Б", "2Т914А-1", "ГТ109Г", "КДС111В", "2Т805А", "С2-33Н", "СП3-19А",
		"Р1-4", "К50-35", "К10-17Б", "BC547B", "AD161", "BZX85C5V1", "2N2222A",
		"1N4148", "2SA1015", "2SK1058", "МП39", "ПЭВ-10", "МЛТ-0.5", "TIP120",
	} {
		p, err := domain.ParseDesignation(d)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		for _, f := range p.Fields {
			n, known := domain.NumericDesignationField(f.Name)
			if !known {
				t.Errorf("%s: поле %s вне реестра полей разбора", d, f.Name)
				continue
			}
			if n != f.IsNum {
				t.Errorf("%s: поле %s: реестр numeric=%v, разбор IsNum=%v", d, f.Name, n, f.IsNum)
			}
		}
	}
}
