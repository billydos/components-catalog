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

// Единый словарь подклассов: отображения букв систем сходятся к кодам,
// обратных расщеплений нет (03 §2; выверка — 07 §4).
func TestSubclasses(t *testing.T) {
	gost := map[rune]string{
		'Т': "bjt", 'П': "fet", 'Д': "rectifier", 'Ц': "rectifier",
		'С': "zener", 'В': "varicap", 'А': "detector", 'И': "tunnel",
		'Г': "generator", 'Л': "led", 'Ф': "photo",
	}
	for sym, code := range gost {
		got, ok := domain.GostSubclassBySymbol(sym)
		if !ok || got != code {
			t.Errorf("gost %c: %q,%v", sym, got, ok)
		}
	}
	pro := map[rune]string{
		'C': "bjt", 'D': "bjt", 'F': "bjt", 'L': "bjt", 'S': "bjt", 'U': "bjt",
		'A': "signal", 'B': "varicap", 'E': "tunnel", 'H': "magnetic",
		'P': "photo", 'Q': "led", 'X': "multiplier", 'Y': "rectifier", 'Z': "zener",
	}
	for sym, code := range pro {
		got, ok := domain.ProSubclassByLetter(sym)
		if !ok || got != code {
			t.Errorf("pro %c: %q,%v", sym, got, ok)
		}
	}
	jis := map[rune]string{
		'A': "bjt", 'B': "bjt", 'C': "bjt", 'D': "bjt",
		'J': "fet", 'K': "fet", 'H': "ujt", 'T': "avalanche",
		'F': "thyristor", 'M': "triac", 'E': "rectifier", 'R': "rectifier",
		'S': "signal", 'Z': "zener", 'V': "varicap", 'G': "gunn", 'Q': "led",
	}
	for sym, code := range jis {
		got, ok := domain.JisSubclassByLetter(sym)
		if !ok || got != code {
			t.Errorf("jis %c: %q,%v", sym, got, ok)
		}
	}
	if _, ok := domain.SubclassByCode("bjt"); !ok {
		t.Error("bjt отсутствует в словаре подклассов")
	}
	if _, ok := domain.SubclassByCode("copper"); ok {
		t.Error("copper отсутствует в словаре подклассов")
	}
	if got := len(domain.Subclasses()); got != 18 {
		t.Errorf("словарь подклассов: %d записей", got)
	}
}

// Словарь способов подстройки и отображения семейств/подклассов
// резисторных и конденсаторных грамматик.
func TestAdjustments(t *testing.T) {
	for code, want := range map[string]string{
		"fixed": "fixed", "variable": "variable", "preset": "preset",
	} {
		a, ok := domain.AdjustmentByCode(code)
		if !ok || a.Code != want {
			t.Errorf("словарь подстройки: %q → %+v", code, a)
		}
	}
	if _, ok := domain.AdjustmentByCode("tunable"); ok {
		t.Error("tunable отсутствует в словаре подстройки")
	}
	for family, want := range map[string]string{
		"С": "fixed", "СП": "variable", "Р": "fixed", "РП": "variable", "НР": "fixed",
	} {
		if got := domain.ResistorFamilyAdjustment(family); got != want {
			t.Errorf("семейство %s: %q, ожидалось %q", family, got, want)
		}
	}
	for prefix, want := range map[string]string{
		"К": "fixed", "КТ": "preset", "КП": "variable", "КН": "fixed", "КС": "fixed",
	} {
		if got := domain.CapacitorPrefixAdjustment(prefix); got != want {
			t.Errorf("подкласс %s: %q, ожидалось %q", prefix, got, want)
		}
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
// (REST/CLI) используют реестр домена — состав обязан покрывать поля,
// которые создают парсеры, и классификационные поля секции fields.
func TestDesignationFieldRegistry(t *testing.T) {
	for code, numeric := range map[string]bool{
		"material": false, "subclass": false, "letters": false,
		"prefix": false, "family": false, "series": false,
		"adjustment": false, "category": false,
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

// Таблица возможностей парсеров: система способна установить поле сама
// (секция fields его не принимает) — состав закреплён против реальных
// разборов; классификационные поля применимы по классам.
func TestParserFieldSets(t *testing.T) {
	parserOwned := map[string]map[string]bool{
		"КТ315Б":  {"material": true, "subclass": true, "assembly": true, "category": false, "adjustment": false},
		"С2-33Н":  {"adjustment": true, "category": false, "material": false},
		"К10-17Б": {"adjustment": true, "category": false, "material": false},
		"BC547B":  {"material": true, "subclass": true, "category": false, "adjustment": false, "assembly": false},
		"2N2222A": {"junctions": true, "material": false, "subclass": false, "assembly": false, "category": false},
		"2SA1015": {"subclass": true, "material": false, "adjustment": false, "category": false},
		"МЛТ-0.5": {"series": true, "material": false, "subclass": false, "adjustment": false, "category": false},
	}
	for d, fields := range parserOwned {
		p, err := domain.ParseDesignation(d)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		for field, want := range fields {
			if got := domain.ParserFieldKnown(p.System, p.Kind, field); got != want {
				t.Errorf("%s (%s/%s): поле %s: ParserFieldKnown=%v, ожидалось %v",
					d, p.System, p.Kind, field, got, want)
			}
		}
	}
	if domain.ClassificationFieldAppliesTo("material", domain.KindResistor) {
		t.Error("material неприменим к резисторам")
	}
	if !domain.ClassificationFieldAppliesTo("material", domain.KindTransistor) {
		t.Error("material применим к транзисторам")
	}
	if !domain.ClassificationFieldAppliesTo("adjustment", domain.KindCapacitor) {
		t.Error("adjustment применим к конденсаторам")
	}
	if !domain.ClassificationFieldAppliesTo("category", domain.KindResistor) {
		t.Error("category применим ко всем классам")
	}
	if domain.ClassificationFieldAppliesTo("dev_number", domain.KindTransistor) {
		t.Error("dev_number — не классификационное поле")
	}
}
