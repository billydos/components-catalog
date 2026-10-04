package catalog

import (
	"context"
	"sync"
)

// Store — транспорт каталога: чтение определений и ревизии каталога
// (план работ 2.1). Реализация над БД — слой storage (этап 3): переносимые
// SELECT по каталожным таблицам и schema_meta. Задача транспорта — только
// доставка строк: бизнес-правила в нём не живут.
type Store interface {
	// LoadSnapshot загружает полный снимок каталога (все определения
	// одним согласованным чтением); Revision снимка читается тем же
	// согласованным чтением и соответствует загруженным строкам.
	LoadSnapshot(ctx context.Context) (*Snapshot, error)

	// CatalogRevision возвращает текущее значение catalog_revision
	// (единственный SELECT schema_meta — проверка кэша на каждом
	// обращении к сервисам).
	CatalogRevision(ctx context.Context) (int64, error)
}

// Cache — кэш снимка каталога в памяти: при каждом обращении проверяется
// catalog_revision; при изменении снимок атомарно заменяется целиком
// (замена указателя — docs/plan/01-architecture.md §2.2). Снимок иммутабелен
// в рамках обработки одного запроса; корректно и для нескольких
// процессов — импорт каталога в общую базу виден без перезапуска.
type Cache struct {
	store Store

	mu    sync.Mutex
	snap  *Snapshot
	rev   int64
	ready bool
}

// NewCache создаёт кэш каталога над транспортом; первый Snapshot(ctx)
// загружает снимок, дальнейшие — только при смене ревизии.
func NewCache(store Store) *Cache {
	return &Cache{store: store}
}

// Snapshot возвращает актуальный снимок каталога. Гонка «прочитали старую
// ревизию — каталог применили — загрузили новый снимок» доброкачественна:
// следующий вызов видит новую ревизию и перезагружает снимок. Ревизия
// кэшируется вместе со снимком из его же согласованного чтения, поэтому
// кэш никогда не отвечает парой «снимок ≠ ревизия».
func (c *Cache) Snapshot(ctx context.Context) (*Snapshot, error) {
	rev, err := c.store.CatalogRevision(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready && c.rev == rev {
		return c.snap, nil
	}
	snap, err := c.store.LoadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	c.snap = snap
	c.rev = snap.Revision
	c.ready = true
	return c.snap, nil
}

// Invalidate сбрасывает кэш (следующий Snapshot загружает снимок заново).
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snap = nil
	c.ready = false
}
