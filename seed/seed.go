// Package seed — стартовые сиды каталога (docs/plan/03-data-model.md §3–§11):
// единицы, условия, группы параметров, именованные правила, системы
// обозначений и применимость к классам, параметры и атрибуты пяти
// классов с применимостью (parameter_kinds/attribute_kinds — D7).
// Применяются при EnsureCreated на пустую базу
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

	rules := make([]catalog.RuleDef, 0, 8)
	for _, r := range catalog.Rules() {
		rules = append(rules, catalog.RuleDef{Code: r.Code()})
	}

	return catalog.Input{
		Kinds:       kinds,
		Systems:     systems,
		SystemKinds: systemKinds,
		Units:       units(),
		Categories:  categories(),
		Conditions:  conditions(),
		Groups:      groups(),
		Parameters:  parameters(),
		Attributes:  attributes(),
		Rules:       rules,
		KindRules:   kindRules(),
	}
}

// units — канонический справочник единиц (docs/plan/03-data-model.md §3).
// Коды — латиница, locale-neutral (D9); названия и символы по локалям —
// бандлы internal/i18n. Производные единицы отображения (мкФ, МОм, кГц) не
// хранятся — их форматирует слой вывода.
func units() []catalog.UnitDef {
	codes := []string{
		"V", "mV", "mA", "uA", "nA", "A",
		"MHz", "dB", "pct", "W", "mW",
		"ohm", "mA_per_V", "uS", "pF", "ps", "ns", "us", "ms",
		"degC", "degC_per_W", "ppm_per_degC", "pct_per_degC",
		"g", "mm", "nm", "mcd",
		"deg", "h", "mN_m", "yr",
	}
	out := make([]catalog.UnitDef, 0, len(codes))
	for _, c := range codes {
		out = append(out, catalog.UnitDef{Code: c})
	}
	return out
}

// categories — стартовый словарь категорий (поле классификации category:
// применение/характеристика прибора, ортогональная подклассу — функции
// перехода). Коды — латиница (D9); названия — бандлы internal/i18n
// (category.<код>), расширения каталога данными без записи в бандле
// отображаются кодом.
func categories() []catalog.CategoryDef {
	codes := []string{
		"general_purpose", "power", "high_voltage", "switching", "pulse",
		"rf", "audio", "lownoise", "fast", "precision", "composite",
	}
	out := make([]catalog.CategoryDef, 0, len(codes))
	for _, c := range codes {
		out = append(out, catalog.CategoryDef{Code: c})
	}
	return out
}

// conditions — стартовый набор условий измерения (docs/plan/03-data-model.md
// §4): положительные, кроме temp и Vgs (AllowNegative).
func conditions() []catalog.ConditionDef {
	return []catalog.ConditionDef{
		{Code: "Vce", Unit: "V"},
		{Code: "Vcb", Unit: "V"},
		{Code: "Veb", Unit: "V"},
		{Code: "Ic", Unit: "mA"},
		{Code: "Ie", Unit: "mA"},
		{Code: "Ib", Unit: "mA"},
		{Code: "Vds", Unit: "V"},
		{Code: "Vgs", Unit: "V", AllowNegative: true},
		{Code: "Id", Unit: "mA"},
		{Code: "Vr", Unit: "V"},
		{Code: "Vf", Unit: "V"},
		{Code: "If", Unit: "mA"},
		{Code: "Iz", Unit: "mA"},
		{Code: "Vnom", Unit: "V"},
		{Code: "Rg", Unit: "ohm"},
		{Code: "Rbe", Unit: "ohm"},
		{Code: "Rload", Unit: "ohm"},
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
// (docs/plan/03-data-model.md §11).
func kindRules() []catalog.KindRuleRef {
	return []catalog.KindRuleRef{
		{Kind: domain.KindCapacitor, Rule: catalog.RuleCapVariantMatrix},
		{Kind: domain.KindRelay, Rule: catalog.RuleRelayVariantCoil},
		{Kind: domain.KindResistor, Rule: catalog.RuleResistorVariantPower},
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

// parameters — стартовые каталоги параметров пяти классов
// (docs/plan/03-data-model.md §6; номенклатура — по терминологии НТД,
// выверка — docs/plan/07-r1-verification.md §9). Применимость вне разделов
// §6.2/§6.4/§6.6/§6.10–§6.11 — пустая (все классы, D7).
func parameters() []catalog.ParameterDef {
	tr := []domain.Kind{domain.KindTransistor}
	di := []domain.Kind{domain.KindDiode}
	re := []domain.Kind{domain.KindResistor}
	rl := []domain.Kind{domain.KindRelay}
	trDiRe := []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor}
	trDiReRl := []domain.Kind{domain.KindTransistor, domain.KindDiode, domain.KindResistor, domain.KindRelay}
	reCa := []domain.Kind{domain.KindResistor, domain.KindCapacitor}
	reCaRl := []domain.Kind{domain.KindResistor, domain.KindCapacitor, domain.KindRelay}
	ruleTempPair := catalog.RuleTempPair
	ruleDims := catalog.RuleCapDimensionsForm

	return []catalog.ParameterDef{
		// Транзисторы — электрические параметры (§6.1); состав наборов
		// условий — схемы Uкэ+Iк, Uкб+Iэ, Uкэ+Iэ, Uкб+Iк, константа
		// Uкэ=0, температурные строки — опц. temp, модуль |h21э| — опц. freq.
		{Code: "hfe", Group: "electrical",
			ValueType: catalog.ValueRange, SortOrder: 10, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vce"), req("Ic"), opt("freq"), opt("temp")),
				set(2, req("Vcb"), req("Ie"), opt("freq"), opt("temp")),
				set(3, req("Vce"), req("Ie"), opt("freq"), opt("temp")),
				set(4, req("Vcb"), req("Ic"), opt("freq"), opt("temp")),
				set(5, fixed("Vce", 0), req("Ie"), opt("freq"), opt("temp")),
				set(6, fixed("Vce", 0), req("Ic"), opt("freq"), opt("temp")),
			}},
		{Code: "hfb", Group: "electrical",
			ValueType: catalog.ValueAtLeast, SortOrder: 20, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vcb"), req("Ie"))}},
		{Code: "fT", Group: "electrical", Unit: "MHz",
			ValueType: catalog.ValueAtLeast, SortOrder: 30, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vce"), req("Ic")),
				set(2, req("Vcb"), req("Ie")),
				set(3, req("Vcb"), req("Ic")),
				set(4, req("Vce"), req("Ie")),
			}},
		{Code: "VceCutoff", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtLeast, SortOrder: 40, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Ic"), opt("temp")),
				set(2, req("Ie"), opt("temp")),
			}},
		{Code: "VceSat", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 50, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Ic"), opt("temp")),
				set(2, req("Ie"), opt("temp")),
			}},
		{Code: "VbeSat", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 60, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Ic"), opt("temp")),
				set(2, req("Ie"), opt("temp")),
			}},
		// Пороговое напряжение база—эмиттер («напряжение отпирания» у
		// 2Т318/КТ318, «напряжение отпускания» у КТ336).
		{Code: "VbeTh", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 70, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vcb"), req("Ie"), opt("temp")),
				set(2, req("Vce"), req("Ib"), opt("temp")),
			}},
		{Code: "Icbo", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 80, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vcb"), opt("temp"))}},
		{Code: "Iebo", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 90, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Veb"), opt("temp"))}},
		{Code: "Iceo", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 100, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vce"), opt("temp"))}},
		{Code: "Icer", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 110, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vce"), req("Rbe"))}},
		{Code: "h11", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueRange, SortOrder: 120, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vce"), req("Ic")),
				set(2, req("Vcb"), req("Ie")),
				set(3, req("Vce"), req("Ie")),
			}},
		// h22 — выходная полная проводимость в схеме с общей базой (h22б).
		{Code: "h22", Group: "electrical", Unit: "uS",
			ValueType: catalog.ValueAtMost, SortOrder: 130, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vcb"), req("Ie"), opt("freq")),
				set(2, req("Vcb"), req("Ic"), opt("freq")),
			}},
		{Code: "Cobo", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 140, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vcb")),
				set(2, fixed("Vcb", 0)),
			}},
		{Code: "Cibo", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 150, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Veb"), opt("temp")),
				set(2, fixed("Veb", 0), opt("temp")),
			}},
		{Code: "Tauc", Group: "electrical", Unit: "ps",
			ValueType: catalog.ValueAtMost, SortOrder: 160, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vcb"), req("Ie")),
				set(2, req("Vcb"), req("Ie"), opt("freq")),
				set(3, req("Vcb"), req("Ic")),
			}},
		{Code: "Ton", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 170, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ic"), req("Ib"))}},
		{Code: "Toff", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 180, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ic"), req("Ib"))}},
		// Время рассасывания (2Т205, 2Т301, 2Т312, 2Т314, 2Т317, 2Т333,
		// 2Т336, 2Т348, 2Т202, 1ТМ115, …).
		{Code: "ts", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 190, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Ic"), req("Ib")),
				set(2, req("Vcb"), req("Ic")),
			}},
		// Коэффициент шума — единственная величина с наборами схем и
		// биполярных (Vce/Vcb, Ic/Ie), и полевых (Vds+Vgs=0, Vds+Id).
		{Code: "NF", Group: "electrical", Unit: "dB",
			ValueType: catalog.ValueAtMost, SortOrder: 200, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vce"), req("Ic"), req("freq"), opt("Rg")),
				set(2, req("Vcb"), req("Ie"), req("freq"), opt("Rg")),
				set(3, req("Vce"), req("Ie"), req("freq"), opt("Rg")),
				set(4, req("Vcb"), req("Ic"), req("freq"), opt("Rg")),
				set(5, req("Vds"), fixed("Vgs", 0), req("freq"), opt("Rg")),
				set(6, req("Vds"), req("Id"), req("freq"), opt("Rg")),
			}},
		{Code: "Pout", Group: "electrical", Unit: "W",
			ValueType: catalog.ValueAtLeast, SortOrder: 210, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Vce"), opt("Ic"))}},
		{Code: "Gp", Group: "electrical", Unit: "dB",
			ValueType: catalog.ValueAtLeast, SortOrder: 220, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Vce"), opt("Ic"))}},
		{Code: "Eta", Group: "electrical", Unit: "pct",
			ValueType: catalog.ValueAtLeast, Ceiling: ceiling(100), SortOrder: 230, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Vce"), opt("Ic"))}},

		// Диоды — электрические параметры (§6.3).
		{Code: "Vf", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 240, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("If"))}},
		{Code: "Ir", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 250, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vr"), opt("temp"))}},
		{Code: "Ctot", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 260, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vr"), opt("freq"))}},
		{Code: "Cratio", Group: "electrical",
			ValueType: catalog.ValueAtLeast, SortOrder: 270, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vr"))}},
		{Code: "trr", Group: "electrical", Unit: "ns",
			ValueType: catalog.ValueAtMost, SortOrder: 280, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("If"))}},
		{Code: "Vz", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueRange, SortOrder: 290, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Iz"))}},
		{Code: "Rdiff", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtMost, SortOrder: 300, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Iz"))}},
		{Code: "AlphaVz", Group: "electrical", Unit: "pct_per_degC",
			ValueType: catalog.ValueAtMost, SortOrder: 310, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Iz"))}},
		{Code: "Gconv", Group: "electrical", Unit: "dB",
			ValueType: catalog.ValueAtMost, SortOrder: 320, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"))}},
		{Code: "Iv", Group: "electrical", Unit: "mcd",
			ValueType: catalog.ValueRange, SortOrder: 330, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("If"))}},
		{Code: "Lambda", Group: "electrical", Unit: "nm",
			ValueType: catalog.ValueRange, SortOrder: 340, Active: true, Kinds: di},

		// Резисторы — электрические параметры (§6.5); Tolerance, nominalRow и Rins
		// применимы также к конденсаторам (D7), Rins — также к реле (§6.10).
		{Code: "Rnom", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueRange, SortOrder: 350, Active: true, Kinds: re},
		{Code: "Tolerance", Group: "electrical", Unit: "pct",
			ValueType: catalog.ValueAtMost, SortOrder: 360, Active: true, Kinds: reCa},
		{Code: "nominalRow", Group: "electrical",
			ValueType: catalog.ValueEnum, SortOrder: 365, Active: true, Kinds: reCa,
			EnumValues: []string{
				// Ряды предпочтительных величин (ГОСТ 28884-90 / МЭК 60063).
				"E3", "E6", "E12", "E24", "E48", "E96", "E192",
			}},
		{Code: "TCR", Group: "electrical", Unit: "ppm_per_degC",
			ValueType: catalog.ValueAtMost, SortOrder: 370, Active: true, Kinds: re},
		{Code: "Vnoise", Group: "electrical",
			ValueType: catalog.ValueAtMost, SortOrder: 380, Active: true, Kinds: re},
		{Code: "Rins", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtLeast, SortOrder: 390, Active: true, Kinds: reCaRl},

		// Конденсаторы — характеристики (§6.8).
		{Code: "Cnom", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueRange, SortOrder: 400, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "TCC", Group: "electrical",
			ValueType: catalog.ValueEnum, SortOrder: 410, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			EnumValues: []string{
				// Нормируемый ТКЕ (линейный), 10⁻⁶/°C.
				"П100", "П120", "П60", "П33", "МП0", "М33", "М47", "М75",
				"М150", "М220", "М330", "М470", "М750", "М700", "М1500", "М1300", "М2200",
				// Ненормируемый (сегнетокерамика), %.
				"Н10", "Н20", "Н30", "Н50", "Н70", "Н90",
				// Международные коды EIA RS-198: класс 1 — линейный ТКЕ
				// (10⁻⁶/°C), класс 2 — буква нижней границы диапазона +
				// цифра верхней + буква предела изменения ёмкости (S9).
				"C0G", "NP0", "U2J", "X5R", "X6S", "X7R", "X7S", "X8R", "Y5V", "Z5U",
			}},
		{Code: "Vnom", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 420, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "DF", Group: "electrical", Unit: "pct",
			ValueType: catalog.ValueAtMost, SortOrder: 430, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, opt("temp"))}},
		{Code: "Ileak", Group: "electrical", Unit: "uA",
			ValueType: catalog.ValueAtMost, SortOrder: 440, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, req("Vnom"), opt("temp"))}},
		{Code: "Esr", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtMost, SortOrder: 450, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, fixed("freq", 0.1))}},
		{Code: "Iripple", Group: "electrical", Unit: "mA",
			ValueType: catalog.ValueAtMost, SortOrder: 460, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("temp"))}},
		{Code: "opTempMin", Group: "electrical", Unit: "degC",
			ValueType: catalog.ValueExact, AllowNegative: true, ValidationRule: ruleTempPair,
			SortOrder: 470, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "opTempMax", Group: "electrical", Unit: "degC",
			ValueType: catalog.ValueExact, ValidationRule: ruleTempPair,
			SortOrder: 480, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},

		// Полевые транзисторы — электрические параметры (§6.1): собственные
		// коды, величины биполярных не дублируются — пересечений с разделом
		// §6.1 биполярных нет (мощность стока — PdMax §6.2, не PcMax);
		// условия измерения — Vds/Vgs/Id, наборы условий — схемы полевых
		// (исключение по наборам — NF, см. выше). Температурные строки
		// (2П103, КП302, КП303, КП305, 2П337, КП322, …) — опц. temp.
		{Code: "Idss", Group: "electrical", Unit: "mA",
			ValueType: catalog.ValueRange, SortOrder: 490, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vds"), fixed("Vgs", 0)),
			}},
		{Code: "gfs", Group: "electrical", Unit: "mA_per_V",
			ValueType: catalog.ValueAtLeast, SortOrder: 500, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vds"), fixed("Vgs", 0), opt("temp")),
				set(2, req("Vds"), req("Id"), opt("temp")),
			}},
		// Активная составляющая выходной проводимости (2П103, КП201, КП307,
		// КП308, КП350); частота — комментарием к значению.
		{Code: "Gos", Group: "electrical", Unit: "uS",
			ValueType: catalog.ValueAtMost, SortOrder: 510, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Vds"), fixed("Vgs", 0)),
				set(2, req("Vds"), req("Id")),
			}},
		{Code: "VgsOff", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueRange, AllowNegative: true,
			SortOrder: 520, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Id"), opt("Vds"), opt("temp"))}},
		{Code: "VgsTh", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueRange, AllowNegative: true,
			SortOrder: 530, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Id"), opt("Vds"))}},
		{Code: "Igss", Group: "electrical", Unit: "nA",
			ValueType: catalog.ValueAtMost, SortOrder: 540, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vgs"), opt("temp"))}},
		{Code: "Ciss", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 550, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vds"), opt("Vgs"))}},
		{Code: "Crss", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 560, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vds"), opt("Vgs"))}},
		{Code: "Coss", Group: "electrical", Unit: "pF",
			ValueType: catalog.ValueAtMost, SortOrder: 570, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Vds"), opt("Vgs"))}},
		// Нормируется и при Uси+Uзи=0 без тока стока (2П302/КП302,
		// 2П303/КП303, 2П308/КП308).
		{Code: "RdsOn", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtMost, SortOrder: 580, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Id"), req("Vgs")),
				set(2, req("Vds"), fixed("Vgs", 0), opt("temp")),
			}},

		// Транзисторы — предельные эксплуатационные данные (§6.2).
		{Code: "VceoMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 10, Active: true, Kinds: tr},
		{Code: "VcbMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 20, Active: true, Kinds: tr},
		{Code: "VebMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 30, Active: true, Kinds: tr},
		{Code: "IcMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 40, Active: true, Kinds: tr},
		{Code: "IbMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 50, Active: true, Kinds: tr},
		// Наибольший постоянный ток эмиттера (2Т118/КТ118 «ток каждого
		// эмиттера», 2Т203/КТ203, КТ209 «ток эмиттера»).
		{Code: "IeMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 60, Active: true, Kinds: tr},
		{Code: "PcMax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 70, Active: true, Kinds: tr},
		{Code: "VdsMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 80, Active: true, Kinds: tr},
		{Code: "IdMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 90, Active: true, Kinds: tr},
		{Code: "IcPulseMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 100, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "PcPulseMax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 110, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		// Импульсные предельные напряжения (ГТ108, ГТ109, 1ТМ115/1Т115,
		// 1Т116, КТ314); tи — pulse_duration, Rбэ («при Rбэ») — Rbe.
		{Code: "VcbPulseMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 120, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"), opt("Rbe"))}},
		{Code: "VcePulseMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 130, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"), opt("Rbe"))}},
		{Code: "VebPulseMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 140, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Общие предельные данные (§6.2/§6.4/§6.6): транзисторы, диоды,
		// резисторы; TempMin/TempMax — также реле (§6.11).
		{Code: "TempMin", Group: "limiting", Unit: "degC",
			ValueType: catalog.ValueExact, AllowNegative: true, ValidationRule: ruleTempPair,
			SortOrder: 150, Active: true, Kinds: trDiReRl},
		{Code: "TempMax", Group: "limiting", Unit: "degC",
			ValueType: catalog.ValueExact, ValidationRule: ruleTempPair,
			SortOrder: 160, Active: true, Kinds: trDiReRl},
		{Code: "TempJunctionMax", Group: "limiting", Unit: "degC",
			ValueType: catalog.ValueExact, SortOrder: 170, Active: true, Kinds: trDiRe},
		{Code: "Rth", Group: "limiting", Unit: "degC_per_W",
			ValueType: catalog.ValueExact, SortOrder: 180, Active: true, Kinds: trDiRe},

		// Диоды — предельные эксплуатационные данные (§6.4).
		{Code: "VrMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 190, Active: true, Kinds: di},
		{Code: "VrPulseMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 200, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "IfMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 210, Active: true, Kinds: di},
		{Code: "IfPulseMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 220, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "Pmax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 230, Active: true, Kinds: di},
		{Code: "IzMin", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 240, Active: true, Kinds: di},
		{Code: "IzMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 250, Active: true, Kinds: di},

		// Резисторы — предельные эксплуатационные данные (§6.6); мощности —
		// вариантные параметры (D6, правило resistor_variant_power).
		{Code: "Pnom", Group: "limiting", Unit: "W",
			ValueType: catalog.ValueExact, SortOrder: 260, Active: true, Kinds: re},
		{Code: "Vmax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 270, Active: true, Kinds: re},
		{Code: "VPulseMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 280, Active: true, Kinds: re,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Полевые транзисторы — предельные данные (§6.2): величины стока и
		// затвора — собственные коды, у биполярных им соответствуют PcMax
		// (мощность коллектора) и IcPulseMax/PcPulseMax (импульсные величины
		// коллектора); пересечений кодов нет.
		{Code: "VgsMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, AllowNegative: true,
			SortOrder: 290, Active: true, Kinds: tr},
		{Code: "PdMax", Group: "limiting", Unit: "mW",
			ValueType: catalog.ValueExact, SortOrder: 300, Active: true, Kinds: tr},
		{Code: "IdPulseMax", Group: "limiting", Unit: "mA",
			ValueType: catalog.ValueExact, SortOrder: 310, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Реле — электрические параметры (§6.10): обмотка, контакты,
		// времена, изоляция, износостойкость; безусловные (контекст
		// измерения расширяется данными).
		{Code: "VcoilNom", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 590, Active: true, Kinds: rl},
		{Code: "Rcoil", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueRange, SortOrder: 600, Active: true, Kinds: rl},
		{Code: "Pcoil", Group: "electrical", Unit: "W",
			ValueType: catalog.ValueAtMost, SortOrder: 610, Active: true, Kinds: rl},
		{Code: "Vpickup", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtMost, SortOrder: 620, Active: true, Kinds: rl},
		{Code: "Vdropout", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtLeast, SortOrder: 630, Active: true, Kinds: rl},
		{Code: "Ipickup", Group: "electrical", Unit: "mA",
			ValueType: catalog.ValueAtMost, SortOrder: 640, Active: true, Kinds: rl},
		{Code: "Rcontact", Group: "electrical", Unit: "ohm",
			ValueType: catalog.ValueAtMost, SortOrder: 650, Active: true, Kinds: rl},
		{Code: "tOperate", Group: "electrical", Unit: "ms",
			ValueType: catalog.ValueAtMost, SortOrder: 660, Active: true, Kinds: rl},
		{Code: "tRelease", Group: "electrical", Unit: "ms",
			ValueType: catalog.ValueAtMost, SortOrder: 670, Active: true, Kinds: rl},
		{Code: "tBounce", Group: "electrical", Unit: "ms",
			ValueType: catalog.ValueAtMost, SortOrder: 680, Active: true, Kinds: rl},
		{Code: "Vtest", Group: "electrical", Unit: "V",
			ValueType: catalog.ValueAtLeast, SortOrder: 690, Active: true, Kinds: rl},
		{Code: "enduranceElec", Group: "electrical",
			ValueType: catalog.ValueAtLeast, SortOrder: 700, Active: true, Kinds: rl},
		{Code: "enduranceMech", Group: "electrical",
			ValueType: catalog.ValueAtLeast, SortOrder: 710, Active: true, Kinds: rl},

		// Реле — предельные данные (§6.11): коммутируемая способность
		// контактов (ток — в А, в отличие от полупроводниковых мА).
		{Code: "VswitchMax", Group: "limiting", Unit: "V",
			ValueType: catalog.ValueExact, SortOrder: 320, Active: true, Kinds: rl},
		{Code: "IswitchMax", Group: "limiting", Unit: "A",
			ValueType: catalog.ValueExact, SortOrder: 330, Active: true, Kinds: rl},

		// Резисторы — предельные данные переменных/подстроечных и
		// многосекционных (§6.6): механика подвижной системы и климат.
		// Применимость к переменным/многосекционным — регламент наполнения,
		// межполевого правила нет.
		{Code: "imbalance", Group: "limiting", Unit: "dB",
			ValueType: catalog.ValueAtMost, SortOrder: 340, Active: true, Kinds: re},
		{Code: "endurance", Group: "limiting",
			ValueType: catalog.ValueAtLeast, SortOrder: 350, Active: true, Kinds: re},
		{Code: "rotationAngle", Group: "limiting", Unit: "deg",
			ValueType: catalog.ValueAtLeast, SortOrder: 360, Active: true, Kinds: re},
		{Code: "switchAngle", Group: "limiting", Unit: "deg",
			ValueType: catalog.ValueAtMost, SortOrder: 370, Active: true, Kinds: re},
		{Code: "staticFrictionTorque", Group: "limiting", Unit: "mN_m",
			ValueType: catalog.ValueAtMost, SortOrder: 380, Active: true, Kinds: re},
		{Code: "humidity", Group: "limiting", Unit: "pct",
			ValueType: catalog.ValueAtMost, SortOrder: 390, Active: true, Kinds: re,
			ConditionSets: []catalog.ConditionSet{set(1, opt("temp"))}},

		// Общие предельные данные надёжности (§6.12) — применимость пустая
		// (все классы, D7); у конденсаторов — единственные представители
		// секции ratings.
		{Code: "operatingLife", Group: "limiting", Unit: "h",
			ValueType: catalog.ValueAtLeast, SortOrder: 400, Active: true},
		{Code: "shelfLife", Group: "limiting", Unit: "yr",
			ValueType: catalog.ValueAtLeast, SortOrder: 410, Active: true},

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
		{Code: "shaftDiameter", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 80, Active: true},
		{Code: "shaftLength", Group: "dimensional", Unit: "mm",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 90, Active: true},
	}
}

// attributes — стартовый набор атрибутов (docs/plan/03-data-model.md §10).
// Применимость без пометки — все классы (D7).
func attributes() []catalog.AttributeDef {
	trDi := []domain.Kind{domain.KindTransistor, domain.KindDiode}
	return []catalog.AttributeDef{
		{Code: "description", Type: catalog.AttrText, SortOrder: 10, Active: true},
		{Code: "structure", Type: catalog.AttrText,
			SortOrder: 20, Active: true, Kinds: trDi},
		{Code: "polarized", Type: catalog.AttrBool,
			SortOrder: 30, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "functionalChar",
			Type: catalog.AttrEnum, SortOrder: 40, Active: true,
			Kinds: []domain.Kind{domain.KindResistor}, EnumValues: []string{
				// Функциональная характеристика переменных резисторов
				// (ГОСТ 10318): А линейная, Б нелинейная (близка
				// к логарифмической), В нелинейная (близка к обратной
				// логарифмической). Международная номенклатура — слова
				// (по образцу ТКЕ §6.8 обе номенклатуры в одном
				// словаре); буквы A/B/C неприменимы: внешне совпадают
				// с кириллицей и противоречат ГОСТ (в маркировке
				// США/Азии A — логарифмическая (audio), B — линейная).
				"А", "Б", "В",
				"linear", "logarithmic", "antilogarithmic",
			}},
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
		{Code: "spec", Type: catalog.AttrTextList, SortOrder: 130, Active: true},
		{Code: "notes", Type: catalog.AttrTextList, SortOrder: 140, Active: true},
		{Code: "yearFrom", Type: catalog.AttrInt,
			ValidationRule: catalog.RuleYearRange, SortOrder: 150, Active: true},
		{Code: "yearTo", Type: catalog.AttrInt,
			ValidationRule: catalog.RuleYearRange, SortOrder: 160, Active: true},
		{Code: "datasheetUrl", Type: catalog.AttrTextList,
			SortOrder: 170, Active: true},
		{Code: "relayType", Type: catalog.AttrEnum, SortOrder: 180, Active: true,
			Kinds: []domain.Kind{domain.KindRelay}, EnumValues: []string{
				// Тип реле по принципу действия (03-data-model.md §10).
				"electromagnetic", "polarized", "reed", "solid_state", "latching",
			}},
		{Code: "contactForm", Type: catalog.AttrEnum, SortOrder: 190, Active: true,
			Kinds: []domain.Kind{domain.KindRelay}, EnumValues: []string{
				// Контактная группа по НТД: число контактов + функция
				// (З замыкающий, Р размыкающий, П переключающий);
				// международные формы IEC — те же группы латиницей
				// (A = З, B = Р, C = П), по образцу ТКЕ §6.8 обе
				// номенклатуры в одном словаре.
				"1З", "1Р", "1П", "2З", "2Р", "2П", "3П", "4П",
				"1A", "1B", "1C", "2A", "2B", "2C", "3C", "4C",
			}},
		{Code: "sections", Type: catalog.AttrInt, SortOrder: 200, Active: true,
			Kinds: []domain.Kind{domain.KindResistor}},
		{Code: "shaftEnd", Type: catalog.AttrEnum, SortOrder: 210, Active: true,
			Kinds: []domain.Kind{domain.KindResistor}, EnumValues: []string{
				// Вид конца вала переменного резистора (03 §10):
				// нейтральные коды (как relayType) для терминов НТД —
				// slotted шлиц, flatted лыска, plain гладкий,
				// threaded резьбовой.
				"slotted", "flatted", "plain", "threaded",
			}},
	}
}
