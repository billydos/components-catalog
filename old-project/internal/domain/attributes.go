package domain

import (
	"fmt"
	"reflect"
	"strings"
)

// TransistorAttributes — описательные атрибуты транзистора (не измеримые величины).
// nil в полях — «не задано»; пустая строка в текстовых полях — ошибка валидации.
type TransistorAttributes struct {
	// Структура проводимости: npn, pnp, n-fet, p-fet...
	Structure *string
	// Технология изготовления: сплавная, сплавно-диффузионная, планарная...
	Technology *string
	// Корпус: «КТ-13», «TO-92»...
	Package *string
	// Материал корпуса: металл, металлокерамика, пластик...
	PackageMaterial *string
	// Цветовая маркировка (когда обозначения на корпусе нет): «красный», «жёлтая точка»...
	ColorMarking *string
	// Цоколёвка: «КБЭ», «1-Э 2-К 3-Б»...
	Pinout *string
	// Повышенная чувствительность к статическому напряжению.
	EsdSensitive *bool
	// Военное исполнение.
	MilitaryGrade *bool
	// Радиационная стойкость.
	RadiationHardened *bool
	// Обозначение ТУ/ОТУ: «ТУ 11.365.001-71»...
	Tu *string
	// Примечание свободным текстом.
	Notes *string
	// Год начала выпуска (>= 1949).
	YearFrom *int
	// Год окончания выпуска; nil — выпускался на момент описания.
	YearTo *int
	// Масса «не более», г.
	MassMax *float64
	// Ссылка на документацию (скан даташита).
	DatasheetUrl *string
}

// attributeFieldNames — имена полей записи через reflect: единый источник
// (SELECT-спецификации чтения, CHECK chk_any_attribute, проверка «задано хоть
// одно поле»); новое поле записи подхватывается автоматически.
var attributeFieldNames = func() []string {
	t := reflect.TypeOf(TransistorAttributes{})
	names := make([]string, t.NumField())
	for i := range names {
		names[i] = t.Field(i).Name
	}
	return names
}()

// AttributeFieldNames — имена всех полей записи в порядке объявления (совпадают с колонками БД).
func AttributeFieldNames() []string {
	return attributeFieldNames
}

// AttributesHaveAnyValue — задано ли хотя бы одно поле (nil во всех полях — секция пуста).
func AttributesHaveAnyValue(a *TransistorAttributes) bool {
	v := reflect.ValueOf(*a)
	for i := 0; i < v.NumField(); i++ {
		if !v.Field(i).IsNil() {
			return true
		}
	}
	return false
}

const (
	MinAttributeYear = 1949
	MaxAttributeYear = 2100
)

func ValidateTransistorAttributes(a *TransistorAttributes) []string {
	var errors []string
	addTextAttribute(a.Structure, "структура", &errors)
	addTextAttribute(a.Technology, "технология", &errors)
	addTextAttribute(a.Package, "корпус", &errors)
	addTextAttribute(a.PackageMaterial, "материал корпуса", &errors)
	addTextAttribute(a.ColorMarking, "цветовая маркировка", &errors)
	addTextAttribute(a.Pinout, "цоколёвка", &errors)
	addTextAttribute(a.Tu, "ТУ", &errors)
	addTextAttribute(a.Notes, "примечание", &errors)
	addTextAttribute(a.DatasheetUrl, "ссылка на документацию", &errors)

	if a.YearFrom != nil && (*a.YearFrom < MinAttributeYear || *a.YearFrom > MaxAttributeYear) {
		errors = append(errors, fmt.Sprintf("год начала выпуска должен быть от %d до %d, получено %d", MinAttributeYear, MaxAttributeYear, *a.YearFrom))
	}
	if a.YearTo != nil && (*a.YearTo < MinAttributeYear || *a.YearTo > MaxAttributeYear) {
		errors = append(errors, fmt.Sprintf("год окончания выпуска должен быть от %d до %d, получено %d", MinAttributeYear, MaxAttributeYear, *a.YearTo))
	}
	if a.YearFrom != nil && a.YearTo != nil && *a.YearFrom >= *a.YearTo {
		errors = append(errors, fmt.Sprintf("год начала выпуска (%d) должен быть меньше года окончания (%d)", *a.YearFrom, *a.YearTo))
	}
	if a.MassMax != nil && *a.MassMax <= 0 {
		errors = append(errors, "масса «не более» должна быть положительной")
	}
	return errors
}

func addTextAttribute(value *string, name string, errors *[]string) {
	if value != nil && len(strings.TrimSpace(*value)) == 0 {
		*errors = append(*errors, fmt.Sprintf("%s: пустое значение (null — «не задано», но не пустая строка)", name))
	}
}
