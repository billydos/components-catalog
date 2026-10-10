package storage

import (
	"net/url"
	"strings"
)

// MaskDSN скрывает секрет в DSN для безопасного отображения (вывод CLI,
// отчёты QA, сообщения тестов): в URL-форме маскируется пароль userinfo,
// в ключевой форме — значения всех ключей password (конфликт ключей у
// pgx решается поздним значением — маскируются все). DSN без
// опознаваемого пароля возвращается без изменений; не разбираемая
// URL-форма маскируется целиком — безопаснее потерять наглядность, чем
// показать секрет.
func MaskDSN(dsn string) string {
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
// остальных частей исходной строки. Пароль с неэкранированными «/ ? #»
// съедает границу authority и «@» не находится в ней — верный признак
// «@» позже границы; такие формы не опознаются надёжно и маскируются
// целиком: потерять наглядность дешевле, чем показать секрет. Форма,
// которую не разбирает net/url (например, «scheme://user:password» без
// хоста — «password» читается как порт), не доверяется вовсе.
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
	authority := rest[:end]
	at := strings.LastIndex(authority, "@")
	if at < 0 {
		if strings.Contains(rest[end:], "@") {
			return "***"
		}
		return dsn
	}
	userinfo := authority[:at]
	colon := strings.Index(userinfo, ":")
	if colon < 0 || userinfo[colon+1:] == "" {
		return dsn
	}
	return dsn[:i+3] + userinfo[:colon+1] + "***" + rest[at:]
}

// maskKeywordDSN — форма «ключ=значение ключ=значение …»: значения всех
// ключей password маскируются. Пары обходятся по правилам разбора
// ключевой формы pgx (пробелы вокруг «=» допускаются; пробелы внутри
// значения — только в кавычках '…'/"…" или после экранирующего «\»),
// поэтому «password=» внутри кавычек значения соседнего ключа ключом
// не считается.
func maskKeywordDSN(dsn string) string {
	var b strings.Builder
	s := strings.TrimLeft(dsn, dsnSpaces)
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			b.WriteString(s)
			break
		}
		key := strings.TrimRight(s[:eq], dsnSpaces)
		s = strings.TrimLeft(s[eq+1:], dsnSpaces)
		var val string
		switch {
		case len(s) > 0 && s[0] == '\'':
			rest := skipQuotedValue(s[1:], '\'')
			val, s = s[:len(s)-len(rest)], rest
		case len(s) > 0 && s[0] == '"':
			rest := skipQuotedValue(s[1:], '"')
			val, s = s[:len(s)-len(rest)], rest
		default:
			rest := skipUnquotedValue(s)
			val, s = s[:len(s)-len(rest)], rest
		}
		if strings.EqualFold(key, "password") {
			b.WriteString(key + "=***")
		} else {
			b.WriteString(key + "=" + val)
		}
		s = strings.TrimLeft(s, dsnSpaces)
		if len(s) > 0 {
			b.WriteByte(' ')
		}
	}
	return b.String()
}
