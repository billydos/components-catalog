package cli

import (
	"fmt"
	"io"

	"github.com/billydos/components-catalog/internal/domain"
)

// Префиксы вывода ошибок CLI — дословный контракт
// (plan/04-module-functionality.md §3); закреплены тестами.
const (
	// ErrorPrefix — префикс ожидаемых ошибок (*domain.Error).
	ErrorPrefix = "Ошибка: "
	// UnexpectedErrorPrefix — префикс прочих (непредвиденных) ошибок.
	UnexpectedErrorPrefix = "Непредвиденная ошибка: "
)

// PrintError печатает ошибку с контрактным префиксом и переводом строки:
// ожидаемые ошибки (*domain.Error) — «Ошибка: », прочие —
// «Непредвиденная ошибка: ». Код выхода при ошибках — 1.
func PrintError(w io.Writer, err error) {
	if err == nil {
		return
	}
	prefix := UnexpectedErrorPrefix
	if _, ok := domain.AsError(err); ok {
		prefix = ErrorPrefix
	}
	fmt.Fprintln(w, prefix+err.Error())
}
