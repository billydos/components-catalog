// Package seed — стартовые сиды каталога (docs/plan/03-data-model.md §3–§10):
// единицы, условия, группы параметров, именованные правила, системы
// обозначений и применимость к классам, реестр семейств series, параметры
// и атрибуты четырёх классов с применимостью (parameter_kinds/
// attribute_kinds — D7). Применяются при EnsureCreated на пустую базу
// (docs/plan/02-database.md §5); состав синхронизирован с реестрами internal/
// domain и реестром правил internal/catalog пин-тестами (seed/seed_test.go).
package seed

import (
	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Catalog возвращает стартовый каталог целиком (одним применением
// поверх пустого снимка — ApplyCatalog(nil, seed.Catalog())).
func Catalog() catalog.Input {
	kinds := make([]catalog.KindDef, 0, len(domain.Kinds()))
	for _, k := range domain.Kinds() {
		kinds = append(kinds, catalog.KindDef{Code: k})
	}

	systems := make([]catalog.SystemDef, 0, len(domain.Systems()))
	for _, s := range domain.Systems() {
		systems = append(systems, catalog.SystemDef{Code: s})
	}

	systemKinds := make([]catalog.SystemKindRef, 0, 16)
	for _, s := range domain.Systems() {
		for _, k := range domain.Kinds() {
			if domain.SystemAppliesToKind(s, k) {
				systemKinds = append(systemKinds, catalog.SystemKindRef{System: s, Kind: k})
			}
		}
	}

	families := make([]catalog.SeriesFamilyDef, 0, 32)
	for _, f := range domain.SeriesFamilies() {
		def := catalog.SeriesFamilyDef{Series: f.Series, Kind: f.Kind}
		if f.Power {
			def.TailSemantic = catalog.TailSemanticPower
		}
		families = append(families, def)
	}

	rules := make([]catalog.RuleDef, 0, 8)
	for _, r := range catalog.Rules() {
		rules = append(rules, catalog.RuleDef{Code: r.Code()})
	}

	return catalog.Input{
		Kinds:          kinds,
		Systems:        systems,
		SystemKinds:    systemKinds,
		SeriesFamilies: families,
		Units:          units(),
		Conditions:     conditions(),
		Groups:         groups(),
		Parameters:     parameters(),
		Attributes:     attributes(),
		Rules:          rules,
		KindRules:      kindRules(),
	}
}

// units — канонический справочник единиц (docs/plan/03-data-model.md §3).
// Коды — латиница, locale-neutral (D9); названия и символы по локалям —
// бандлы internal/i18n. Производные единицы отображения (мкФ, МОм, кГц) не
// хранятся — их форматирует слой вывода.
func units() []catalog.UnitDef {
	codes := []string{
		"V", "mV", "mA", "uA", "A",
		"MHz", "dB", "pct", "W", "mW",
		"ohm", "pF", "ps", "ns", "us",
		"degC", "degC_per_W", "ppm_per_degC", "pct_per_degC",
		"g", "mm", "nm", "mcd",
	}
	out := make([]catalog.UnitDef, 0, len(codes))
	for _, c := range codes {
		out = append(out, catalog.UnitDef{Code: c})
	}
	return out
}

// conditions — стартовый набор условий измерения (docs/plan/03-data-model.md
// §4): положительные, кроме temp (AllowNegative).
func conditions() []catalog.ConditionDef {
	return []catalog.ConditionDef{
		{Code: "Uke", Unit: "V"},
		{Code: "Ukb", Unit: "V"},
		{Code: "Ueb", Unit: "V"},
		{Code: "Ik", Unit: "mA"},
		{Code: "Ie", Unit: "mA"},
		{Code: "Ib", Unit: "mA"},
		{Code: "Uobr", Unit: "V"},
		{Code: "Upr", Unit: "V"},
		{Code: "Ipr", Unit: "mA"},
		{Code: "Ist", Unit: "mA"},
		{Code: "Unom", Unit: "V"},
		{Code: "Rg", Unit: "ohm"},
		{Code: "Rbe", Unit: "ohm"},
		{Code: "Rn", Unit: "ohm"},
		{Code: "freq", Unit: "MHz"},
		{Code: "pulse_duration", Unit: "us"},
		{Code: "temp", Unit: "degC", AllowNegative: true},
	}
}

// groups — группы параметров: секции файла наполнения и REST
// (docs/plan/03-data-model.md §5).
func groups() []catalog.GroupDef {
	return []catalog.GroupDef{
		{Code: "electrical", SectionName: "parameters", SortOrder: 10},
		{Code: "limiting", SectionName: "ratings", SortOrder: 20},
		{Code: "dimensional", SectionName: "dimensions", SortOrder: 30},
	}
}

// kindRules — привязка именованных правил к классам записей
// (docs/plan/03-data-model.md §7, §10).
func kindRules() []catalog.KindRuleRef {
	return []catalog.KindRuleRef{
		{Kind: domain.KindCapacitor, Rule: "cap_variant_matrix"},
		{Kind: domain.KindResistor, Rule: "resistor_variant_power"},
	}
}

// Помощники построения строк каталога.

func req(code string) catalog.ConditionSetItem {
	return catalog.ConditionSetItem{Condition: code, Mode: catalog.ModeRequired}
}

func opt(code string) catalog.ConditionSetItem {
	return catalog.ConditionSetItem{Condition: code, Mode: catalog.ModeOptional}
}

func fixed(code string, value float64) catalog.ConditionSetItem {
	return catalog.ConditionSetItem{Condition: code, Mode: catalog.ModeRequired, FixedValue: &value}
}

func set(no int, items ...catalog.ConditionSetItem) catalog.ConditionSet {
	return catalog.ConditionSet{No: no, Items: items}
}

func ceiling(v float64) *float64 { return &v }

// parameters — стартовые каталоги параметров четырёх классов
// (docs/plan/03-data-model.md §6; номенклатура — по терминологии НТД,
// выверка — docs/plan/07-r1-verification.md §9). Применимость вне разделов
// §6.2/§6.4/§6.6 — пустая (все классы, D7).
func parameters() []catalog.ParameterDef {
	tr := []domain.Kind{domain.KindTransistor}
	di := []domain.Kind{domain.KindDiode}
	re := []domain.Kind{domain.KindResistor}
	trDiRe := []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor}
	reCa := []domain.Kind{domain.KindResistor, domain.KindCapacitor}
	ruleTempPair := "temp_pair"
	ruleDims := "cap_dimensions_form"

	return []catalog.ParameterDef{
		// Транзисторы — электрические параметры (§6.1).
		{Code: "h21e", Group: "electrical",
			ValueType: catalog.ValueRange, SortOrder: 10, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Uke"), req("Ik")),
				set(2, req("Ukb"), req("Ie")),
			}},
		{Code: "h21b", Group: "electrical",
			ValueType: catalog.ValueAtLeast, SortOrder: 20, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"), req("Ie"))}},
		{Code: "FGran", Group: "electrical", Unit: "MHz",
			ValueType: catalog.ValueAtLeast, SortOrder: 30, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Uke"), req("Ik")),
				set(2, req("Ukb"), req("Ie")),
			}},
		{Code: "UGran", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtLeast, SortOrder: 40, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik")), set(2, req("Ie"))}},
		{Code: "UkeNas", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 50, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik")), set(2, req("Ie"))}},
		{Code: "UbeNas", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 60, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik")), set(2, req("Ie"))}},
		{Code: "Ikbo", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 70, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"), opt("temp"))}},
		{Code: "Iebo", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 80, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ueb"), opt("temp"))}},
		{Code: "Ikeo", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 90, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uke"), opt("temp"))}},
		{Code: "Ikep", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 100, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uke"), req("Rbe"))}},
		{Code: "h11", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueRange, SortOrder: 110, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uke"), req("Ik"))}},
		{Code: "Ck", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 120, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"))}},
		{Code: "Ce", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 130, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ueb"))}},
		{Code: "Tauk", Group: "electrical", Unit: "ps",
			ValueType: catalog.ValueAtMost, SortOrder: 140, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"), req("Ie"))}},
		{Code: "Ton", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 150, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik"), req("Ib"))}},
		{Code: "Toff", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 160, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik"), req("Ib"))}},
		{Code: "KShum", Group: "electrical", Unit: "dB",
			ValueType: catalog.ValueAtMost, SortOrder: 170, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Uke"), req("Ik"), req("freq"), opt("Rg")),
				set(2, req("Ukb"), req("Ie"), req("freq"), opt("Rg")),
			}},
		{Code: "PVyh", Group: "electrical", Unit: "W",
			ValueType: catalog.ValueAtLeast, SortOrder: 180, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Uke"), opt("Ik"))}},
		{Code: "KUr", Group: "electrical", Unit: "dB",
			ValueType: catalog.ValueAtLeast, SortOrder: 190, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Uke"), opt("Ik"))}},
		{Code: "Kpd", Group: "electrical", Unit: "pct",
			ValueType: catalog.ValueAtLeast, Ceiling: ceiling(100), SortOrder: 200, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Uke"), opt("Ik"))}},

		// Диоды — электрические параметры (§6.3).
		{Code: "Upr", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 210, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ipr"))}},
		{Code: "Iobr", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 220, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uobr"), opt("temp"))}},
		{Code: "Cn", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 230, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uobr"), opt("freq"))}},
		{Code: "Cp", Group: "electrical",
			ValueType: catalog.ValueAtLeast, SortOrder: 240, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uobr"))}},
		{Code: "trr", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 250, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ipr"))}},
		{Code: "Ust", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueRange, SortOrder: 260, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ist"))}},
		{Code: "Rdiff", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtMost, SortOrder: 270, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ist"))}},
		{Code: "TkUst", Group: "electrical", Unit: "pct_per_degC",
			ValueType: catalog.ValueAtMost, SortOrder: 280, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ist"))}},
		{Code: "Kpr", Group: "electrical", Unit: "dB",
			ValueType: catalog.ValueAtMost, SortOrder: 290, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"))}},
		{Code: "Iv", Group: "electrical", Unit: "mcd",
			ValueType: catalog.ValueRange, SortOrder: 300, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ipr"))}},
		{Code: "Lambda", Group: "electrical", Unit: "nm",
			ValueType: catalog.ValueRange, SortOrder: 310, Active: true, Kinds: di},

		// Резисторы — электрические параметры (§6.5); Dop и Riz применимы
		// также к конденсаторам (D7).
		{Code: "Rnom", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueRange, SortOrder: 320, Active: true, Kinds: re},
		{Code: "Dop", Group: "electrical", Unit: "pct",
			ValueType: catalog.ValueAtMost, SortOrder: 330, Active: true, Kinds: reCa},
		{Code: "TKS", Group: "electrical", Unit: "ppm_per_degC",
			ValueType: catalog.ValueAtMost, SortOrder: 340, Active: true, Kinds: re},
		{Code: "Ush", Group: "electrical",
			ValueType: catalog.ValueAtMost, SortOrder: 350, Active: true, Kinds: re},
		{Code: "Riz", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtLeast, SortOrder: 360, Active: true, Kinds: reCa},

		// Конденсаторы — характеристики (§6.8).
		{Code: "Cnom", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueRange, SortOrder: 370, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "TKE", Group: "electrical",
			ValueType: catalog.ValueEnum, SortOrder: 380, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			EnumValues: []string{
				// Нормируемый ТКЕ (линейный), 10⁻⁶/°C.
				"П100", "П120", "П60", "П33", "МП0", "М33", "М47", "М75",
				"М150", "М220", "М330", "М470", "М750", "М700", "М1500", "М1300", "М2200",
				// Ненормируемый (сегнетокерамика), %.
				"Н10", "Н20", "Н30", "Н50", "Н70", "Н90",
			}},
		{Code: "Unom", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 390, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "Tgd", Group: "electrical", Unit: "pct",
			ValueType: catalog.ValueAtMost, SortOrder: 400, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, opt("temp"))}},
		{Code: "Iut", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 410, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, req("Unom"), opt("temp"))}},
		{Code: "Esr", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtMost, SortOrder: 420, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, fixed("freq", 0.1))}},
		{Code: "Iripple", Group: "electrical", Unit: "mA",
			ValueType: catalog.ValueAtMost, SortOrder: 430, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("temp"))}},
		{Code: "opTempMin", Group: "electrical", Unit: "degC",
			ValueType: catalog.ValueExact, AllowNegative: true, ValidationRule: ruleTempPair,
			SortOrder: 440, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "opTempMax", Group: "electrical", Unit: "degC",
			ValueType: catalog.ValueExact, ValidationRule: ruleTempPair,
			SortOrder: 450, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},

		// Транзисторы — предельные эксплуатационные данные (§6.2).
		{Code: "UkeoMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 10, Active: true, Kinds: tr},
		{Code: "UkbMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 20, Active: true, Kinds: tr},
		{Code: "UbeMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 30, Active: true, Kinds: tr},
		{Code: "IkMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 40, Active: true, Kinds: tr},
		{Code: "IbMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 50, Active: true, Kinds: tr},
		{Code: "PkMax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 60, Active: true, Kinds: tr},
		{Code: "UsiMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 70, Active: true, Kinds: tr},
		{Code: "IsiMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 80, Active: true, Kinds: tr},
		{Code: "IkPulseMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 90, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "PkPulseMax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 100, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Общие предельные данные (§6.2/§6.4/§6.6): транзисторы, диоды,
		// резисторы.
		{Code: "TempMin", Group: "limiting", Unit: "degC",
			ValueType: catalog.ValueExact, AllowNegative: true, ValidationRule: ruleTempPair,
			SortOrder: 110, Active: true, Kinds: trDiRe},
		{Code: "TempMax", Group: "limiting", Unit: "degC",
			ValueType: catalog.ValueExact, ValidationRule: ruleTempPair,
			SortOrder: 120, Active: true, Kinds: trDiRe},
		{Code: "TempJunctionMax", Group: "limiting", Unit: "degC",
			ValueType: catalog.ValueExact, SortOrder: 130, Active: true, Kinds: trDiRe},
		{Code: "Rth", Group: "limiting", Unit: "degC_per_W",
			ValueType: catalog.ValueExact, SortOrder: 140, Active: true, Kinds: trDiRe},

		// Диоды — предельные эксплуатационные данные (§6.4).
		{Code: "UobrMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 150, Active: true, Kinds: di},
		{Code: "UobrImpMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 160, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "IprMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 170, Active: true, Kinds: di},
		{Code: "IprImpMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 180, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "Pmax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 190, Active: true, Kinds: di},
		{Code: "IstMin", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 200, Active: true, Kinds: di},
		{Code: "IstMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 210, Active: true, Kinds: di},

		// Резисторы — предельные эксплуатационные данные (§6.6); мощности —
		// вариантные параметры (D6, правило resistor_variant_power).
		{Code: "Pnom", Group: "limiting", Unit: "W",
			ValueType: catalog.ValueExact, SortOrder: 220, Active: true, Kinds: re},
		{Code: "Umax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 230, Active: true, Kinds: re},
		{Code: "UimpMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 240, Active: true, Kinds: re,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Массогабаритные данные (§6.7) — все классы.
		{Code: "massMax", Group: "dimensional", Unit: "g",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 10, Active: true},
		{Code: "length", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 20, Active: true},
		{Code: "width", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 30, Active: true},
		{Code: "height", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 40, Active: true},
		{Code: "diameter", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 50, Active: true},
		{Code: "leadLength", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 60, Active: true},
		{Code: "leadPitch", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 70, Active: true},
	}
}

// attributes — стартовый набор атрибутов (docs/plan/03-data-model.md §9).
// Применимость без пометки — все классы (D7).
func attributes() []catalog.AttributeDef {
	trDi := []domain.Kind{domain.KindTransistor, domain.KindDiode}
	return []catalog.AttributeDef{
		{Code: "category", Type: catalog.AttrText, SortOrder: 10, Active: true},
		{Code: "structure", Type: catalog.AttrText,
			SortOrder: 20, Active: true, Kinds: trDi},
		{Code: "polarized", Type: catalog.AttrBool,
			SortOrder: 30, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "functionalChar",
			Type: catalog.AttrEnum, SortOrder: 40, Active: true,
			Kinds: []domain.Kind{domain.KindResistor}, EnumValues: []string{"А", "Б", "В"}},
		{Code: "technology", Type: catalog.AttrText, SortOrder: 50, Active: true},
		{Code: "package", Type: catalog.AttrText, SortOrder: 60, Active: true},
		{Code: "packageMaterial", Type: catalog.AttrText, SortOrder: 70, Active: true},
		{Code: "colorMarking", Type: catalog.AttrText, SortOrder: 80, Active: true},
		{Code: "pinout", Type: catalog.AttrText, SortOrder: 90, Active: true},
		{Code: "esdSensitive", Type: catalog.AttrBool,
			SortOrder: 100, Active: true},
		{Code: "militaryGrade", Type: catalog.AttrBool,
			SortOrder: 110, Active: true},
		{Code: "radiationHardened", Type: catalog.AttrBool,
			SortOrder: 120, Active: true},
		{Code: "tu", Type: catalog.AttrText, SortOrder: 130, Active: true},
		{Code: "notes", Type: catalog.AttrText, SortOrder: 140, Active: true},
		{Code: "yearFrom", Type: catalog.AttrInt,
			ValidationRule: "year_range", SortOrder: 150, Active: true},
		{Code: "yearTo", Type: catalog.AttrInt,
			ValidationRule: "year_range", SortOrder: 160, Active: true},
		{Code: "datasheetUrl", Type: catalog.AttrText,
			SortOrder: 170, Active: true},
	}
}
