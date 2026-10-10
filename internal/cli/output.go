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
	prefix := i18n.Message(lang, string(domain.MsgCliUnexpectedPrefix))
	if _, ok := domain.AsError(err); ok {
		prefix = i18n.Message(lang, string(domain.MsgCliErrPrefix))
	}
	fmt.Fprintln(w, prefix+errorString(lang, err))
}

// errorString — текст ошибки по локали: каждая доменная ошибка дерева
// (цепочки Unwrap и errors.Join) рендерится каталогом по её собственным
// MsgID+Args; обёртки сохраняют собственный рендер (префикс «%s: %w»,
// соединение Join). Замена — по вычисленному каноническому тексту
// конкретной ошибки, а не по первому вхождению подстроки.
func errorString(lang i18n.Language, err error) string {
	if s, ok := localizedErrorText(lang, err); ok {
		return s
	}
	return err.Error()
}

// localizedErrorText реконструирует текст ошибки с локализованными
// доменными ошибками; false — доменных ошибок в дереве нет. Сегмент
// дочерней ошибки ищется в тексте узла по порядку следования ветвей Join.
func localizedErrorText(lang i18n.Language, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if de, ok := err.(*domain.Error); ok {
		return i18n.Message(lang, string(de.MsgID), de.Args...), true
	}
	if inner := errors.Unwrap(err); inner != nil {
		s, ok := localizedErrorText(lang, inner)
		if !ok {
			return "", false
		}
		return strings.Replace(err.Error(), inner.Error(), s, 1), true
	}
	if joiner, ok := err.(interface{ Unwrap() []error }); ok {
		out := err.Error()
		from, found := 0, false
		for _, child := range joiner.Unwrap() {
			if child == nil {
				continue
			}
			s, ok := localizedErrorText(lang, child)
			if !ok {
				continue
			}
			found = true
			canonical := child.Error()
			i := strings.Index(out[from:], canonical)
			if i < 0 {
				return err.Error(), true
			}
			i += from
			out = out[:i] + s + out[i+len(canonical):]
			from = i + len(s)
		}
		if found {
			return out, true
		}
		return "", false
	}
	return "", false
}
