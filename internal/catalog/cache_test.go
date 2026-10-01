package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
)

// fakeStore — транспорт каталога для тестов кэша: ревизия и число загрузок.
type fakeStore struct {
	rev    int64
	loads  int
	revErr error
}

func (s *fakeStore) CatalogRevision(context.Context) (int64, error) {
	if s.revErr != nil {
		return 0, s.revErr
	}
	return s.rev, nil
}

func (s *fakeStore) LoadSnapshot(context.Context) (*catalog.Snapshot, error) {
	s.loads++
	return &catalog.Snapshot{Revision: s.rev, Units: []catalog.UnitDef{{Code: "В", Name: "вольт", Symbol: "В"}}}, nil
}

// Кэш перезагружает снимок только при смене catalog_revision
// (единственный SELECT schema_meta на каждом обращении — 01 §2.2).
func TestCacheReloadsOnRevisionChange(t *testing.T) {
	store := &fakeStore{rev: 1}
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

	store.rev = 2
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
	store := &fakeStore{rev: 7}
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
	store.revErr = want
	if _, err := cache.Snapshot(ctx); !errors.Is(err, want) {
		t.Fatalf("ошибка транспорта не проброшена: %v", err)
	}
}
