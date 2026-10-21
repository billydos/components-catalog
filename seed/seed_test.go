package seed_test

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/seed"
)

// Сиды применяются к пустому каталогу без нарушений метасхемы.
func TestSeedAppliesCleanly(t *testing.T) {
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		for _, p := range probs {
			t.Errorf("проблема: %s", p.Message)
		}
		t.Fatal("стартовый каталог не проходит метасхему")
	}
	if len(snap.Parameters) != 122 || len(snap.Attributes) != 21 ||
		len(snap.Units) != 31 || len(snap.Conditions) != 20 {
		t.Fatalf("состав каталога: параметры %d, атрибуты %d, единицы %d, условия %d",
			len(snap.Parameters), len(snap.Attributes), len(snap.Units), len(snap.Conditions))
	}
}

// Реестры классов и систем обозначений — код ⇄ сиды (пин-тест матрицы
// 03 §1.1–1.2).
func TestSeedKindsAndSystemsPin(t *testing.T) {
	in := seed.Catalog()

	wantKinds := []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor, domain.KindCapacitor, domain.KindRelay}
	gotKinds := make([]domain.Kind, 0, len(in.Kinds))
	for _, k := range in.Kinds {
		gotKinds = append(gotKinds, k.Code)
	}
	if !slices.Equal(gotKinds, wantKinds) {
		t.Fatalf("классы сидов: %v", gotKinds)
	}

	wantSystems := domain.Systems()
	gotSystems := make([]domain.System, 0, len(in.Systems))
	for _, s := range in.Systems {
		gotSystems = append(gotSystems, s.Code)
	}
	if !slices.Equal(gotSystems, wantSystems) {
		t.Fatalf("системы сидов: %v", gotSystems)
	}

	// Применимость систем к классам — вся матрица.
	if len(in.SystemKinds) != 16 {
		t.Fatalf("привязок систем: %d, ожидалось 16", len(in.SystemKinds))
	}
	for _, s := range wantSystems {
		for _, k := range wantKinds {
			want := domain.SystemAppliesToKind(s, k)
			got := slices.ContainsFunc(in.SystemKinds, func(r catalog.SystemKindRef) bool {
				return r.System == s && r.Kind == k
			})
			if got != want {
				t.Errorf("привязка %s→%s: сиды %v, реестр %v", string(s), string(k), got, want)
			}
		}
	}
}

// Строки validation_rules — пин кодов реестра правил (описания — бандлы
// internal/i18n: rule.<код>, D9).
func TestSeedRulesPin(t *testing.T) {
	in := seed.Catalog()
	want := []catalog.RuleDef{
		{Code: "cap_dimensions_form"},
		{Code: "cap_variant_matrix"},
		{Code: "relay_variant_coil"},
		{Code: "resistor_variant_power"},
		{Code: "temp_pair"},
		{Code: "year_range"},
	}
	if !slices.Equal(in.Rules, want) {
		t.Fatalf("правила сидов:\n got:  %v\n want: %v", in.Rules, want)
	}
	kindRules := []catalog.KindRuleRef{
		{Kind: domain.KindCapacitor, Rule: "cap_variant_matrix"},
		{Kind: domain.KindRelay, Rule: "relay_variant_coil"},
		{Kind: domain.KindResistor, Rule: "resistor_variant_power"},
	}
	if !slices.Equal(in.KindRules, kindRules) {
		t.Fatalf("привязки правил классов: %v", in.KindRules)
	}
}

// Enum ТКЕ (TCC) — выверенная таблица (03 §6.8, 07 §6): нормируемый и
// ненормируемый ряды, варианты написания — отдельные значения;
// международные коды EIA (класс 1 и класс 2, 07 §6/S9).
func TestSeedTCCEnum(t *testing.T) {
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		t.Fatalf("проблемы применения сидов: %v", probs)
	}
	p, ok := snap.Parameter("TCC")
	if !ok {
		t.Fatal("TCC отсутствует")
	}
	want := []string{
		"П100", "П120", "П60", "П33", "МП0", "М33", "М47", "М75",
		"М150", "М220", "М330", "М470", "М750", "М700", "М1500", "М1300", "М2200",
		"Н10", "Н20", "Н30", "Н50", "Н70", "Н90",
		"C0G", "NP0", "U2J", "X5R", "X6S", "X7R", "X7S", "X8R", "Y5V", "Z5U",
	}
	if !slices.Equal(p.EnumValues, want) {
		t.Fatalf("TCC:\n got:  %v\n want: %v", p.EnumValues, want)
	}
	if p.Unit != "" || p.ValueType != catalog.ValueEnum {
		t.Fatalf("TCC: тип %s, единица %q", string(p.ValueType), p.Unit)
	}
}

// Enum номинальных рядов — ГОСТ 28884-90 / МЭК 60063 (03 §6.5,
// 07 §11): параметр многозначный в записи (серия может выпускаться
// в нескольких рядах по градациям допуска).
func TestSeedNominalRowEnum(t *testing.T) {
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		t.Fatalf("проблемы применения сидов: %v", probs)
	}
	p, ok := snap.Parameter("nominalRow")
	if !ok {
		t.Fatal("nominalRow отсутствует")
	}
	want := []string{"E3", "E6", "E12", "E24", "E48", "E96", "E192"}
	if !slices.Equal(p.EnumValues, want) {
		t.Fatalf("номинальные ряды:\n got:  %v\n want: %v", p.EnumValues, want)
	}
	if p.Unit != "" || p.ValueType != catalog.ValueEnum {
		t.Fatalf("nominalRow: тип %s, единица %q", string(p.ValueType), p.Unit)
	}
	if !p.AppliesTo(domain.KindResistor) || !p.AppliesTo(domain.KindCapacitor) || p.AppliesTo(domain.KindTransistor) {
		t.Errorf("nominalRow: применимость %v", p.Kinds)
	}
}

// Выборочные сверки параметров и атрибутов по 03 §6, §10.
func TestSeedSpotChecks(t *testing.T) {
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		t.Fatalf("проблемы применения сидов: %v", probs)
	}
	check := func(code string) *catalog.ParameterDef {
		t.Helper()
		p, ok := snap.Parameter(code)
		if !ok {
			t.Fatalf("параметр %s отсутствует", code)
		}
		return p
	}
	if eta := check("Eta"); eta.Ceiling == nil || *eta.Ceiling != 100 {
		t.Errorf("Eta: потолок %v, ожидался 100", eta.Ceiling)
	}
	tmin := check("TempMin")
	if !tmin.AllowNegative || tmin.ValidationRule != "temp_pair" {
		t.Errorf("TempMin: allow_negative %v, правило %q", tmin.AllowNegative, tmin.ValidationRule)
	}
	if !slices.Equal(tmin.Kinds, []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor, domain.KindRelay}) {
		t.Errorf("TempMin: применимость %v", tmin.Kinds)
	}
	tolerance := check("Tolerance")
	if !tolerance.AppliesTo(domain.KindResistor) || !tolerance.AppliesTo(domain.KindCapacitor) || tolerance.AppliesTo(domain.KindTransistor) {
		t.Errorf("Tolerance: применимость %v", tolerance.Kinds)
	}
	esr := check("Esr")
	if len(esr.ConditionSets) != 1 || len(esr.ConditionSets[0].Items) != 1 ||
		esr.ConditionSets[0].Items[0].FixedValue == nil || *esr.ConditionSets[0].Items[0].FixedValue != 0.1 {
		t.Errorf("Esr: набор условий %+v", esr.ConditionSets)
	}
	pulse := check("IcPulseMax")
	if len(pulse.ConditionSets) != 1 || len(pulse.ConditionSets[0].Items) != 1 ||
		pulse.ConditionSets[0].Items[0].Mode != catalog.ModeRequired ||
		pulse.ConditionSets[0].Items[0].Condition != "pulse_duration" {
		t.Errorf("IcPulseMax: набор условий %+v", pulse.ConditionSets)
	}
	// Полевые транзисторы: собственные коды величин стока/затвора, без
	// пересечений с биполярными (03 §6.1–6.2).
	pdMax := check("PdMax")
	if pdMax.Unit != "mW" || pdMax.ValueType != catalog.ValueExact || len(pdMax.ConditionSets) != 0 ||
		!pdMax.AppliesTo(domain.KindTransistor) {
		t.Errorf("PdMax: %+v", pdMax)
	}
	idPulse := check("IdPulseMax")
	if len(idPulse.ConditionSets) != 1 || len(idPulse.ConditionSets[0].Items) != 1 ||
		idPulse.ConditionSets[0].Items[0].Mode != catalog.ModeRequired ||
		idPulse.ConditionSets[0].Items[0].Condition != "pulse_duration" {
		t.Errorf("IdPulseMax: набор условий %+v", idPulse.ConditionSets)
	}
	if s, _ := snap.Parameter("gfs"); s.Unit != "mA_per_V" || s.ValueType != catalog.ValueAtLeast ||
		len(s.ConditionSets) != 2 {
		t.Errorf("gfs: единица %q, тип %s, наборы %+v", s.Unit, string(s.ValueType), s.ConditionSets)
	}
	vgsOff := check("VgsOff")
	if !vgsOff.AllowNegative || vgsOff.ValueType != catalog.ValueRange ||
		len(vgsOff.ConditionSets) != 1 || vgsOff.ConditionSets[0].Items[0].Condition != "Id" {
		t.Errorf("VgsOff: %+v", vgsOff)
	}
	if c, ok := snap.Condition("Vgs"); !ok || c.Unit != "V" || !c.AllowNegative {
		t.Errorf("условие Vgs: %+v", c)
	}
	if c, ok := snap.Condition("Id"); !ok || c.Unit != "mA" || c.AllowNegative {
		t.Errorf("условие Id: %+v", c)
	}
	if u, ok := snap.Unit("nA"); !ok {
		t.Errorf("единица nA: %+v", u)
	}
	if u, _ := snap.Parameter("Vnoise"); u.Unit != "" {
		t.Errorf("Vnoise: единица %q (должна быть NULL)", u.Unit)
	}
	if cr, _ := snap.Parameter("Cratio"); cr.Unit != "" {
		t.Errorf("Cratio: единица %q (должна быть NULL)", cr.Unit)
	}
	// Реле: обмотка/контакты/времена (03 §6.10–§6.11); безусловные,
	// ток коммутации — в A, износостойкость — без единицы.
	for _, code := range []string{"VcoilNom", "Rcoil", "Pcoil", "Vpickup", "Vdropout",
		"Ipickup", "Rcontact", "tOperate", "tRelease", "tBounce", "Vtest",
		"enduranceElec", "enduranceMech", "VswitchMax", "IswitchMax"} {
		p := check(code)
		if !p.AppliesTo(domain.KindRelay) || p.AppliesTo(domain.KindTransistor) {
			t.Errorf("%s: применимость %v", code, p.Kinds)
		}
		if len(p.ConditionSets) != 0 {
			t.Errorf("%s: наборы условий %+v (должен быть безусловным)", code, p.ConditionSets)
		}
	}
	if p := check("IswitchMax"); p.Unit != "A" || p.Group != "limiting" || p.ValueType != catalog.ValueExact {
		t.Errorf("IswitchMax: единица %q, группа %q, тип %s", p.Unit, p.Group, string(p.ValueType))
	}
	if p := check("tOperate"); p.Unit != "ms" || p.ValueType != catalog.ValueAtMost {
		t.Errorf("tOperate: единица %q, тип %s", p.Unit, string(p.ValueType))
	}
	for _, code := range []string{"enduranceElec", "enduranceMech"} {
		if p := check(code); p.Unit != "" || p.Group != "electrical" {
			t.Errorf("%s: единица %q, группа %q", code, p.Unit, p.Group)
		}
	}
	if p := check("Rins"); !p.AppliesTo(domain.KindRelay) || !p.AppliesTo(domain.KindCapacitor) {
		t.Errorf("Rins: применимость %v", p.Kinds)
	}
	// Резисторы — предельные данные переменных/многосекционных и климат,
	// общие данные надёжности — все классы (03 §6.6, §6.12).
	for _, code := range []string{"imbalance", "endurance", "rotationAngle",
		"switchAngle", "staticFrictionTorque", "humidity"} {
		p := check(code)
		if p.Group != "limiting" || !p.AppliesTo(domain.KindResistor) || p.AppliesTo(domain.KindTransistor) {
			t.Errorf("%s: группа %q, применимость %v", code, p.Group, p.Kinds)
		}
	}
	for code, want := range map[string]struct {
		unit  string
		vt    catalog.ValueType
		conds int
	}{
		"imbalance":            {"dB", catalog.ValueAtMost, 0},
		"endurance":            {"", catalog.ValueAtLeast, 0},
		"rotationAngle":        {"deg", catalog.ValueAtLeast, 0},
		"switchAngle":          {"deg", catalog.ValueAtMost, 0},
		"staticFrictionTorque": {"mN_m", catalog.ValueAtMost, 0},
		"humidity":             {"pct", catalog.ValueAtMost, 1},
		"operatingLife":        {"h", catalog.ValueAtLeast, 0},
		"shelfLife":            {"yr", catalog.ValueAtLeast, 0},
		"hfe":                  {"", catalog.ValueLimits, 6},
		"h11":                  {"ohm", catalog.ValueLimits, 3},
	} {
		p := check(code)
		if p.Unit != want.unit || p.ValueType != want.vt || len(p.ConditionSets) != want.conds {
			t.Errorf("%s: единица %q, тип %s, наборы условий %+v", code, p.Unit, string(p.ValueType), p.ConditionSets)
		}
	}
	if p := check("humidity"); len(p.ConditionSets) != 1 || len(p.ConditionSets[0].Items) != 1 ||
		p.ConditionSets[0].Items[0].Condition != "temp" ||
		p.ConditionSets[0].Items[0].Mode != catalog.ModeOptional {
		t.Errorf("humidity: набор условий %+v", p.ConditionSets)
	}
	for _, code := range []string{"operatingLife", "shelfLife"} {
		p := check(code)
		if p.Group != "limiting" || len(p.Kinds) != 0 {
			t.Errorf("%s: группа %q, применимость %v", code, p.Group, p.Kinds)
		}
	}
	for _, code := range []string{"massMax", "length", "width", "height", "diameter", "leadLength", "leadPitch", "shaftDiameter", "shaftLength"} {
		if p := check(code); p.ValidationRule != "cap_dimensions_form" || p.ValueType != catalog.ValueExact {
			t.Errorf("%s: правило %q, тип %s", code, p.ValidationRule, string(p.ValueType))
		}
	}
	// Группы: секции файла наполнения.
	sections := map[string]string{"electrical": "parameters", "limiting": "ratings", "dimensional": "dimensions"}
	for _, g := range snap.Groups {
		if sections[g.Code] != g.SectionName {
			t.Errorf("группа %s: секция %q", g.Code, g.SectionName)
		}
	}
	// Attributes: типы и привязки правил.
	attr := func(code string) *catalog.AttributeDef {
		t.Helper()
		a, ok := snap.Attribute(code)
		if !ok {
			t.Fatalf("атрибут %s отсутствует", code)
		}
		return a
	}
	if a := attr("functionalChar"); a.Type != catalog.AttrEnum ||
		!slices.Equal(a.EnumValues, []string{"А", "Б", "В", "linear", "logarithmic", "antilogarithmic"}) ||
		!a.AppliesTo(domain.KindResistor) || a.AppliesTo(domain.KindCapacitor) {
		t.Errorf("functionalChar: %+v", a)
	}
	// Конструктивные признаки переменных резисторов (03 §10):
	// число секций и вид конца вала — уровни записи/исполнения (D12).
	if a := attr("sections"); a.Type != catalog.AttrInt || !a.AppliesTo(domain.KindResistor) ||
		a.AppliesTo(domain.KindCapacitor) {
		t.Errorf("sections: %+v", a)
	}
	if a := attr("shaftEnd"); a.Type != catalog.AttrEnum || !a.AppliesTo(domain.KindResistor) ||
		!slices.Equal(a.EnumValues, []string{"slotted", "flatted", "plain", "threaded"}) {
		t.Errorf("shaftEnd: %+v", a)
	}
	if a := attr("polarized"); a.Type != catalog.AttrBool || a.AppliesTo(domain.KindResistor) {
		t.Errorf("polarized: %+v", a)
	}
	if a := attr("structure"); !a.AppliesTo(domain.KindTransistor) || !a.AppliesTo(domain.KindDiode) || a.AppliesTo(domain.KindResistor) {
		t.Errorf("structure: применимость %v", a.Kinds)
	}
	// Текстовые атрибуты-списки: заметки, ТУ и ссылки на документацию (03 §10).
	for _, code := range []string{"notes", "spec", "datasheetUrl"} {
		if a := attr(code); a.Type != catalog.AttrTextList || len(a.Kinds) != 0 {
			t.Errorf("%s: %+v", code, a)
		}
	}
	// Атрибуты реле: enum-тип реле и контактная группа (03 §10).
	if a := attr("relayType"); a.Type != catalog.AttrEnum || !a.AppliesTo(domain.KindRelay) ||
		a.AppliesTo(domain.KindCapacitor) ||
		!slices.Equal(a.EnumValues, []string{"electromagnetic", "polarized", "reed", "solid_state", "latching"}) {
		t.Errorf("relayType: %+v", a)
	}
	if a := attr("contactForm"); a.Type != catalog.AttrEnum || !a.AppliesTo(domain.KindRelay) ||
		!slices.Equal(a.EnumValues, []string{
			"1З", "1Р", "1П", "2З", "2Р", "2П", "3П", "4П",
			"1A", "1B", "1C", "2A", "2B", "2C", "3C", "4C",
		}) {
		t.Errorf("contactForm: %+v", a)
	}
	for _, code := range []string{"yearFrom", "yearTo"} {
		if a := attr(code); a.ValidationRule != "year_range" || a.Type != catalog.AttrInt {
			t.Errorf("%s: %+v", code, a)
		}
	}
	if a := attr("description"); len(a.Kinds) != 0 {
		t.Errorf("description: применимость должна быть «все классы», задано %v", a.Kinds)
	}
}

// expectedBundleKeys — полное ожидаемое множество ключей бандлов i18n:
// коды сидов (классы, системы+описания, единицы имя+символ, условия,
// группы, параметры, атрибуты, правила), материалы domain.Materials
// и поля разбора domain.DesignationFieldCodes. Новая группа ключей
// бандла обязана расширять это множество тем же изменением.
func expectedBundleKeys(t *testing.T) map[string]bool {
	t.Helper()
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		t.Fatalf("проблемы применения сидов: %v", probs)
	}
	want := make(map[string]bool, 300)
	for _, k := range snap.Kinds {
		want["kind."+string(k.Code)] = true
	}
	for _, s := range snap.Systems {
		want["system."+string(s.Code)] = true
		want["system."+string(s.Code)+".description"] = true
	}
	for _, u := range snap.Units {
		want["unit."+u.Code+".name"] = true
		want["unit."+u.Code+".symbol"] = true
	}
	for _, c := range snap.Categories {
		want["category."+c.Code] = true
	}
	for _, c := range snap.Conditions {
		want["condition."+c.Code] = true
	}
	for _, g := range snap.Groups {
		want["group."+g.Code] = true
	}
	for _, p := range snap.Parameters {
		want["param."+p.Code] = true
	}
	for _, a := range snap.Attributes {
		want["attr."+a.Code] = true
	}
	for _, r := range snap.Rules {
		want["rule."+r.Code] = true
	}
	for _, m := range domain.Materials() {
		want["material."+m.Code] = true
	}
	for _, s := range domain.Subclasses() {
		want["subclass."+s.Code] = true
	}
	for _, a := range domain.Adjustments() {
		want["adjustment."+a.Code] = true
	}
	for _, f := range domain.DesignationFieldCodes() {
		want["field."+f] = true
	}
	return want
}

// Полнота бандлов i18n (D9): каждый ожидаемый ключ каталога имеет
// непустую строку в en (канонический) и ru; расширение каталога кодом —
// вместе с записями в бандлах в том же изменении.
func TestSeedI18nCompleteness(t *testing.T) {
	for key := range expectedBundleKeys(t) {
		if !i18n.HasString(i18n.En, key) {
			t.Errorf("бандл en: отсутствует ключ %q", key)
		}
		if !i18n.HasString(i18n.Ru, key) {
			t.Errorf("бандл ru: отсутствует ключ %q", key)
		}
	}
}

// Обратное направление полноты: каждый ключ бандла соответствует коду
// каталога — «сироты» (ключи без кода после удаления или переименования
// кода) не накапливаются.
func TestSeedI18nNoOrphans(t *testing.T) {
	want := expectedBundleKeys(t)
	for _, l := range i18n.Languages() {
		for _, k := range i18n.Keys(l) {
			if !want[k] {
				t.Errorf("бандл %s: ключ %q не соответствует ни одному коду каталога", string(l), k)
			}
		}
	}
}

// Коды единиц сидов — латиница, locale-neutral (D9).
func TestSeedUnitCodesLatin(t *testing.T) {
	for _, u := range seed.Catalog().Units {
		for _, r := range u.Code {
			if r < '!' || r > '~' {
				t.Errorf("код единицы %q содержит не-ASCII символ %q", u.Code, string(r))
			}
		}
	}
}
