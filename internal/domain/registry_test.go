package domain_test

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Реестры классов и систем — код ⇄ будущие сиды (kinds, designation_systems,
// designation_system_kinds); пин-тест фиксирует состав и применимость
// (plan/03-data-model.md §1.1–1.2). Этап 2 синхронизирует сиды с этими
// таблицами.
func TestKindsPin(t *testing.T) {
	want := []domain.Kind{"transistor", "diode", "resistor", "capacitor"}
	if !slices.Equal(domain.Kinds(), want) {
		t.Errorf("реестр классов изменился: %v", domain.Kinds())
	}
	names := map[domain.Kind]string{
		domain.KindTransistor: "транзисторы",
		domain.KindDiode:      "диоды",
		domain.KindResistor:   "резисторы",
		domain.KindCapacitor:  "конденсаторы",
	}
	for k, name := range names {
		if !k.IsValid() {
			t.Errorf("класс %s не валиден", k)
		}
		if k.Name() != name {
			t.Errorf("класс %s: название %q", k, k.Name())
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
			t.Errorf("система %s не валидна", s)
		}
		if s.Name() == "" {
			t.Errorf("система %s без названия", s)
		}
		if s.Description() == "" {
			t.Errorf("система %s без описания", s)
		}
	}
	if domain.System("din").IsValid() {
		t.Error("посторонняя система ошибочно валидна")
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
		'Г': "германий", '1': "германий",
		'К': "кремний", '2': "кремний",
		'А': "соединения галлия", '3': "соединения галлия",
		'И': "соединения индия", '4': "соединения индия",
		'Д': "соединения карбида", '5': "соединения карбида",
		'П': "соединения прочих металлов", '6': "соединения прочих металлов",
	}
	for sym, name := range gost {
		m, ok := domain.GostMaterialBySymbol(sym)
		if !ok || m.Name != name {
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
		'A': "германий",
		'B': "кремний",
		'C': "арсенид галлия",
	}
	for sym, name := range pro {
		m, ok := domain.ProMaterialByLetter(sym)
		if !ok || m.Name != name {
			t.Errorf("буква pro %c: %+v", sym, m)
		}
	}
	if _, ok := domain.ProMaterialByLetter('R'); ok {
		t.Error("R — соединения без p-n-перехода, вне словаря классов модуля")
	}
	if _, ok := domain.MaterialByName("кремний"); !ok {
		t.Error("кремний отсутствует в словаре")
	}
	if _, ok := domain.MaterialByName("медь"); ok {
		t.Error("медь отсутствует в словаре материалов модуля")
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
	if !ok || f.Text != "кремний" || f.String() != "кремний" {
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
