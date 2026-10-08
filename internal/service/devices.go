package service

import (
	"context"
	"sort"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// DeviceService — операции над записями устройств
// (docs/plan/04-module-functionality.md §1.1).
type DeviceService struct {
	app *App
}

// resolvedAnalog — разрешённая ссылка-аналог: id и обозначение цели.
type resolvedAnalog struct {
	id          int64
	designation string
	note        string
}

// PendingDesignations сообщает, будет ли обозначение создано этим же
// прогоном импорта: разрешение исходящих ссылок-аналогов на записи,
// встречающиеся в файле позже (dry-run без записи в БД).
type PendingDesignations func(kind domain.Kind, designation string) bool

// Upsert применяет запись наполнения с семантикой секций
// (docs/plan/02-database.md §6): разбор обозначения → транзакция → слитое
// состояние (вход + текущие секции) → валидация движком → каноническое
// сравнение (совпадение — Skipped без записи в БД и без инкремента
// ревизий) → применение секций целиком + инкремент data_revision.
// Ошибка в любом значении секции — запись не применяется вовсе.
func (s *DeviceService) Upsert(ctx context.Context, in DeviceInput) (Outcome, error) {
	return s.applyUpsert(ctx, in, nil, nil, true)
}

// DryRun проверяет запись наполнения без изменения базы и возвращает
// исход, который вернул бы Upsert (import --dry-run). snap — снимок
// каталога для проверки (nil — актуальный из кэша; dry-run файла,
// расширяющего каталог, проверяет записи по гипотетическому снимку);
// pending разрешает ссылки на обозначения, создаваемые этим же прогоном.
func (s *DeviceService) DryRun(ctx context.Context, in DeviceInput, snap *catalog.Snapshot,
	pending PendingDesignations) (Outcome, error) {
	return s.applyUpsert(ctx, in, snap, pending, false)
}

func (s *DeviceService) applyUpsert(ctx context.Context, in DeviceInput,
	snapOverride *catalog.Snapshot, pending PendingDesignations, write bool) (Outcome, error) {
	if strings.TrimSpace(in.Name) == "" {
		return "", domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcRecordNameMissing)
	}
	if err := validateInputShape(&in); err != nil {
		return "", err
	}

	p, err := s.app.designations.parse(ctx, in.Name, in.System, in.Kind)
	if err != nil {
		return "", err
	}

	snap := snapOverride
	if snap == nil {
		snap, err = s.app.cache.Snapshot(ctx)
		if err != nil {
			return "", err
		}
	}
	// Явные классификационные поля (секция fields): валидация после
	// разбора — известны класс и система записи.
	var explicit []domain.Field
	if in.Fields != nil {
		explicit, err = validateExplicitFields(p, *in.Fields, snap)
		if err != nil {
			return "", err
		}
	}
	// Имена секций — из групп каталога (неизвестная секция — ошибка,
	// значения не теряются молча).
	checkSections := func(sections []SectionInput) error {
		for _, sec := range sections {
			if _, ok := snap.GroupBySection(sec.Section); !ok {
				return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSectionUnknown, sec.Section)
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
		analogs, err = resolveAnalogs(ctx, tx, p.Kind, p.Designation, *in.Analogs, pending)
		if err != nil {
			return "", err
		}
	}

	var cur *deviceState
	if dev != nil {
		cur, err = merger.loadState(ctx, tx, dev.ID, p)
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
	merged := merger.merge(cur, &in, explicit, stateAnalogs)

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

	if dev == nil {
		if !write {
			return OutcomeAdded, nil // записи нет — откатит defer (транзакция без DML)
		}
		return OutcomeAdded, s.writeUpsert(ctx, tx, 0, p, in, cur, merged, explicit, analogs, snap)
	}
	if !write {
		return OutcomeUpdatedExisting, nil // откатит defer (транзакция без DML)
	}
	return OutcomeUpdatedExisting, s.writeUpsert(ctx, tx, dev.ID, p, in, cur, merged, explicit, analogs, snap)
}

// writeUpsert применяет слитое состояние записи в БД (секции целиком)
// и инкрементирует data_revision. Явные классификационные поля (explicit):
// при создании дополняют продукты разбора, при обновлении заменяют прежний
// набор (удаляются имена прежних и новых явных полей, вставляются новые).
func (s *DeviceService) writeUpsert(ctx context.Context, tx *storage.Tx, deviceID int64,
	p domain.ParsedDesignation, in DeviceInput, cur, merged *deviceState,
	explicit []domain.Field, analogs []resolvedAnalog, snap *catalog.Snapshot) error {
	if deviceID == 0 {
		id, _, err := tx.InsertDevice(ctx, p.Kind, p.System, p.Designation)
		if err != nil {
			return err
		}
		deviceID = id
		if err := tx.InsertDesignationFields(ctx, deviceID, p.Fields); err != nil {
			return err
		}
		if len(explicit) > 0 {
			if err := tx.InsertDesignationFields(ctx, deviceID, explicit); err != nil {
				return err
			}
		}
	} else if in.Fields != nil {
		if err := tx.DeleteDesignationFields(ctx, deviceID,
			fieldNamesUnion(cur.fields, explicit)); err != nil {
			return err
		}
		if len(explicit) > 0 {
			if err := tx.InsertDesignationFields(ctx, deviceID, explicit); err != nil {
				return err
			}
		}
	}

	if in.Attributes != nil {
		rows := make([]storage.AttrRow, 0, len(merged.attributes))
		for _, a := range merged.attributes {
			rows = append(rows, storage.AttrRow{
				Attribute: a.Attribute, Text: a.Text, Num: a.Num, Bool: a.Bool,
			})
		}
		if err := tx.ReplaceAttributes(ctx, deviceID, rows); err != nil {
			return err
		}
	}
	for _, sec := range in.Sections {
		g, _ := snap.GroupBySection(sec.Section)
		if err := tx.DeleteTypeGroupValues(ctx, deviceID, g.Code); err != nil {
			return err
		}
		vals := make([]storage.ParamValue, 0, len(merged.groupValues[g.Code]))
		for _, v := range merged.groupValues[g.Code] {
			vals = append(vals, storage.ParamValue{
				Parameter: v.Parameter, Exact: v.Exact, Min: v.Min, Max: v.Max,
				Text: v.Text, Conditions: condsToStorage(v.Conditions),
			})
		}
		if err := tx.InsertValues(ctx, deviceID, nil, vals); err != nil {
			return err
		}
	}
	if in.Variants != nil {
		if err := tx.DeleteVariants(ctx, deviceID); err != nil {
			return err
		}
		for i, v := range merged.variants {
			vid, err := tx.InsertVariant(ctx, deviceID, v.Label, i)
			if err != nil {
				return err
			}
			vals := make([]storage.ParamValue, 0, len(v.Values))
			for _, val := range v.Values {
				vals = append(vals, storage.ParamValue{
					Parameter: val.Parameter, Exact: val.Exact, Min: val.Min,
					Max: val.Max, Text: val.Text, Conditions: condsToStorage(val.Conditions),
				})
			}
			if err := tx.InsertValues(ctx, deviceID, &vid, vals); err != nil {
				return err
			}
		}
	}
	if in.Manufacturers != nil {
		if err := tx.ReplaceManufacturers(ctx, deviceID, merged.manufacturers); err != nil {
			return err
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
			return err
		}
	}

	if err := tx.BumpDataRevision(ctx); err != nil {
		return err
	}
	return tx.Commit()
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
				return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSectionMissing)
			}
			if seen[sec.Section] {
				return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSectionDuplicate, sec.Section)
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
			return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcAttrCodeMissing)
		}
		if seenAttrs[a.Attribute] {
			return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcAttrDuplicate, a.Attribute)
		}
		seenAttrs[a.Attribute] = true
	}
	if in.Manufacturers != nil {
		seen := make(map[string]bool, len(*in.Manufacturers))
		for i, name := range *in.Manufacturers {
			trimmed := strings.TrimSpace(name)
			(*in.Manufacturers)[i] = trimmed
			if trimmed == "" {
				return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcManufacturerEmpty)
			}
			if seen[trimmed] {
				return domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcManufacturerDuplicate, trimmed)
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
// (структурная валидация — docs/plan/03-data-model.md §8): канонизация,
// запрет самоссылки и дубликатов, существование цели; note — характер
// замены именно в этом направлении. pending (dry-run) допускает цель,
// которая будет создана этим же прогоном импорта позже (прямые ссылки
// на позднейшие записи файла).
func resolveAnalogs(ctx context.Context, tx *storage.Tx, kind domain.Kind,
	ownerDesignation string, list []AnalogInput, pending PendingDesignations) ([]resolvedAnalog, error) {
	seen := make(map[string]bool, len(list))
	out := make([]resolvedAnalog, 0, len(list))
	for _, a := range list {
		if strings.TrimSpace(a.Designation) == "" {
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcAnalogEmpty)
		}
		canonical, err := domain.Canonicalize(a.Designation)
		if err != nil {
			return nil, err
		}
		if canonical == ownerDesignation {
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcAnalogSelf, canonical)
		}
		if seen[canonical] {
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcAnalogDuplicate, canonical)
		}
		seen[canonical] = true
		dev, err := tx.FindDevice(ctx, kind, canonical)
		if err != nil {
			return nil, err
		}
		if dev == nil {
			if pending != nil && pending(kind, canonical) {
				// Цель будет создана этим же прогоном: dry-run без записи,
				// идентификатор не используется.
				out = append(out, resolvedAnalog{designation: canonical, note: strings.TrimSpace(a.Note)})
				continue
			}
			return nil, domain.NewErrorf(domain.CodeNotFound, domain.MsgSvcAnalogNotFound, canonical, string(kind))
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
			return nil, false, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgKindUnknown, string(kind))
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
	cards, err := s.buildCards(ctx, snap, []int64{dev.ID})
	if err != nil {
		return nil, false, err
	}
	if len(cards) == 0 {
		return nil, false, nil
	}
	return cards[0], true, nil
}

// GetByID возвращает карточку по стабильному id записи.
func (s *DeviceService) GetByID(ctx context.Context, id int64) (*Card, bool, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	cards, err := s.buildCards(ctx, snap, []int64{id})
	if err != nil {
		return nil, false, err
	}
	if len(cards) == 0 {
		return nil, false, nil
	}
	return cards[0], true, nil
}

// GetByIDs возвращает карточки по списку стабильных id пакетным чтением
// дочерних таблиц (экспорт — без запросов на каждую запись): порядок
// результата — по порядку id входа, параллельно удалённые записи
// пропускаются.
func (s *DeviceService) GetByIDs(ctx context.Context, ids []int64) ([]*Card, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildCards(ctx, snap, ids)
}

// Delete удаляет запись (каскад из devices по всем дочерним таблицам,
// включая обе стороны device_analogs) с чисткой сирот производителей;
// false — запись не найдена. Удаление инкрементирует data_revision.
func (s *DeviceService) Delete(ctx context.Context, kind domain.Kind, designation string) (bool, error) {
	if kind != "" && !kind.IsValid() {
		return false, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgKindUnknown, string(kind))
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
