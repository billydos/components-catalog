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
	if len(snap.Parameters) != 76 || len(snap.Attributes) != 17 ||
		len(snap.Units) != 23 || len(snap.Conditions) != 17 {
		t.Fatalf("состав каталога: параметры %d, атрибуты %d, единицы %d, условия %d",
			len(snap.Parameters), len(snap.Attributes), len(snap.Units), len(snap.Conditions))
	}
}

// Реестры классов и систем обозначений — код ⇄ сиды (пин-тест матрицы
// 03 §1.1–1.2).
func TestSeedKindsAndSystemsPin(t *testing.T) {
	in := seed.Catalog()

	wantKinds := []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor, domain.KindCapacitor}
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
	if len(in.SystemKinds) != 19 {
		t.Fatalf("привязок систем: %d, ожидалось 19", len(in.SystemKinds))
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

// Реестр семейств series — сиды ⇄ domain.SeriesFamilies (tail_semantic
// «power» ⇄ Power), 03 §2.1–2.3.
func TestSeedSeriesFamiliesPin(t *testing.T) {
	in := seed.Catalog()
	want := domain.SeriesFamilies()
	if len(in.SeriesFamilies) != len(want) {
		t.Fatalf("семейств: %d, ожидалось %d", len(in.SeriesFamilies), len(want))
	}
	for i := range want {
		got := in.SeriesFamilies[i]
		if got.Series != want[i].Series || got.Kind != want[i].Kind {
			t.Errorf("семейство #%d: %+v ≠ %+v", i, got, want[i])
		}
		tail := ""
		if want[i].Power {
			tail = catalog.TailSemanticPower
		}
		if got.TailSemantic != tail {
			t.Errorf("семейство %s: семантика хвоста %q ≠ %q", want[i].Series, got.TailSemantic, tail)
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
		{Code: "resistor_variant_power"},
		{Code: "temp_pair"},
		{Code: "year_range"},
	}
	if !slices.Equal(in.Rules, want) {
		t.Fatalf("правила сидов:\n got:  %v\n want: %v", in.Rules, want)
	}
	kindRules := []catalog.KindRuleRef{
		{Kind: domain.KindCapacitor, Rule: "cap_variant_matrix"},
		{Kind: domain.KindResistor, Rule: "resistor_variant_power"},
	}
	if !slices.Equal(in.KindRules, kindRules) {
		t.Fatalf("привязки правил классов: %v", in.KindRules)
	}
}

// Enum ТКЕ — выверенная таблица (03 §6.8, 07 §6): нормируемый и
// ненормируемый ряды, варианты написания — отдельные значения.
func TestSeedTKEEnum(t *testing.T) {
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		t.Fatalf("проблемы применения сидов: %v", probs)
	}
	p, ok := snap.Parameter("TKE")
	if !ok {
		t.Fatal("TKE отсутствует")
	}
	want := []string{
		"П100", "П120", "П60", "П33", "МП0", "М33", "М47", "М75",
		"М150", "М220", "М330", "М470", "М750", "М700", "М1500", "М1300", "М2200",
		"Н10", "Н20", "Н30", "Н50", "Н70", "Н90",
	}
	if !slices.Equal(p.EnumValues, want) {
		t.Fatalf("ТКЕ:\n got:  %v\n want: %v", p.EnumValues, want)
	}
	if p.Unit != "" || p.ValueType != catalog.ValueEnum {
		t.Fatalf("TKE: тип %s, единица %q", string(p.ValueType), p.Unit)
	}
}

// Выборочные сверки параметров и атрибутов по 03 §6, §9.
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
	if kpd := check("Kpd"); kpd.Ceiling == nil || *kpd.Ceiling != 100 {
		t.Errorf("Kpd: потолок %v, ожидался 100", kpd.Ceiling)
	}
	tmin := check("TempMin")
	if !tmin.AllowNegative || tmin.ValidationRule != "temp_pair" {
		t.Errorf("TempMin: allow_negative %v, правило %q", tmin.AllowNegative, tmin.ValidationRule)
	}
	if !slices.Equal(tmin.Kinds, []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor}) {
		t.Errorf("TempMin: применимость %v", tmin.Kinds)
	}
	dop := check("Dop")
	if !dop.AppliesTo(domain.KindResistor) || !dop.AppliesTo(domain.KindCapacitor) || dop.AppliesTo(domain.KindTransistor) {
		t.Errorf("Dop: применимость %v", dop.Kinds)
	}
	esr := check("Esr")
	if len(esr.ConditionSets) != 1 || len(esr.ConditionSets[0].Items) != 1 ||
		esr.ConditionSets[0].Items[0].FixedValue == nil || *esr.ConditionSets[0].Items[0].FixedValue != 0.1 {
		t.Errorf("Esr: набор условий %+v", esr.ConditionSets)
	}
	pulse := check("IkPulseMax")
	if len(pulse.ConditionSets) != 1 || len(pulse.ConditionSets[0].Items) != 1 ||
		pulse.ConditionSets[0].Items[0].Mode != catalog.ModeRequired ||
		pulse.ConditionSets[0].Items[0].Condition != "pulse_duration" {
		t.Errorf("IkPulseMax: набор условий %+v", pulse.ConditionSets)
	}
	if u, _ := snap.Parameter("Ush"); u.Unit != "" {
		t.Errorf("Ush: единица %q (должна быть NULL)", u.Unit)
	}
	if cp, _ := snap.Parameter("Cp"); cp.Unit != "" {
		t.Errorf("Cp: единица %q (должна быть NULL)", cp.Unit)
	}
	for _, code := range []string{"massMax", "length", "width", "height", "diameter", "leadLength", "leadPitch"} {
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
	if a := attr("functionalChar"); a.Type != catalog.AttrEnum || !slices.Equal(a.EnumValues, []string{"А", "Б", "В"}) ||
		!a.AppliesTo(domain.KindResistor) || a.AppliesTo(domain.KindCapacitor) {
		t.Errorf("functionalChar: %+v", a)
	}
	if a := attr("polarized"); a.Type != catalog.AttrBool || a.AppliesTo(domain.KindResistor) {
		t.Errorf("polarized: %+v", a)
	}
	if a := attr("structure"); !a.AppliesTo(domain.KindTransistor) || !a.AppliesTo(domain.KindDiode) || a.AppliesTo(domain.KindResistor) {
		t.Errorf("structure: применимость %v", a.Kinds)
	}
	for _, code := range []string{"yearFrom", "yearTo"} {
		if a := attr(code); a.ValidationRule != "year_range" || a.Type != catalog.AttrInt {
			t.Errorf("%s: %+v", code, a)
		}
	}
	if a := attr("category"); len(a.Kinds) != 0 {
		t.Errorf("category: применимость должна быть «все классы», задано %v", a.Kinds)
	}
}

// Полнота бандлов i18n (D9): каждый код сидов (классы, системы+описания,
// семейства, единицы имя+символ, условия, группы, параметры, атрибуты,
// правила) имеет строки в en (канонический) и ru; расширение каталога
// кодом — вместе с записями в бандлах в том же изменении.
func TestSeedI18nCompleteness(t *testing.T) {
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) != 0 {
		t.Fatalf("проблемы применения сидов: %v", probs)
	}
	require := func(lang i18n.Language, key string) {
		t.Helper()
		if !i18n.HasString(lang, key) {
			t.Errorf("бандл %s: отсутствует ключ %q", string(lang), key)
		}
	}
	for _, k := range snap.Kinds {
		require(i18n.En, "kind."+string(k.Code))
		require(i18n.Ru, "kind."+string(k.Code))
	}
	for _, s := range snap.Systems {
		require(i18n.En, "system."+string(s.Code))
		require(i18n.En, "system."+string(s.Code)+".description")
		require(i18n.Ru, "system."+string(s.Code))
		require(i18n.Ru, "system."+string(s.Code)+".description")
	}
	for _, f := range snap.SeriesFamilies {
		require(i18n.En, "family."+f.Series)
		require(i18n.Ru, "family."+f.Series)
	}
	for _, u := range snap.Units {
		require(i18n.En, "unit."+u.Code+".name")
		require(i18n.En, "unit."+u.Code+".symbol")
		require(i18n.Ru, "unit."+u.Code+".name")
		require(i18n.Ru, "unit."+u.Code+".symbol")
	}
	for _, c := range snap.Conditions {
		require(i18n.En, "condition."+c.Code)
		require(i18n.Ru, "condition."+c.Code)
	}
	for _, g := range snap.Groups {
		require(i18n.En, "group."+g.Code)
		require(i18n.Ru, "group."+g.Code)
	}
	for _, p := range snap.Parameters {
		require(i18n.En, "param."+p.Code)
		require(i18n.Ru, "param."+p.Code)
	}
	for _, a := range snap.Attributes {
		require(i18n.En, "attr."+a.Code)
		require(i18n.Ru, "attr."+a.Code)
	}
	for _, r := range snap.Rules {
		require(i18n.En, "rule."+r.Code)
		require(i18n.Ru, "rule."+r.Code)
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
