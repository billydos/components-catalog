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
		kinds = append(kinds, catalog.KindDef{Code: k, Name: k.Name()})
	}

	systems := make([]catalog.SystemDef, 0, len(domain.Systems()))
	for _, s := range domain.Systems() {
		systems = append(systems, catalog.SystemDef{Code: s, Name: s.Name(), Description: s.Description()})
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
		def := catalog.SeriesFamilyDef{Series: f.Series, Kind: f.Kind, Name: f.Name}
		if f.Power {
			def.TailSemantic = catalog.TailSemanticPower
		}
		families = append(families, def)
	}

	rules := make([]catalog.RuleDef, 0, 8)
	for _, r := range catalog.Rules() {
		rules = append(rules, catalog.RuleDef{Code: r.Code(), Description: r.Description()})
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
// Производные единицы отображения (мкФ, МОм, кГц) не хранятся — их
// форматирует слой вывода.
func units() []catalog.UnitDef {
	return []catalog.UnitDef{
		{Code: "В", Name: "вольт", Symbol: "В"},
		{Code: "мВ", Name: "милливольт", Symbol: "мВ"},
		{Code: "мА", Name: "миллиампер", Symbol: "мА"},
		{Code: "мкА", Name: "микроампер", Symbol: "мкА"},
		{Code: "А", Name: "ампер", Symbol: "А"},
		{Code: "МГц", Name: "мегагерц", Symbol: "МГц"},
		{Code: "дБ", Name: "децибел", Symbol: "дБ"},
		{Code: "%", Name: "процент", Symbol: "%"},
		{Code: "Вт", Name: "ватт", Symbol: "Вт"},
		{Code: "мВт", Name: "милливатт", Symbol: "мВт"},
		{Code: "Ом", Name: "ом", Symbol: "Ом"},
		{Code: "пФ", Name: "пикофарад", Symbol: "пФ"},
		{Code: "пс", Name: "пикосекунда", Symbol: "пс"},
		{Code: "нс", Name: "наносекунда", Symbol: "нс"},
		{Code: "мкс", Name: "микросекунда", Symbol: "мкс"},
		{Code: "°C", Name: "градус Цельсия", Symbol: "°C"},
		{Code: "°C/Вт", Name: "градус Цельсия на ватт", Symbol: "°C/Вт"},
		{Code: "10⁻⁶/°C", Name: "миллионная доля на градус Цельсия", Symbol: "10⁻⁶/°C"},
		{Code: "%/°C", Name: "процент на градус Цельсия", Symbol: "%/°C"},
		{Code: "г", Name: "грамм", Symbol: "г"},
		{Code: "мм", Name: "миллиметр", Symbol: "мм"},
		{Code: "нм", Name: "нанометр", Symbol: "нм"},
		{Code: "мкд", Name: "милликандела", Symbol: "мкд"},
	}
}

// conditions — стартовый набор условий измерения (docs/plan/03-data-model.md
// §4): положительные, кроме temp (AllowNegative).
func conditions() []catalog.ConditionDef {
	return []catalog.ConditionDef{
		{Code: "Uke", Name: "напряжение коллектор-эмиттер", Unit: "В"},
		{Code: "Ukb", Name: "напряжение коллектор-база", Unit: "В"},
		{Code: "Ueb", Name: "напряжение эмиттер-база", Unit: "В"},
		{Code: "Ik", Name: "ток коллектора", Unit: "мА"},
		{Code: "Ie", Name: "ток эмиттера", Unit: "мА"},
		{Code: "Ib", Name: "ток базы", Unit: "мА"},
		{Code: "Uobr", Name: "обратное напряжение (диоды)", Unit: "В"},
		{Code: "Upr", Name: "прямое напряжение (диоды)", Unit: "В"},
		{Code: "Ipr", Name: "прямой ток (диоды)", Unit: "мА"},
		{Code: "Ist", Name: "ток стабилизации (стабилитроны)", Unit: "мА"},
		{Code: "Unom", Name: "номинальное напряжение (условие тока утечки)", Unit: "В"},
		{Code: "Rg", Name: "сопротивление генератора", Unit: "Ом"},
		{Code: "Rbe", Name: "сопротивление в цепи база-эмиттер", Unit: "Ом"},
		{Code: "Rn", Name: "сопротивление нагрузки", Unit: "Ом"},
		{Code: "freq", Name: "частота", Unit: "МГц"},
		{Code: "pulse_duration", Name: "длительность импульса", Unit: "мкс"},
		{Code: "temp", Name: "температура среды (может быть отрицательной)", Unit: "°C", AllowNegative: true},
	}
}

// groups — группы параметров: секции файла наполнения и REST
// (docs/plan/03-data-model.md §5).
func groups() []catalog.GroupDef {
	return []catalog.GroupDef{
		{Code: "electrical", SectionName: "parameters", DisplayName: "Электрические параметры", SortOrder: 10},
		{Code: "limiting", SectionName: "ratings", DisplayName: "Предельные эксплуатационные данные", SortOrder: 20},
		{Code: "dimensional", SectionName: "dimensions", DisplayName: "Массогабаритные данные", SortOrder: 30},
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
		{Code: "h21e", Group: "electrical", DisplayName: "статический коэффициент передачи тока h21э",
			ValueType: catalog.ValueRange, SortOrder: 10, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Uke"), req("Ik")),
				set(2, req("Ukb"), req("Ie")),
			}},
		{Code: "h21b", Group: "electrical", DisplayName: "коэффициент передачи тока в схеме с общей базой h21б",
			ValueType: catalog.ValueAtLeast, SortOrder: 20, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"), req("Ie"))}},
		{Code: "FGran", Group: "electrical", DisplayName: "граничная частота передачи тока", Unit: "МГц",
			ValueType: catalog.ValueAtLeast, SortOrder: 30, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Uke"), req("Ik")),
				set(2, req("Ukb"), req("Ie")),
			}},
		{Code: "UGran", Group: "electrical", DisplayName: "граничное напряжение", Unit: "В",
			ValueType: catalog.ValueAtLeast, SortOrder: 40, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik")), set(2, req("Ie"))}},
		{Code: "UkeNas", Group: "electrical", DisplayName: "напряжение насыщения коллектор-эмиттер", Unit: "В",
			ValueType: catalog.ValueAtMost, SortOrder: 50, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik")), set(2, req("Ie"))}},
		{Code: "UbeNas", Group: "electrical", DisplayName: "напряжение насыщения база-эмиттер", Unit: "В",
			ValueType: catalog.ValueAtMost, SortOrder: 60, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik")), set(2, req("Ie"))}},
		{Code: "Ikbo", Group: "electrical", DisplayName: "обратный ток коллектора", Unit: "мкА",
			ValueType: catalog.ValueAtMost, SortOrder: 70, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"), opt("temp"))}},
		{Code: "Iebo", Group: "electrical", DisplayName: "обратный ток эмиттера", Unit: "мкА",
			ValueType: catalog.ValueAtMost, SortOrder: 80, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ueb"), opt("temp"))}},
		{Code: "Ikeo", Group: "electrical", DisplayName: "обратный ток коллектор-эмиттер", Unit: "мкА",
			ValueType: catalog.ValueAtMost, SortOrder: 90, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uke"), opt("temp"))}},
		{Code: "Ikep", Group: "electrical",
			DisplayName: "ток коллектор-эмиттер при заданном сопротивлении цепи база-эмиттер", Unit: "мкА",
			ValueType: catalog.ValueAtMost, SortOrder: 100, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uke"), req("Rbe"))}},
		{Code: "h11", Group: "electrical", DisplayName: "входное сопротивление в схеме с общим эмиттером (h11)", Unit: "Ом",
			ValueType: catalog.ValueRange, SortOrder: 110, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uke"), req("Ik"))}},
		{Code: "Ck", Group: "electrical", DisplayName: "ёмкость коллекторного перехода", Unit: "пФ",
			ValueType: catalog.ValueAtMost, SortOrder: 120, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"))}},
		{Code: "Ce", Group: "electrical", DisplayName: "ёмкость эмиттерного перехода", Unit: "пФ",
			ValueType: catalog.ValueAtMost, SortOrder: 130, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ueb"))}},
		{Code: "Tauk", Group: "electrical", DisplayName: "постоянная времени коллекторной цепи", Unit: "пс",
			ValueType: catalog.ValueAtMost, SortOrder: 140, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ukb"), req("Ie"))}},
		{Code: "Ton", Group: "electrical", DisplayName: "время включения", Unit: "нс",
			ValueType: catalog.ValueAtMost, SortOrder: 150, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik"), req("Ib"))}},
		{Code: "Toff", Group: "electrical", DisplayName: "время выключения", Unit: "нс",
			ValueType: catalog.ValueAtMost, SortOrder: 160, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ik"), req("Ib"))}},
		{Code: "KShum", Group: "electrical", DisplayName: "коэффициент шума", Unit: "дБ",
			ValueType: catalog.ValueAtMost, SortOrder: 170, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{
				set(1, req("Uke"), req("Ik"), req("freq"), opt("Rg")),
				set(2, req("Ukb"), req("Ie"), req("freq"), opt("Rg")),
			}},
		{Code: "PVyh", Group: "electrical", DisplayName: "выходная мощность", Unit: "Вт",
			ValueType: catalog.ValueAtLeast, SortOrder: 180, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Uke"), opt("Ik"))}},
		{Code: "KUr", Group: "electrical", DisplayName: "коэффициент усиления по мощности", Unit: "дБ",
			ValueType: catalog.ValueAtLeast, SortOrder: 190, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Uke"), opt("Ik"))}},
		{Code: "Kpd", Group: "electrical", DisplayName: "коэффициент полезного действия", Unit: "%",
			ValueType: catalog.ValueAtLeast, Ceiling: ceiling(100), SortOrder: 200, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("Uke"), opt("Ik"))}},

		// Диоды — электрические параметры (§6.3).
		{Code: "Upr", Group: "electrical", DisplayName: "прямое напряжение", Unit: "В",
			ValueType: catalog.ValueAtMost, SortOrder: 210, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ipr"))}},
		{Code: "Iobr", Group: "electrical", DisplayName: "обратный ток", Unit: "мкА",
			ValueType: catalog.ValueAtMost, SortOrder: 220, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uobr"), opt("temp"))}},
		{Code: "Cn", Group: "electrical", DisplayName: "общая ёмкость", Unit: "пФ",
			ValueType: catalog.ValueAtMost, SortOrder: 230, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uobr"), opt("freq"))}},
		{Code: "Cp", Group: "electrical", DisplayName: "коэффициент перекрытия по ёмкости, раз",
			ValueType: catalog.ValueAtLeast, SortOrder: 240, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Uobr"))}},
		{Code: "trr", Group: "electrical", DisplayName: "время обратного восстановления", Unit: "нс",
			ValueType: catalog.ValueAtMost, SortOrder: 250, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ipr"))}},
		{Code: "Ust", Group: "electrical", DisplayName: "напряжение стабилизации", Unit: "В",
			ValueType: catalog.ValueRange, SortOrder: 260, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ist"))}},
		{Code: "Rdiff", Group: "electrical", DisplayName: "дифференциальное сопротивление", Unit: "Ом",
			ValueType: catalog.ValueAtMost, SortOrder: 270, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ist"))}},
		{Code: "TkUst", Group: "electrical", DisplayName: "температурный коэффициент напряжения стабилизации", Unit: "%/°C",
			ValueType: catalog.ValueAtMost, SortOrder: 280, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ist"))}},
		{Code: "Kpr", Group: "electrical", DisplayName: "коэффициент преобразования", Unit: "дБ",
			ValueType: catalog.ValueAtMost, SortOrder: 290, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"))}},
		{Code: "Iv", Group: "electrical", DisplayName: "сила света", Unit: "мкд",
			ValueType: catalog.ValueRange, SortOrder: 300, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("Ipr"))}},
		{Code: "Lambda", Group: "electrical", DisplayName: "длина волны излучения", Unit: "нм",
			ValueType: catalog.ValueRange, SortOrder: 310, Active: true, Kinds: di},

		// Резисторы — электрические параметры (§6.5); Dop и Riz применимы
		// также к конденсаторам (D7).
		{Code: "Rnom", Group: "electrical", DisplayName: "номинальное сопротивление (границы ряда)", Unit: "Ом",
			ValueType: catalog.ValueRange, SortOrder: 320, Active: true, Kinds: re},
		{Code: "Dop", Group: "electrical", DisplayName: "допуск", Unit: "%",
			ValueType: catalog.ValueAtMost, SortOrder: 330, Active: true, Kinds: reCa},
		{Code: "TKS", Group: "electrical", DisplayName: "температурный коэффициент сопротивления", Unit: "10⁻⁶/°C",
			ValueType: catalog.ValueAtMost, SortOrder: 340, Active: true, Kinds: re},
		{Code: "Ush", Group: "electrical", DisplayName: "уровень собственных шумов, мкВ/В",
			ValueType: catalog.ValueAtMost, SortOrder: 350, Active: true, Kinds: re},
		{Code: "Riz", Group: "electrical", DisplayName: "сопротивление изоляции", Unit: "Ом",
			ValueType: catalog.ValueAtLeast, SortOrder: 360, Active: true, Kinds: reCa},

		// Конденсаторы — характеристики (§6.8).
		{Code: "Cnom", Group: "electrical", DisplayName: "номинальная ёмкость (границы ряда)", Unit: "пФ",
			ValueType: catalog.ValueRange, SortOrder: 370, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "TKE", Group: "electrical", DisplayName: "температурный коэффициент ёмкости (ТКЕ)",
			ValueType: catalog.ValueEnum, SortOrder: 380, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			EnumValues: []string{
				// Нормируемый ТКЕ (линейный), 10⁻⁶/°C.
				"П100", "П120", "П60", "П33", "МП0", "М33", "М47", "М75",
				"М150", "М220", "М330", "М470", "М750", "М700", "М1500", "М1300", "М2200",
				// Ненормируемый (сегнетокерамика), %.
				"Н10", "Н20", "Н30", "Н50", "Н70", "Н90",
			}},
		{Code: "Unom", Group: "electrical", DisplayName: "номинальное напряжение", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 390, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "Tgd", Group: "electrical", DisplayName: "тангенс угла потерь", Unit: "%",
			ValueType: catalog.ValueAtMost, SortOrder: 400, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, opt("temp"))}},
		{Code: "Iut", Group: "electrical", DisplayName: "ток утечки", Unit: "мкА",
			ValueType: catalog.ValueAtMost, SortOrder: 410, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, req("Unom"), opt("temp"))}},
		{Code: "Esr", Group: "electrical", DisplayName: "эквивалентное последовательное сопротивление", Unit: "Ом",
			ValueType: catalog.ValueAtMost, SortOrder: 420, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, fixed("freq", 0.1))}},
		{Code: "Iripple", Group: "electrical", DisplayName: "допустимый ток пульсаций", Unit: "мА",
			ValueType: catalog.ValueAtMost, SortOrder: 430, Active: true, Kinds: []domain.Kind{domain.KindCapacitor},
			ConditionSets: []catalog.ConditionSet{set(1, req("freq"), opt("temp"))}},
		{Code: "opTempMin", Group: "electrical", DisplayName: "наименьшая рабочая температура", Unit: "°C",
			ValueType: catalog.ValueExact, AllowNegative: true, ValidationRule: ruleTempPair,
			SortOrder: 440, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "opTempMax", Group: "electrical", DisplayName: "наибольшая рабочая температура", Unit: "°C",
			ValueType: catalog.ValueExact, ValidationRule: ruleTempPair,
			SortOrder: 450, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},

		// Транзисторы — предельные эксплуатационные данные (§6.2).
		{Code: "UkeoMax", Group: "limiting", DisplayName: "наибольшее напряжение коллектор-эмиттер", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 10, Active: true, Kinds: tr},
		{Code: "UkbMax", Group: "limiting", DisplayName: "наибольшее напряжение коллектор-база", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 20, Active: true, Kinds: tr},
		{Code: "UbeMax", Group: "limiting", DisplayName: "наибольшее напряжение эмиттер-база", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 30, Active: true, Kinds: tr},
		{Code: "IkMax", Group: "limiting", DisplayName: "наибольший ток коллектора", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 40, Active: true, Kinds: tr},
		{Code: "IbMax", Group: "limiting", DisplayName: "наибольший ток базы", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 50, Active: true, Kinds: tr},
		{Code: "PkMax", Group: "limiting",
			DisplayName: "наибольшая рассеиваемая мощность коллектора (у полевых — мощность стока)", Unit: "мВт",
			ValueType: catalog.ValueExact, SortOrder: 60, Active: true, Kinds: tr},
		{Code: "UsiMax", Group: "limiting", DisplayName: "наибольшее напряжение сток-исток", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 70, Active: true, Kinds: tr},
		{Code: "IsiMax", Group: "limiting", DisplayName: "наибольший ток стока", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 80, Active: true, Kinds: tr},
		{Code: "IkPulseMax", Group: "limiting", DisplayName: "наибольший импульсный ток коллектора", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 90, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "PkPulseMax", Group: "limiting", DisplayName: "наибольшая импульсная рассеиваемая мощность", Unit: "мВт",
			ValueType: catalog.ValueExact, SortOrder: 100, Active: true, Kinds: tr,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Общие предельные данные (§6.2/§6.4/§6.6): транзисторы, диоды,
		// резисторы.
		{Code: "TempMin", Group: "limiting", DisplayName: "наименьшая температура среды", Unit: "°C",
			ValueType: catalog.ValueExact, AllowNegative: true, ValidationRule: ruleTempPair,
			SortOrder: 110, Active: true, Kinds: trDiRe},
		{Code: "TempMax", Group: "limiting", DisplayName: "наибольшая температура среды", Unit: "°C",
			ValueType: catalog.ValueExact, ValidationRule: ruleTempPair,
			SortOrder: 120, Active: true, Kinds: trDiRe},
		{Code: "TempJunctionMax", Group: "limiting", DisplayName: "наибольшая температура перехода", Unit: "°C",
			ValueType: catalog.ValueExact, SortOrder: 130, Active: true, Kinds: trDiRe},
		{Code: "Rth", Group: "limiting", DisplayName: "тепловое сопротивление", Unit: "°C/Вт",
			ValueType: catalog.ValueExact, SortOrder: 140, Active: true, Kinds: trDiRe},

		// Диоды — предельные эксплуатационные данные (§6.4).
		{Code: "UobrMax", Group: "limiting", DisplayName: "наибольшее постоянное обратное напряжение", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 150, Active: true, Kinds: di},
		{Code: "UobrImpMax", Group: "limiting", DisplayName: "наибольшее импульсное обратное напряжение", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 160, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "IprMax", Group: "limiting", DisplayName: "наибольший прямой ток", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 170, Active: true, Kinds: di},
		{Code: "IprImpMax", Group: "limiting", DisplayName: "наибольший импульсный прямой ток", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 180, Active: true, Kinds: di,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},
		{Code: "Pmax", Group: "limiting", DisplayName: "наибольшая рассеиваемая мощность", Unit: "мВт",
			ValueType: catalog.ValueExact, SortOrder: 190, Active: true, Kinds: di},
		{Code: "IstMin", Group: "limiting", DisplayName: "наименьший ток стабилизации", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 200, Active: true, Kinds: di},
		{Code: "IstMax", Group: "limiting", DisplayName: "наибольший ток стабилизации", Unit: "мА",
			ValueType: catalog.ValueExact, SortOrder: 210, Active: true, Kinds: di},

		// Резисторы — предельные эксплуатационные данные (§6.6); мощности —
		// вариантные параметры (D6, правило resistor_variant_power).
		{Code: "Pnom", Group: "limiting", DisplayName: "номинальная мощность рассеяния", Unit: "Вт",
			ValueType: catalog.ValueExact, SortOrder: 220, Active: true, Kinds: re},
		{Code: "Umax", Group: "limiting", DisplayName: "наибольшее рабочее напряжение", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 230, Active: true, Kinds: re},
		{Code: "UimpMax", Group: "limiting", DisplayName: "наибольшее импульсное напряжение", Unit: "В",
			ValueType: catalog.ValueExact, SortOrder: 240, Active: true, Kinds: re,
			ConditionSets: []catalog.ConditionSet{set(1, req("pulse_duration"))}},

		// Массогабаритные данные (§6.7) — все классы.
		{Code: "massMax", Group: "dimensional", DisplayName: "масса (наибольшая)", Unit: "г",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 10, Active: true},
		{Code: "length", Group: "dimensional", DisplayName: "длина", Unit: "мм",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 20, Active: true},
		{Code: "width", Group: "dimensional", DisplayName: "ширина", Unit: "мм",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 30, Active: true},
		{Code: "height", Group: "dimensional", DisplayName: "высота", Unit: "мм",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 40, Active: true},
		{Code: "diameter", Group: "dimensional", DisplayName: "диаметр", Unit: "мм",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 50, Active: true},
		{Code: "leadLength", Group: "dimensional", DisplayName: "длина выводов", Unit: "мм",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 60, Active: true},
		{Code: "leadPitch", Group: "dimensional", DisplayName: "шаг выводов", Unit: "мм",
			ValueType: catalog.ValueExact, ValidationRule: ruleDims, SortOrder: 70, Active: true},
	}
}

// attributes — стартовый набор атрибутов (docs/plan/03-data-model.md §9).
// Применимость без пометки — все классы (D7).
func attributes() []catalog.AttributeDef {
	trDi := []domain.Kind{domain.KindTransistor, domain.KindDiode}
	return []catalog.AttributeDef{
		{Code: "category", DisplayName: "категория прибора", Type: catalog.AttrText, SortOrder: 10, Active: true},
		{Code: "structure", DisplayName: "структура (p-n-p / n-p-n, тип канала)", Type: catalog.AttrText,
			SortOrder: 20, Active: true, Kinds: trDi},
		{Code: "polarized", DisplayName: "полярный (электролитический)", Type: catalog.AttrBool,
			SortOrder: 30, Active: true, Kinds: []domain.Kind{domain.KindCapacitor}},
		{Code: "functionalChar", DisplayName: "функциональная характеристика (закон изменения сопротивления)",
			Type: catalog.AttrEnum, SortOrder: 40, Active: true,
			Kinds: []domain.Kind{domain.KindResistor}, EnumValues: []string{"А", "Б", "В"}},
		{Code: "technology", DisplayName: "технология", Type: catalog.AttrText, SortOrder: 50, Active: true},
		{Code: "package", DisplayName: "корпус", Type: catalog.AttrText, SortOrder: 60, Active: true},
		{Code: "packageMaterial", DisplayName: "материал корпуса", Type: catalog.AttrText, SortOrder: 70, Active: true},
		{Code: "colorMarking", DisplayName: "цветовая маркировка", Type: catalog.AttrText, SortOrder: 80, Active: true},
		{Code: "pinout", DisplayName: "цоколёвка", Type: catalog.AttrText, SortOrder: 90, Active: true},
		{Code: "esdSensitive", DisplayName: "чувствительность к статическому электричеству", Type: catalog.AttrBool,
			SortOrder: 100, Active: true},
		{Code: "militaryGrade", DisplayName: "приёмка для военного применения", Type: catalog.AttrBool,
			SortOrder: 110, Active: true},
		{Code: "radiationHardened", DisplayName: "стойкость к ионизирующему излучению", Type: catalog.AttrBool,
			SortOrder: 120, Active: true},
		{Code: "tu", DisplayName: "технические условия", Type: catalog.AttrText, SortOrder: 130, Active: true},
		{Code: "notes", DisplayName: "примечания", Type: catalog.AttrText, SortOrder: 140, Active: true},
		{Code: "yearFrom", DisplayName: "год начала выпуска", Type: catalog.AttrInt,
			ValidationRule: "year_range", SortOrder: 150, Active: true},
		{Code: "yearTo", DisplayName: "год окончания выпуска", Type: catalog.AttrInt,
			ValidationRule: "year_range", SortOrder: 160, Active: true},
		{Code: "datasheetUrl", DisplayName: "ссылка на документацию (datasheet)", Type: catalog.AttrText,
			SortOrder: 170, Active: true},
	}
}
