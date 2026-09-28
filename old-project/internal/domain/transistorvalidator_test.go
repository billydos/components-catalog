package domain

import "testing"

func validTransistor() Transistor {
	return Transistor{
		Material:          'К',
		Subclass:          'Т',
		Feature:           3,
		DevelopmentNumber: 315,
		Letters:           "Б",
	}
}

func TestValidTransistor_NoErrors(t *testing.T) {
	if errors := ValidateTransistor(validTransistor()); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
	withOptions := validTransistor()
	withOptions.Modification = Ptr(2)
	withOptions.ChipVariant = Ptr(3)
	if errors := ValidateTransistor(withOptions); len(errors) > 0 {
		t.Errorf("ожидалось отсутствие ошибок, получено: %v", errors)
	}
}

func TestInvalidMaterial_ProducesError(t *testing.T) {
	for _, material := range []rune{'Х', 'K', '5'} {
		transistor := validTransistor()
		transistor.Material = material
		hasErrorsWithFragment(t, ValidateTransistor(transistor), "материал")
	}
}

func TestInvalidSubclass_ProducesError(t *testing.T) {
	transistor := validTransistor()
	transistor.Subclass = 'Б'
	hasErrorsWithFragment(t, ValidateTransistor(transistor), "подкласс")
}

func TestInvalidFeature_ProducesError(t *testing.T) {
	for _, feature := range []int{0, 10} {
		transistor := validTransistor()
		transistor.Feature = feature
		hasErrorsWithFragment(t, ValidateTransistor(transistor), "признак")
	}
}

func TestInvalidDevelopmentNumber_ProducesError(t *testing.T) {
	for _, number := range []int{0, 1000} {
		transistor := validTransistor()
		transistor.DevelopmentNumber = number
		hasErrorsWithFragment(t, ValidateTransistor(transistor), "номер разработки")
	}
}

func TestInvalidLetters_ProduceError(t *testing.T) {
	for _, letters := range []string{"", "БВГ", "б"} {
		transistor := validTransistor()
		transistor.Letters = letters
		hasErrorsWithFragment(t, ValidateTransistor(transistor), "классификация")
	}
}

func TestInvalidModification_ProducesError(t *testing.T) {
	for _, modification := range []int{0, 10} {
		transistor := validTransistor()
		transistor.Modification = Ptr(modification)
		hasErrorsWithFragment(t, ValidateTransistor(transistor), "модификация")
	}
}

func TestInvalidChipVariant_ProducesError(t *testing.T) {
	for _, chip := range []int{0, 7} {
		transistor := validTransistor()
		transistor.ChipVariant = Ptr(chip)
		hasErrorsWithFragment(t, ValidateTransistor(transistor), "бескорпусное")
	}
}
