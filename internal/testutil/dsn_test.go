package testutil

import (
	"testing"

	"github.com/billydos/components-catalog/internal/storage"
)

// Локальная maskDSN — копия storage.MaskDSN (импорт storage в пакет
// запрещён циклом через тесты storage): пин закрепляет эквивалентность
// на представительных формах DSN.
func TestMaskDSNMatchesStorage(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:secret@localhost:5432/db?sslmode=disable",
		"postgres://user@localhost/db",
		"postgres://localhost/db",
		"postgres://user:@localhost/db",
		"postgresql://u:p%40x@h:5432/db#frag",
		"host=localhost user=u password=secret dbname=db",
		"host=localhost password='s p a c e' dbname=db",
		`host=localhost password="quo\"te" dbname=db`,
		"host=localhost xpassword=nope password=real",
		"no secret here user=u",
		"",
	} {
		if got, want := maskDSN(dsn), storage.MaskDSN(dsn); got != want {
			t.Errorf("maskDSN(%q) = %q, storage.MaskDSN = %q", dsn, got, want)
		}
	}
}
