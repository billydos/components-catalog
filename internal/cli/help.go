package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// usageText — общая справка (catalogctl без аргументов и catalogctl help)
// по локали вывода (--lang; строки каталога сообщений cli_usage).
func usageText(lang i18n.Language) string {
	return i18n.Message(lang, "cli_usage")
}

// Тексты справки (эталон переносов строк; канонический en — cli_usage
// бандла, ru — cli_usage ru-бандла).
const usageTextRu = `catalogctl — консольная утилита справочника электронных компонентов

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
  --lang en|ru                      язык отображаемых строк (по умолчанию en)
  --dry-run                         контрольный прогон без записи в базу
  --format jsonc|yaml|ndjson        формат экспорта (по умолчанию jsonc)

Ожидаемые ошибки выводятся с префиксом «Ошибка: », прочие —
«Непредвиденная ошибка: »; код выхода при ошибках — 1.
`

const usageTextEn = `catalogctl — console utility of the electronic components catalog

Commands:
  init                              create and initialize the database (catalog seeds)
  parse <designation>…              parse designations (kind and system autodetect)
  add <designation>…                add designation records (--kind for ambiguous ones)
  import <file> [--dry-run]         import a fill file (jsonc/yaml/ndjson;
                                    «-» — stdin, jsonc format)
  list [filters]                    list records (--kind --system --material --subclass
                                    --junctions --group --series --number --letters
                                    --q --limit --offset)
  info <designation>                record card
  find <designation>                exact search; suggestion of equivalent by material (gost)
  delete <designation>… [--dry-run] deletion (cascade)
  count [--kind]                    number of records
  export [--kind] [--format]        export records (round-trip; jsonc|yaml|ndjson)
  catalog export [--format]         export the catalog
  catalog import <file> [--dry-run] extend the catalog (catalog section)
  catalog list                      catalog reference (kinds, systems, groups, parameters)
  help [command]                    this help

Common options:
  --dialect sqlite|postgres         storage dialect (sqlite by default)
  --db <path>                       sqlite database file (catalog.db by default)
  --dsn <string>                    connection string (takes precedence over --db)
  --kind <code>                     device kind (transistor|diode|resistor|capacitor)
  --system <code>                   designation system (gost|ost|pro|jedec|jis|series|other)
  --lang en|ru                      display language (en by default)
  --dry-run                         dry run without writing to the database
  --format jsonc|yaml|ndjson        export format (jsonc by default)

Expected errors are printed with the «Error: » prefix, others —
«Unexpected error: »; exit code on errors — 1.
`

// runHelp — справка по командам (help [команда] [--lang en|ru]).
func runHelp(args []string, stdout, stderr io.Writer) int {
	lang := i18n.En
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--lang" {
			if l, ok := i18n.ParseLanguage(args[i+1]); ok {
				lang = l
			}
		}
	}
	if len(args) == 0 {
		fmt.Fprint(stdout, usageText(lang))
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		PrintError(stderr, i18n.En, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgCliUnknownCommand, args[0]))
		return 1
	}
	fmt.Fprintf(stdout, "%s catalogctl %s\n", i18n.Message(lang, "cli_usage_word"), cmd.usage)
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
