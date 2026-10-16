package storage

import (
	"context"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
)

// Поток изменений записей (change feed — features.log CC-2): по одному
// событию на каждую применённую операцию над записью устройства,
// источник для инкрементальной синхронизации производных индексов сайта
// (закрывает и запись в обход сайт-обёртки — catalogctl напрямую в ту же
// базу). События — про данные (data_revision): изменения каталога
// (catalog_revision) в фид не входят — для них ETag GET /catalog. Это
// не аудит: акторов и состояний before/after события не несут (история
// изменений — зона сайта).

// ChangeEvent — строка change_events: операция над записью, зафиксированная
// транзакцией изменения. Revision — значение data_revision ПОСЛЕ
// инкремента в той же транзакции; курсор потребителя — монотонный ID
// события: id присваивается вставкой и остаётся плотным курсором при
// любом числе событий в транзакции и при инкрементах ревизии мимо фида.
type ChangeEvent struct {
	ID          int64
	Revision    int64
	Op          string // created | updated | deleted — коды задаёт сервисный слой
	Kind        domain.Kind
	Designation string
	DeviceID    int64
}

// RecordChange инкрементирует data_revision и записывает событие изменения
// записи в той же транзакции — единый путь операций записи устройств:
// событие и изменение данных атомарны (откат транзакции не оставляет ни
// того, ни другого), вызывается один раз на транзакцию записи. Новое
// значение счётчика читается после UPDATE здесь же, в той же транзакции
// (не выносится за её пределы): SQLite — одно выделенное соединение
// записи, PostgreSQL — строка schema_meta заблокирована UPDATE до конца
// транзакции — конкурентный писатель не увидит промежуточное значение.
func (t *Tx) RecordChange(ctx context.Context, op string, kind domain.Kind,
	designation string, deviceID int64) error {
	if err := t.bumpRevision(ctx, metaDataRevision); err != nil {
		return err
	}
	var value string
	if err := t.queryRow(ctx,
		`SELECT value FROM schema_meta WHERE key = @key`,
		map[string]any{"key": metaDataRevision}).Scan(&value); err != nil {
		return err
	}
	revision, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return err
	}
	_, err = t.exec(ctx, `
INSERT INTO change_events(revision, op, kind_code, designation, device_id)
VALUES (@revision, @op, @kind, @designation, @device)`,
		map[string]any{
			"revision": revision, "op": op, "kind": string(kind),
			"designation": designation, "device": deviceID,
		})
	return err
}

// LoadChangeEvents возвращает события с идентификатором больше after
// в порядке возрастания id (курсор — монотонный id события, не ревизия)
// и текущий data_revision одним согласованным чтением: транзакция пула
// чтения даёт страницу событий и ревизию из одного снимка.
func (d *DB) LoadChangeEvents(ctx context.Context, after int64, limit int) ([]ChangeEvent, int64, error) {
	tx, err := d.BeginReadOnly(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	rows, err := tx.query(ctx, `
SELECT id, revision, op, kind_code, designation, device_id FROM change_events
WHERE id > @after
ORDER BY id
LIMIT @limit`,
		map[string]any{"after": after, "limit": limit})
	if err != nil {
		return nil, 0, err
	}
	var out []ChangeEvent
	for rows.Next() {
		var e ChangeEvent
		if err := rows.Scan(&e.ID, &e.Revision, &e.Op, &e.Kind, &e.Designation, &e.DeviceID); err != nil {
			rows.Close()
			return nil, 0, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	_, data, err := tx.Revisions(ctx)
	if err != nil {
		return nil, 0, err
	}
	return out, data, nil
}
