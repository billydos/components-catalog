package cli

import (
	"fmt"
	"strings"

	"soviettransistors/internal/domain"
)

// describeParameters — раздел справки о параметрах: строки собираются из
// каталога, а не дублируются вручную.
func describeParameters() string {
	lines := []string{"Электрические параметры (jsonc/yaml, ключ \"parameters\"), коды:"}
	for _, info := range domain.ParameterCatalog {
		unit := info.Unit
		if unit == "" {
			unit = "раз"
		}
		bounds := ""
		switch info.Direction {
		case domain.AtLeast:
			bounds = "min"
		case domain.AtMost:
			bounds = "max"
		case domain.AtLeastOrRange:
			bounds = "min [max]"
		}
		line := fmt.Sprintf("  %s (%s) — %s: %s; условия: %s", info.Code, unit, info.DisplayName, bounds, info.Conditions.Describe())
		if info.ValueCeiling != nil {
			line += fmt.Sprintf(" (не более %s %s)", domain.Fmt(*info.ValueCeiling), unit)
		}
		lines = append(lines, line)
	}
	var keyParts []string
	for _, key := range domain.AllConditionKeys {
		keyParts = append(keyParts, fmt.Sprintf("%s (%s)", domain.ConditionJsoncKey(key), domain.ConditionUnit(key)))
	}
	lines = append(lines, fmt.Sprintf("  min/max — значение, условия: %s, temp (°C).", strings.Join(keyParts, ", ")))
	return strings.Join(lines, "\n")
}

func printHelp() int {
	fmt.Println(`Справочник советских транзисторов.

Использование: soviettransistors <команда> [аргументы] [--db <путь>] [--dry-run]

Команды:
  init                                  создать таблицы (команды записи создают базу автоматически)
  import <файл.jsonc|.yaml>             импорт: обозначения, атрибуты, параметры, предельные данные
  add <обозначение> [<обозначение>...]  добавить транзисторы по обозначению (через парсер)
  parse <обозначение>...                разобрать обозначение без обращения к базе
  list [фильтры]                        вывести обозначения из базы
  info <обозначение>                    карточка: атрибуты, производители, параметры, предельные данные
  find <обозначение>                    найти запись по точному обозначению (при отсутствии —
                                        подсказка равнозначной по материалу: Г/1, К/2, А/3, И/4)
  delete <обозначение>...               удалить записи (каскадно с параметрами и предельными)
  count                                 количество записей

Опции:
  --db <путь>     путь к базе SQLite (по умолчанию ./transistors.db или переменная TRANSISTOR_DB)
  --dry-run       только проверка, без записи в базу

Команды list/info/find/count/delete работают только с существующей базой (не создают её)

Фильтры list (--опция=значение или --опция значение):
  --material=Г|1|К|2|А|3|И|4   --subclass=Т|П   --assembly=true|false
  --feature=1..9   --number=1..999   --letters=А|АМ
  --modification=1..9   --chip=1..6   --limit=N

Примеры:
  soviettransistors add КТ315Б
  soviettransistors import sample-data.jsonc
  soviettransistors info КТ315Б
  soviettransistors list --material=К --subclass=П

Формат обозначения: <материал><подкласс>[С]<признак><номер><буквы>[<модификация>][-<бескорп.>]
  материал:   Г/1 — германий, К/2 — кремний, А/3 — арсенид галлия, И/4 — индий
  подкласс:   Т — биполярный, П — полевой; С — сборка
  признак:    цифра 1–9
  номер:      01–999
  буквы:      1–2 заглавные русские буквы
  модификация: цифра 1–9
  бескорпусное исполнение: дефис и цифра 1–6`)
	fmt.Println()
	fmt.Println(describeParameters())
	fmt.Println(`Атрибуты (jsonc/yaml, ключ "attributes"): structure (npn/pnp/n-fet...), technology, package,
  packageMaterial, colorMarking, pinout, esdSensitive/militaryGrade/radiationHardened (bool),
  tu, notes, yearFrom/yearTo (1949–2100), massMax (г), datasheetUrl;
  manufacturers — массив названий заводов (null — не менять, [] — очистить).
Предельные данные (jsonc/yaml, ключ "ratings"): UkeMax/UkbMax/UbeMax/UkeoMax (В), IkMax/IbMax (мА),
  PkMax (мВт), IkPulseMax/PkPulseMax + pulseDuration (мкс), tempMin/tempMax/tempJunctionMax (°C),
  Rth (°C/Вт).`)
	return 0
}
