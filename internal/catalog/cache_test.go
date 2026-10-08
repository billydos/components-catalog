package catalog_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/billydos/components-catalog/internal/catalog"
)

// fakeStore — транспорт каталога для тестов кэша: ревизия и число загрузок.
// gate (если задан) задерживает LoadSnapshot; snapRev (если не 0) — ревизия
// возвращаемого снимка (модель загрузки, начатой до записи каталога);
// entered сигнализирует вход в LoadSnapshot.
type fakeStore struct {
	mu      sync.Mutex
	rev     int64
	snapRev int64
	loads   int
	revErr  error
	gate    chan struct{}
	entered chan struct{}
}

func (s *fakeStore) SetRev(rev int64) {
	s.mu.Lock()
	s.rev = rev
	s.mu.Unlock()
}

func (s *fakeStore) CatalogRevision(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revErr != nil {
		return 0, s.revErr
	}
	return s.rev, nil
}

func (s *fakeStore) LoadSnapshot(context.Context) (*catalog.Snapshot, error) {
	s.mu.Lock()
	s.loads++
	rev := s.snapRev
	if rev == 0 {
		rev = s.rev
	}
	gate, entered := s.gate, s.entered
	s.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if gate != nil {
		<-gate
	}
	return &catalog.Snapshot{Revision: rev, Units: []catalog.UnitDef{{Code: "V"}}}, nil
}

// Кэш перезагружает снимок только при смене catalog_revision
// (единственный SELECT schema_meta на каждом обращении — 01 §2.2).
func TestCacheReloadsOnRevisionChange(t *testing.T) {
	store := &fakeStore{}
	store.SetRev(1)
	cache := catalog.NewCache(store)
	ctx := context.Background()

	snap1, err := cache.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if store.loads != 1 {
		t.Fatalf("загрузок: %d", store.loads)
	}
	for range 3 {
		snap, err := cache.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if snap != snap1 {
			t.Fatal("снимок заменён без смены ревизии")
		}
	}
	if store.loads != 1 {
		t.Fatalf("кэш не работает: загрузок %d", store.loads)
	}

	store.SetRev(2)
	snap2, err := cache.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if store.loads != 2 {
		t.Fatalf("снимок не перезагружен: загрузок %d", store.loads)
	}
	if snap2 == snap1 || snap2.Revision != 2 {
		t.Fatal("снимок не заменён целиком")
	}
}

func TestCacheInvalidateAndErrors(t *testing.T) {
	store := &fakeStore{}
	store.SetRev(7)
	cache := catalog.NewCache(store)
	ctx := context.Background()

	if _, err := cache.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate()
	if _, err := cache.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if store.loads != 2 {
		t.Fatalf("Invalidate не сбросил кэш: загрузок %d", store.loads)
	}

	want := errors.New("бд недоступна")
	store.mu.Lock()
	store.revErr = want
	store.mu.Unlock()
	if _, err := cache.Snapshot(ctx); !errors.Is(err, want) {
		t.Fatalf("ошибка транспорта не проброшена: %v", err)
	}
}

// Single-flight: параллельные читатели при пустом/устаревшем кэше грузят
// снимок один раз — загрузку ведёт первый, остальные ждут её результат.
func TestCacheSingleFlight(t *testing.T) {
	store := &fakeStore{gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	store.SetRev(1)
	cache := catalog.NewCache(store)
	ctx := context.Background()

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	snaps := make([]*catalog.Snapshot, n)
	for i := range n {
		go func() {
			defer wg.Done()
			snap, err := cache.Snapshot(ctx)
			if err != nil {
				t.Error(err)
			}
			snaps[i] = snap
		}()
	}
	// Загрузка стартовала и держится открытой, пока читатели собраны:
	// все они делят один результат, а не гоняют собственные загрузки.
	<-store.entered
	time.Sleep(20 * time.Millisecond) // читатели успевают занять место ожидания
	close(store.gate)
	wg.Wait()

	store.mu.Lock()
	loads := store.loads
	store.mu.Unlock()
	if loads != 1 {
		t.Fatalf("параллельные читатели вызвали %d загрузок, ожидалась 1", loads)
	}
	for i, snap := range snaps {
		if snap == nil || snap != snaps[0] {
			t.Fatalf("читатель %d получил другой снимок: %v", i, snap)
		}
	}
}

// Дождавшийся чужой загрузки при несовпадении ревизии перечитывает ревизию
// и загружает свежий снимок: загруженный другим вызовом мог начаться до
// записи каталога и оказаться старее ревизии, уже видимой читателю.
func TestCacheWaiterReloadsStaleLoad(t *testing.T) {
	store := &fakeStore{gate: make(chan struct{}), entered: make(chan struct{}, 2)}
	store.SetRev(1)
	// Первая загрузка вернёт снимок ревизии 1, начатый до смены ревизии.
	store.mu.Lock()
	store.snapRev = 1
	store.mu.Unlock()
	cache := catalog.NewCache(store)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := cache.Snapshot(ctx); err != nil {
			t.Error(err)
		}
	}()
	<-store.entered // первая загрузка в полёте
	store.SetRev(2) // каталог применили до её завершения
	go func() {
		defer wg.Done()
		snap, err := cache.Snapshot(ctx) // читатель уже видит ревизию 2
		if err != nil {
			t.Error(err)
			return
		}
		if snap.Revision != 2 {
			t.Errorf("читатель получил снимок ревизии %d, ожидалась 2", snap.Revision)
		}
	}()
	// Последующие загрузки возвращают текущую ревизию хранилища.
	store.mu.Lock()
	store.snapRev = 0
	store.mu.Unlock()
	close(store.gate)
	wg.Wait()

	store.mu.Lock()
	loads := store.loads
	store.mu.Unlock()
	if loads != 2 {
		t.Fatalf("загрузок %d, ожидалось 2", loads)
	}
	snap, err := cache.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Revision != 2 {
		t.Fatalf("кэш отвечает снимком ревизии %d, ожидалась 2", snap.Revision)
	}
}
