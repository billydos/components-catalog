package importer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Importer — импорт/экспорт файлов наполнения поверх сервисного слоя
// приложения (plan/04-module-functionality.md §1.1 ImportService).
type Importer struct {
	app *service.App
}

// New создаёт импортёра над приложением.
func New(app *service.App) *Importer { return &Importer{app: app} }

// Report — итог прогона импорта: счётчики исходов записей и накопленные
// проблемы (Issues-модель: полный список за один прогон).
type Report struct {
	Name           string
	Format         Format
	DryRun         bool
	Records        int // записей в файле (включая отвергнутые)
	Added          int
	Updated        int
	Skipped        int
	Rejected       int // записи, не применённые из-за проблем
	CatalogApplied bool
	Issues         []Issue
}

// HasIssues сообщает, были ли проблемы прогона.
func (r *Report) HasIssues() bool { return len(r.Issues) > 0 }

// Import импортирует файл наполнения (формат — по расширению имени).
// Жёсткая ошибка (возвращается как error, прогона нет) — только синтаксис
// формата; проблемы формы и значений накапливаются в Report.Issues:
// корректные записи применяются, ошибочные — нет (запись не применяется
// вовсе, частичное применение записи запрещено).
func (m *Importer) Import(ctx context.Context, r io.Reader, name string, dryRun bool) (Report, error) {
	format, err := FormatByFilename(name)
	if err != nil {
		return Report{}, err
	}
	switch format {
	case FormatNDJSON:
		return m.importNDJSON(ctx, r, name, dryRun)
	default:
		return m.importDocument(ctx, r, name, format, dryRun)
	}
}

// ImportCatalogFile применяет только секцию catalog файла наполнения
// (catalog import): записи в файле не допускаются — для них есть import.
func (m *Importer) ImportCatalogFile(ctx context.Context, r io.Reader, name string, dryRun bool) (Report, error) {
	rep, err := m.Import(ctx, r, name, dryRun)
	if err != nil {
		return rep, err
	}
	if rep.Records > 0 {
		return rep, domain.NewError(domain.CodeInvalidImportFile,
			"файл содержит записи классов; catalog import применяется к файлам только с секцией catalog")
	}
	return rep, nil
}

// importDocument — jsonc/yaml: документ читается целиком; секция catalog
// применяется ДО чтения записей (блок catalog предшествует записям,
// использующим вводимые им определения), записи — в порядке файла.
func (m *Importer) importDocument(ctx context.Context, r io.Reader, name string, format Format, dryRun bool) (Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Report{}, err
	}
	root, err := parseTree(data, format)
	if err != nil {
		return Report{}, err
	}
	snap, err := m.app.Snapshot(ctx)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Name: name, Format: format, DryRun: dryRun}

	var catTree value
	if root.kind == kindObject {
		if catVal, ok := root.has("catalog"); ok {
			catTree = catVal
		}
	}
	snap, err = m.applyCatalog(ctx, catTree, snap, dryRun, &rep)
	if err != nil {
		return rep, err
	}

	doc, issues := ReadDocument(root, snap)
	rep.Issues = append(rep.Issues, issues...)
	rep.Records = len(doc.Records) + doc.Rejected
	rep.Rejected = doc.Rejected
	m.applyRecords(ctx, doc.Records, snap, dryRun, &rep)
	return rep, nil
}

// importNDJSON — построчный стриминг: строки catalog сливаются и
// применяются до первой записи (блок catalog предшествует записям),
// записи применяются по мере чтения.
func (m *Importer) importNDJSON(ctx context.Context, r io.Reader, name string, dryRun bool) (Report, error) {
	snap, err := m.app.Snapshot(ctx)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Name: name, Format: FormatNDJSON, DryRun: dryRun}

	var catTree value
	sc := newNDJSONScanner(r)
	catalogFlushed := false
	var records []Record
	flushCatalog := func() error {
		if catalogFlushed {
			return nil
		}
		catalogFlushed = true
		var err error
		snap, err = m.applyCatalog(ctx, catTree, snap, dryRun, &rep)
		return err
	}
	for {
		line, number, ok, err := sc.next()
		if err != nil {
			return rep, err
		}
		if !ok {
			break
		}
		v, err := parseLineJSON(line, number)
		if err != nil {
			return rep, err
		}
		if v.kind != kindObject {
			rep.Issues = append(rep.Issues, Issue{
				Line: number, Code: domain.CodeInvalidImportFile,
				Message: "строка должна быть объектом-обёрткой {\"<класс>\": <запись>} либо {\"catalog\": …}",
			})
			continue
		}
		if catVal, isCat := v.has("catalog"); isCat {
			if catalogFlushed {
				rep.Issues = append(rep.Issues, Issue{
					Line: number, Code: domain.CodeInvalidImportFile,
					Message: "блок catalog должен предшествовать записям",
				})
				continue
			}
			// Строки catalog сливаются до валидации метасхемы.
			merged, err := mergeCatalogTrees(catTree, catVal)
			if err != nil {
				return rep, err
			}
			catTree = merged
			continue
		}
		if err := flushCatalog(); err != nil {
			return rep, err
		}
		rec, ok, issues := ReadRecordLine(snap, v, number)
		rep.Issues = append(rep.Issues, issues...)
		rep.Records++
		if ok {
			records = append(records, rec)
		} else {
			rep.Rejected++
		}
	}
	if err := flushCatalog(); err != nil {
		return rep, err
	}
	m.applyRecords(ctx, records, snap, dryRun, &rep)
	return rep, nil
}

// mergeCatalogTrees объединяет деревья секции catalog (слияние строк
// NDJSON до валидации метасхемы — plan/04 §4).
func mergeCatalogTrees(a, b value) (value, error) {
	if a.kind == kindNull {
		return b, nil
	}
	if b.kind == kindNull {
		return a, nil
	}
	if a.kind != kindObject || b.kind != kindObject {
		return value{}, domain.NewError(domain.CodeInvalidImportFile,
			"catalog должен быть объектом с подразделами")
	}
	out := a
	for _, m := range b.members {
		existing, has := out.has(m.name)
		if !has {
			out.members = append(out.members, m)
			continue
		}
		if existing.kind != kindArray || m.value.kind != kindArray {
			return value{}, domain.NewError(domain.CodeInvalidImportFile,
				"подраздел каталога «"+m.name+"» должен быть массивом в каждой строке catalog")
		}
		existing.items = append(existing.items, m.value.items...)
		for i := range out.members {
			if out.members[i].name == m.name {
				out.members[i].value = existing
			}
		}
	}
	return out, nil
}

// applyCatalog применяет секцию catalog: чтение формы → ApplyCatalog
// (метасхема — единственная точка валидации определений) → запись в БД.
// Возвращает снимок для чтения записей: после применения — актуальный,
// в dry-run — гипотетический (расширения каталога видимы проверке записей
// без записи в БД).
func (m *Importer) applyCatalog(ctx context.Context, tree value, cur *catalog.Snapshot,
	dryRun bool, rep *Report) (*catalog.Snapshot, error) {
	if tree.kind == kindNull {
		return cur, nil
	}
	in, issues := ReadCatalogSection(tree)
	rep.Issues = append(rep.Issues, issues...)
	out, probs := catalog.ApplyCatalog(cur, in)
	for _, p := range probs {
		rep.Issues = append(rep.Issues, Issue{Code: p.Code, Message: p.Message})
	}
	if len(probs) > 0 {
		// Каталог не применён: записи читаются по текущему снимку и
		// получают собственные проблемы — полный список за прогон.
		return cur, nil
	}
	if dryRun {
		return out, nil
	}
	if err := m.app.Services().Catalog.Import(ctx, in); err != nil {
		return nil, err
	}
	rep.CatalogApplied = true
	return m.app.Snapshot(ctx)
}

// pendingRecord — запись в многопроходной очереди применения: последняя
// ошибка (разрешение аналога) и признак холостого создания без секции
// аналогов (взаимные ссылки).
type pendingRecord struct {
	rec     Record
	lastErr error
	created bool
}

// applyRecords применяет записи: многопроходное разрешение исходящих
// ссылок-аналогов (прямые ссылки на позднейшие записи файла), исходы
// считаются по записям. В dry-run записи проверяются без записи в БД;
// ссылки на создаваемые этим же прогоном обозначения допускаются.
func (m *Importer) applyRecords(ctx context.Context, records []Record, snap *catalog.Snapshot,
	dryRun bool, rep *Report) {
	if len(records) == 0 {
		return
	}
	pending := make(map[domain.Kind]map[string]bool, 4)
	for _, rec := range records {
		canonical, err := domain.Canonicalize(rec.Input.Name)
		if err != nil {
			continue // проблема записи даст её собственное применение
		}
		byKind, ok := pending[rec.Kind]
		if !ok {
			byKind = make(map[string]bool)
			pending[rec.Kind] = byKind
		}
		byKind[canonical] = true
	}
	pendingFn := func(kind domain.Kind, designation string) bool {
		return pending[kind][designation]
	}

	queue := make([]*pendingRecord, 0, len(records))
	for i := range records {
		queue = append(queue, &pendingRecord{rec: records[i]})
	}
	recordOutcome := func(rec Record, outcome service.Outcome) {
		switch outcome {
		case service.OutcomeAdded:
			rep.Added++
		case service.OutcomeUpdatedExisting:
			rep.Updated++
		case service.OutcomeSkipped:
			rep.Skipped++
		}
	}
	recordIssue := func(rec Record, err error) {
		rep.Rejected++
		rep.Issues = append(rep.Issues, issueFromError(rec, err))
	}

	if dryRun {
		for _, p := range queue {
			outcome, err := m.app.Services().Devices.DryRun(ctx, p.rec.Input, snap, pendingFn)
			if err != nil {
				recordIssue(p.rec, err)
				continue
			}
			recordOutcome(p.rec, outcome)
		}
		return
	}

	// Проходы повторяются, пока прогресс есть: ссылка на позднейшую запись
	// файла разрешается после её вставки. Взаимные ссылки (A → B и B → A в
	// одном файле) блокируют друг друга — цикл разрывает холостое создание
	// одной из записей без секции аналогов (03 §8: импорт воспроизводит
	// состояние независимо от порядка записей).
	for len(queue) > 0 {
		progress := false
		var next []*pendingRecord
		for _, p := range queue {
			outcome, err := m.app.Services().Devices.Upsert(ctx, p.rec.Input)
			if err != nil && isAnalogNotFound(err) {
				p.lastErr = err
				next = append(next, p)
				continue
			}
			switch {
			case err != nil:
				recordIssue(p.rec, err)
			case p.created:
				// Запись создана холостым применением — в этом прогоне она новая.
				rep.Added++
			default:
				recordOutcome(p.rec, outcome)
			}
			progress = true
		}
		queue = next
		if progress || len(queue) == 0 {
			continue
		}
		var advanced bool
		queue, advanced = m.breakAnalogDeadlock(ctx, queue, pendingFn, recordIssue)
		if !advanced {
			break
		}
	}
	for _, p := range queue {
		if p.created {
			// Компенсация: запись, созданная холостым применением, удаляется —
			// контракт «запись не применяется вовсе» сохраняется (холостое
			// создание в отчёт не входило).
			_, _ = m.app.Services().Devices.Delete(ctx, p.rec.Kind, p.rec.Input.Name)
		}
		recordIssue(p.rec, p.lastErr)
	}
}

// breakAnalogDeadlock разрывает взаимную блокировку ссылок-аналогов:
// холостое применение (без секции аналогов) первой подходящей записи —
// новая, все цели её ссылок создаются этим же прогоном. Полное состояние
// применит следующий проход; при неудаче вызывающая сторона удаляет
// холостую запись (компенсация). Возвращает очередь (возможно, без
// безнадёжной записи) и признак продвижения.
func (m *Importer) breakAnalogDeadlock(ctx context.Context, queue []*pendingRecord,
	pendingFn service.PendingDesignations, recordIssue func(Record, error)) ([]*pendingRecord, bool) {
	for i, p := range queue {
		if p.created || p.rec.Input.Analogs == nil {
			continue
		}
		if !allAnalogTargetsPending(p.rec, pendingFn) {
			continue
		}
		exists, err := m.deviceExists(ctx, p.rec)
		if err != nil || exists {
			continue
		}
		stripped := p.rec.Input
		stripped.Analogs = nil
		if _, err := m.app.Services().Devices.Upsert(ctx, stripped); err != nil {
			if isAnalogNotFound(err) {
				continue
			}
			recordIssue(p.rec, err)
			rest := append(append([]*pendingRecord{}, queue[:i]...), queue[i+1:]...)
			return rest, true
		}
		p.created = true
		return queue, true
	}
	return queue, false
}

// allAnalogTargetsPending сообщает, разрешатся ли все ссылки записи этим
// же прогоном (цели присутствуют в файле).
func allAnalogTargetsPending(rec Record, pendingFn service.PendingDesignations) bool {
	for _, a := range *rec.Input.Analogs {
		canonical, err := domain.Canonicalize(a.Designation)
		if err != nil || !pendingFn(rec.Kind, canonical) {
			return false
		}
	}
	return true
}

// deviceExists сообщает, существует ли запись в базе.
func (m *Importer) deviceExists(ctx context.Context, rec Record) (bool, error) {
	p, err := m.app.Services().Designations.ParseForSystem(ctx, rec.Input.Name, rec.Input.System, rec.Input.Kind)
	if err != nil {
		return false, err
	}
	_, found, err := m.app.Services().Devices.Get(ctx, p.Kind, p.Designation)
	return found, err
}

// isAnalogNotFound — ошибка разрешения аналога (цель не найдена): единственный
// источник not_found в Upsert.
func isAnalogNotFound(err error) bool {
	de, ok := domain.AsError(err)
	return ok && de.Code == domain.CodeNotFound
}

// issueFromError — проблема записи из ошибки домена.
func issueFromError(rec Record, err error) Issue {
	code := domain.CodeValidationFailed
	if de, ok := domain.AsError(err); ok {
		code = de.Code
	}
	return Issue{Record: rec.Input.Name, Line: rec.Line, Code: code, Message: err.Error()}
}

// Summary — строка итога прогона для CLI (контракт вывода).
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "записей: %d, добавлено: %d, обновлено: %d, без изменений: %d", r.Records, r.Added, r.Updated, r.Skipped)
	if r.Rejected > 0 {
		fmt.Fprintf(&b, ", отвергнуто: %d", r.Rejected)
	}
	if r.CatalogApplied {
		b.WriteString(", каталог расширен")
	}
	return b.String()
}
