package domain

import "strings"

// ParameterKind — вид электрического параметра; порядок каталога — канонический.
type ParameterKind string

const (
	KindH21E                             ParameterKind = "H21E"
	KindH21B                             ParameterKind = "H21B"
	KindCutoffFrequency                  ParameterKind = "CutoffFrequency"
	KindCutoffVoltage                    ParameterKind = "CutoffVoltage"
	KindCollectorEmitterSaturation       ParameterKind = "CollectorEmitterSaturation"
	KindBaseEmitterSaturation            ParameterKind = "BaseEmitterSaturation"
	KindCollectorCutoffCurrent           ParameterKind = "CollectorCutoffCurrent"
	KindEmitterCutoffCurrent             ParameterKind = "EmitterCutoffCurrent"
	KindCollectorEmitterCutoffCurrent    ParameterKind = "CollectorEmitterCutoffCurrent"
	KindCollectorEmitterCutoffCurrentRbe ParameterKind = "CollectorEmitterCutoffCurrentRbe"
	KindInputResistance                  ParameterKind = "InputResistance"
	KindCollectorJunctionCapacitance     ParameterKind = "CollectorJunctionCapacitance"
	KindEmitterJunctionCapacitance       ParameterKind = "EmitterJunctionCapacitance"
	KindFeedbackTimeConstant             ParameterKind = "FeedbackTimeConstant"
	KindSwitchOnTime                     ParameterKind = "SwitchOnTime"
	KindSwitchOffTime                    ParameterKind = "SwitchOffTime"
	KindNoiseFigure                      ParameterKind = "NoiseFigure"
	KindOutputPower                      ParameterKind = "OutputPower"
	KindPowerGain                        ParameterKind = "PowerGain"
	KindCollectorEfficiency              ParameterKind = "CollectorEfficiency"
)

// BoundDirection — «не менее» (только min), «не более» (только max)
// или «не менее либо диапазон» (min обязателен, max опционален).
type BoundDirection int

const (
	AtLeast BoundDirection = iota
	AtMost
	AtLeastOrRange
)

// ParameterInfo — параметр справочника: код jsonc, название, единица,
// правило границ, спецификация условий и потолок значения.
type ParameterInfo struct {
	Kind         ParameterKind
	Code         string
	DisplayName  string
	Unit         string // "" — безразмерный
	Direction    BoundDirection
	Conditions   *ConditionSpec
	ValueCeiling *float64
}

// ConditionSpecs — именованные спецификации условий, на которые ссылается каталог.
var (
	// SpecPairUkeIkOrUkbIe — пара Uкэ + Iк (ОЭ) либо Uкб + Iэ (ОБ).
	SpecPairUkeIkOrUkbIe = NewConditionSpec(
		RequireConditionKeys(CondUke, CondIk),
		RequireConditionKeys(CondUkb, CondIe),
	)
	// SpecExactlyOneCurrent — только один ток: Iк либо Iэ.
	SpecExactlyOneCurrent = NewConditionSpec(
		RequireConditionKeys(CondIk),
		RequireConditionKeys(CondIe),
	)
	// SpecOnlyUkb — только Uкб.
	SpecOnlyUkb = NewConditionSpec(RequireConditionKeys(CondUkb))
	// SpecOnlyUeb — только Uэб.
	SpecOnlyUeb = NewConditionSpec(RequireConditionKeys(CondUeb))
	// SpecOnlyUke — только Uкэ.
	SpecOnlyUke = NewConditionSpec(RequireConditionKeys(CondUke))
	// SpecUkeAndIk — Uкэ и Iк.
	SpecUkeAndIk = NewConditionSpec(RequireConditionKeys(CondUke, CondIk))
	// SpecUkbAndIe — Uкб и Iэ (общая база).
	SpecUkbAndIe = NewConditionSpec(RequireConditionKeys(CondUkb, CondIe))
	// SpecUkeAndRbe — Uкэ и сопротивление Rбэ.
	SpecUkeAndRbe = NewConditionSpec(RequireConditionKeys(CondUke, CondRbe))
	// SpecIkAndIb — Iк и Iб.
	SpecIkAndIb = NewConditionSpec(RequireConditionKeys(CondIk, CondIb))
	// SpecPairPlusFrequency — пара Uкэ + Iк либо Uкб + Iэ, обязательная частота
	// и необязательный Rг (KShum).
	SpecPairPlusFrequency = NewConditionSpec(
		RequireConditionKeys(CondUke, CondIk, CondFreq).Optionally(CondRg),
		RequireConditionKeys(CondUkb, CondIe, CondFreq).Optionally(CondRg),
	)
	// SpecFrequencyPlusOptionalUkeIk — обязательная частота; Uкэ + Iк задаются
	// вместе или не задаются вовсе.
	SpecFrequencyPlusOptionalUkeIk = NewConditionSpec(
		RequireConditionKeys(CondFreq),
		RequireConditionKeys(CondFreq, CondUke, CondIk),
	)
)

// ParameterCatalog — каталог параметров (единый источник валидатора, справки,
// списка полей jsonc и CHECK-ограничений); порядок — канонический порядок вида.
var ParameterCatalog = []ParameterInfo{
	{KindH21E, "h21e", "Статический коэффициент передачи тока (ОЭ)", "", AtLeastOrRange, SpecPairUkeIkOrUkbIe, nil},
	{KindH21B, "h21b", "Коэффициент передачи тока (ОБ)", "", AtLeastOrRange, SpecUkbAndIe, nil},
	{KindCutoffFrequency, "FGran", "Граничная частота коэффициента передачи тока", "МГц", AtLeast, SpecPairUkeIkOrUkbIe, nil},
	{KindCutoffVoltage, "UGran", "Граничное напряжение", "В", AtLeast, SpecExactlyOneCurrent, nil},
	{KindCollectorEmitterSaturation, "UkeNas", "Напряжение насыщения коллектор-эмиттер", "В", AtMost, SpecExactlyOneCurrent, nil},
	{KindBaseEmitterSaturation, "UbeNas", "Напряжение насыщения база-эмиттер", "В", AtMost, SpecExactlyOneCurrent, nil},
	{KindCollectorCutoffCurrent, "Ikbo", "Обратный ток коллектора", "мкА", AtMost, SpecOnlyUkb, nil},
	{KindEmitterCutoffCurrent, "Iebo", "Обратный ток эмиттера", "мкА", AtMost, SpecOnlyUeb, nil},
	{KindCollectorEmitterCutoffCurrent, "Ikeo", "Обратный ток коллектор-эмиттер", "мкА", AtMost, SpecOnlyUke, nil},
	{KindCollectorEmitterCutoffCurrentRbe, "Ikep", "Обратный ток коллектор-эмиттер при заданном Rбэ", "мкА", AtMost, SpecUkeAndRbe, nil},
	{KindInputResistance, "h11", "Входное сопротивление", "Ом", AtLeast, SpecUkeAndIk, nil},
	{KindCollectorJunctionCapacitance, "Ck", "Ёмкость коллекторного перехода", "пФ", AtMost, SpecOnlyUkb, nil},
	{KindEmitterJunctionCapacitance, "Ce", "Ёмкость эмиттерного перехода", "пФ", AtMost, SpecOnlyUeb, nil},
	{KindFeedbackTimeConstant, "Tauk", "Постоянная времени цепи обратной связи", "пс", AtMost, SpecUkbAndIe, nil},
	{KindSwitchOnTime, "Ton", "Время включения", "нс", AtMost, SpecIkAndIb, nil},
	{KindSwitchOffTime, "Toff", "Время выключения (рассасывания)", "нс", AtMost, SpecIkAndIb, nil},
	{KindNoiseFigure, "KShum", "Коэффициент шума", "дБ", AtMost, SpecPairPlusFrequency, nil},
	{KindOutputPower, "PVyh", "Выходная мощность", "Вт", AtLeast, SpecFrequencyPlusOptionalUkeIk, nil},
	{KindPowerGain, "KUr", "Коэффициент усиления по мощности", "дБ", AtLeast, SpecFrequencyPlusOptionalUkeIk, nil},
	{KindCollectorEfficiency, "Kpd", "КПД коллектора", "%", AtLeast, SpecFrequencyPlusOptionalUkeIk, Ptr(100.0)},
}

var (
	parameterByKind     = buildParameterByKind()
	parameterKindByCode = buildParameterKindByCode()
)

func buildParameterByKind() map[ParameterKind]ParameterInfo {
	byKind := make(map[ParameterKind]ParameterInfo, len(ParameterCatalog))
	for _, info := range ParameterCatalog {
		byKind[info.Kind] = info
	}
	return byKind
}

func buildParameterKindByCode() map[string]ParameterKind {
	byCode := make(map[string]ParameterKind, len(ParameterCatalog))
	for _, info := range ParameterCatalog {
		byCode[info.Code] = info.Kind
	}
	return byCode
}

func ParameterInfoOf(kind ParameterKind) ParameterInfo {
	return parameterByKind[kind]
}

func TryParameterKindByCode(code string) (ParameterKind, bool) {
	kind, ok := parameterKindByCode[code]
	return kind, ok
}

func ParameterCodesList() string {
	codes := make([]string, len(ParameterCatalog))
	for i, info := range ParameterCatalog {
		codes[i] = info.Code
	}
	return strings.Join(codes, ", ")
}

// ElectricalParameter — электрический параметр при одном наборе условий
// (единицы канонические: В, мА, мкА, МГц, дБ/%, Ом, пс, нс, пФ, °C).
type ElectricalParameter struct {
	Kind     ParameterKind
	ValueMin *float64
	ValueMax *float64
	Uke      *float64
	Ukb      *float64
	Ueb      *float64
	Ik       *float64
	Ie       *float64
	Ib       *float64
	Freq     *float64
	Rg       *float64
	Rbe      *float64
	Temp     *float64
}
