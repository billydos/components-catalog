package service

import (
	"context"

	"github.com/billydos/components-catalog/internal/domain"
)

// ChangeOp — код операции события потока изменений: стабильные коды
// REST/Go API (created | updated | deleted), отображаемых строк нет —
// это данные для синхронизации производных индексов, не UI.
type ChangeOp string

// Реестр операций потока изменений.
const (
	// ChangeOpCreated — запись создана (исход Upsert/Create — Added).
	ChangeOpCreated ChangeOp = "created"
	// ChangeOpUpdated — секции существующей записи применены
	// (исход UpdatedExisting).
	ChangeOpUpdated ChangeOp = "updated"
	// ChangeOpDeleted — запись удалена (каскад), включая компенсационные
	// удаления импортёра.
	ChangeOpDeleted ChangeOp = "deleted"
)

// ChangeEvent — событие потока изменений записей: одна применённая
// операция над записью устройства. Revision — data_revision транзакции
// события (после инкремента); курсор потребителя — монотонный ID
// (события одной ревизии различаются id). События покрывают только
// изменения данных: изменения каталога отслеживаются ETag GET /catalog.
type ChangeEvent struct {
	ID          int64
	Revision    int64
	Op          ChangeOp
	Kind        domain.Kind
	Designation string
	DeviceID    int64
}

// ChangesPage — страница потока изменений с текущей data_revision
// (согласованы: читаются одним снимком пула чтения).
type ChangesPage struct {
	Events       []ChangeEvent
	DataRevision int64
}

// Changes возвращает события изменений записей с идентификатором больше
// after в порядке возрастания id — не более limit штук — вместе с текущей
// data_revision. Курсор — id последнего полученного события (after = 0 —
// с начала фида); ревизия курсором быть не может: её связь с событиями —
// деталь пути записи (сегодня один инкремент на событие, но возможны
// несколько событий в одной транзакции либо инкремент мимо фида), id —
// плотный монотонный курсор вставки. Валидация —
// CodeValidationFailed: after ≥ 0, 1 ≤ limit ≤ ChangesLimitMax
// (встраиваемым потребителям Go API — тот же контракт, что у REST).
func (s *DeviceService) Changes(ctx context.Context, after int64, limit int) (ChangesPage, error) {
	if after < 0 {
		return ChangesPage{}, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgApiAfterNonNeg)
	}
	if limit < 1 || limit > ChangesLimitMax {
		return ChangesPage{}, domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgApiLimitRange, ChangesLimitMax)
	}
	rows, dataRevision, err := s.app.db.LoadChangeEvents(ctx, after, limit)
	if err != nil {
		return ChangesPage{}, err
	}
	page := ChangesPage{Events: make([]ChangeEvent, 0, len(rows)), DataRevision: dataRevision}
	for _, r := range rows {
		page.Events = append(page.Events, ChangeEvent{
			ID: r.ID, Revision: r.Revision, Op: ChangeOp(r.Op),
			Kind: r.Kind, Designation: r.Designation, DeviceID: r.DeviceID,
		})
	}
	return page, nil
}
