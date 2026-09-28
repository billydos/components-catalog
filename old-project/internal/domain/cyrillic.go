package domain

// IsUpperLetter — заглавная буква русского алфавита (А–Я, Ё).
func IsUpperLetter(c rune) bool {
	return (c >= 'А' && c <= 'Я') || c == 'Ё'
}

// IsUpperLetters — строка из одной или двух заглавных русских букв.
func IsUpperLetters(s string) bool {
	if s == "" {
		return false
	}
	letters := []rune(s)
	if len(letters) > 2 {
		return false
	}
	for _, c := range letters {
		if !IsUpperLetter(c) {
			return false
		}
	}
	return true
}
