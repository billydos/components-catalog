package storage

import (
	"context"
	"database/sql"

	"github.com/billydos/components-catalog/internal/domain"
)

// Фасетные агрегаты значений параметров по классу (features.log CC-5):
// границы численных значений для слайдеров параметрических фильтров
// и частоты текстовых (enum) значений. Агрегаты — по фактическим строкам
// parameter_values записей класса (тип в целом и исполнения — как
// у фильтров поиска); доступ — индексами idx_pv_code_* (02-database.md §3).

// ParamBounds — численные границы значений параметра по записям класса:
// нижняя — MIN по нижним границам строк (value_min, точные значения —
// value_exact), верхняя — MAX по верхним (value_max, точные —
// value_exact); nil — строк такой формы нет. Семантика — как
// у параметрических фильтров поиска (paramCondSQL): границы фильтра
// применяются и к value_min/value_max, и к value_exact, поэтому слайдер
// по этим границам не отсекает записи, которые фильтр нашёл бы.
type ParamBounds struct {
	Min, Max *float64
}

// ParamValueFreq — текстовое значение параметра и число записей класса,
// имеющих его (значение в данных типа и нескольких исполнениях одной
// записи считается один раз — столько записей отфильтрует поиск).
type ParamValueFreq struct {
	Text    string
	Devices int
}

// ParameterBounds — MIN/MAX по параметрам записей класса одним
// GROUP BY-запросом (COALESCE/MIN/MAX переносимы между диалектами);
// параметры без численных строк в результате отсутствуют. Вызывается
// в транзакции чтения — согласованный снимок с прочими агрегатами
// (data_revision, частоты).
func (t *Tx) ParameterBounds(ctx context.Context, kind domain.Kind) (map[string]ParamBounds, error) {
	rows, err := t.query(ctx, `
SELECT pv.parameter_code,
       MIN(COALESCE(pv.value_min, pv.value_exact)),
       MAX(COALESCE(pv.value_max, pv.value_exact))
FROM parameter_values pv
JOIN devices d ON d.id = pv.device_id
WHERE d.kind_code = @kind
GROUP BY pv.parameter_code
ORDER BY pv.parameter_code`,
		map[string]any{"kind": string(kind)})
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck — чтение завершилось
	out := make(map[string]ParamBounds)
	for rows.Next() {
		var code string
		var minv, maxv sql.NullFloat64
		if err := rows.Scan(&code, &minv, &maxv); err != nil {
			return nil, err
		}
		var b ParamBounds
		if minv.Valid {
			f := minv.Float64
			b.Min = &f
		}
		if maxv.Valid {
			f := maxv.Float64
			b.Max = &f
		}
		out[code] = b
	}
	return out, rows.Err()
}

// ParameterValueFreqs — частоты текстовых значений параметров класса:
// GROUP BY (parameter_code, value_text), счётчик — число записей класса
// (COUNT(DISTINCT d.id)); порядок значений параметра — по убыванию
// частоты, затем по тексту (детерминированный порядок фасета).
func (t *Tx) ParameterValueFreqs(ctx context.Context, kind domain.Kind) (map[string][]ParamValueFreq, error) {
	rows, err := t.query(ctx, `
SELECT pv.parameter_code, pv.value_text, COUNT(DISTINCT d.id)
FROM parameter_values pv
JOIN devices d ON d.id = pv.device_id
WHERE d.kind_code = @kind AND pv.value_text IS NOT NULL
GROUP BY pv.parameter_code, pv.value_text
ORDER BY pv.parameter_code, COUNT(DISTINCT d.id) DESC, pv.value_text`,
		map[string]any{"kind": string(kind)})
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck — чтение завершилось
	out := make(map[string][]ParamValueFreq)
	for rows.Next() {
		var code, text string
		var n int
		if err := rows.Scan(&code, &text, &n); err != nil {
			return nil, err
		}
		out[code] = append(out[code], ParamValueFreq{Text: text, Devices: n})
	}
	return out, rows.Err()
}
