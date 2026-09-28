package domain

import (
	"fmt"
	"reflect"
)

// MaximumRatings — предельные эксплуатационные данные
// (единицы канонические: В, мА, мВт, мкс, °C, °C/Вт).
type MaximumRatings struct {
	UkeMax *float64
	UkbMax *float64
	UbeMax *float64
	// Постоянное напряжение коллектор-эмиттер при разомкнутой базе, В.
	UkeoMax *float64
	IkMax   *float64
	IbMax   *float64
	PkMax   *float64
	// Импульсный ток коллектора, мА (при PulseDuration).
	IkPulseMax *float64
	// Импульсная рассеиваемая мощность, мВт (при PulseDuration).
	PkPulseMax *float64
	// Длительность импульса, мкс; обязательна при импульсных значениях.
	PulseDuration *float64
	// Минимальная температура среды, °C (может быть отрицательной).
	TempMin *float64
	// Максимальная температура среды, °C.
	TempMax *float64
	// Максимальная температура перехода, °C.
	TempJunctionMax *float64
	// Тепловое сопротивление переход-корпус, °C/Вт.
	Rth *float64
}

// ratingFieldNames — имена полей предельных данных через reflect: единый
// источник (ключи jsonc в importer'е; список может использовать storage),
// по образцу attributeFieldNames.
var ratingFieldNames = func() []string {
	t := reflect.TypeOf(MaximumRatings{})
	names := make([]string, t.NumField())
	for i := range names {
		names[i] = t.Field(i).Name
	}
	return names
}()

// RatingFieldNames — имена всех полей предельных данных в порядке объявления
// (совпадают с ключами jsonc и колонками БД).
func RatingFieldNames() []string {
	return ratingFieldNames
}

func ValidateMaximumRatings(r *MaximumRatings) []string {
	var errors []string
	addPositiveLimit(r.UkeMax, "Uкэ макс", &errors)
	addPositiveLimit(r.UkbMax, "Uкб макс", &errors)
	addPositiveLimit(r.UbeMax, "Uбэ макс", &errors)
	addPositiveLimit(r.UkeoMax, "Uкэо макс", &errors)
	addPositiveLimit(r.IkMax, "Iк макс", &errors)
	addPositiveLimit(r.IbMax, "Iб макс", &errors)
	addPositiveLimit(r.PkMax, "Pк макс", &errors)
	addPositiveLimit(r.IkPulseMax, "импульсный Iк макс", &errors)
	addPositiveLimit(r.PkPulseMax, "импульсная Pк макс", &errors)
	addPositiveLimit(r.PulseDuration, "длительность импульса", &errors)
	addPositiveLimit(r.TempJunctionMax, "T перехода макс", &errors)
	addPositiveLimit(r.Rth, "тепловое сопротивление", &errors)

	if r.TempMin != nil && r.TempMax != nil && *r.TempMin >= *r.TempMax {
		errors = append(errors, fmt.Sprintf("Tмин (%s) должна быть меньше Tмакс (%s)", Fmt(*r.TempMin), Fmt(*r.TempMax)))
	}

	hasPulseValues := r.IkPulseMax != nil || r.PkPulseMax != nil
	if hasPulseValues && r.PulseDuration == nil {
		errors = append(errors, "длительность импульса (pulseDuration, мкс) обязательна, если задан импульсный ток или мощность")
	}
	if !hasPulseValues && r.PulseDuration != nil {
		errors = append(errors, "длительность импульса задана, но импульсные ток/мощность не заданы")
	}
	return errors
}

func addPositiveLimit(value *float64, name string, errors *[]string) {
	if value != nil && *value <= 0 {
		*errors = append(*errors, fmt.Sprintf("%s: значение должно быть положительным", name))
	}
}
