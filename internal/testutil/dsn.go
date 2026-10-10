// Локальная копия storage.MaskDSN: импорт storage в пакет testutil
// запрещён циклом (тесты storage импортируют testutil — тот же приём,
// что и у списка catalogTables). Эквивалентность оригиналу закреплена
// пин-тестом dsn_test.go.

package testutil

import (
	"net/url"
	"strings"
)

// maskDSN скрывает секрет в DSN для безопасного отображения (вывод CLI,
// отчёты QA, сообщения тестов): в URL-форме маскируется пароль userinfo,
// в ключевой форме — значения всех ключей password (конфликт ключей у
// pgx решается поздним значением — маскируются все). DSN без
// опознаваемого пароля возвращается без изменений; не разбираемая
// URL-форма маскируется целиком — безопаснее потерять наглядность, чем
// показать секрет.
func maskDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}
	if strings.Contains(dsn, "://") {
		return maskURLDSN(dsn)
	}
	return maskKeywordDSN(dsn)
}

// maskURLDSN — форма «схема://user:password@host/db?параметры»: пароль
// userinfo (до «@», разделяющего authority) замещается без перекодировки
// остальных частей исходной строки. Форма, которую не разбирает net/url
// (например, «scheme://user:password» без хоста — «password» читается
// как порт), не доверяется вовсе.
func maskURLDSN(dsn string) string {
	if _, err := url.Parse(dsn); err != nil {
		return "***"
	}
	i := strings.Index(dsn, "://")
	if i < 0 {
		return dsn
	}
	rest := dsn[i+3:]
	end := len(rest)
	for j := 0; j < len(rest); j++ {
		if rest[j] == '/' || rest[j] == '?' || rest[j] == '#' {
			end = j
			break
		}
	}
	at := strings.LastIndex(rest[:end], "@")
	if at < 0 {
		return dsn
	}
	userinfo := rest[:at]
	colon := strings.Index(userinfo, ":")
	if colon < 0 || userinfo[colon+1:] == "" {
		return dsn
	}
	return dsn[:i+3] + userinfo[:colon+1] + "***" + rest[at:]
}

// maskKeywordDSN — форма «ключ=значение ключ=значение …»: значения всех
// ключей password маскируются. Незакавыченное значение продолжается до
// первого неэкранированного пробела (обратная косая черта экранирует
// следующий символ — как при разборе ключевой формы pgx); открывающая
// кавычка продлевает значение до парной (незакрытая — до конца строки).
func maskKeywordDSN(dsn string) string {
	lower := strings.ToLower(dsn)
	var b strings.Builder
	written := 0
	search := 0
	for {
		i := strings.Index(lower[search:], "password=")
		if i < 0 {
			break
		}
		i += search
		if i > 0 && !isSpaceByte(dsn[i-1]) {
			search = i + len("password=")
			continue
		}
		start := i + len("password=")
		end := start
		if end < len(dsn) && (dsn[end] == '\'' || dsn[end] == '"') {
			quote := dsn[end]
			end++
			for end < len(dsn) && dsn[end] != quote {
				end++
			}
			if end < len(dsn) {
				end++
			}
		} else {
			escaped := false
			for end < len(dsn) {
				c := dsn[end]
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if isSpaceByte(c) {
					break
				}
				end++
			}
		}
		b.WriteString(dsn[written:start])
		b.WriteString("***")
		written = end
		search = end
	}
	b.WriteString(dsn[written:])
	return b.String()
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
