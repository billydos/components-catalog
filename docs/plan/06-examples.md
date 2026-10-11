# 06. Примеры записи наполнения

Примеры иллюстрируют форму записи наполнения по каталогам `03-data-model.md` (классы, системы обозначений, параметры, атрибуты, варианты, аналоги); корневая структура файлов наполнения и форматы (jsonc/yaml/ndjson) — `04-module-functionality.md`.

Форма секций групп параметров **едина для всех групп** (`parameters`, `ratings`, `dimensions`) — список объектов `{ "parameter": <код>, значение, условия }`; значение — `value` (exact) | `min`/`max` | `text` (text/enum). Условия — соседние ключи по коду условия.

Примеры разбиты по классам — один блок кода соответствует секции одного класса в файле наполнения. Внутри блока записи сгруппированы по системам обозначений, **по две–три записи на систему, всего до десяти записей на класс**. Записи минимальны: они демонстрируют форму записи и отличия систем обозначений, а не полноту данных типа; поля грамматик систем в файле не задаются — их даёт разбор обозначения; классификационные словарные поля (материал, подкласс, подстройка, категория) для систем, не кодирующих их обозначением, — секцией `fields` (D10). Ключ `system` проставлен каждой записи явно — чтобы система читалась из самой записи, без комментариев; в рабочем наполнении он нужен только для `other` (автодетекта нет) и при неоднозначности автодетекта, в остальных случаях систему определяет автодетект (формы `gost`/`ost` у резисторов не пересекаются — `С`/`СП` против `Р`/`РП`). Аналоги в примерах ссылаются только на записи своего блока; ссылки направленные (`03-data-model.md` §8): секция `analogs` записи задаёт её исходящие ссылки (встречные отображаются на карточке получателя). Предметные атрибуции примеров выверены — `07-r1-verification.md`.

## 1. Транзисторы

```jsonc
{
  "transistors": [
    // gost (ГОСТ 10862-64): материал и подкласс кодируются обозначением
    {
      "name": "КТ315Б",
      "system": "gost",
      "attributes": {
        "structure": "npn", "package": "КТ-13", "spec": "ЖК3.365.200ТУ",
        "yearFrom": 1967, "yearTo": 1992
      },
      "manufacturers": ["Восход", "Терма"],
      "parameters": [
        { "parameter": "hfe",  "min": 50, "max": 350, "Vce": 10, "Ic": 1 },
        { "parameter": "Icbo",  "max": 0.5, "Vcb": 10, "temp": 25 }
      ],
      "ratings": [
        { "parameter": "VceoMax", "value": 20 },
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
        { "parameter": "hfe", "min": 50, "max": 120, "Vce": 5, "Ic": 10 },
        { "parameter": "Icbo", "max": 100, "Vcb": 5 }
      ],
      "ratings": [
        { "parameter": "VceoMax", "value": 15 },
        { "parameter": "PcMax", "value": 150 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ]
    },

    // pro (PRO ELECTRON): материал и класс прибора — первые две буквы
    {
      "name": "BC547B",                     // B — кремний, C — маломощный НЧ; суффикс B — группа по усилению
      "system": "pro",
      "attributes": { "structure": "npn", "package": "TO-92" },
      "parameters": [
        { "parameter": "hfe", "min": 200, "max": 450, "Vce": 5, "Ic": 2 },
        { "parameter": "fT", "min": 300, "Vce": 5, "Ic": 2 }
      ],
      "ratings": [
        { "parameter": "VceoMax", "value": 45 },
        { "parameter": "IcMax", "value": 100 },
        { "parameter": "PcMax", "value": 500 }
      ]
    },
    {
      "name": "AD161",                      // A — германий, D — мощный НЧ
      "system": "pro",
      "attributes": { "structure": "pnp", "package": "TO-66" },
      "parameters": [ { "parameter": "hfe", "min": 80, "max": 200, "Vce": 2, "Ic": 500 } ],
      "ratings": [
        { "parameter": "VceoMax", "value": 32 },
        { "parameter": "IcMax", "value": 1000 },
        { "parameter": "PcMax", "value": 6000 },
        { "parameter": "TempMin", "value": -40 },
        { "parameter": "TempMax", "value": 90 }
      ]
    },

    // jedec: материал/подкласс/категория не кодируются — секцией fields; один номер — много производителей
    {
      "name": "2N3055",
      "system": "jedec",
      "fields": { "material": "si", "subclass": "bjt", "category": "power" },
      "attributes": {
        "structure": "npn", "description": "низкочастотный мощный", "package": "TO-3"
      },
      "manufacturers": ["Motorola", "RCA", "ST"],
      "parameters": [ { "parameter": "hfe", "min": 20, "max": 70, "Vce": 4, "Ic": 4000 } ],
      "ratings": [
        { "parameter": "VceoMax", "value": 60 },
        { "parameter": "IcMax", "value": 15000 },
        { "parameter": "PcMax", "value": 115000 },
        { "parameter": "TempJunctionMax", "value": 200 }
      ]
    },
    {
      "name": "2N3904",
      "system": "jedec",
      "fields": { "material": "si", "subclass": "bjt", "category": "general_purpose" },
      "attributes": { "structure": "npn", "description": "универсальный", "package": "TO-92" },
      "parameters": [ { "parameter": "hfe", "min": 100, "max": 300, "Vce": 10, "Ic": 10 } ],
      "ratings": [ { "parameter": "VceoMax", "value": 40 }, { "parameter": "IcMax", "value": 200 } ],
      "analogs": ["КТ315Б"]
    },

    // jis (JIS/EIAJ): переходы и класс кодируются — A p-n-p ВЧ, C n-p-n ВЧ, K n-канал ПТ
    {
      "name": "2SC1815",                    // группа hFE — суффиксом обозначения
      "system": "jis",
      "attributes": { "structure": "npn", "package": "TO-92" },
      "parameters": [ { "parameter": "hfe", "min": 70, "max": 700, "Vce": 6, "Ic": 2 } ],
      "ratings": [
        { "parameter": "VceoMax", "value": 50 },
        { "parameter": "IcMax", "value": 150 },
        { "parameter": "PcMax", "value": 400 }
      ]
    },
    {
      "name": "2SK1058",
      "system": "jis",
      "attributes": { "structure": "n-канал", "package": "TO-3P" },
      "ratings": [
        { "parameter": "VdsMax", "value": 160 },
        { "parameter": "IdMax", "value": 7000 },
        { "parameter": "PdMax", "value": 100000 }
      ]
    },

    // other: досистемные семейства и MIL-формы — разбора нет, ключ system обязателен; семейство и классификация — секцией fields
    {
      "name": "П214",
      "system": "other",
      "fields": { "material": "ge", "subclass": "bjt", "category": "power", "series": "П", "dev_number": 214 },
      "attributes": { "structure": "pnp", "description": "низкочастотный мощный" },
      "parameters": [ { "parameter": "hfe", "min": 20, "max": 80, "Vce": 10, "Ic": 2000 } ],
      "ratings": [
        { "parameter": "VceoMax", "value": 60 },
        { "parameter": "IcMax", "value": 5000 },
        { "parameter": "PcMax", "value": 10000 },
        { "parameter": "TempMax", "value": 85 }
      ],
      "analogs": ["AD161"]                              // пара из разных систем — главный случай
    },
    {
      "name": "JANTX2N3055",                // MIL-форма на базе JEDEC-номера — вне строгих систем
      "system": "other",
      "fields": { "material": "si", "subclass": "bjt", "category": "power" },
      "attributes": { "structure": "npn", "description": "низкочастотный мощный", "package": "TO-3", "militaryGrade": true },
      "ratings": [
        { "parameter": "VceoMax", "value": 60 },
        { "parameter": "IcMax", "value": 15000 },
        { "parameter": "PcMax", "value": 115000 }
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
      "name": "КС168А",                     // стабилитрон 6.8 В
      "system": "gost",
      "parameters": [
        { "parameter": "Vz", "min": 6.5, "max": 7.1, "Iz": 5 },
        { "parameter": "Rdiff", "max": 25, "Iz": 5 },
        { "parameter": "AlphaVz", "max": 0.05, "Iz": 5 }
      ],
      "ratings": [
        { "parameter": "Pmax", "value": 300 },
        { "parameter": "IzMin", "value": 5 },
        { "parameter": "IzMax", "value": 40 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 100 }
      ]
    },
    {
      "name": "АЛ307Б",                     // светодиод красный
      "system": "gost",
      "parameters": [
        { "parameter": "Vf", "max": 2, "If": 10 },
        { "parameter": "Iv", "min": 0.5, "max": 2, "If": 10 },
        { "parameter": "Lambda", "min": 650, "max": 675 }
      ],
      "ratings": [
        { "parameter": "IfMax", "value": 20 },
        { "parameter": "VrMax", "value": 5 },
        { "parameter": "Pmax", "value": 100 }
      ]
    },

    // pro: A германий / B кремний; вторая буква — A маломощный диод, Y мощный выпрямитель, Z стабилитрон
    {
      "name": "AA119",
      "system": "pro",
      "parameters": [ { "parameter": "Vf", "max": 1, "If": 10 } ],
      "ratings": [
        { "parameter": "VrMax", "value": 45 },
        { "parameter": "IfMax", "value": 100 },
        { "parameter": "TempMax", "value": 70 }
      ]
    },
    {
      "name": "BZX85C5V1",                  // BZX85 — базовый номер (B кремний, Z стабилитрон, X85 промышленная регистрация); суффикс C5V1 — допуск ±5 %, 5,1 В (V — десятичная запятая)
      "system": "pro",
      "parameters": [
        { "parameter": "Vz", "min": 4.8, "max": 5.4, "Iz": 45 },
        { "parameter": "Rdiff", "max": 15, "Iz": 45 }
      ],
      "ratings": [
        { "parameter": "Pmax", "value": 1300 },
        { "parameter": "IzMin", "value": 5 },
        { "parameter": "IzMax", "value": 240 }
      ]
    },

    // jedec: 1N — один переход; номер — регистрация
    {
      "name": "1N4148",
      "system": "jedec",
      "attributes": { "description": "импульсный" },
      "parameters": [
        { "parameter": "Vf", "max": 1, "If": 10 },
        { "parameter": "trr", "max": 4, "If": 10 },
        { "parameter": "Ir", "max": 0.025, "Vr": 20 }
      ],
      "ratings": [
        { "parameter": "VrMax", "value": 100 },
        { "parameter": "IfMax", "value": 200 },
        { "parameter": "Pmax", "value": 500 },
        { "parameter": "TempMin", "value": -65 },
        { "parameter": "TempMax", "value": 150 }
      ]
    },
    {
      "name": "1N4007",
      "system": "jedec",
      "attributes": { "description": "выпрямительный" },
      "parameters": [
        { "parameter": "Vf", "max": 1.1, "If": 1000 },
        { "parameter": "Ir", "max": 10, "Vr": 700 }
      ],
      "ratings": [
        { "parameter": "VrMax", "value": 1000 },
        { "parameter": "IfMax", "value": 1000 },
        { "parameter": "TempJunctionMax", "value": 150 }
      ]
    },

    // jis: 1S — диод; S переключный, R выпрямительный; старые регистрации — без буквы класса
    {
      "name": "1S2076",                     // старая форма без буквы класса; современные диоды несут букву (S сигнальный, R выпрямительный, V варикап, Z стабилитрон)
      "system": "jis",
      "attributes": { "description": "импульсный" },
      "parameters": [
        { "parameter": "Vf", "max": 1, "If": 10 },
        { "parameter": "trr", "max": 4, "If": 10 }
      ],
      "ratings": [ { "parameter": "VrMax", "value": 60 }, { "parameter": "IfMax", "value": 100 } ]
    },
    {
      "name": "1SR154-400",                 // суффикс — напряжение
      "system": "jis",
      "attributes": { "description": "выпрямительный быстрый" },
      "parameters": [
        { "parameter": "Vf", "max": 1.3, "If": 1000 },
        { "parameter": "trr", "max": 60, "If": 1000 }
      ],
      "ratings": [ { "parameter": "VrMax", "value": 400 }, { "parameter": "IfMax", "value": 1000 } ]
    },

    // other: советские семейства вне строгого ГОСТ; семейство — секцией fields
    {
      "name": "Д226",
      "system": "other",
      "fields": { "series": "Д", "dev_number": 226 },
      "attributes": { "description": "выпрямительный" },
      "parameters": [
        { "parameter": "Vf", "max": 1, "If": 300 },
        { "parameter": "Ir", "max": 100, "Vr": 400 }
      ],
      "ratings": [
        { "parameter": "VrMax", "value": 400 },
        { "parameter": "IfMax", "value": 300 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ],
      "analogs": [ { "name": "1N4007", "note": "с ограничениями: запас по напряжению и току" } ]
    },

    // other: фирменные варианты JEDEC-номеров; ключ system обязателен
    {
      "name": "LL4148",
      "system": "other",
      "attributes": { "description": "импульсный", "package": "miniMELF" },
      "parameters": [
        { "parameter": "Vf", "max": 1, "If": 10 },
        { "parameter": "trr", "max": 4, "If": 10 }
      ],
      "ratings": [ { "parameter": "VrMax", "value": 100 }, { "parameter": "IfMax", "value": 200 } ],
      "analogs": ["1N4148"]
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
        { "parameter": "Tolerance", "max": 5 },
        { "parameter": "TCR", "max": 500 }
      ],
      "ratings": [
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 125 }
      ],
      "variants": [                                     // ряд мощностей — варианты с обязательным Pnom (правило resistor_variant_power)
        {
          "label": "0.125 Вт",
          "ratings": [ { "parameter": "Pnom", "value": 0.125 }, { "parameter": "Vmax", "value": 250 } ],
          "dimensions": [ { "parameter": "massMax", "value": 0.15 } ]
        },
        {
          "label": "1 Вт",
          "ratings": [ { "parameter": "Pnom", "value": 1 }, { "parameter": "Vmax", "value": 500 } ],
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
        { "parameter": "Tolerance", "max": 20 },
        { "parameter": "TCR", "max": 1500 }
      ],
      "ratings": [ { "parameter": "Pnom", "value": 0.5 } ]
    },
    {
      "name": "С5-16МВ",
      "system": "gost",
      "attributes": { "technology": "проволочные" },
      "parameters": [
        { "parameter": "Rnom", "min": 0.1, "max": 100000 },
        { "parameter": "Tolerance", "max": 1 },
        { "parameter": "TCR", "max": 100 },
        { "parameter": "Vnoise", "max": 1 }
      ],
      "ratings": [
        { "parameter": "Pnom", "value": 10 },
        { "parameter": "Vmax", "value": 750 },
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
      "parameters": [ { "parameter": "Rnom", "min": 1, "max": 1000000 }, { "parameter": "Tolerance", "max": 10 } ],
      "ratings": [ { "parameter": "Pnom", "value": 0.25 }, { "parameter": "Vmax", "value": 250 } ]
    },
    {
      "name": "РП1-46",
      "system": "ost",
      "attributes": { "functionalChar": "Б" },
      "parameters": [ { "parameter": "Rnom", "min": 1000, "max": 1000000 }, { "parameter": "Tolerance", "max": 20 } ],
      "ratings": [ { "parameter": "Pnom", "value": 0.5 } ]
    },

    // other: досистемные семейства; семейство — полем fields, мощность — параметром Pnom
    {
      "name": "МЛТ-0.5",
      "system": "other",
      "fields": { "series": "МЛТ" },
      "parameters": [
        { "parameter": "Rnom", "min": 1, "max": 5100000 },
        { "parameter": "Tolerance", "max": 10 },                        // ±2; ±5; ±10 %
        { "parameter": "nominalRow", "text": "E24" },             // ряды номиналов: E24 при ±5/±10 %,
        { "parameter": "nominalRow", "text": "E96" },             // E96 при ±2 %
        { "parameter": "TCR", "max": 1200 },
        { "parameter": "Rins", "min": 10000000000 }
      ],
      "ratings": [
        { "parameter": "Pnom", "value": 0.5 },
        { "parameter": "Vmax", "value": 350 },
        { "parameter": "TempMin", "value": -60 },
        { "parameter": "TempMax", "value": 70 }
      ],
      "analogs": ["С2-33Н"]
    },
    {
      "name": "ПЭВ-10",
      "system": "other",
      "fields": { "series": "ПЭВ" },
      "attributes": { "technology": "проволочные" },
      "parameters": [
        { "parameter": "Rnom", "min": 1.8, "max": 10000 },
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "TCR", "max": 300 }
      ],
      "ratings": [ { "parameter": "Pnom", "value": 10 }, { "parameter": "Vmax", "value": 1500 } ]
    },

    // other: фирменные имена и MIL-форма; ключ system обязателен
    {
      "name": "KNP-100",
      "system": "other",
      "attributes": { "package": "цементный" },
      "parameters": [ { "parameter": "Rnom", "min": 0.1, "max": 100000 }, { "parameter": "Tolerance", "max": 5 } ],
      "ratings": [ { "parameter": "Pnom", "value": 5 } ]
    },
    {
      "name": "CFR-25",
      "system": "other",
      "parameters": [ { "parameter": "Rnom", "min": 1, "max": 10000000 }, { "parameter": "Tolerance", "max": 5 } ],
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
      "attributes": { "polarized": true, "spec": "ОЖ0.464.214ТУ" },
      "parameters": [                       // тип в целом (variant_id NULL)
        { "parameter": "Tolerance", "max": 50 },                              // +50/−20 %
        { "parameter": "DF", "max": 30, "temp": 20 }                   // 30 % (6,3–16 В) … 10 % (160 В)
      ],
      "variants": [
        {
          "label": "160 В",
          "parameters": [
            { "parameter": "Vnom", "value": 160 },
            { "parameter": "Cnom", "min": 1000000, "max": 100000000 }  // 1–100 мкФ
          ],
          "dimensions": [ { "parameter": "diameter", "value": 18 },    // радиальный цилиндрический набор
                          { "parameter": "height", "value": 35 },      // высота корпуса
                          { "parameter": "massMax", "value": 17 } ]
        },
        {
          "label": "25 В",
          "parameters": [
            { "parameter": "Vnom", "value": 25 },
            { "parameter": "Cnom", "min": 22000000, "max": 4700000000 }   // 22–4700 мкФ
          ],
          "dimensions": [ { "parameter": "diameter", "value": 16 },
                          { "parameter": "height", "value": 30 },
                          { "parameter": "leadPitch", "value": 7.5 },   // шаг выводов
                          { "parameter": "massMax", "value": 12 } ]
        }
      ]
    },

    // Керамический: варианты не нужны, габариты постоянны для типа
    {
      "name": "К10-17Б",
      "system": "gost",
      "attributes": { "package": "монолитный", "spec": "ОЖ0.460.172ТУ" },
      "parameters": [
        { "parameter": "Cnom", "min": 2.2, "max": 2200000 },        // 2,2 пФ – 2,2 мкФ (по группам ТКЕ)
        { "parameter": "TCC", "text": "Н50" },
        { "parameter": "Vnom", "value": 50 }
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
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "Vnom", "value": 250 },
        { "parameter": "DF", "max": 1 },
        { "parameter": "Rins", "min": 10000000000 },
        { "parameter": "opTempMin", "value": -60 },
        { "parameter": "opTempMax", "value": 70 }
      ]
    },

    // other: слюдяные и металлобумажные семейства; семейство — секцией fields
    {
      "name": "КСО-2",
      "system": "other",
      "fields": { "series": "КСО", "dev_number": 2 },
      "parameters": [
        { "parameter": "Cnom", "min": 51, "max": 3300 },
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "Vnom", "value": 500 },
        { "parameter": "DF", "max": 0.1 },
        { "parameter": "Rins", "min": 10000000000 },
        { "parameter": "opTempMin", "value": -60 },
        { "parameter": "opTempMax", "value": 70 }
      ]
    },
    {
      "name": "МБМ",
      "system": "other",
      "fields": { "series": "МБМ" },
      "parameters": [
        { "parameter": "Cnom", "min": 47000, "max": 470000 },      // 0.047–0.47 мкФ
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "Vnom", "value": 250 },
        { "parameter": "DF", "max": 1 },
        { "parameter": "opTempMin", "value": -60 },
        { "parameter": "opTempMax", "value": 70 }
      ],
      "analogs": ["К42-19"]
    },
    {
      "name": "МБГЧ-1",
      "system": "other",
      "fields": { "series": "МБГЧ", "dev_number": 1 },
      "parameters": [
        { "parameter": "Cnom", "min": 100000, "max": 1000000 },    // 0.1–1 мкФ
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "Vnom", "value": 500 },
        { "parameter": "DF", "max": 1 },
        { "parameter": "opTempMin", "value": -60 },
        { "parameter": "opTempMax", "value": 70 }
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
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "Vnom", "value": 50 },
        { "parameter": "opTempMin", "value": -55 },
        { "parameter": "opTempMax", "value": 125 }
      ]
    },
    {
      "name": "T491B476K016AT",
      "system": "other",
      "attributes": { "polarized": true, "technology": "танталовые", "package": "B" },
      "parameters": [
        { "parameter": "Cnom", "min": 47000000, "max": 47000000 }, // 47 мкФ
        { "parameter": "Tolerance", "max": 10 },
        { "parameter": "Vnom", "value": 16 },
        { "parameter": "Ileak", "max": 8, "Vnom": 16 },
        { "parameter": "opTempMin", "value": -55 },
        { "parameter": "opTempMax", "value": 125 }
      ]
    }
  ]
}
```

Семантика секций: отсутствует/`null` — не менять; задана — заменить целиком; `[]`/`{}` — очистить; ошибка в любом значении секции (включая любой вариант) — запись не применяется вовсе. `manufacturers` — отдельная секция записи со стандартной семантикой (`null` — не менять список, `[]` — очистить). Секция `variants` заменяет набор исполнений целиком вместе с их значениями; секция `analogs` заменяет целиком исходящие направленные ссылки записи (встречные ссылки других записей не затрагиваются — `03-data-model.md` §8). Ключ `system` необязателен (по умолчанию — автодетект) и обязателен для `other`. Порядок элементов массивов секций — это `sort_order` значений и вариантов.
