package domain

import "fmt"

func ValidateTransistor(t Transistor) []string {
	var errors []string
	if !IsValidMaterialSymbol(t.Material) {
		errors = append(errors, fmt.Sprintf("материал «%c» должен быть Г/1, К/2, А/3 или И/4", t.Material))
	}
	if t.Subclass != 'Т' && t.Subclass != 'П' {
		errors = append(errors, fmt.Sprintf("подкласс «%c» должен быть Т или П", t.Subclass))
	}
	if t.Feature < 1 || t.Feature > 9 {
		errors = append(errors, fmt.Sprintf("характерный эксплуатационный признак %d должен быть от 1 до 9", t.Feature))
	}
	if t.DevelopmentNumber < 1 || t.DevelopmentNumber > 999 {
		errors = append(errors, fmt.Sprintf("порядковый номер разработки %d должен быть от 1 до 999", t.DevelopmentNumber))
	}
	if !IsUpperLetters(t.Letters) {
		errors = append(errors, fmt.Sprintf("классификация «%s» должна быть одной или двумя заглавными русскими буквами", t.Letters))
	}
	if t.Modification != nil && (*t.Modification < 1 || *t.Modification > 9) {
		errors = append(errors, fmt.Sprintf("модификация %d должна быть от 1 до 9", *t.Modification))
	}
	if t.ChipVariant != nil && (*t.ChipVariant < 1 || *t.ChipVariant > 6) {
		errors = append(errors, fmt.Sprintf("бескорпусное исполнение %d должно быть от 1 до 6", *t.ChipVariant))
	}
	return errors
}
