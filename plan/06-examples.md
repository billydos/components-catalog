# 06. Примеры записи наполнения

Примеры иллюстрируют форму записи наполнения по каталогам `03-data-model.md` (классы, системы обозначений, параметры, атрибуты, варианты, аналоги); корневая структура файлов наполнения и форматы (jsonc/yaml/ndjson) — `04-module-functionality.md`.

Форма секций групп параметров **едина для всех групп** (`parameters`, `ratings`, `dimensions`) — список объектов `{ "parameter": <код>, значение, условия }`; значение — `value` (exact) | `min`/`max` | `text` (text/enum). Условия — соседние ключи по `jsonc_key` условия.

Примеры разбиты по классам — один блок кода соответствует секции одного класса в файле наполнения. Внутри блока записи сгруппированы по системам обозначений, **по три записи на систему**. Записи минимальны: они демонстрируют форму записи и отличия систем обозначений, а не полноту данных типа; поля разбора (материал, подкласс, семейство) в файле не задаются — их даёт разбор обозначения. Ключ `system` проставлен каждой записи явно — чтобы система читалась из самой записи, без комментариев; в рабочем наполнении он нужен только для `other` (автодетекта нет) и при неоднозначности автодетекта, в остальных случаях систему определяет автодетект (формы `gost`/`ost` у резисторов не пересекаются — `С`/`СП` против `Р`/`РП`). Аналоги в примерах ссылаются только на записи своего блока. Предметные атрибуции примеров выверены — `07-r1-verification.md`.

## 1. Транзисторы

```jsonc
{
  "transistors": [
    // gost (ГОСТ 10862-64): материал и подкласс кодируются обозначением
    {
      "name": "КТ315Б",
      "system": "gost",
      "attributes": {
        "structure": "npn", "package": "КТ-13", "tu": "ЖК3.365.200ТУ",
        "yearFrom": 1967, "yearTo": 1992,
        "manufacturers": ["Восход", "Терма"]
      },
      "parameters": [
        { "parameter": "h21e",  "min": 50, "max": 350, "Uke": 10, "Ik": 1 },
        { "parameter": "Ikbo",  "max": 0.5, "Ukb": 10, "temp": 25 }
      ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 20 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 100 }
      ],
      "analogs": ["BC547B", "2SC1815"]
    },
    {
      "name": "ГТ109Г",                     // Г — германий, Т — биполярный
      "system": "gost",
      "attributes": { "structure": "pnp" },
      "parameters": [
        { "parameter": "h21e", "min": 50, "max": 120, "Uke": 5, "Ik": 10 },
        { "parameter": "Ikbo", "max": 100, "Ukb": 5 }
      ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 15 },
        { "parameter": "PkMax", "value": 150 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ]
    },
    {
      "name": "2Т914А-1",                   // пара «2Т» ≡ «КТ» физически равнозначна, записи раздельные; -1 — бескорпусное
      "system": "gost",
      "attributes": { "structure": "npn" },
      "parameters": [ { "parameter": "PVyh", "min": 2, "freq": 400 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 36 },
        { "parameter": "PkMax", "value": 5000 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 100 }
      ]
    },

    // pro (PRO ELECTRON): материал и класс прибора — первые две буквы
    {
      "name": "BC547B",                     // B — кремний, C — маломощный НЧ; суффикс B — группа по усилению
      "system": "pro",
      "attributes": { "structure": "npn", "package": "TO-92" },
      "parameters": [
        { "parameter": "h21e", "min": 200, "max": 450, "Uke": 5, "Ik": 2 },
        { "parameter": "FGran", "min": 300, "Uke": 5, "Ik": 2 }
      ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 45 },
        { "parameter": "IkMax", "value": 100 },
        { "parameter": "PkMax", "value": 500 }
      ]
    },
    {
      "name": "AD161",                      // A — германий, D — мощный НЧ
      "system": "pro",
      "attributes": { "structure": "pnp", "package": "TO-66" },
      "parameters": [ { "parameter": "h21e", "min": 80, "max": 200, "Uke": 2, "Ik": 500 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 32 },
        { "parameter": "IkMax", "value": 1000 },
        { "parameter": "PkMax", "value": 6000 },
        { "parameter": "TempMin", "value": -40 },
        { "parameter": "TempMax", "value": 90 }
      ]
    },
    {
      "name": "BF245",                      // полевой: параметры полевых (ток стока, отсечка) — вне стартового каталога `03-data-model.md` §6.1
      "system": "pro",
      "attributes": { "structure": "n-канал" },
      "ratings": [
        { "parameter": "UkeoMax", "value": 30 },
        { "parameter": "IkMax", "value": 25 },
        { "parameter": "PkMax", "value": 300 }
      ]
    },

    // jedec: материал и категория не кодируются — задаются атрибутами; один номер — много производителей
    {
      "name": "2N2222A",
      "system": "jedec",
      "attributes": { "structure": "npn", "category": "универсальный", "package": "TO-18" },
      "parameters": [ { "parameter": "h21e", "min": 100, "max": 300, "Uke": 10, "Ik": 10 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 40 },
        { "parameter": "IkMax", "value": 800 },
        { "parameter": "TempMin", "value": -65 },
        { "parameter": "TempMax", "value": 150 }
      ]
    },
    {
      "name": "2N3055",
      "system": "jedec",
      "attributes": {
        "structure": "npn", "category": "низкочастотный мощный", "package": "TO-3",
        "manufacturers": ["Motorola", "RCA", "ST"]
      },
      "parameters": [ { "parameter": "h21e", "min": 20, "max": 70, "Uke": 4, "Ik": 4000 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 60 },
        { "parameter": "IkMax", "value": 15000 },
        { "parameter": "PkMax", "value": 115000 },
        { "parameter": "TempJunctionMax", "value": 200 }
      ]
    },
    {
      "name": "2N3904",
      "system": "jedec",
      "attributes": { "structure": "npn", "category": "универсальный", "package": "TO-92" },
      "parameters": [ { "parameter": "h21e", "min": 100, "max": 300, "Uke": 10, "Ik": 10 } ],
      "ratings": [ { "parameter": "UkeoMax", "value": 40 }, { "parameter": "IkMax", "value": 200 } ],
      "analogs": ["КТ315Б"]
    },

    // jis (JIS/EIAJ): переходы и класс кодируются — A p-n-p ВЧ, C n-p-n ВЧ, K n-канал ПТ
    {
      "name": "2SA1015",
      "system": "jis",
      "attributes": { "structure": "pnp", "package": "TO-92" },
      "parameters": [ { "parameter": "h21e", "min": 120, "max": 240, "Uke": 6, "Ik": 2 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 50 },
        { "parameter": "IkMax", "value": 150 },
        { "parameter": "PkMax", "value": 400 }
      ]
    },
    {
      "name": "2SC1815",                    // группа hFE — суффиксом обозначения
      "system": "jis",
      "attributes": { "structure": "npn", "package": "TO-92" },
      "parameters": [ { "parameter": "h21e", "min": 70, "max": 700, "Uke": 6, "Ik": 2 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 50 },
        { "parameter": "IkMax", "value": 150 },
        { "parameter": "PkMax", "value": 400 }
      ]
    },
    {
      "name": "2SK1058",
      "system": "jis",
      "attributes": { "structure": "n-канал", "package": "TO-3P" },
      "ratings": [
        { "parameter": "UkeoMax", "value": 160 },
        { "parameter": "IkMax", "value": 7000 },
        { "parameter": "PkMax", "value": 100000 }
      ]
    },

    // series: семейство из реестра series_families + слабый хвост; материал и категория — атрибутами
    {
      "name": "МП39",
      "system": "series",
      "attributes": { "structure": "pnp", "category": "низкочастотный" },
      "parameters": [
        { "parameter": "h21e", "min": 20, "max": 50, "Uke": 5, "Ik": 5 },
        { "parameter": "Ikbo", "max": 15, "Ukb": 5 }
      ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 15 },
        { "parameter": "IkMax", "value": 150 },
        { "parameter": "PkMax", "value": 150 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ]
    },
    {
      "name": "П214",
      "system": "series",
      "attributes": { "structure": "pnp", "category": "низкочастотный мощный" },
      "parameters": [ { "parameter": "h21e", "min": 20, "max": 80, "Uke": 10, "Ik": 2000 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 60 },
        { "parameter": "IkMax", "value": 5000 },
        { "parameter": "PkMax", "value": 10000 },
        { "parameter": "TempMax", "value": 85 }
      ],
      "analogs": ["AD161"]                              // пара из разных систем — главный случай
    },
    {
      "name": "TIP120",
      "system": "series",
      "attributes": { "structure": "npn", "category": "составной", "package": "TO-220" },
      "parameters": [ { "parameter": "h21e", "min": 1000, "Uke": 3, "Ik": 3000 } ],
      "ratings": [
        { "parameter": "UkeoMax", "value": 60 },
        { "parameter": "IkMax", "value": 5000 },
        { "parameter": "PkMax", "value": 65000 }
      ]
    },

    // other: разбора нет, ключ system обязателен; классификация — атрибутами
    {
      "name": "MJE340",
      "system": "other",
      "attributes": { "structure": "npn", "category": "универсальный", "package": "TO-126" },
      "ratings": [
        { "parameter": "UkeoMax", "value": 300 },
        { "parameter": "IkMax", "value": 500 },
        { "parameter": "PkMax", "value": 20000 }
      ]
    },
    {
      "name": "IRF540",
      "system": "other",
      "attributes": { "structure": "n-канал", "category": "МОП мощный", "package": "TO-220" },
      "ratings": [
        { "parameter": "UkeoMax", "value": 100 },
        { "parameter": "IkMax", "value": 28000 },
        { "parameter": "PkMax", "value": 150000 }
      ]
    },
    {
      "name": "JANTX2N3055",                // MIL-форма на базе JEDEC-номера — вне строгих систем
      "system": "other",
      "attributes": { "structure": "npn", "category": "низкочастотный мощный", "package": "TO-3", "militaryGrade": true },
      "ratings": [
        { "parameter": "UkeoMax", "value": 60 },
        { "parameter": "IkMax", "value": 15000 },
        { "parameter": "PkMax", "value": 115000 }
      ]
    }
  ]
}
```

## 2. Диоды

```jsonc
{
  "diodes": [
    // gost (ГОСТ 10862-64): Д выпрямительный, ДС сборка, С стабилитрон, Л светодиод
    {
      "name": "КДС111В",                    // сборка: признак С после подкласса Д
      "system": "gost",
      "parameters": [ { "parameter": "Upr", "max": 1, "Ipr": 5 } ],
      "ratings": [
        { "parameter": "UobrMax", "value": 50 },
        { "parameter": "IprMax", "value": 20 }
      ]
    },
    {
      "name": "КС168А",                     // стабилитрон 6.8 В
      "system": "gost",
      "parameters": [
        { "parameter": "Ust", "min": 6.5, "max": 7.1, "Ist": 5 },
        { "parameter": "Rdiff", "max": 25, "Ist": 5 },
        { "parameter": "TkUst", "max": 0.05, "Ist": 5 }
      ],
      "ratings": [
        { "parameter": "Pmax", "value": 300 },
        { "parameter": "IstMin", "value": 5 },
        { "parameter": "IstMax", "value": 40 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 100 }
      ]
    },
    {
      "name": "АЛ307Б",                     // светодиод красный
      "system": "gost",
      "parameters": [
        { "parameter": "Upr", "max": 2, "Ipr": 10 },
        { "parameter": "Iv", "min": 0.5, "max": 2, "Ipr": 10 },
        { "parameter": "Lambda", "min": 650, "max": 675 }
      ],
      "ratings": [
        { "parameter": "IprMax", "value": 20 },
        { "parameter": "UobrMax", "value": 5 },
        { "parameter": "Pmax", "value": 100 }
      ]
    },

    // pro: A германий / B кремний; вторая буква — A маломощный диод, Y мощный выпрямитель, Z стабилитрон
    {
      "name": "AA119",
      "system": "pro",
      "parameters": [ { "parameter": "Upr", "max": 1, "Ipr": 10 } ],
      "ratings": [
        { "parameter": "UobrMax", "value": 45 },
        { "parameter": "IprMax", "value": 100 },
        { "parameter": "TempMax", "value": 70 }
      ]
    },
    {
      "name": "BY133",
      "system": "pro",
      "parameters": [
        { "parameter": "Upr", "max": 1.1, "Ipr": 1000 },
        { "parameter": "Iobr", "max": 5, "Uobr": 600 }
      ],
      "ratings": [
        { "parameter": "UobrMax", "value": 600 },
        { "parameter": "IprMax", "value": 1000 },
        { "parameter": "TempJunctionMax", "value": 150 }
      ]
    },
    {
      "name": "BZX85C5V1",                  // BZX85 — базовый номер (B кремний, Z стабилитрон, X85 промышленная регистрация); суффикс C5V1 — допуск ±5 %, 5,1 В (V — десятичная запятая)
      "system": "pro",
      "parameters": [
        { "parameter": "Ust", "min": 4.8, "max": 5.4, "Ist": 45 },
        { "parameter": "Rdiff", "max": 15, "Ist": 45 }
      ],
      "ratings": [
        { "parameter": "Pmax", "value": 1300 },
        { "parameter": "IstMin", "value": 5 },
        { "parameter": "IstMax", "value": 240 }
      ]
    },

    // jedec: 1N — один переход; номер — регистрация
    {
      "name": "1N4148",
      "system": "jedec",
      "attributes": { "category": "импульсный" },
      "parameters": [
        { "parameter": "Upr", "max": 1, "Ipr": 10 },
        { "parameter": "trr", "max": 4, "Ipr": 10 },
        { "parameter": "Cn", "max": 4, "Uobr": 0 },
        { "parameter": "Iobr", "max": 0.025, "Uobr": 20 }
      ],
      "ratings": [
        { "parameter": "UobrMax", "value": 100 },
        { "parameter": "IprMax", "value": 200 },
        { "parameter": "Pmax", "value": 500 },
        { "parameter": "TempMin", "value": -65 },
        { "parameter": "TempMax", "value": 150 }
      ]
    },
    {
      "name": "1N4007",
      "system": "jedec",
      "attributes": { "category": "выпрямительный" },
      "parameters": [
        { "parameter": "Upr", "max": 1.1, "Ipr": 1000 },
        { "parameter": "Iobr", "max": 10, "Uobr": 700 }
      ],
      "ratings": [
        { "parameter": "UobrMax", "value": 1000 },
        { "parameter": "IprMax", "value": 1000 },
        { "parameter": "TempJunctionMax", "value": 150 }
      ]
    },
    {
      "name": "1N5408",
      "system": "jedec",
      "attributes": { "category": "выпрямительный" },
      "parameters": [ { "parameter": "Upr", "max": 1.1, "Ipr": 3000 } ],
      "ratings": [
        { "parameter": "UobrMax", "value": 1000 },
        { "parameter": "IprMax", "value": 3000 },
        { "parameter": "TempJunctionMax", "value": 150 }
      ]
    },

    // jis: 1S — диод; S переключный, R выпрямительный; старые регистрации — без буквы класса
    {
      "name": "1S2076",                     // старая форма без буквы класса; современные диоды несут букву (S сигнальный, R выпрямительный, V варикап, Z стабилитрон)
      "system": "jis",
      "attributes": { "category": "импульсный" },
      "parameters": [
        { "parameter": "Upr", "max": 1, "Ipr": 10 },
        { "parameter": "trr", "max": 4, "Ipr": 10 }
      ],
      "ratings": [ { "parameter": "UobrMax", "value": 60 }, { "parameter": "IprMax", "value": 100 } ]
    },
    {
      "name": "1SS352",
      "system": "jis",
      "attributes": { "category": "импульсный" },
      "parameters": [ { "parameter": "trr", "max": 4, "Ipr": 10 } ],
      "ratings": [ { "parameter": "UobrMax", "value": 35 }, { "parameter": "IprMax", "value": 100 } ]
    },
    {
      "name": "1SR154-400",                 // суффикс — напряжение
      "system": "jis",
      "attributes": { "category": "выпрямительный быстрый" },
      "parameters": [
        { "parameter": "Upr", "max": 1.3, "Ipr": 1000 },
        { "parameter": "trr", "max": 60, "Ipr": 1000 }
      ],
      "ratings": [ { "parameter": "UobrMax", "value": 400 }, { "parameter": "IprMax", "value": 1000 } ]
    },

    // series: советские семейства вне строгого ГОСТ; категория — атрибутом
    {
      "name": "Д226",
      "system": "series",
      "attributes": { "category": "выпрямительный" },
      "parameters": [
        { "parameter": "Upr", "max": 1, "Ipr": 300 },
        { "parameter": "Iobr", "max": 100, "Uobr": 400 }
      ],
      "ratings": [
        { "parameter": "UobrMax", "value": 400 },
        { "parameter": "IprMax", "value": 300 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ],
      "analogs": [ { "name": "1N4007", "note": "с ограничениями: запас по напряжению и току" } ]
    },
    {
      "name": "Д814А",
      "system": "series",
      "attributes": { "category": "стабилитрон" },
      "parameters": [
        { "parameter": "Ust", "min": 7, "max": 8.5, "Ist": 5 },
        { "parameter": "Rdiff", "max": 6, "Ist": 5 },
        { "parameter": "TkUst", "max": 0.07, "Ist": 5 }
      ],
      "ratings": [
        { "parameter": "Pmax", "value": 340 },
        { "parameter": "IstMin", "value": 3 },
        { "parameter": "IstMax", "value": 40 }
      ]
    },
    {
      "name": "Д2Б",
      "system": "series",
      "attributes": { "category": "универсальный" },
      "parameters": [
        { "parameter": "Upr", "max": 1, "Ipr": 10 },
        { "parameter": "Iobr", "max": 100, "Uobr": 30 }
      ],
      "ratings": [ { "parameter": "UobrMax", "value": 30 }, { "parameter": "IprMax", "value": 25 } ]
    },

    // other: фирменные варианты JEDEC-номеров и MIL-форма; ключ system обязателен
    {
      "name": "LL4148",
      "system": "other",
      "attributes": { "category": "импульсный", "package": "miniMELF" },
      "parameters": [
        { "parameter": "Upr", "max": 1, "Ipr": 10 },
        { "parameter": "trr", "max": 4, "Ipr": 10 }
      ],
      "ratings": [ { "parameter": "UobrMax", "value": 100 }, { "parameter": "IprMax", "value": 200 } ],
      "analogs": ["1N4148"]
    },
    {
      "name": "UF4007",
      "system": "other",
      "attributes": { "category": "выпрямительный быстрый" },
      "parameters": [
        { "parameter": "Upr", "max": 1.7, "Ipr": 1000 },
        { "parameter": "trr", "max": 75, "Ipr": 1000 }
      ],
      "ratings": [ { "parameter": "UobrMax", "value": 1000 }, { "parameter": "IprMax", "value": 1000 } ]
    },
    {
      "name": "JAN1N4007",
      "system": "other",
      "attributes": { "category": "выпрямительный", "militaryGrade": true },
      "ratings": [ { "parameter": "UobrMax", "value": 1000 }, { "parameter": "IprMax", "value": 1000 } ]
    }
  ]
}
```

## 3. Резисторы

```jsonc
{
  "resistors": [
    // gost (ГОСТ 3453-68): С — постоянные, СП — переменные; группа — материал/технология
    // (1 углеродистые, 2 металлодиэлектрические, 3 композиционные плёночные, 4 композиционные объёмные, 5 проволочные, 6 металлизированные)
    {
      "name": "С2-33Н",
      "system": "gost",
      "attributes": { "technology": "металлодиэлектрические" },
      "parameters": [
        { "parameter": "Rnom", "min": 1, "max": 10000000 },
        { "parameter": "Dop", "max": 5 },
        { "parameter": "TKS", "max": 500 }
      ],
      "ratings": [
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 125 }
      ],
      "variants": [                                     // ряд мощностей — варианты с обязательным Pnom (правило resistor_variant_power)
        {
          "label": "0.125 Вт",
          "ratings": [ { "parameter": "Pnom", "value": 0.125 }, { "parameter": "Umax", "value": 250 } ],
          "dimensions": [ { "parameter": "massMax", "value": 0.15 } ]
        },
        {
          "label": "1 Вт",
          "ratings": [ { "parameter": "Pnom", "value": 1 }, { "parameter": "Umax", "value": 500 } ],
          "dimensions": [ { "parameter": "massMax", "value": 0.8 } ]
        }
      ]
    },
    {
      "name": "СП3-19А",                    // переменный; закон изменения сопротивления — атрибут functionalChar
      "system": "gost",
      "attributes": { "functionalChar": "А" },
      "parameters": [
        { "parameter": "Rnom", "min": 1000, "max": 2200000 },
        { "parameter": "Dop", "max": 20 },
        { "parameter": "TKS", "max": 1500 }
      ],
      "ratings": [ { "parameter": "Pnom", "value": 0.5 } ]
    },
    {
      "name": "С5-16МВ",
      "system": "gost",
      "attributes": { "technology": "проволочные" },
      "parameters": [
        { "parameter": "Rnom", "min": 0.1, "max": 100000 },
        { "parameter": "Dop", "max": 1 },
        { "parameter": "TKS", "max": 100 },
        { "parameter": "Ush", "max": 1 }
      ],
      "ratings": [
        { "parameter": "Pnom", "value": 10 },
        { "parameter": "Umax", "value": 750 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 155 }
      ]
    },

    // ost (ОСТ 11.074.009-78): Р — постоянные, РП — переменные, НР — наборы; группа — материал (1 непроволочные,
    // 2 проволочные/металлофольговые); приведены канонические примеры из текста стандарта.
    // Формы с ГОСТ 3453-68 не пересекаются (С/СП против Р/РП) — автодетект однозначен;
    // обозначения вида С1-4, СП3-4, С4-2 — форма ГОСТ 3453-68 (система gost).
    {
      "name": "Р1-4",
      "system": "ost",
      "attributes": { "technology": "непроволочные" },
      "parameters": [ { "parameter": "Rnom", "min": 1, "max": 1000000 }, { "parameter": "Dop", "max": 10 } ],
      "ratings": [ { "parameter": "Pnom", "value": 0.25 }, { "parameter": "Umax", "value": 250 } ]
    },
    {
      "name": "РП1-46",
      "system": "ost",
      "attributes": { "functionalChar": "Б" },
      "parameters": [ { "parameter": "Rnom", "min": 1000, "max": 1000000 }, { "parameter": "Dop", "max": 20 } ],
      "ratings": [ { "parameter": "Pnom", "value": 0.5 } ]
    },

    // series: мощность — хвостом обозначения у семейств с tail_semantic = power
    {
      "name": "МЛТ-0.5",
      "system": "series",
      "parameters": [
        { "parameter": "Rnom", "min": 1, "max": 5100000 },
        { "parameter": "Dop", "max": 20 },
        { "parameter": "TKS", "max": 1200 },
        { "parameter": "Riz", "min": 10000000000 }
      ],
      "ratings": [
        { "parameter": "Pnom", "value": 0.5 },
        { "parameter": "Umax", "value": 350 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ],
      "analogs": ["С2-33Н"]
    },
    {
      "name": "ВС-0.5",
      "system": "series",
      "parameters": [
        { "parameter": "Rnom", "min": 27, "max": 10000000 },
        { "parameter": "Dop", "max": 20 },
        { "parameter": "TKS", "max": 2000 }
      ],
      "ratings": [ { "parameter": "Pnom", "value": 0.5 }, { "parameter": "Umax", "value": 500 } ]
    },
    {
      "name": "ПЭВ-10",
      "system": "series",
      "attributes": { "technology": "проволочные" },
      "parameters": [
        { "parameter": "Rnom", "min": 1.8, "max": 10000 },
        { "parameter": "Dop", "max": 10 },
        { "parameter": "TKS", "max": 300 }
      ],
      "ratings": [ { "parameter": "Pnom", "value": 10 }, { "parameter": "Umax", "value": 1500 } ]
    },

    // other: фирменные имена и MIL-форма; ключ system обязателен
    {
      "name": "KNP-100",
      "system": "other",
      "attributes": { "package": "цементный" },
      "parameters": [ { "parameter": "Rnom", "min": 0.1, "max": 100000 }, { "parameter": "Dop", "max": 5 } ],
      "ratings": [ { "parameter": "Pnom", "value": 5 } ]
    },
    {
      "name": "CFR-25",
      "system": "other",
      "parameters": [ { "parameter": "Rnom", "min": 1, "max": 10000000 }, { "parameter": "Dop", "max": 5 } ],
      "ratings": [ { "parameter": "Pnom", "value": 0.25 } ]
    },
    {
      "name": "M39009/04-0003",             // MIL-PRF-39009 (проволочные мощные, established reliability); /04 — лист-спецификация; значения — по листу
      "system": "other",
      "attributes": { "technology": "проволочные", "militaryGrade": true }
    }
  ]
}
```

## 4. Конденсаторы

```jsonc
{
  "capacitors": [
    // gost (единая система, действующая кодификация — ГОСТ Р 57440-2017): К — постоянной ёмкости
    // (К50 алюминиевые электролитические, К10 керамические, К42 бумажные металлизированные);
    // КТ/КП/КН — подстроечные/переменной ёмкости/нелинейные (КТ4-25 и др., вне стартового наполнения)
    // Электролитический: данные типа в целом + матрица исполнений «ёмкость × напряжение»
    {
      "name": "К50-35",
      "system": "gost",
      "attributes": { "polarized": true, "tu": "ОЖ0.464.036ТУ", "yearFrom": 1980 },
      "parameters": [                       // тип в целом (variant_id NULL)
        { "parameter": "Dop", "max": 20 },
        { "parameter": "Tgd", "max": 0.15, "temp": 20 }
      ],
      "variants": [
        {
          "label": "160 В",
          "parameters": [                   // характеристики исполнения; ёмкость — в пФ
            { "parameter": "Unom", "value": 160 },
            { "parameter": "Cnom", "min": 1000000, "max": 10000000 }   // 1–10 мкФ — только на 160 В
          ],
          "dimensions": [ { "parameter": "diameter", "value": 8 },
                          { "parameter": "leadLength", "value": 12 },
                          { "parameter": "massMax", "value": 1.5 } ]
        },
        {
          "label": "25 В",
          "parameters": [
            { "parameter": "Unom", "value": 25 },
            { "parameter": "Cnom", "min": 47000000, "max": 4700000000 }   // 47–4700 мкФ
          ],
          "dimensions": [ { "parameter": "diameter", "value": 10 },
                          { "parameter": "leadLength", "value": 16 },
                          { "parameter": "massMax", "value": 3 } ]
        }
      ]
    },

    // Керамический: варианты не нужны, габариты постоянны для типа
    {
      "name": "К10-17Б",
      "system": "gost",
      "attributes": { "package": "монолитный", "tu": "ОЖ0.464.036ТУ", "yearFrom": 1980 },
      "parameters": [
        { "parameter": "Cnom", "min": 22, "max": 1000000 },        // 22 пФ – 1 мкФ
        { "parameter": "TKE", "text": "Н30" },
        { "parameter": "Unom", "value": 25 }
      ],
      "dimensions": [ { "parameter": "massMax", "value": 1 },
                      { "parameter": "length", "value": 6 },
                      { "parameter": "width", "value": 4 },
                      { "parameter": "height", "value": 5 } ]
    },

    {
      "name": "К42-19",
      "system": "gost",
      "parameters": [
        { "parameter": "Cnom", "min": 10000, "max": 1000000 },     // 0.01–1 мкФ
        { "parameter": "Dop", "max": 10 },
        { "parameter": "Unom", "value": 250 },
        { "parameter": "Tgd", "max": 1 },
        { "parameter": "Riz", "min": 10000000000 },
        { "parameter": "tempMin", "value": -60 },
        { "parameter": "tempMax", "value": 70 }
      ]
    },

    // series: слюдяные и металлобумажные семейства
    {
      "name": "КСО-2",
      "system": "series",
      "parameters": [
        { "parameter": "Cnom", "min": 51, "max": 3300 },
        { "parameter": "Dop", "max": 10 },
        { "parameter": "Unom", "value": 500 },
        { "parameter": "Tgd", "max": 0.1 },
        { "parameter": "Riz", "min": 10000000000 },
        { "parameter": "tempMin", "value": -60 },
        { "parameter": "tempMax", "value": 70 }
      ]
    },
    {
      "name": "МБМ",
      "system": "series",
      "parameters": [
        { "parameter": "Cnom", "min": 47000, "max": 470000 },      // 0.047–0.47 мкФ
        { "parameter": "Dop", "max": 10 },
        { "parameter": "Unom", "value": 250 },
        { "parameter": "Tgd", "max": 1 },
        { "parameter": "tempMin", "value": -60 },
        { "parameter": "tempMax", "value": 70 }
      ],
      "analogs": ["К42-19"]
    },
    {
      "name": "МБГЧ-1",
      "system": "series",
      "parameters": [
        { "parameter": "Cnom", "min": 100000, "max": 1000000 },    // 0.1–1 мкФ
        { "parameter": "Dop", "max": 10 },
        { "parameter": "Unom", "value": 500 },
        { "parameter": "Tgd", "max": 1 },
        { "parameter": "tempMin", "value": -60 },
        { "parameter": "tempMax", "value": 70 }
      ]
    },

    // other: полные заказные коды производителей и MIL-форма; ключ system обязателен
    {
      "name": "M39003/01-6045",             // MIL-PRF-39003 (танталовые, established reliability); /01 — лист-спецификация; значения — по листу
      "system": "other",
      "attributes": { "polarized": true, "militaryGrade": true }
    },
    {
      "name": "GRM188R71H104KA93D",
      "system": "other",
      "attributes": { "technology": "керамика X7R", "package": "0603" },
      "parameters": [
        { "parameter": "Cnom", "min": 100000, "max": 100000 },     // 0.1 мкФ — единичный номинал кода, границы равны
        { "parameter": "Dop", "max": 10 },
        { "parameter": "Unom", "value": 50 },
        { "parameter": "tempMin", "value": -55 },
        { "parameter": "tempMax", "value": 125 }
      ]
    },
    {
      "name": "T491B476K016AT",
      "system": "other",
      "attributes": { "polarized": true, "technology": "танталовые", "package": "B" },
      "parameters": [
        { "parameter": "Cnom", "min": 47000000, "max": 47000000 }, // 47 мкФ
        { "parameter": "Dop", "max": 10 },
        { "parameter": "Unom", "value": 16 },
        { "parameter": "Iut", "max": 8, "Unom": 16 },
        { "parameter": "tempMin", "value": -55 },
        { "parameter": "tempMax", "value": 125 }
      ]
    }
  ]
}
```

Семантика секций: отсутствует/`null` — не менять; задана — заменить целиком; `[]`/`{}` — очистить; ошибка в любом значении секции (включая любой вариант) — запись не применяется вовсе; `manufacturers` внутри `attributes` — `null` «не менять список». Секция `variants` заменяет набор исполнений целиком вместе с их значениями; секция `analogs` заменяет список аналогов целиком (как `manufacturers`). Ключ `system` необязателен (по умолчанию — автодетект) и обязателен для `other`.
