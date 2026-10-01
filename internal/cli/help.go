package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/billydos/components-catalog/internal/domain"
)

// usageText — общая справка (catalogctl без аргументов и catalogctl help).
const usageText = `catalogctl — консольная утилита справочника электронных компонентов

Команды:
  init                              создать и инициализировать базу (сиди каталога)
  parse <обозначение>…              разбор обозначений (автодетект класса и системы)
  add <обозначение>…                добавить записи-обозначения (--kind для неоднозначных)
  import <файл> [--dry-run]         импорт файла наполнения (jsonc/yaml/ndjson;
                                    «-» — stdin, формат jsonc)
  list [фильтры]                    список записей (--kind --system --material --subclass
                                    --junctions --group --series --number --letters
                                    --q --limit --offset)
  info <обозначение>                карточка записи
  find <обозначение>                точный поиск; подсказка равнозначной по материалу (gost)
  delete <обозначение>… [--dry-run] удаление (каскад)
  count [--kind]                    число записей
  export [--kind] [--format]        экспорт записей (round-trip; jsonc|yaml|ndjson)
  catalog export [--format]         экспорт каталога
  catalog import <файл> [--dry-run] расширение каталога (секция catalog)
  catalog list                      справка из каталога (классы, системы, группы, параметры)
  help [команда]                    эта справка

Общие опции:
  --dialect sqlite|postgres         диалект хранилища (по умолчанию sqlite)
  --db <путь>                       файл базы sqlite (по умолчанию catalog.db)
  --dsn <строка>                    строка подключения (приоритетнее --db)
  --kind <код>                      класс приборов (transistor|diode|resistor|capacitor)
  --system <код>                    система обозначений (gost|ost|pro|jedec|jis|series|other)
  --dry-run                         контрольный прогон без записи в базу
  --format jsonc|yaml|ndjson        формат экспорта (по умолчанию jsonc)

Ожидаемые ошибки выводятся с префиксом «Ошибка: », прочие —
«Непредвиденная ошибка: »; код выхода при ошибках — 1.
`

// runHelp — справка по командам (help [команда]).
func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		PrintError(stderr, domain.NewError(domain.CodeValidationFailed,
			fmt.Sprintf("неизвестная команда «%s»; справка: catalogctl help", args[0])))
		return 1
	}
	fmt.Fprintf(stdout, "формат: catalogctl %s\n", cmd.usage)
	return 0
}

// командаСправка sorted — используется тестом полноты справки.
func commandNames() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
