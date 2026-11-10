package service

import (
	"context"
	"slices"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// Источники данных записи (план 03 §13, D13): глобальный реестр
// с библиографическим описанием (code — стабильный ключ формата
// наполнения, назначаемый редактором) и направленными ссылками записей
// с заметкой места в источнике. Источники — данные записей (как
// производители): изменения определений инкрементируют data_revision,
// не порождая событий потока по отдельным записям.

// Границы года издания источника: шире year_range атрибутов записи
// (1949–2100) — источники включают досистемные справочники первой
// половины XX века.
const (
	sourceYearMin = 1900
	sourceYearMax = 2100
)

// SourceService — реестр источников данных записей: валидация
// определений корневой секции sources, применение (upsert по коду)
// и чтение.
type SourceService struct {
	app *App
}

// ValidateSourceDefs проверяет определения источников по снимку
// каталога: код (непустой, строчная латиница/цифры/подчёркивание),
// название, тип (словарь source_types), год в границах, списки
// авторов/редакторов/ссылок без пустых элементов и повторов,
// уникальность кодов в наборе. Ошибки — значения (CodeValidationFailed;
// неизвестный словарю тип — CodeUnknownSource).
func (s *SourceService) ValidateSourceDefs(defs []SourceInput, snap *catalog.Snapshot) error {
	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		if d.Code == "" {
			return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSourceCodeMissing)
		}
		if !sourceCodeValid(d.Code) {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceCodeCharset, d.Code)
		}
		if seen[d.Code] {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceDuplicateCode, d.Code)
		}
		seen[d.Code] = true
		if d.Title == "" {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceTitleMissing, d.Code)
		}
		if d.Type == "" {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceTypeMissing, d.Code)
		}
		if _, ok := snap.SourceType(d.Type); !ok {
			return domain.NewErrorf(domain.CodeUnknownSource,
				domain.MsgSvcSourceTypeUnknown, d.Code, d.Type,
				strings.Join(snap.SourceTypeCodes(), ", "))
		}
		if d.Year != nil && (*d.Year < sourceYearMin || *d.Year > sourceYearMax) {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceYearRange, d.Code, sourceYearMin, sourceYearMax)
		}
		// Фиксированный порядок проверки списков — детерминированное
		// сообщение при нескольких проблемных списках.
		for _, lc := range []struct {
			field string
			list  []string
		}{
			{"authors", d.Authors}, {"editors", d.Editors}, {"urls", d.URLs},
		} {
			if err := checkSourceList(d.Code, lc.field, lc.list); err != nil {
				return err
			}
		}
	}
	return nil
}

// sourceCodeValid — стабильный ключ реестра: строчная латиница, цифры,
// подчёркивание (переименование кода — несовместимое изменение формата).
func sourceCodeValid(code string) bool {
	if code == "" {
		return false
	}
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

// checkSourceList — элементы списка источника: непустые строки без
// повторов (nil — список не задан, допустим; пустой непустой-nil
// список от nil не отличим — «убрать ключ» читателя и есть пустой
// срез).
func checkSourceList(code, field string, list []string) error {
	seen := make(map[string]bool, len(list))
	for _, v := range list {
		if v == "" {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceListElementBad, code, field)
		}
		if seen[v] {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSourceListDuplicate, code, field, v)
		}
		seen[v] = true
	}
	return nil
}

// Apply применяет определения источников (upsert по коду: вставка новой
// строки реестра либо обновление полей и замещение лиц/ссылок целиком).
// Возвращает true, если хотя бы одна строка изменилась: изменение
// инкрементирует data_revision — карточки записей показывают источники,
// ETag /components обязан измениться; повторный импорт того же файла
// ревизию не двигает (идемпотентность). Ошибки валидации — до открытия
// транзакции.
func (s *SourceService) Apply(ctx context.Context, defs []SourceInput, snap *catalog.Snapshot) (bool, error) {
	if len(defs) == 0 {
		return false, nil
	}
	if err := s.ValidateSourceDefs(defs, snap); err != nil {
		return false, err
	}
	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	changed := false
	for _, d := range defs {
		cur, err := tx.LoadSourceByCode(ctx, d.Code)
		if err != nil {
			return false, err
		}
		row := sourceRowOf(d)
		if cur == nil {
			_, inserted, err := tx.InsertSource(ctx, row)
			if err != nil {
				return false, err
			}
			// Проигранная гонка вставки (параллельный писатель закоммитил
			// тот же код между чтением и вставкой) — не изменение этого
			// применения: ревизию не двигаем.
			if inserted {
				changed = true
			}
			continue
		}
		if !sourceEqual(*cur, row) {
			if err := tx.UpdateSource(ctx, cur.ID, row); err != nil {
				return false, err
			}
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := tx.BumpDataRevision(ctx); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ListSources читает весь реестр источников в порядке кода: экспорт
// пишет корневую секцию sources полного реестра — восстановление базы
// из экспортов не теряет источники, на которые не ссылается ни одна
// запись (в том числе при выгрузке одного класса).
func (s *SourceService) ListSources(ctx context.Context) ([]SourceInput, error) {
	rows, err := s.app.db.LoadSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SourceInput, len(rows))
	for i := range rows {
		out[i] = SourceInput{
			Code: rows[i].Code, Title: rows[i].Title, Type: rows[i].Type,
			Year: rows[i].Year, Publisher: rows[i].Publisher,
			Edition: rows[i].Edition, ISBN: rows[i].ISBN,
			Authors: rows[i].Authors, Editors: rows[i].Editors, URLs: rows[i].URLs,
		}
	}
	return out, nil
}

// sourceRowOf — строка хранилища из входного определения.
func sourceRowOf(d SourceInput) storage.SourceRow {
	return storage.SourceRow{
		Code: d.Code, Title: d.Title, Type: d.Type, Year: d.Year,
		Publisher: d.Publisher, Edition: d.Edition, ISBN: d.ISBN,
		Authors: d.Authors, Editors: d.Editors, URLs: d.URLs,
	}
}

// sourceEqual — сравнение определения с текущей строкой (идемпотентность
// применения: полное совпадение полей и списков — не изменение).
func sourceEqual(cur, row storage.SourceRow) bool {
	eqYear := func() bool {
		switch {
		case cur.Year == nil && row.Year == nil:
			return true
		case cur.Year == nil || row.Year == nil:
			return false
		default:
			return *cur.Year == *row.Year
		}
	}
	return cur.Code == row.Code && cur.Title == row.Title && cur.Type == row.Type &&
		eqYear() && cur.Publisher == row.Publisher && cur.Edition == row.Edition &&
		cur.ISBN == row.ISBN && slices.Equal(cur.Authors, row.Authors) &&
		slices.Equal(cur.Editors, row.Editors) && slices.Equal(cur.URLs, row.URLs)
}

// resolveDeviceSources разрешает ссылки записи на источники по реестру
// (в транзакции чтения либо записи — единый снимок с прочими чтениями
// конвейера): пустые кода и повторы — ошибки, неизвестный код —
// unknown_source; возвращаются id строк реестра с заметками в
// каноническом порядке по коду. pendingSource — коды, определённые
// корневой секцией sources этого же файла: dry-run не применяет реестр,
// ссылки на них валидны без строки в базе (применение корневой секции
// предшествует записям).
func resolveDeviceSources(ctx context.Context, tx *storage.Tx, in []SourceRefInput,
	pendingSource func(code string) bool) ([]storage.DeviceSourceRef, error) {
	if len(in) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(in))
	refs := make([]resolvedRef, 0, len(in))
	for _, ref := range in {
		if ref.Code == "" {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcDeviceSourceEmpty)
		}
		if seen[ref.Code] {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcDeviceSourceDuplicate, ref.Code)
		}
		seen[ref.Code] = true
		refs = append(refs, resolvedRef{code: ref.Code, note: ref.Note})
	}
	slices.SortStableFunc(refs, func(x, y resolvedRef) int { return strings.Compare(x.code, y.code) })
	out := make([]storage.DeviceSourceRef, 0, len(refs))
	for _, r := range refs {
		id, found, err := tx.FindSourceID(ctx, r.code)
		if err != nil {
			return nil, err
		}
		if !found {
			if pendingSource != nil && pendingSource(r.code) {
				continue
			}
			return nil, domain.NewErrorf(domain.CodeUnknownSource,
				domain.MsgSvcDeviceSourceNotFound, r.code)
		}
		out = append(out, storage.DeviceSourceRef{SourceID: id, Note: r.note})
	}
	return out, nil
}

// stateSourcesOf — ссылки состояния из разрешённых строк (код теряется
// при подстановке id — состояние сравнивает по коду, поэтому коды
// берутся из входа в том же каноническом порядке).
func stateSourcesOf(in []SourceRefInput) []stateSource {
	refs := make([]resolvedRef, 0, len(in))
	for _, ref := range in {
		refs = append(refs, resolvedRef{code: ref.Code, note: ref.Note})
	}
	slices.SortStableFunc(refs, func(x, y resolvedRef) int { return strings.Compare(x.code, y.code) })
	out := make([]stateSource, 0, len(refs))
	for _, r := range refs {
		out = append(out, stateSource{Code: r.code, Note: r.note})
	}
	return out
}

// resolvedRef — временная форма ссылки до подстановки id.
type resolvedRef struct {
	code string
	note string
}
