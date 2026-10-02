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
// отвергает неизвестный класс), ревизии — из schema_meta.
func (a *App) Stats(ctx context.Context) (Stats, error) {
	snap, err := a.cache.Snapshot(ctx)
	if err != nil {
		return Stats{}, err
	}
	catRev, dataRev, err := a.db.Revisions(ctx)
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
		n, err := a.db.CountDevices(ctx, k.Code)
		if err != nil {
			return Stats{}, err
		}
		st.Counts[k.Code] = n
		st.Total += n
	}
	return st, nil
}
