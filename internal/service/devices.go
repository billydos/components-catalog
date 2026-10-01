package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// DeviceService — операции над записями устройств
// (plan/04-module-functionality.md §1.1).
type DeviceService struct {
	app *App
}

// resolvedAnalog — разрешённая ссылка-аналог: id и обозначение цели.
type resolvedAnalog struct {
	id          int64
	designation string
	note        string
}

// Upsert применяет запись наполнения с семантикой секций
// (plan/02-database.md §6): разбор обозначения → транзакция → слитое
// состояние (вход + текущие секции) → валидация движком → каноническое
// сравнение (совпадение — Skipped без записи в БД и без инкремента
// ревизий) → применение секций целиком + инкремент data_revision.
// Ошибка в любом значении секции — запись не применяется вовсе.
func (s *DeviceService) Upsert(ctx context.Context, in DeviceInput) (Outcome, error) {
	if strings.TrimSpace(in.Name) == "" {
		return "", domain.NewError(domain.CodeValidationFailed, "не задано обозначение записи")
	}
	if err := validateInputShape(&in); err != nil {
		return "", err
	}

	p, err := s.app.designations.parse(ctx, in.Name, in.System, in.Kind)
	if err != nil {
		return "", err
	}

	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return "", err
	}
	// Имена секций — из групп каталога (неизвестная секция — ошибка,
	// значения не теряются молча).
	checkSections := func(sections []SectionInput) error {
		for _, sec := range sections {
			if _, ok := snap.GroupBySection(sec.Section); !ok {
				return domain.NewError(domain.CodeValidationFailed,
					fmt.Sprintf("секция «%s» не соответствует ни одной группе каталога", sec.Section))
			}
		}
		return nil
	}
	if err := checkSections(in.Sections); err != nil {
		return "", err
	}
	for _, v := range safeVariants(&in) {
		if err := checkSections(v.Sections); err != nil {
			return "", err
		}
	}
	merger := &stateMerger{snap: snap}

	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции

	dev, err := tx.FindDevice(ctx, p.Kind, p.Designation)
	if err != nil {
		return "", err
	}

	var analogs []resolvedAnalog
	if in.Analogs != nil {
		analogs, err = resolveAnalogs(ctx, tx, p.Kind, p.Designation, *in.Analogs)
		if err != nil {
			return "", err
		}
	}

	var cur *deviceState
	if dev != nil {
		cur, err = merger.loadState(ctx, tx, dev.ID)
		if err != nil {
			return "", err
		}
	} else {
		cur = &deviceState{groupValues: make(map[string][]stateValue)}
	}

	var stateAnalogs []stateAnalog
	for _, a := range analogs {
		stateAnalogs = append(stateAnalogs, stateAnalog{Designation: a.designation, Note: a.note})
	}
	merged := merger.merge(cur, &in, stateAnalogs)

	// Валидация слитого состояния — единственная точка записи (R5).
	catDev := merger.toCatalogDevice(p, merged)
	engine := catalog.NewEngine(snap)
	if probs := engine.ValidateDevice(&catDev); len(probs) > 0 {
		return "", probs[0].Err()
	}

	if dev != nil && sameState(cur, merged) {
		// Ничего не менялось: записи в БД нет, ревизии не инкрементируются.
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return OutcomeSkipped, nil
	}

	outcome := OutcomeUpdatedExisting
	deviceID := int64(0)
	if dev != nil {
		deviceID = dev.ID
	} else {
		id, _, err := tx.InsertDevice(ctx, p.Kind, p.System, p.Designation)
		if err != nil {
			return "", err
		}
		deviceID = id
		if err := tx.InsertDesignationFields(ctx, deviceID, p.Fields); err != nil {
			return "", err
		}
		outcome = OutcomeAdded
	}

	if in.Attributes != nil {
		rows := make([]storage.AttrRow, 0, len(merged.attributes))
		for _, a := range merged.attributes {
			rows = append(rows, storage.AttrRow{
				Attribute: a.Attribute, Text: a.Text, Num: a.Num, Bool: a.Bool,
			})
		}
		if err := tx.ReplaceAttributes(ctx, deviceID, rows); err != nil {
			return "", err
		}
	}
	for _, sec := range in.Sections {
		g, _ := snap.GroupBySection(sec.Section)
		if err := tx.DeleteTypeGroupValues(ctx, deviceID, g.Code); err != nil {
			return "", err
		}
		vals := make([]storage.ParamValue, 0, len(merged.groupValues[g.Code]))
		for _, v := range merged.groupValues[g.Code] {
			vals = append(vals, storage.ParamValue{
				Parameter: v.Parameter, Exact: v.Exact, Min: v.Min, Max: v.Max,
				Text: v.Text, Conditions: condsToStorage(v.Conditions),
			})
		}
		if err := tx.InsertValues(ctx, deviceID, nil, vals); err != nil {
			return "", err
		}
	}
	if in.Variants != nil {
		if err := tx.DeleteVariants(ctx, deviceID); err != nil {
			return "", err
		}
		for i, v := range merged.variants {
			vid, err := tx.InsertVariant(ctx, deviceID, v.Label, i)
			if err != nil {
				return "", err
			}
			vals := make([]storage.ParamValue, 0, len(v.Values))
			for _, val := range v.Values {
				vals = append(vals, storage.ParamValue{
					Parameter: val.Parameter, Exact: val.Exact, Min: val.Min,
					Max: val.Max, Text: val.Text, Conditions: condsToStorage(val.Conditions),
				})
			}
			if err := tx.InsertValues(ctx, deviceID, &vid, vals); err != nil {
				return "", err
			}
		}
	}
	if in.Manufacturers != nil {
		if err := tx.ReplaceManufacturers(ctx, deviceID, merged.manufacturers); err != nil {
			return "", err
		}
	}
	if in.Analogs != nil {
		rows := make([]storage.AnalogRow, 0, len(analogs))
		for _, a := range analogs {
			rows = append(rows, storage.AnalogRow{
				TargetID: a.id, Designation: a.designation, System: "", Note: a.note,
			})
		}
		if err := tx.ReplaceAnalogs(ctx, deviceID, rows); err != nil {
			return "", err
		}
	}

	if err := tx.BumpDataRevision(ctx); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return outcome, nil
}

func condsToStorage(conds []catalog.ConditionValue) []storage.Cond {
	if len(conds) == 0 {
		return nil
	}
	out := make([]storage.Cond, 0, len(conds))
	for _, c := range conds {
		out = append(out, storage.Cond{Code: c.Condition, Value: c.Value})
	}
	return out
}

// validateInputShape проверяет форму входа до разбора и транзакции:
// имена секций (в пределах своей области: секции записи и секции каждого
// исполнения независимы), дубликаты атрибутов, имена производителей.
// Значения проверяет движок валидации (по слитому состоянию).
func validateInputShape(in *DeviceInput) error {
	checkSections := func(sections []SectionInput) error {
		seen := make(map[string]bool, len(sections))
		for _, sec := range sections {
			if strings.TrimSpace(sec.Section) == "" {
				return domain.NewError(domain.CodeValidationFailed,
					"не задано имя секции группы параметров")
			}
			if seen[sec.Section] {
				return domain.NewError(domain.CodeValidationFailed,
					fmt.Sprintf("секция «%s» задана повторно", sec.Section))
			}
			seen[sec.Section] = true
		}
		return nil
	}
	if err := checkSections(in.Sections); err != nil {
		return err
	}
	seenAttrs := make(map[string]bool, len(in.Attributes))
	for _, a := range in.Attributes {
		if strings.TrimSpace(a.Attribute) == "" {
			return domain.NewError(domain.CodeValidationFailed, "не задан код атрибута")
		}
		if seenAttrs[a.Attribute] {
			return domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("атрибут «%s» задан повторно", a.Attribute))
		}
		seenAttrs[a.Attribute] = true
	}
	if in.Manufacturers != nil {
		seen := make(map[string]bool, len(*in.Manufacturers))
		for i, name := range *in.Manufacturers {
			trimmed := strings.TrimSpace(name)
			(*in.Manufacturers)[i] = trimmed
			if trimmed == "" {
				return domain.NewError(domain.CodeValidationFailed, "пустое имя производителя")
			}
			if seen[trimmed] {
				return domain.NewError(domain.CodeValidationFailed,
					fmt.Sprintf("производитель «%s» задан повторно", trimmed))
			}
			seen[trimmed] = true
		}
	}
	for _, v := range safeVariants(in) {
		if err := checkSections(v.Sections); err != nil {
			return err
		}
	}
	return nil
}

func safeVariants(in *DeviceInput) []VariantInput {
	if in.Variants == nil {
		return nil
	}
	return *in.Variants
}

// resolveAnalogs разрешает обозначения аналогов в пределах класса записи
// (структурная валидация — plan/03-data-model.md §8): канонизация,
// запрет самоссылки и дубликатов, существование цели; note — характер
// замены именно в этом направлении.
func resolveAnalogs(ctx context.Context, tx *storage.Tx, kind domain.Kind,
	ownerDesignation string, list []AnalogInput) ([]resolvedAnalog, error) {
	seen := make(map[string]bool, len(list))
	out := make([]resolvedAnalog, 0, len(list))
	for _, a := range list {
		if strings.TrimSpace(a.Designation) == "" {
			return nil, domain.NewError(domain.CodeValidationFailed, "пустое обозначение аналога")
		}
		canonical, err := domain.Canonicalize(a.Designation)
		if err != nil {
			return nil, err
		}
		if canonical == ownerDesignation {
			return nil, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("запись «%s» не может быть аналогом самой себя", canonical))
		}
		if seen[canonical] {
			return nil, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("аналог «%s» задан повторно", canonical))
		}
		seen[canonical] = true
		dev, err := tx.FindDevice(ctx, kind, canonical)
		if err != nil {
			return nil, err
		}
		if dev == nil {
			return nil, domain.NewError(domain.CodeNotFound,
				fmt.Sprintf("аналог «%s» не найден в классе %s", canonical, string(kind)))
		}
		out = append(out, resolvedAnalog{
			id: dev.ID, designation: canonical, note: strings.TrimSpace(a.Note),
		})
	}
	// Стабильный порядок для канонического сравнения состояний.
	sort.SliceStable(out, func(i, j int) bool { return out[i].designation < out[j].designation })
	return out, nil
}

// Get возвращает карточку по классу и обозначению (точный ключ).
// kind == "" — любой класс (обозначение уникально в пределах класса,
// при коллизии между классами вернётся детерминированно первая запись).
func (s *DeviceService) Get(ctx context.Context, kind domain.Kind, designation string) (*Card, bool, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	if kind != "" {
		if _, ok := snap.Kind(kind); !ok {
			return nil, false, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("неизвестный класс приборов «%s»", string(kind)))
		}
	}
	canonical, err := domain.Canonicalize(designation)
	if err != nil {
		return nil, false, err
	}
	var dev *storage.DeviceRow
	if kind != "" {
		dev, err = s.app.db.FindDevice(ctx, kind, canonical)
	} else {
		dev, err = s.app.db.FindDeviceAnyKind(ctx, canonical)
	}
	if err != nil {
		return nil, false, err
	}
	if dev == nil {
		return nil, false, nil
	}
	card, err := s.buildCard(ctx, snap, dev)
	if err != nil {
		return nil, false, err
	}
	return card, true, nil
}

// GetByID возвращает карточку по стабильному id записи.
func (s *DeviceService) GetByID(ctx context.Context, id int64) (*Card, bool, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	dev, err := s.app.db.FindDeviceByID(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if dev == nil {
		return nil, false, nil
	}
	card, err := s.buildCard(ctx, snap, dev)
	if err != nil {
		return nil, false, err
	}
	return card, true, nil
}

// Delete удаляет запись (каскад из devices по всем дочерним таблицам,
// включая обе стороны device_analogs) с чисткой сирот производителей;
// false — запись не найдена. Удаление инкрементирует data_revision.
func (s *DeviceService) Delete(ctx context.Context, kind domain.Kind, designation string) (bool, error) {
	if kind != "" && !kind.IsValid() {
		return false, domain.NewError(domain.CodeValidationFailed,
			fmt.Sprintf("неизвестный класс приборов «%s»", string(kind)))
	}
	canonical, err := domain.Canonicalize(designation)
	if err != nil {
		return false, err
	}
	var dev *storage.DeviceRow
	if kind != "" {
		dev, err = s.app.db.FindDevice(ctx, kind, canonical)
	} else {
		dev, err = s.app.db.FindDeviceAnyKind(ctx, canonical)
	}
	if err != nil {
		return false, err
	}
	if dev == nil {
		return false, nil
	}
	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	if err := tx.DeleteDevice(ctx, dev.ID); err != nil {
		return false, err
	}
	if err := tx.BumpDataRevision(ctx); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Count возвращает число записей (класса, если задан).
func (s *DeviceService) Count(ctx context.Context, kind *domain.Kind) (int, error) {
	var k domain.Kind
	if kind != nil {
		k = *kind
	}
	return s.app.db.CountDevices(ctx, k)
}
