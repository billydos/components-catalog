# Ответы на ревью расширения каталога

Дата: 2026-11-23. Предмет: `catalog-extension-review.md` (ревью от 2026-11-22).
Файлы: `catalog-extension.jsonc`, `data/*.jsonc` (284 файла, 1004 записи).

## 1. Статус

Ревью принято к исполнению. Разделы 1–3 отработаны, вопросы раздела 4 закрыты ниже. 
Расширение сокращено со 114 до 105 параметров: 20 кодов-близнецов исключены, 11 действующих кодов расширены наборами условий, 16 кодов переименованы. 
Все 284 файла данных проходят `catalogctl import --dry-run` без Issues; сводный импорт — 1004 записи, 0 ошибок.

## 2. Переименования (раздел 2 ревью)

| Было | Стало | Комментарий |
|---|---|---|
| `VceoBreakdown` | `VbrCeo` | по правилу V(BR)CEO |
| `VeboBreakdown` | `VbrEbo` | унификация с `VbrAvalanche` |
| `TCRVgsDiff` | `TCVgsDiff` | TCV — температурный коэффициент напряжения |
| `TCRVbeDiff` | `TCVbeDiff` | то же |
| `TCRVcbDiff` | `TCVcbDiff` | то же |
| `TCRIbDiff` | `TCIbDiff` | то же, ток |
| `tSetup` | `tSettle` | settling time, не цифровой setup |
| `VebF` | `VebForward` | полное слово вместо одиночной `F` |
| `RinsCh` | `RinsChannel` | по образцу `RinsCase` |
| `VdsSubMax` | `VdSubMax` | сток-подложка, не drain-source |
| `VccMax` | `Vc1c2Max` | та же величина, что условие `Vc1c2` |
| условие `Vcc` | исключено | используется `Vc1c2` (в т.ч. в `IccLeak`) |
| `GpControl` | `GpAgc` | глубина регулирования = AGC-диапазон |
| `Klin` | `linearity` | словесный код по §6.9д |
| `En` | `VnDensity` | единая конвенция `Vn`/`VnDensity` |
| `VgsQuad` | `VgsQuadratic` | самодокументируемый квалификатор |
| `IgTh` | `IdTh` | см. вопрос 3: это ток стока, не затвора |

## 3. Исключения и слияния (раздел 3 ревью)

| Исключено | Используется | Что сделано |
|---|---|---|
| `h21e` | `hfe` | 266 строк переведены на `hfe`; дубликатов условий нет |
| `h11b` | `h11` | 84 строки; в наборы `h11` добавлено необязательное `freq` |
| `VbeOn`, `VbeOnMin` | `VbeTh` | 44 строки; `VbeTh` переведён в `limits`, добавлены наборы `Ie`/`Ic` |
| `TauOs` | `Tauc` | 361 строка; см. вопрос 1 |
| `fTMax` | `fT` | 13 строк; `fT` переведён в `limits`, добавлен пятый набор |
| `VgsOffMax/Min` | `VgsOff` | 60 строк; `VgsOff` → `limits` |
| `VgsThMax/Min` | `VgsTh` | 2 строки; `VgsTh` → `limits` |
| `IdssMax/Min` | `Idss` | 41 строка; `Idss` → `limits`, добавлен `temp` |
| `VceCutoffMax` | `VceCutoff` | 5 строк; `VceCutoff` → `limits` |
| `IpeakMax`, `IvalleyMin` | `Ipeak`, `Ivalley` | 16 строк; оба кода сразу `limits` |
| `IdOffMax` | `IdOff` | 16 строк; `IdOff` → `limits` |
| `Ciss0`, `Crss0` | `Ciss`, `Crss` | 4 строки; добавлен набор с фиксированным `Vds=0` |
| `GpCb` | `Gp` | 44 строки; добавлен набор `Vcb+freq` |
| `IeboUjt` | `Iebo` | 18 строк; добавлено условие `Vbb`, набор `Vbb+temp` |

## 4. Ответы на вопросы (раздел 4 ревью)

### 1. `TauOs` vs `Tauc`

Подтверждено: это одна величина. Глоссарий (с. 11–12) различает только названия: `τк` — постоянная времени коллекторной цепи, `τос` — постоянная времени цепи обратной связи; физически обе равны `rб·Cк`. Ни один лист справочника не даёт `τк` и `τос` одновременно (аудит 361 строки `TauOs`: 0 записей с обоими кодами). `TauOs` исключён, все строки переведены на `Tauc`.

### 2. `fT` «не более»

Чтение подтверждено. 2Т127 (с. 27): «Граничная частота коэффициента передачи тока в схеме ОЭ, не более 100 кГц». Аналогично 2Т126 (с. 56–57), ГТ115 (с. 46–47), ГТ328 (с. 242–243). `fT` переопределён как `limits` (четыре действующих набора + пятый «все опциональные»), поэтому «не менее» и «не более» выражаются одним кодом.

### 3. `IgTh`

Это не ток затвора. 2П301 (с. 316): «Ток порога при Uзи=6,5 В, Uси=6,5 В, не менее: 2П301А, 2П301Б — 500 мкА; 2П301В, КП301Б, КП301В, КП301Г — 10 мкА». По смыслу это ток стока в точке порога; код переименован в `IdTh` (at_least, мкА, условия `Vgs`+`Vds`).

### 4. `Imod`

Подтверждено. 2Т117 (с. 49): «Ток модуляции, не менее 10 мА». Параметр однопереходного транзистора; оставлен.

### 5. `fGen`

Подтверждено. 2Т117 (с. 49): «Максимальная частота генерации 200 кГц»; КТ119 (с. 53): «Предельная частота генерации при Uб2б1=10 В, не менее 200 кГц». Код `fGen` (at_least, Гц) оставлен.

### 6. `GpControl`

Переименован в `GpAgc`. Источник: ГТ346 (с. 246–247), КТ3128 (с. 270–271): «Глубина регулирования коэффициента усиления по мощности». Это AGC-диапазон; `GpAgc` точнее.

### 7. `Vcontrol`, `VcbControlMax`

Это управляющие переходы монолитной переключательной структуры 2Т118/КТ118 (с. 50–51): `Vcontrol` — «Напряжение на управляющих переходах» при Iб=20 мА (≤1 В); `VcbControlMax` — «Напряжение управления между коллектором и базой структуры» (15 В). Коды оставлены; семантика зафиксирована в комментариях расширения. При желании `Vcontrol` можно переименовать в `Vctrl`, но это не меняет смысла.

### 8. `VgsQuad`

Переименован в `VgsQuadratic`. Источник: 2П306/КП306 (с. 330–331): «участок квадратичности переходной характеристики». Новый код читается без расшифровки.

### 9. `Klin`

Переименован в `linearity`. Источник: ГТ404 (с. 36–37): «коэффициент линейности» — отношение h21э при Iэ=3 мА к h21э при Iэ=300 мА. Международного символа нет; словесный код `linearity` соответствует §6.9д.

### 10. `Vn` и `En`

Принята одна конвенция: `Vn` — интегральное шумовое напряжение (мкВ), `VnDensity` — спектральная плотность (нВ/√Гц). `En` переименован в `VnDensity`. Источники: 2П338 (с. 377–378) — 2,5 мкВ; 2П333 (с. 369–370) — 5,5…20 нВ/√Гц. Разные единицы и способ нормирования, поэтому два кода, но одно семейство.

### 11. `PinMax`/`PinPulseMax`/`PinAvgMax`

Это предельные данные: «Средняя/импульсная СВЧ входная мощность» (2Т3132, с. 235–236; 3П325, с. 358–360). Все три кода переведены в группу `limiting`; строки данных перенесены из `parameters` в `ratings`.

### 12. `PinOverloadMin`

Термин подтверждён: «порог перегрузки» — минимальная входная СВЧ-мощность, при которой наступает сжатие усиления (3П331, с. 374–375; 3П339, с. 378–379). Это электрический параметр, не предельное данное; оставлен в `parameters`.

### 13. `VcerMax`

Условие `Rbe` вынесено в набор: `Rbe` required + `Vce` optional, плюс all-optional набор для строк без `Rbe`. `Vce` — это значение параметра, а не условие; в наборе он необязателен. Из 628 строк `VcerMax` 435 получили структурированный `Rbe` (из примечаний), 193 оставлены без `Rbe` — в источнике он не указан. Примеры: 2Т201 (с. 28–30, Rбэ≤2 кОм), 2Т306 (с. 159–161, Rбэ=3 кОм).

### 14. `TCRbb`

Добавлен `allow_negative: true`. Источник: 2Т117 (с. 49): «Температурный коэффициент межбазового сопротивления 0,1…0,9 %/°C»; в других приборах знак может быть отрицательным.


## 5. Дополнительно

- `IdTh` вместо `IgTh` (вопрос 3); данные 2П301/КП301 обновлены.

- `VbrCeo`/`VbrEbo` вместо `…Breakdown`; `TC*` вместо `TCR*`; `VebForward`, `RinsChannel`, `VdSubMax`, `Vc1c2Max`, `tSettle`.

- `Vcc` удалён; `IccLeak` и `Vc1c2Max` используют `Vc1c2`.

- `VcerMax`: 435 строк с `Rbe`; остальные 193 — без `Rbe` (нет в источнике).

- `PinMax`/`PinPulseMax`/`PinAvgMax` — группа `limiting`, секция `ratings`.

- `TCRbb` — `allow_negative: true`.


## 6. i18n

Бандлы `internal/i18n` в этом рабочем каталоге отсутствуют (есть только `cli/catalogctl.exe`), поэтому правка en+ru здесь не выполнялась. Ниже — полный список новых кодов (94) с предлагаемыми en/ru именами для переноса в бандлы. Действующие коды, переопределённые в расширении (`fT`, `VgsOff`, `VgsTh`, `Idss`, `VceCutoff`, `Ciss`, `Crss`, `Gp`, `Iebo`, `VbeTh`, `h11`), уже имеют записи в бандлах; новые ключи для них не нужны.

| Ключ | en | ru |
|---|---|---|
| `param.h12b` | reverse voltage feedback ratio, common base | Коэффициент обратной связи по напряжению в схеме ОБ (h12б). |
| `param.h12e` | reverse voltage feedback ratio, common emitter | Коэффициент обратной связи по напряжению в схеме ОЭ (h12э). |
| `param.Cce` | collector-emitter lead capacitance | Конструктивная ёмкость между выводами коллектора и эмиттера. |
| `param.Llead` | lead inductance | Индуктивность выводов (эмиттера и базы); каноническая единица мкГн. |
| `param.fOpMax` | maximum operating frequency | Максимальная рабочая частота транзистора (у конденсаторов — fMax). |
| `param.IeOpt` | optimum emitter current | Оптимальный ток эмиттера, соответствующий максимальному усилению. |
| `param.GpAgc` | AGC power gain control range | Глубина регулирования коэффициента усиления по мощности. |
| `param.Vn` | noise voltage | Шумовое напряжение (в отличие от коэффициента шума NF). |
| `param.VgsDiff` | gate-source voltage difference (pair) | Разность напряжений затвор-исток транзисторов подобранной пары. |
| `param.TCVgsDiff` | gate-source voltage difference temperature coefficient | Температурный уход разности напряжений затвор-исток пары. |
| `param.tSettle` | settling time | Время установления. |
| `param.IdOff` | off-state drain current | Ток стока при заданном (закрывающем) напряжении затвор-исток. |
| `param.IdTh` | threshold drain current | Ток порога (ток стока при пороговом напряжении; не ток затвора). |
| `param.VcerMax` | maximum collector-emitter voltage with base-emitter resistor | Наибольшее напряжение коллектор-эмиттер при заданном Rбэ; условие Rbe вынесено в набор. |
| `param.VcerPulseMax` | maximum pulse collector-emitter voltage with base-emitter resistor | Наибольшее импульсное напряжение коллектор-эмиттер при заданном Rбэ. |
| `param.VgdMax` | maximum gate-drain voltage | Наибольшее напряжение затвор-сток. |
| `param.VgsMin` | maximum negative gate-source voltage | Наибольшее отрицательное напряжение затвор-исток (дополняет VgsMax). |
| `param.IcSatMax` | maximum saturation collector current | Наибольший постоянный ток коллектора в режиме насыщения. |
| `param.RthCase` | thermal resistance junction-case | Тепловое сопротивление переход-корпус / переход-среда (уточняют Rth). |
| `param.RthAmbient` | thermal resistance junction-ambient | Тепловое сопротивление переход-среда. |
| `param.TempCaseMax` | maximum case temperature | Наибольшая температура корпуса. |
| `param.PdPulseMax` | maximum pulse drain power dissipation | Импульсные предельные данные полевых транзисторов. |
| `param.VdsPulseMax` | maximum pulse drain-source voltage | Наибольшее импульсное напряжение сток-исток. |
| `param.VgsPulseMax` | maximum pulse gate-source voltage | Наибольшее импульсное напряжение затвор-исток. |
| `param.EtaUjt` | unijunction intrinsic standoff ratio | Коэффициент передачи однопереходного транзистора (η). |
| `param.fGen` | maximum generation frequency | Предельная частота генерации однопереходного транзистора. |
| `param.Veb1Sat` | emitter-base1 saturation voltage | Напряжение насыщения эмиттер-база1 однопереходного транзистора. |
| `param.Ipeak` | peak-point current | Ток включения (пиковый ток) однопереходного транзистора. |
| `param.Ivalley` | valley-point current | Ток выключения (ток впадины) однопереходного транзистора. |
| `param.Rbb` | inter-base resistance | Межбазовое сопротивление однопереходного транзистора. |
| `param.VbbMax` | maximum inter-base voltage | Межбазовое напряжение (предельное). |
| `param.IePulseMax` | maximum pulse emitter current | Импульсный ток эмиттера. |
| `param.Ron` | on-state key resistance | Сопротивление открытого ключа (биполярный ключ). |
| `param.RonMismatch` | on-state key resistance mismatch | Асимметрия сопротивления открытого ключа. |
| `param.tonKey` | key turn-on time | Время включения/выключения ключа при заданной схеме (условия — notes). |
| `param.toffKey` | key turn-off time | Время выключения ключа при заданной схеме (условия — notes). |
| `param.tonUjt` | unijunction turn-on time | Время включения однопереходного транзистора. |
| `param.Imod` | modulation current | Ток модуляции однопереходного транзистора. |
| `param.TCRbb` | inter-base resistance temperature coefficient | Температурный коэффициент межбазового сопротивления. |
| `param.PeMax` | maximum emitter power dissipation | Наибольшая рассеиваемая мощность эмиттера (однопереходный). |
| `param.tr` | rise time | Время нарастания/спада/задержки импульса. |
| `param.tf` | fall time | Время спада импульса. |
| `param.td` | delay time | Время задержки импульса. |
| `param.RinPulse` | pulse large-signal input resistance | Импульсное входное сопротивление в режиме большого сигнала. |
| `param.IcAvgMax` | maximum average collector current | Наибольший средний ток коллектора. |
| `param.linearity` | linearity factor | Коэффициент линейности: отношение h21э при малом и большом токе эмиттера (формула — в примечании записи). |
| `param.VebForward` | emitter-base forward voltage | Прямое падение напряжения на эмиттерном переходе при отключённом коллекторе. |
| `param.Rb` | base resistance | Сопротивление базы (rб); условия могут не указываться. |
| `param.VbrCeo` | collector-emitter breakdown voltage | Пробивное напряжение коллектор-эмиттер при заданном обратном токе коллектора (не менее); отличается от граничного напряжения VceCutoff. |
| `param.h22e` | output conductance, common emitter | Выходная полная проводимость в схеме ОЭ (h22э). |
| `param.VbrEbo` | emitter-base breakdown voltage | Пробивное напряжение эмиттер-база при заданном токе эмиттера. |
| `param.VonKey` | on-state key voltage | Падение напряжения на открытом ключе. |
| `param.IoffKey` | off-state key current | Ток закрытого ключа. |
| `param.Vcontrol` | control junction voltage | Напряжение на управляющих переходах. |
| `param.VcbControlMax` | maximum control collector-base voltage | Предельное напряжение управления коллектор-база структуры. |
| `param.VeeoMax` | maximum emitter-emitter voltage | Предельное напряжение на закрытом ключе между эмиттерами. |
| `param.Rcb` | collector-base resistance | Сопротивление коллектор-база. |
| `param.PebCwMax` | maximum continuous emitter-base RF power | Непрерывная СВЧ-мощность в цепи эмиттер-база. |
| `param.PebPulseMax` | maximum pulse emitter-base RF power | Импульсная СВЧ-мощность в цепи эмиттер-база. |
| `param.Ices` | collector-emitter cutoff current, base shorted | Обратный ток коллектор-эмиттер при короткозамкнутой цепи база-эмиттер (Rбэ=0). |
| `param.PinMax` | maximum input RF power | Входная СВЧ-мощность (непрерывная/импульсная). |
| `param.PinPulseMax` | maximum pulse input RF power | Импульсная входная СВЧ-мощность. |
| `param.PinAvgMax` | maximum average input RF power | Средняя СВЧ-мощность на входе. |
| `param.IleakPair` | inter-transistor leakage current | Ток утечки между транзисторами сборки. |
| `param.IbPulseMax` | maximum pulse base current | Наибольший импульсный ток базы. |
| `param.hfeRatio` | static current gain ratio (pair) | Отношение статических коэффициентов передачи тока двух транзисторов подобранной пары/сборки. |
| `param.VbeDiff` | base-emitter voltage difference (pair) | Разность напряжений база-эмиттер двух транзисторов пары/сборки. |
| `param.VcbDiff` | collector-base voltage difference (pair) | Разность напряжений коллектор-база двух транзисторов пары/сборки. |
| `param.VbrAvalanche` | avalanche breakdown voltage | Напряжение лавинного пробоя. |
| `param.IcAvalancheMax` | maximum avalanche collector current | Ток коллектора в лавинном режиме (предельный). |
| `param.TCVbeDiff` | base-emitter voltage difference temperature coefficient | Температурный коэффициент разности напряжений эмиттер-база пары. |
| `param.TCVcbDiff` | collector-base voltage difference temperature coefficient | Температурный коэффициент разности напряжений коллектор-база пары. |
| `param.TCIbDiff` | base current difference temperature coefficient | Температурный коэффициент разности токов базы пары. |
| `param.IccLeak` | inter-collector leakage current | Ток утечки между коллекторами транзисторов сборки. |
| `param.Vc1c2Max` | maximum collector-collector voltage | Постоянное напряжение между коллекторами транзисторов сборки. |
| `param.VnDensity` | noise voltage spectral density | Спектральная плотность ЭДС шума. |
| `param.Qn` | noise charge | Среднеквадратичный шумовой заряд. |
| `param.IgfMax` | maximum DC forward gate current | Постоянный прямой ток затвора (предельный). |
| `param.VsbMax` | maximum source-substrate voltage | Наибольшее напряжение исток-подложка. |
| `param.RinsChannel` | channel-case insulation resistance | Сопротивление изоляции канал-корпус. |
| `param.gfsMismatch` | transconductance mismatch (pair) | Относительная разность крутизны транзисторов пары. |
| `param.IdssMismatch` | initial drain current mismatch (pair) | Относительная разность начальных токов стока транзисторов пары. |
| `param.VgsOffMismatch` | cutoff voltage mismatch (pair) | Относительная разность напряжений отсечки транзисторов пары. |
| `param.Yin` | input admittance | Полная входная проводимость полевого транзистора на высокой частоте. |
| `param.Yout` | output admittance | Полная выходная проводимость полевого транзистора на высокой частоте. |
| `param.Rin` | input resistance | Входное сопротивление полевого транзистора на высокой частоте. |
| `param.VgsQuadratic` | quadratic region gate-source voltage | Участок квадратичности переходной характеристики. |
| `param.gfs2` | second-gate transconductance | Крутизна характеристики по второму затвору. |
| `param.VdSubMax` | maximum drain-substrate voltage | Наибольшее напряжение сток-подложка. |
| `param.VggMax` | maximum gate-gate voltage | Наибольшее напряжение между первым и вторым затворами. |
| `param.Vgs` | gate-source voltage at specified drain current | Напряжение затвор-исток при заданном токе стока (не пороговое). |
| `param.PinOverloadMin` | minimum overload input power | Порог перегрузки (минимальная входная СВЧ-мощность сжатия усиления). |
| `param.IdssRatio` | initial drain current ratio (pair) | Отношение начальных токов стока двух транзисторов пары. |
| `param.gfsRatio` | transconductance ratio (pair) | Отношение крутизны двух транзисторов пары. |
| `unit.uV` | microvolt | микровольт |
| `unit.uV_per_degC` | microvolt per degree Celsius | микровольт на градус Цельсия |
| `unit.nA_per_degC` | nanoampere per degree Celsius | наноампер на градус Цельсия |
| `unit.nV_per_sqrtHz` | nanovolt per square root hertz | нановольт на корень из герца |
| `unit.fC` | femtocoulomb | фемтокулон |
| `condition.Vc1c2` | collector-collector voltage | напряжение между коллекторами |
| `condition.Vbb` | inter-base voltage | межбазовое напряжение |

## 7. Проверка

- `catalogctl catalog import catalog-extension.jsonc --dry-run` — без Issues.

- `catalogctl import data/<файл>.jsonc --dry-run` — 284 файла, 0 Issues.

- Сводный импорт в чистую БД — 1004 записи, 0 ошибок.

