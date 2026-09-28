// Package domain — обозначения, каталог параметров, валидация: без зависимостей
// от хранилища. Семантика NULL: nil в полях-указателях означает «не задано».
package domain

import "fmt"

// UserError — ожидаемая ошибка диалога с пользователем (некорректный аргумент,
// данные или отсутствие записи): CLI печатает её с префиксом «Ошибка: ».
type UserError struct {
	Message string
}

func (e *UserError) Error() string { return e.Message }

func NewUserError(format string, args ...any) *UserError {
	return &UserError{Message: fmt.Sprintf(format, args...)}
}

// Ptr — адрес значения: помощник для необязательных полей (аналог C# int?/double?).
func Ptr[T any](v T) *T {
	return &v
}
