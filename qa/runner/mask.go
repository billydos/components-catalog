// Централизованная маскировка секретов DSN на границе отчёта: вывод
// дочерних команд (go test, catalogctl, restsrv) и ошибки драйверов
// включают строки подключения «best effort», а отчёт прогона коммитится
// в репозиторий как свидетельство. Точечная маскировка мест подключения
// не покрывает все пути утечки, поэтому маскируются все строки,
// попадающие в отчёт (report.line): точные вхождения известного DSN ноги
// (любая форма, включая ключевую — storage.MaskDSN) и userinfo URL-формы
// в произвольном тексте.
package main

import (
	"strings"

	"github.com/billydos/components-catalog/internal/storage"
)

// maskSecrets скрывает пароли DSN в строке отчёта: сначала известные
// строки подключения заменяются маскированной формой целиком, затем
// userinfo «схема://user:password@» маскируется и в остальном тексте —
// вывод может содержать переписанный или чужой DSN, не совпадающий с
// зарегистрированным.
func maskSecrets(s string, secrets []string) string {
	for _, dsn := range secrets {
		if dsn != "" {
			s = strings.ReplaceAll(s, dsn, storage.MaskDSN(dsn))
		}
	}
	return maskURLPasswords(s)
}

// maskURLPasswords маскирует пароли userinfo всех URL-подобных фрагментов
// строки («схема://user:password@host…»). Authority ограничивается первым
// из «/?#» или пробельного символа, пароль выделяется по последнему «@»
// до границы (пароль сам может содержать «@») и первому «:» userinfo —
// как в storage.maskURLDSN, но по каждому вхождению в произвольном
// тексте. Фрагменты без пароля не меняются; повторная маскировка уже
// замаскированного фрагмента («user:***@») — тождественная.
func maskURLPasswords(s string) string {
	const sep = "://"
	var b strings.Builder
	written := 0 // сколько уже переписано в builder
	search := 0  // откуда искать следующий «://» (пропущенный не повторяется)
	for {
		i := strings.Index(s[search:], sep)
		if i < 0 {
			break
		}
		i += search
		rest := i + len(sep)
		end := len(s)
		for j := rest; j < len(s); j++ {
			if c := s[j]; c == '/' || c == '?' || c == '#' ||
				c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				end = j
				break
			}
		}
		at := strings.LastIndex(s[rest:end], "@")
		if at < 0 {
			search = rest
			continue
		}
		// userinfo — до «@»; пустой пароль («user:@») не маскируется.
		colon := strings.Index(s[rest:rest+at], ":")
		if colon < 0 || colon+1 == at {
			search = rest
			continue
		}
		b.WriteString(s[written : rest+colon+1])
		b.WriteString("***")
		written = rest + at
		search = written
	}
	b.WriteString(s[written:])
	return b.String()
}
