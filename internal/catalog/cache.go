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

	mu      sync.Mutex
	snap    *Snapshot
	rev     int64
	ready   bool
	loading *loadFlight
}

// loadFlight — однократная загрузка снимка (single-flight): грузит первый
// обратившийся, остальные ждут закрытия done и читают res (запись res —
// до close, потому видима каждому дождавшемуся).
type loadFlight struct {
	done chan struct{}
	res  loadResult
}

// loadResult — результат однократной загрузки снимка.
type loadResult struct {
	snap *Snapshot
	err  error
}

// NewCache создаёт кэш каталога над транспортом; первый Snapshot(ctx)
// загружает снимок, дальнейшие — только при смене ревизии.
func NewCache(store Store) *Cache {
	return &Cache{store: store}
}

// Snapshot возвращает актуальный снимок каталога. Загрузку ведёт один
// вызов (single-flight): параллельные читатели не блокируются мьютексом
// на время загрузки, а ждут её результата; отменённый ждущий возвращается
// с ошибкой своего контекста, не дожидаясь загрузки (при отмене вызова
// ждать нечего). Дождавшийся при несовпадении ревизии перечитывает её
// и повторяет цикл (снимок мог быть загружен до свежей записи каталога).
// Гонка «прочитали старую ревизию — каталог
// применили — загрузили новый снимок» доброкачественна: следующий вызов
// видит новую ревизию и перезагружает снимок. Ревизия кэшируется вместе
// со снимком из его же согласованного чтения, поэтому кэш никогда не
// отвечает парой «снимок ≠ ревизия».
func (c *Cache) Snapshot(ctx context.Context) (*Snapshot, error) {
	for {
		rev, err := c.store.CatalogRevision(ctx)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.ready && c.rev == rev {
			snap := c.snap
			c.mu.Unlock()
			return snap, nil
		}
		if c.loading == nil {
			fl := &loadFlight{done: make(chan struct{})}
			c.loading = fl
			c.mu.Unlock()
			snap, err := c.store.LoadSnapshot(ctx)
			if err != nil {
				fl.res = loadResult{err: err}
			} else {
				fl.res = loadResult{snap: snap}
				c.mu.Lock()
				c.snap = snap
				c.rev = snap.Revision
				c.ready = true
				c.mu.Unlock()
			}
			c.mu.Lock()
			c.loading = nil
			c.mu.Unlock()
			close(fl.done)
			if err != nil {
				return nil, err
			}
			return snap, nil
		}
		fl := c.loading
		c.mu.Unlock()
		select {
		case <-fl.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if fl.res.err != nil {
			return nil, fl.res.err
		}
	}
}

// Invalidate сбрасывает кэш (следующий Snapshot загружает снимок заново).
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snap = nil
	c.ready = false
}
