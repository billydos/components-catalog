package testutil

import (
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/storage"
)

// TestCatalogTablesMatchDDL — список очистки синхронен с DDL хранилища:
// storage.TableNames — единый источник (импорт storage в testutil
// запрещён циклом через тесты storage, поэтому здесь копия).
func TestCatalogTablesMatchDDL(t *testing.T) {
	want := storage.TableNames()
	got := slices.Clone(catalogTables)
	slices.Sort(got)
	for _, tbl := range want {
		if !slices.Contains(got, tbl) {
			t.Errorf("таблица %s из DDL отсутствует в списке очистки testutil", tbl)
		}
	}
	if len(got) != len(want) {
		t.Errorf("список очистки testutil: %d таблиц, в DDL — %d "+
			"(лишние имена не проверяются, длина обязана совпадать)", len(got), len(want))
	}
}
