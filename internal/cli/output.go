package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Префиксы вывода ошибок CLI — строки каталога сообщений (cli_err_prefix,
// cli_unexpected_prefix; этап 8.3): канонический en, ru — бандл; закреплены
// тестами. Код выхода при ошибках — 1.

// PrintError печатает ошибку с префиксом локали и переводом строки:
// ожидаемые ошибки (*domain.Error) — «Error: »/«Ошибка: », прочие —
// «Unexpected error: »/«Непредвиденная ошибка: ».
func PrintError(w io.Writer, lang i18n.Language, err error) {
	if err == nil {
		return
	}
	prefix := i18n.Message(lang, "cli_unexpected_prefix")
	if _, ok := domain.AsError(err); ok {
		prefix = i18n.Message(lang, "cli_err_prefix")
	}
	fmt.Fprintln(w, prefix+errorString(lang, err))
}

// errorString — текст ошибки по локали: ожидаемая ошибка домена
// рендерится каталогом (MsgID+Args), обёртки «%s: %w» сохраняют префикс.
func errorString(lang i18n.Language, err error) string {
	var de *domain.Error
	for e := err; e != nil; e = errors.Unwrap(e) {
		if d, ok := e.(*domain.Error); ok {
			de = d
		}
	}
	if de == nil {
		return err.Error()
	}
	loc := i18n.Message(lang, string(de.MsgID), de.Args...)
	return strings.Replace(err.Error(), de.Message, loc, 1)
}
