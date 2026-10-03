package i18n

// Каталог сообщений (этап 8.3 плана работ, решение D9). Ключ — стабильный
// MsgID (латиница snake_case, константы в internal/domain/messages.go);
// значение — формат fmt с позиционными аргументами. Канонический язык —
// en: его форматы обязательны для всех MsgID реестра (полнота закреплена
// тестом полноты), ru — полная локаль. Рендер: запрошенная локаль → en →
// сам MsgID (последняя инстанция — защитный fallback, не контракт).

import (
	"fmt"
	"strconv"
)

// msgArg — аргумент-сообщение: MsgID с собственными аргументами,
// разворачиваемый при рендере (например, фрагменты «ожидалось: …» в
// ошибках разбора обозначений).
type msgArg struct {
	id   string
	args []any
}

// Arg возвращает аргумент-сообщение: при рендере родительского формата
// подставляется локализованный текст вложенного MsgID.
func Arg(id string, args ...any) msgArg { return msgArg{id: id, args: args} }

// renderArgs разворачивает аргументы-сообщения рекурсивно.
func renderArgs(l Language, args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		if m, ok := a.(msgArg); ok {
			out[i] = Message(l, m.id, m.args...)
			continue
		}
		out[i] = a
	}
	return out
}

// Canonical — канонический (en) текст сообщения по MsgID; неизвестный
// MsgID — сам идентификатор (защитный fallback).
func Canonical(id string, args ...any) string {
	f, ok := messageFormatsEn[id]
	if !ok || f == "" {
		return id
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, renderArgs(En, args)...)
}

// Message — текст сообщения по локали: ru → en (fallback) → MsgID.
func Message(l Language, id string, args ...any) string {
	if !l.IsValid() {
		l = En
	}
	if l != En {
		if f, ok := messageFormatsRu[id]; ok && f != "" {
			if len(args) == 0 {
				return f
			}
			return fmt.Sprintf(f, renderArgs(l, args)...)
		}
	}
	return Canonical(id, args...)
}

// HasMessage — наличие формата у MsgID в языке (для тестов полноты).
func HasMessage(l Language, id string) bool {
	var m map[string]string
	switch l {
	case En:
		m = messageFormatsEn
	case Ru:
		m = messageFormatsRu
	}
	f, ok := m[id]
	return ok && f != ""
}

// MessageIDs — все MsgID канонического каталога (для тестов полноты).
func MessageIDs() []string {
	out := make([]string, 0, len(messageFormatsEn))
	for id := range messageFormatsEn {
		out = append(out, id)
	}
	return out
}

// DecimalSeparator — разделитель дробной части по локали (en — точка,
// ru — запятая; для отображения чисел, этап 8.4).
func DecimalSeparator(l Language) string {
	if l == Ru {
		return ","
	}
	return "."
}

// FormatNumber — компактная запись числа с разделителем локали
// (этап 8.4: слой вывода).
func FormatNumber(l Language, v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if d := DecimalSeparator(l); d != "." {
		for i := 0; i < len(s); i++ {
			if s[i] == '.' {
				return s[:i] + d + s[i+1:]
			}
		}
	}
	return s
}
