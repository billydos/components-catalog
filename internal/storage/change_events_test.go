package storage

import (
	"context"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Запись событий через RecordChange (путь записи устройств) и чтение
// курсором: after=0 — все события, after=k — хвост, за концом — пусто;
// порядок — по возрастанию id; ревизия события — data_revision после
// инкремента в той же транзакции.
func TestChangeEventsCursor(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openDevicesDB(t, dialect)
			ctx := context.Background()

			record := func(op, designation string) {
				t.Helper()
				tx, err := db.Begin(ctx)
				if err != nil {
					t.Fatalf("begin: %v", err)
				}
				id, _, err := tx.InsertDevice(ctx, domain.KindResistor, "gost", designation)
				if err != nil {
					t.Fatalf("вставка %s: %v", designation, err)
				}
				if err := tx.RecordChange(ctx, op, domain.KindResistor, designation, id); err != nil {
					t.Fatalf("событие %s: %v", designation, err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit: %v", err)
				}
			}
			record("created", "D1")
			record("updated", "D1")
			record("deleted", "D1")
			record("created", "D2")

			events, dataRev, err := db.LoadChangeEvents(ctx, 0, 100)
			if err != nil {
				t.Fatalf("чтение фида: %v", err)
			}
			if len(events) != 4 {
				t.Fatalf("событий: %d, ожидалось 4: %+v", len(events), events)
			}
			ops := make([]string, 0, len(events))
			for i, e := range events {
				ops = append(ops, e.Op)
				if i > 0 && e.ID <= events[i-1].ID {
					t.Fatalf("порядок id не возрастает: %+v", events)
				}
				// data_revision после EnsureCreated — 1: инкремент i+1-го
				// события даёт ревизию i+2.
				if e.Revision != int64(i+2) {
					t.Fatalf("ревизия события %d: %d, ожидалась %d", e.ID, e.Revision, i+2)
				}
				if e.Kind != domain.KindResistor || e.Designation == "" || e.DeviceID == 0 {
					t.Fatalf("событие: %+v", e)
				}
			}
			if dataRev != 5 {
				t.Fatalf("data_revision: %d, ожидалась 5", dataRev)
			}
			wantOps := []string{"created", "updated", "deleted", "created"}
			for i, op := range ops {
				if op != wantOps[i] {
					t.Fatalf("операции: %v, ожидались %v", ops, wantOps)
				}
			}

			// Курсор: хвост после k-го события.
			tail, _, err := db.LoadChangeEvents(ctx, events[1].ID, 100)
			if err != nil {
				t.Fatalf("хвост: %v", err)
			}
			if len(tail) != 2 || tail[0].ID != events[2].ID || tail[1].ID != events[3].ID {
				t.Fatalf("хвост после %d: %+v", events[1].ID, tail)
			}

			// За концом фида — пустой результат.
			beyond, rev, err := db.LoadChangeEvents(ctx, events[3].ID, 100)
			if err != nil {
				t.Fatalf("за концом: %v", err)
			}
			if len(beyond) != 0 || rev != dataRev {
				t.Fatalf("за концом: %+v (rev %d)", beyond, rev)
			}

			// limit ограничивает страницу.
			head, _, err := db.LoadChangeEvents(ctx, 0, 2)
			if err != nil {
				t.Fatalf("лимит: %v", err)
			}
			if len(head) != 2 || head[0].ID != events[0].ID || head[1].ID != events[1].ID {
				t.Fatalf("страница по лимиту: %+v", head)
			}
		})
	}
}
