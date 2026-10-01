package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// CatalogService — операции с каталогом: снимок для UI и валидации,
// импорт (расширение каталога — план 02 §5.4). Экспорт — этап 4
// (реестр форматов наполнения).
type CatalogService struct {
	app *App
}

// Snapshot возвращает актуальный снимок каталога (кэш с проверкой
// catalog_revision на каждом обращении — смена каталога другим процессом
// видна без перезапуска).
func (s *CatalogService) Snapshot(ctx context.Context) (*catalog.Snapshot, error) {
	return s.app.cache.Snapshot(ctx)
}

// Import применяет каталог upsert'ом по коду после валидации метасхемы
// (ApplyCatalog: вставка новой строки либо обновление полей; enum-значения,
// наборы условий и применимость к классам замещаются целиком; непустой
// список проблем — отказ целиком). Изменение каталога инкрементирует
// catalog_revision атомарно в той же транзакции.
func (s *CatalogService) Import(ctx context.Context, in catalog.Input) error {
	cur, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return err
	}
	if _, problems := catalog.ApplyCatalog(cur, in); len(problems) > 0 {
		return problems[0].Err()
	}
	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	if err := tx.WriteCatalog(ctx, in); err != nil {
		return err
	}
	if err := tx.BumpCatalogRevision(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.app.cache.Invalidate()
	return nil
}

// Search выполняет поиск: подстрока обозначения, фильтры полей/атрибутов/
// параметров (применимость к классу запроса — D7, ошибки валидации
// запроса), сортировка с финальным (kind, designation), пагинация
// (limit ≤ 200); вариантные параметрические фильтры применяются в
// пределах одного исполнения (plan/04-module-functionality.md §1.1).
func (s *DeviceService) Search(ctx context.Context, q SearchQuery) (SearchPage, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return SearchPage{}, err
	}
	req, err := buildSearchRequest(snap, q)
	if err != nil {
		return SearchPage{}, err
	}
	rows, total, err := s.app.db.Search(ctx, req)
	if err != nil {
		return SearchPage{}, err
	}
	page := SearchPage{
		Items:  make([]SearchItem, 0, len(rows)),
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
	}
	for _, r := range rows {
		item := SearchItem{
			ID:          r.ID,
			Kind:        domain.Kind(r.Kind),
			System:      domain.System(r.System),
			Designation: r.Designation,
		}
		for _, f := range r.Fields {
			if f.Text != nil {
				item.Fields = append(item.Fields, domain.TextField(f.Name, *f.Text))
			} else if f.Num != nil {
				item.Fields = append(item.Fields, domain.NumField(f.Name, *f.Num))
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// buildSearchRequest валидирует запрос по каталогу и собирает транспортный
// запрос хранилища.
func buildSearchRequest(snap *catalog.Snapshot, q SearchQuery) (storage.SearchRequest, error) {
	req := storage.SearchRequest{
		Kind:   string(q.Kind),
		System: string(q.System),
		Query:  normalizeQuery(q.Query),
		Limit:  q.Limit,
		Offset: q.Offset,
	}
	if req.Limit <= 0 {
		req.Limit = searchDefaultLimit
	}
	if req.Limit > searchMaxLimit {
		req.Limit = searchMaxLimit
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	if q.Kind != "" {
		if _, ok := snap.Kind(q.Kind); !ok {
			return req, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("неизвестный класс приборов «%s»", string(q.Kind)))
		}
	}
	if q.System != "" {
		if _, ok := snap.System(q.System); !ok {
			return req, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("неизвестная система обозначений «%s»", string(q.System)))
		}
	}
	for _, f := range q.Fields {
		if f.Field == "" {
			return req, domain.NewError(domain.CodeValidationFailed, "не задано поле фильтра обозначения")
		}
		req.Fields = append(req.Fields, storage.FieldCond{
			Field: f.Field, Text: f.Text, Num: f.Num, HasNum: f.HasNum, Op: storage.FilterOp(f.Op),
		})
	}
	for _, a := range q.Attributes {
		def, ok := snap.Attribute(a.Attribute)
		if !ok {
			return req, domain.NewError(domain.CodeUnknownAttribute,
				fmt.Sprintf("неизвестный атрибут «%s»", a.Attribute))
		}
		if q.Kind != "" && !def.AppliesTo(q.Kind) {
			return req, domain.NewError(domain.CodeAttributeNotApplicable,
				fmt.Sprintf("атрибут «%s» неприменим к классу %s", a.Attribute, string(q.Kind)))
		}
		cond := storage.AttrCond{Code: a.Attribute, Text: a.Text, Num: a.Num, HasNum: a.HasNum, Op: storage.FilterOp(a.Op)}
		switch def.Type {
		case catalog.AttrText, catalog.AttrEnum:
			if a.HasNum {
				return req, domain.NewError(domain.CodeValidationFailed,
					fmt.Sprintf("фильтр атрибута «%s»: ожидается текстовое значение", a.Attribute))
			}
		default:
			if !a.HasNum {
				return req, domain.NewError(domain.CodeValidationFailed,
					fmt.Sprintf("фильтр атрибута «%s»: ожидается число", a.Attribute))
			}
		}
		req.Attrs = append(req.Attrs, cond)
	}
	for _, p := range q.Parameters {
		def, ok := snap.Parameter(p.Parameter)
		if !ok {
			return req, domain.NewError(domain.CodeUnknownParameter,
				fmt.Sprintf("неизвестный параметр «%s»", p.Parameter))
		}
		if q.Kind != "" && !def.AppliesTo(q.Kind) {
			return req, domain.NewError(domain.CodeParameterNotApplicable,
				fmt.Sprintf("параметр «%s» неприменим к классу %s", p.Parameter, string(q.Kind)))
		}
		hasNum := p.Min != nil || p.Max != nil || p.Exact != nil
		hasText := p.Text != ""
		if !hasNum && !hasText {
			return req, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("фильтр параметра «%s»: задайте значение", p.Parameter))
		}
		cond := storage.ParamCond{Code: p.Parameter, Text: p.Text, HasText: hasText}
		if def.ValueType == catalog.ValueText || def.ValueType == catalog.ValueEnum {
			if hasNum {
				return req, domain.NewError(domain.CodeValidationFailed,
					fmt.Sprintf("фильтр параметра «%s»: ожидается текстовое значение", p.Parameter))
			}
		} else if hasText {
			return req, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("фильтр параметра «%s»: ожидается число", p.Parameter))
		}
		if p.Min != nil {
			cond.Min, cond.HasMin = *p.Min, true
		}
		if p.Max != nil {
			cond.Max, cond.HasMax = *p.Max, true
		}
		if p.Exact != nil {
			cond.Exact, cond.HasExact = *p.Exact, true
		}
		req.Params = append(req.Params, cond)
	}
	for _, so := range q.Sort {
		// "designation"/"id" — колонки записи; прочие ключи — коды полей
		// разбора (значение передаётся параметром, неизвестное поле даёт
		// NULL — записи без поля в конце порядка).
		req.Sorts = append(req.Sorts, storage.SortKey{Key: so.Key, Numeric: so.Numeric, Desc: so.Desc})
	}
	return req, nil
}

// normalizeQuery приводит подстроку поиска к каноническому регистру
// (хранятся канонические обозначения; символы подстановки экранирует
// хранилище — LIKE с ESCAPE переносим).
func normalizeQuery(q string) string {
	return strings.ToUpper(strings.TrimSpace(q))
}
