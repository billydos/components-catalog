package service

import (
	"context"
	"slices"
	"sort"
	"strconv"
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

// PendingDesignations — план прогона импорта для dry-run: обозначения
// записей, которые прогон создаст (Records — разрешение исходящих
// ссылок-аналогов на записи, встречающиеся в файле позже; реальный
// импорт разрешает их многопроходно), и последовательная симуляция
// применённой части прогона: обозначения уже проверенных записей
// (Created) и заявки их исторических названий (Synonym). Конфликт
// уникальности синонима dry-run обязан видеть в том же месте файла,
// что и реальный импорт, — прогон ничего не пишет в БД. Поля
// необязательны (nil — знания нет); Created и Synonym регистрируются
// по порядку файла после успешной проверки записи.
type PendingDesignations struct {
	Records func(kind domain.Kind, designation string) bool
	Created func(kind domain.Kind, designation string) bool
	Synonym func(kind domain.Kind, designation string) (ownerDesignation string, claimed bool)
}

// Upsert применяет запись наполнения с семантикой секций
// (docs/plan/02-database.md §6): разбор обозначения → слитое
// состояние (вход + текущие секции) → каноническое сравнение (побайтово
// тот же вход — Skipped до валидации, без записи в БД и без инкремента
// ревизий: эволюция каталога не делает существующие записи неотрекаемыми)
// → валидация движком → применение секций целиком + инкремент
// data_revision. Ошибка в любом значении секции — запись не применяется
// вовсе.
func (s *DeviceService) Upsert(ctx context.Context, in DeviceInput) (Outcome, error) {
	return s.applyUpsert(ctx, in, nil, PendingDesignations{}, true, false)
}

// Create применяет запись наполнения только как создание (POST REST):
// тот же конвейер проверок, что Upsert, но существующая запись
// (kind, designation) — найденная в транзакции либо столкнувшаяся
// по уникальному ключу при вставке (параллельный создатель) — отказ
// already_exists без применения секций и без изменения базы: 409
// перестаёт мутировать по построению.
func (s *DeviceService) Create(ctx context.Context, in DeviceInput) (Outcome, error) {
	return s.applyUpsert(ctx, in, nil, PendingDesignations{}, true, true)
}

// DryRun проверяет запись наполнения без изменения базы и возвращает
// исход, который вернул бы Upsert (import --dry-run). snap — снимок
// каталога для проверки (nil — актуальный из кэша; dry-run файла,
// расширяющего каталог, проверяет записи по гипотетическому снимку);
// pending разрешает ссылки на обозначения, создаваемые этим же прогоном.
func (s *DeviceService) DryRun(ctx context.Context, in DeviceInput, snap *catalog.Snapshot,
	pending PendingDesignations) (Outcome, error) {
	return s.applyUpsert(ctx, in, snap, pending, false, false)
}

func (s *DeviceService) applyUpsert(ctx context.Context, in DeviceInput,
	snapOverride *catalog.Snapshot, pending PendingDesignations, write, createOnly bool) (Outcome, error) {
	if strings.TrimSpace(in.Name) == "" {
		return "", domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcRecordNameMissing)
	}
	if err := validateInputShape(&in); err != nil {
		return "", err
	}

	p, err := s.app.designations.ParseForSystem(ctx, in.Name, in.System, in.Kind)
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

	// Фаза чтения — транзакция пула чтения: проверки без DML не занимают
	// выделенное соединение записи (SQLite) и не блокируют писателей.
	dev, cur, merged, analogs, err := func() (*storage.DeviceRow, *deviceState, *deviceState, []resolvedAnalog, error) {
		rtx, err := s.app.db.BeginReadOnly(ctx)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		defer rtx.Rollback() //nolint:errcheck — откат транзакции чтения
		return s.readMerged(ctx, rtx, merger, p, &in, explicit, pending)
	}()
	if err != nil {
		return "", err
	}
	if dev != nil && createOnly {
		return "", domain.NewErrorf(domain.CodeAlreadyExists,
			domain.MsgSvcRecordAlreadyExists, p.Designation)
	}
	// Каноническое сравнение до валидации: побайтово тот же вход — Skipped
	// без записи в БД и без инкремента ревизий — эволюция каталога
	// (деактивация параметра/атрибута) не делает существующие записи
	// неотрекаемыми; изменённый upsert по-прежнему валидируется и может
	// быть отвергнут (значения деактивированных параметров секции
	// заменяются на актуальные — ограничение зафиксировано в плане 04).
	if outcome, err, done := skippedOrValidated(merger, snap, p, dev, cur, merged); done {
		return outcome, err
	}
	outcome := OutcomeUpdatedExisting
	if dev == nil {
		outcome = OutcomeAdded
	}
	if !write {
		return outcome, nil // dry-run: записи нет — транзакция чтения откатится
	}

	// Фаза записи — атомарная транзакция записи: состояние перечитывается
	// (между фазами мог писать конкурент), валидация повторяется по
	// текущему состоянию. Свежесть снимка валидации перепроверяется
	// ревизией в самой транзакции: при дрейфе транзакция откатывается,
	// снимок обновляется из кэша ВНЕ соединения записи (DB-чтение внутри
	// Tx самоблокируется на :memory:/MaxOpen=1 — db.go) и попытка
	// повторяется по свежему снимку; коды групп стабильны (каталог
	// эволюционирует добавлением и деактивацией), слитое состояние
	// пересобирать не нужно. Повторный дрейф — гонка ничтожного окна:
	// попытка продолжается с текущим снимком.
	for attempt := 0; ; attempt++ {
		tx, err := s.app.db.Begin(ctx)
		if err != nil {
			return "", err
		}
		dev, cur, merged, analogs, err = s.readMerged(ctx, tx, merger, p, &in, explicit, pending)
		if err != nil {
			tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
			return "", err
		}
		if snapOverride == nil {
			rev, rerr := tx.CatalogRevision(ctx)
			if rerr != nil {
				tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
				return "", rerr
			}
			if rev != snap.Revision && attempt == 0 {
				tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
				fresh, ferr := s.app.cache.Snapshot(ctx)
				if ferr != nil {
					return "", ferr
				}
				snap = fresh
				merger = &stateMerger{snap: snap}
				continue
			}
			// Повторный дрейф (вторая попытка уже на свежем снимке) —
			// гонка ничтожного окна, продолжаем с текущим снимком.
		}
		if dev != nil && createOnly {
			tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
			return "", domain.NewErrorf(domain.CodeAlreadyExists,
				domain.MsgSvcRecordAlreadyExists, p.Designation)
		}
		if outcome, err, done := skippedOrValidatedCommit(merger, snap, p, tx, dev, cur, merged); done {
			return outcome, err
		}
		if dev == nil {
			return OutcomeAdded, s.writeUpsert(ctx, tx, 0, p, in, cur, merged, explicit, analogs, snap, createOnly)
		}
		return OutcomeUpdatedExisting, s.writeUpsert(ctx, tx, dev.ID, p, in, cur, merged, explicit, analogs, snap, false)
	}
}

// readMerged читает запись в транзакции (чтения либо записи) и строит
// слитое состояние: поиск по ключу, текущие секции, разрешение аналогов
// и синонимов.
func (s *DeviceService) readMerged(ctx context.Context, tx *storage.Tx, merger *stateMerger,
	p domain.ParsedDesignation, in *DeviceInput, explicit []domain.Field,
	pending PendingDesignations) (*storage.DeviceRow, *deviceState, *deviceState, []resolvedAnalog, error) {
	dev, err := tx.FindDevice(ctx, p.Kind, p.Designation)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var deviceID int64
	if dev != nil {
		deviceID = dev.ID
	} else {
		// Обратная сторона уникальности синонима: создание записи
		// с обозначением, занятым историческим именем другой записи
		// (в том числе заявленным ранее записью этого же прогона —
		// dry-run), отвергается — синоним не может молча оказаться
		// в тени точного ключа (инвариант план 03 §9).
		owner, err := tx.FindSynonymOwner(ctx, p.Kind, p.Designation)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if owner == nil && pending.Synonym != nil {
			if claimOwner, claimed := pending.Synonym(p.Kind, p.Designation); claimed {
				return nil, nil, nil, nil, domain.NewErrorf(domain.CodeAlreadyExists,
					domain.MsgSvcSynonymOwnerConflict, p.Designation, claimOwner)
			}
		}
		if owner != nil {
			return nil, nil, nil, nil, domain.NewErrorf(domain.CodeAlreadyExists,
				domain.MsgSvcSynonymOwnerConflict, p.Designation, owner.Designation)
		}
	}
	var cur *deviceState
	if dev != nil {
		cur, err = merger.loadState(ctx, tx, deviceID, p)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	} else {
		cur = &deviceState{groupValues: make(map[string][]stateValue)}
	}
	var analogs []resolvedAnalog
	if in.Analogs != nil {
		analogs, err = resolveAnalogs(ctx, tx, p.Kind, p.Designation, *in.Analogs, pending)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	var synonyms []stateSynonym
	if in.Synonyms != nil {
		synonyms, err = resolveSynonyms(ctx, tx, p.Kind, p.Designation, *in.Synonyms, pending, deviceID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	var stateAnalogs []stateAnalog
	for _, a := range analogs {
		stateAnalogs = append(stateAnalogs, stateAnalog{Designation: a.designation, Note: a.note})
	}
	merged := merger.merge(cur, in, explicit, stateAnalogs, synonyms)
	return dev, cur, merged, analogs, nil
}

// skippedOrValidated — каноническое сравнение и валидация слитого
// состояния без записи (фаза чтения): побайтово тот же вход — Skipped;
// иначе запись (новая либо изменённая) проверяется движком. done=true —
// исход определён (Skipped либо ошибка валидации), записи не требуется.
func skippedOrValidated(merger *stateMerger, snap *catalog.Snapshot, p domain.ParsedDesignation,
	dev *storage.DeviceRow, cur, merged *deviceState) (Outcome, error, bool) {
	if dev != nil && sameState(cur, merged) {
		return OutcomeSkipped, nil, true
	}
	catDev := merger.toCatalogDevice(p, merged)
	engine := catalog.NewEngine(snap)
	if probs := engine.ValidateDevice(&catDev); len(probs) > 0 {
		return "", probs[0].Err(), true
	}
	return "", nil, false
}

// skippedOrValidatedCommit — то же в фазе записи: Skipped фиксируется
// пустой транзакцией, проблема валидации — отказ с откатом.
func skippedOrValidatedCommit(merger *stateMerger, snap *catalog.Snapshot, p domain.ParsedDesignation,
	tx *storage.Tx, dev *storage.DeviceRow, cur, merged *deviceState) (Outcome, error, bool) {
	if dev != nil && sameState(cur, merged) {
		if err := tx.Commit(); err != nil {
			return "", err, true
		}
		return OutcomeSkipped, nil, true
	}
	catDev := merger.toCatalogDevice(p, merged)
	engine := catalog.NewEngine(snap)
	if probs := engine.ValidateDevice(&catDev); len(probs) > 0 {
		return "", probs[0].Err(), true
	}
	return "", nil, false
}

// writeUpsert применяет слитое состояние записи в БД (секции целиком)
// и фиксирует изменение: инкремент data_revision + событие change_events —
// одной транзакцией (deviceID == 0 — создание, событие op=created; иначе
// обновление, op=updated; исход Skipped сюда не доходит — каноническое
// сравнение выполняется раньше, без записи в БД). Инвариант полей:
// хранимые поля = продукты разбора обозначения ∪ явные классификационные
// поля (explicit) ∪ производное поле base (для строгих систем — продукт
// парсера, для записей other — вычисляется по явным series/dev_number
// слитого состояния): при создании все наборы вставляются; при обновлении
// перезаписываются — удаляются имена объединения (прошлые хранимые ∪
// новые продукты ∪ явные поля ∪ base), вставляются новые продукты
// разбора, действующие явные поля и base (при заданной секции fields
// устаревшие продукты прежней грамматики удаляются; без секции поля,
// ставшие явными, сохраняются).
func (s *DeviceService) writeUpsert(ctx context.Context, tx *storage.Tx, deviceID int64,
	p domain.ParsedDesignation, in DeviceInput, cur, merged *deviceState,
	explicit []domain.Field, analogs []resolvedAnalog,
	snap *catalog.Snapshot, createOnly bool) error {
	op := ChangeOpUpdated
	if deviceID == 0 {
		op = ChangeOpCreated
	}
	var otherBase []domain.Field
	if p.System == domain.SystemOther {
		otherBase = append(otherBase, baseFieldOfOther(p, merged.fields))
	}
	if deviceID == 0 {
		id, inserted, err := tx.InsertDevice(ctx, p.Kind, p.System, p.Designation)
		if err != nil {
			return err
		}
		if !inserted && createOnly {
			// Запись появилась между проверкой в транзакции и вставкой
			// (параллельный создатель) — откат без применения секций.
			return domain.NewErrorf(domain.CodeAlreadyExists,
				domain.MsgSvcRecordAlreadyExists, p.Designation)
		}
		deviceID = id
		if err := tx.InsertDesignationFields(ctx, deviceID, p.Fields); err != nil {
			return err
		}
		if len(otherBase) > 0 {
			if err := tx.InsertDesignationFields(ctx, deviceID, otherBase); err != nil {
				return err
			}
		}
		if len(explicit) > 0 {
			if err := tx.InsertDesignationFields(ctx, deviceID, explicit); err != nil {
				return err
			}
		}
	} else {
		keptExplicit := explicit
		if in.Fields == nil {
			keptExplicit = cur.fields // секция не задана — явные поля сохраняются
		}
		pastNames := append(append([]domain.Field(nil), cur.fields...), keptExplicit...)
		names := fieldNamesUnion(p.Fields, pastNames)
		if !slices.Contains(names, domain.FieldBase) {
			names = append(names, domain.FieldBase)
		}
		if err := tx.DeleteDesignationFields(ctx, deviceID, names); err != nil {
			return err
		}
		if err := tx.InsertDesignationFields(ctx, deviceID, p.Fields); err != nil {
			return err
		}
		if len(otherBase) > 0 {
			if err := tx.InsertDesignationFields(ctx, deviceID, otherBase); err != nil {
				return err
			}
		}
		if len(keptExplicit) > 0 {
			if err := tx.InsertDesignationFields(ctx, deviceID, keptExplicit); err != nil {
				return err
			}
		}
	}

	if in.Attributes != nil {
		rows := make([]storage.AttrRow, 0, len(merged.attributes))
		for _, a := range merged.attributes {
			rows = append(rows, storage.AttrRow{
				Attribute: a.Attribute, Text: a.Text, Texts: a.Texts,
				Num: a.Num, Bool: a.Bool,
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
	if in.Synonyms != nil {
		rows := make([]storage.SynonymRow, 0, len(merged.synonyms))
		for i, syn := range merged.synonyms {
			rows = append(rows, storage.SynonymRow{
				Designation: syn.Designation, Note: syn.Note,
				YearFrom: syn.YearFrom, YearTo: syn.YearTo, SortOrder: i,
			})
		}
		if err := tx.ReplaceSynonyms(ctx, deviceID, rows); err != nil {
			return err
		}
	}

	if err := tx.RecordChange(ctx, string(op), p.Kind, p.Designation, deviceID); err != nil {
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

// baseFieldOfOther — производное поле base (тип) записи other: парсер
// полей не устанавливает, тип собирается по явным полям series и
// dev_number слитого состояния (МП39; МП41А → МП41); без семейства —
// само обозначение (группа типа вырождается в запись — план 03 §2.6).
func baseFieldOfOther(p domain.ParsedDesignation, explicit []domain.Field) domain.Field {
	series, hasSeries := "", false
	dev, hasDev := 0.0, false
	for _, f := range explicit {
		switch f.Name {
		case "series":
			series, hasSeries = f.Text, true
		case "dev_number":
			dev, hasDev = f.Num, true
		}
	}
	switch {
	case hasSeries && hasDev:
		return domain.TextField(domain.FieldBase, series+strconv.FormatFloat(dev, 'f', -1, 64))
	case hasSeries:
		return domain.TextField(domain.FieldBase, series)
	default:
		return domain.TextField(domain.FieldBase, p.Designation)
	}
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
			if pending.Records != nil && pending.Records(kind, canonical) {
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

// resolveSynonyms канонизирует исторические названия записи (план 03 §9)
// и проверяет уникальность обозначения-синонима в пределах класса:
// синоним не совпадает с обозначением самой записи, с обозначением
// другой записи и не принадлежит двум записям — ни в БД, ни среди
// применённой части этого же прогона (pending: Created — созданные
// записи, Synonym — заявки синонимов; deviceID — текущая
// запись-владелец, её собственный синоним при замене допустим).
// Годы употребления имени — те же границы, что у правила year_range
// (1949–2100), при задании обоих — по возрастанию. Возвращает синонимы
// в каноническом порядке (по обозначению) — он же sort_order применения.
func resolveSynonyms(ctx context.Context, tx *storage.Tx, kind domain.Kind,
	ownerDesignation string, list []SynonymInput, pending PendingDesignations,
	deviceID int64) ([]stateSynonym, error) {
	seen := make(map[string]bool, len(list))
	out := make([]stateSynonym, 0, len(list))
	for _, s := range list {
		if strings.TrimSpace(s.Designation) == "" {
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSynonymEmpty)
		}
		canonical, err := domain.Canonicalize(s.Designation)
		if err != nil {
			return nil, err
		}
		if canonical == ownerDesignation {
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSynonymSelf, canonical)
		}
		if seen[canonical] {
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgSvcSynonymDuplicate, canonical)
		}
		seen[canonical] = true
		if err := checkSynonymYears(canonical, s.YearFrom, s.YearTo); err != nil {
			return nil, err
		}
		dev, err := tx.FindDevice(ctx, kind, canonical)
		if err != nil {
			return nil, err
		}
		if dev != nil || (pending.Created != nil && pending.Created(kind, canonical)) {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSynonymConflictRecord, canonical, string(kind))
		}
		owner, err := tx.FindSynonymOwner(ctx, kind, canonical)
		if err != nil {
			return nil, err
		}
		if owner != nil && owner.DeviceID != deviceID {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSynonymConflictOther, canonical, owner.Designation)
		}
		if pending.Synonym != nil {
			// Заявка ранее проверенной записи этого же прогона (dry-run):
			// реальный импорт увидел бы строку в БД.
			if claimOwner, claimed := pending.Synonym(kind, canonical); claimed && claimOwner != ownerDesignation {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgSvcSynonymConflictOther, canonical, claimOwner)
			}
		}
		out = append(out, stateSynonym{
			Designation: canonical, Note: strings.TrimSpace(s.Note),
			YearFrom: s.YearFrom, YearTo: s.YearTo,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Designation < out[j].Designation })
	return out, nil
}

// checkSynonymYears — границы и порядок годов употребления имени
// (границы общие с правилом year_range атрибутов — catalog.YearRange*).
func checkSynonymYears(designation string, from, to *int) error {
	for _, y := range []*int{from, to} {
		if y != nil && (*y < catalog.YearRangeMin || *y > catalog.YearRangeMax) {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcSynonymYearRange, designation)
		}
	}
	if from != nil && to != nil && *from >= *to {
		return domain.NewErrorf(domain.CodeValidationFailed,
			domain.MsgSvcSynonymYearOrder, designation)
	}
	return nil
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

// ListIDsByKind перечисляет id записей класса в порядке (designation)
// одним запросом — последовательная выгрузка без дрейфа страниц
// (параллельные вставки/удаления не дают дубли и пропуски выборки).
func (s *DeviceService) ListIDsByKind(ctx context.Context, kind domain.Kind) ([]int64, error) {
	return s.app.db.ListDeviceIDsByKind(ctx, kind)
}

// Delete удаляет запись (каскад из devices по всем дочерним таблицам,
// включая обе стороны device_analogs) с чисткой сирот производителей;
// false — запись не найдена. Класс валидируется по снимку каталога —
// единому источнику истины (расширяемому импортом), как в Get/Search.
// Удаление атомарно по ключу в транзакции записи (без окна между поиском
// и удалением): конкурентное удаление того же обозначения оставляет этот
// вызов без «пустого» успеха и без инкремента ревизии. Удаление
// инкрементирует data_revision и пишет событие change_events (op=deleted;
// компенсационные удаления импортёра идут этим же путём — событие
// возникает само).
func (s *DeviceService) Delete(ctx context.Context, kind domain.Kind, designation string) (bool, error) {
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	if kind != "" {
		if _, ok := snap.Kind(kind); !ok {
			return false, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgKindUnknown, string(kind))
		}
	}
	canonical, err := domain.Canonicalize(designation)
	if err != nil {
		return false, err
	}
	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck — откат нефиксированной транзакции
	var dev *storage.DeviceRow
	if kind != "" {
		dev, err = tx.DeleteDeviceByKey(ctx, kind, canonical)
	} else {
		dev, err = tx.DeleteDeviceAnyKindByKey(ctx, canonical)
	}
	if err != nil {
		return false, err
	}
	if dev == nil {
		return false, nil
	}
	if err := tx.RecordChange(ctx, string(ChangeOpDeleted), dev.Kind, dev.Designation, dev.ID); err != nil {
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
