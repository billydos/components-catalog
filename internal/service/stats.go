package service

import (
	"context"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// Stats — агрегаты для REST /api/v1/stats: количества записей по классам,
// версия схемы модуля и счётчики ревизий (docs/plan/04-module-functionality.md
// §2).
type Stats struct {
	Counts          map[domain.Kind]int
	Total           int
	SchemaVersion   int
	CatalogRevision int64
	DataRevision    int64
}

// Stats собирает агрегаты приложения: снимок задаёт реестр классов
// (устройства вне реестра классов невозможны — движок валидации
// отвергает неизвестный класс), количества и ревизии — в одной
// транзакции чтения (согласованный снимок, без смешения состояний
// при конкурентной записи).
func (a *App) Stats(ctx context.Context) (Stats, error) {
	snap, err := a.cache.Snapshot(ctx)
	if err != nil {
		return Stats{}, err
	}
	tx, err := a.db.BeginReadOnly(ctx)
	if err != nil {
		return Stats{}, err
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	catRev, dataRev, err := tx.Revisions(ctx)
	if err != nil {
		return Stats{}, err
	}
	byKind, err := tx.CountDevicesByKind(ctx)
	if err != nil {
		return Stats{}, err
	}
	st := Stats{
		Counts:          make(map[domain.Kind]int, len(snap.Kinds)),
		SchemaVersion:   storage.SchemaVersion,
		CatalogRevision: catRev,
		DataRevision:    dataRev,
	}
	for _, k := range snap.Kinds {
		n := byKind[string(k.Code)]
		st.Counts[k.Code] = n
		st.Total += n
	}
	return st, nil
}

// ParameterStats — фасетные агрегаты значений параметров класса
// (features.log CC-5): min/max численных параметров для слайдеров
// параметрических фильтров и частоты текстовых (enum) значений.
type ParameterStats struct {
	Kind         domain.Kind
	DataRevision int64
	Parameters   []ParameterStat
}

// ParameterStat — агрегат одного параметра: границы численных значений
// (Min/Max, nil — значений такой формы нет; семантика — как у фильтров
// поиска: границы применяются к value_min/value_max и value_exact) либо
// частоты текстовых значений по записям класса.
type ParameterStat struct {
	Parameter string
	Unit      string
	Min, Max  *float64
	Values    []ParameterValueFreq
}

// ParameterValueFreq — текстовое значение параметра и число записей
// класса, имеющих его.
type ParameterValueFreq struct {
	Text  string
	Count int
}

// ParameterStats собирает фасетные агрегаты класса: границы и частоты —
// по фактическим данным записей класса (тип в целом и исполнения — как
// у фильтров поиска), отбор и разбивка по типу значения — по каталогу;
// порядок параметров — снимок каталога (алфавит кодов, как /catalog),
// значения — по убыванию частоты, затем по тексту. Параметры без данных
// в классе в результат не входят. Класс обязан существовать в каталоге
// (неизвестный — ошибка валидации).
func (s *DeviceService) ParameterStats(ctx context.Context, kind domain.Kind) (ParameterStats, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return ParameterStats{}, err
	}
	if _, ok := snap.Kind(kind); !ok {
		return ParameterStats{}, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgKindUnknown, string(kind))
	}
	tx, err := s.app.db.BeginReadOnly(ctx)
	if err != nil {
		return ParameterStats{}, err
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	_, dataRev, err := tx.Revisions(ctx)
	if err != nil {
		return ParameterStats{}, err
	}
	bounds, err := tx.ParameterBounds(ctx, kind)
	if err != nil {
		return ParameterStats{}, err
	}
	freqs, err := tx.ParameterValueFreqs(ctx, kind)
	if err != nil {
		return ParameterStats{}, err
	}
	out := ParameterStats{Kind: kind, DataRevision: dataRev}
	for i := range snap.Parameters {
		p := &snap.Parameters[i]
		// Неприменимый к классу параметр не показывается, даже если его
		// данные остались после изменения каталога (применимость замещается
		// без чистки данных): фильтр поиска по нему для этого класса —
		// 400 parameter_not_applicable, слайдер был бы неработоспособен.
		// Неактивные параметры с данными остаются — как и у фильтров поиска.
		if !p.AppliesTo(kind) {
			continue
		}
		st := ParameterStat{Parameter: p.Code, Unit: p.Unit}
		if p.ValueType.Numeric() {
			b := bounds[p.Code]
			st.Min, st.Max = b.Min, b.Max
			if st.Min == nil && st.Max == nil {
				continue // численных данных нет (только строки без границ)
			}
		} else {
			for _, v := range freqs[p.Code] {
				st.Values = append(st.Values, ParameterValueFreq{Text: v.Text, Count: v.Devices})
			}
			if len(st.Values) == 0 {
				continue
			}
		}
		out.Parameters = append(out.Parameters, st)
	}
	return out, nil
}
