package importer

import (
	"context"
	"io"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Importer — импорт/экспорт файлов наполнения поверх сервисного слоя
// приложения (docs/plan/04-module-functionality.md §1.1 ImportService).
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
		return m.importNDJSON(ctx, r, name, dryRun, false)
	default:
		return m.importDocument(ctx, r, name, format, dryRun, false)
	}
}

// ImportCatalogFile применяет только секцию catalog файла наполнения
// (catalog import): записи в файле не допускаются — для них есть import.
// Смешанный файл отвергается до каких-либо записей в БД: признак «есть
// записи» известен фазе чтения, применение каталога отложено до проверки.
func (m *Importer) ImportCatalogFile(ctx context.Context, r io.Reader, name string, dryRun bool) (Report, error) {
	format, err := FormatByFilename(name)
	if err != nil {
		return Report{}, err
	}
	switch format {
	case FormatNDJSON:
		return m.importNDJSON(ctx, r, name, dryRun, true)
	default:
		return m.importDocument(ctx, r, name, format, dryRun, true)
	}
}

// importDocument — jsonc/yaml: документ читается целиком; секция catalog
// планируется до чтения записей (блок catalog предшествует записям,
// использующим вводимые им определения), записи — в порядке файла.
// catalogOnly — режим catalog import: применение каталога отложено до
// проверки отсутствия записей (смешанный файл — отказ до записи в БД).
func (m *Importer) importDocument(ctx context.Context, r io.Reader, name string, format Format,
	dryRun, catalogOnly bool) (Report, error) {
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
	plan := m.planCatalog(catTree, snap, &rep)
	doc, issues := ReadDocument(root, plan.read)
	rep.Issues = append(rep.Issues, issues...)
	rep.Records = len(doc.Records) + doc.Rejected
	rep.Rejected = doc.Rejected
	if catalogOnly && rep.Records > 0 {
		return rep, domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgImportCatalogRecordsMixed)
	}
	readSnap, err := m.commitCatalog(ctx, plan, dryRun, &rep)
	if err != nil {
		return rep, err
	}
	m.applyRecords(ctx, doc.Records, readSnap, dryRun, &rep)
	return rep, nil
}

// importNDJSON — построчное чтение: строки catalog сливаются и
// планируются до первой записи (блок catalog предшествует записям).
// Записи буферизуются и применяются после цикла — файл применяется по
// принципу «всё-или-ничего» на синтаксисе (жёсткая ошибка прерывает
// прогон до применения буфера). catalogOnly — режим catalog import:
// применение каталога отложено до проверки отсутствия записей.
func (m *Importer) importNDJSON(ctx context.Context, r io.Reader, name string,
	dryRun, catalogOnly bool) (Report, error) {
	snap, err := m.app.Snapshot(ctx)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Name: name, Format: FormatNDJSON, DryRun: dryRun}

	var catTree value
	sc := newNDJSONScanner(r)
	catalogFlushed := false
	catTainted := false // проблема строки catalog: каталог не применяется
	var plan catalogPlan
	var records []Record
	flushCatalog := func() {
		if catalogFlushed {
			return
		}
		catalogFlushed = true
		if catTainted {
			// Ошибка слияния строк catalog: каталог не применяется, записи
			// читаются по текущему снимку и получают собственные проблемы.
			plan = catalogPlan{read: snap}
			return
		}
		plan = m.planCatalog(catTree, snap, &rep)
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
			rep.Issues = append(rep.Issues, issuef(number, domain.MsgImportNdjsonLineObject))
			continue
		}
		if catVal, isCat := v.has("catalog"); isCat {
			if catalogFlushed {
				rep.Issues = append(rep.Issues, issuef(number, domain.MsgImportNdjsonCatalogOrder))
				continue
			}
			if len(v.members) != 1 {
				// catalog и запись в одной строке-обёртке: строка не
				// сливается в каталог и не читается как запись.
				rep.Issues = append(rep.Issues, issuef(number, domain.MsgImportNdjsonWrapperSingle))
				continue
			}
			// Строки catalog сливаются до валидации метасхемы; ошибка
			// слияния — проблема строки (строка пропускается, каталог
			// не применяется), жёсткий отказ — только синтаксис.
			merged, err := mergeCatalogTrees(catTree, catVal)
			if err != nil {
				iss := issuef(number, domain.MsgImportCatalogPlain)
				if de, ok := domain.AsError(err); ok {
					iss = issuef(number, de.MsgID, de.Args...)
					iss.Code = de.Code
				}
				rep.Issues = append(rep.Issues, iss)
				catTainted = true
				continue
			}
			catTree = merged
			continue
		}
		flushCatalog()
		rec, ok, issues := ReadRecordLine(plan.read, v, number)
		rep.Issues = append(rep.Issues, issues...)
		rep.Records++
		if ok {
			records = append(records, rec)
		} else {
			rep.Rejected++
		}
	}
	flushCatalog()
	if catalogOnly && rep.Records > 0 {
		return rep, domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgImportCatalogRecordsMixed)
	}
	readSnap, err := m.commitCatalog(ctx, plan, dryRun, &rep)
	if err != nil {
		return rep, err
	}
	m.applyRecords(ctx, records, readSnap, dryRun, &rep)
	return rep, nil
}

// mergeCatalogTrees объединяет деревья секции catalog (слияние строк
// NDJSON до валидации метасхемы — docs/plan/04 §4).
func mergeCatalogTrees(a, b value) (value, error) {
	if a.kind == kindNull {
		return b, nil
	}
	if b.kind == kindNull {
		return a, nil
	}
	if a.kind != kindObject || b.kind != kindObject {
		return value{}, domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgImportCatalogPlain)
	}
	out := a
	for _, m := range b.members {
		existing, has := out.has(m.name)
		if !has {
			out.members = append(out.members, m)
			continue
		}
		if existing.kind != kindArray || m.value.kind != kindArray {
			return value{}, domain.NewErrorf(domain.CodeInvalidImportFile,
				domain.MsgImportNdjsonSubsection, m.name)
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

// catalogPlan — прочитанная секция catalog: вход метасхемы и план
// применения. Гейт — проблемы читателя (форма секции) и метасхемы
// отключают применение целиком: частичное применение каталога запрещено.
type catalogPlan struct {
	in    catalog.Input     // вход метасхемы (применяется при apply)
	read  *catalog.Snapshot // снимок чтения записей (не применяется — текущий)
	apply bool              // вход пригоден к применению
}

// planCatalog читает секцию catalog и валидирует её метасхемой без
// записи в БД. read — снимок для чтения записей: при проблемах формы
// либо метасхемы — текущий (каталог не будет применён, записи получат
// собственные проблемы — полный список за прогон), иначе — гипотетический
// (расширения каталога видимы проверке записей).
func (m *Importer) planCatalog(tree value, cur *catalog.Snapshot, rep *Report) catalogPlan {
	if tree.kind == kindNull {
		return catalogPlan{read: cur}
	}
	in, issues := ReadCatalogSection(tree)
	rep.Issues = append(rep.Issues, issues...)
	out, probs := catalog.ApplyCatalog(cur, in)
	for _, p := range probs {
		iss := issuef(0, p.MsgID, p.Args...)
		iss.Code = p.Code
		rep.Issues = append(rep.Issues, iss)
	}
	if len(issues) > 0 || len(probs) > 0 {
		return catalogPlan{in: in, read: cur}
	}
	return catalogPlan{in: in, read: out, apply: true}
}

// commitCatalog применяет план каталога: в dry-run — без записи (снимок
// чтения — гипотетический), иначе — Catalog.Import и свежий снимок БД.
func (m *Importer) commitCatalog(ctx context.Context, plan catalogPlan,
	dryRun bool, rep *Report) (*catalog.Snapshot, error) {
	if !plan.apply || dryRun {
		return plan.read, nil
	}
	if err := m.app.Services().Catalog.Import(ctx, plan.in); err != nil {
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
	// recordIssue выводит запись из очереди проблемой прогона; холостое
	// создание компенсируется удалением — контракт «запись не применяется
	// вовсе» (холостое создание в отчёт не входило). Штатное удаление
	// двигает data_revision, поэтому отвергнутая запись оставляет шум в
	// счётчике ревизий — принятый компромисс: служебного пути записи мимо
	// счётчика ревизий нет, целостность отчёта важнее точности счётчика.
	// Ошибка компенсации — проблема прогона, а не тихая потеря.
	recordIssue := func(p *pendingRecord, err error) {
		rep.Rejected++
		rep.Issues = append(rep.Issues, issueFromError(p.rec, err))
		if !p.created {
			return
		}
		if _, derr := m.app.Services().Devices.Delete(ctx, p.rec.Kind, p.rec.Input.Name); derr != nil {
			iss := issuef(p.rec.Line, domain.MsgInternalError, derr.Error())
			iss.Record = p.rec.Input.Name
			rep.Issues = append(rep.Issues, iss)
		}
	}

	if dryRun {
		for _, p := range queue {
			outcome, err := m.app.Services().Devices.DryRun(ctx, p.rec.Input, snap, pendingFn)
			if err != nil {
				recordIssue(p, err)
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
				recordIssue(p, err)
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
		// Компенсация холостого создания — внутри recordIssue.
		recordIssue(p, p.lastErr)
	}
}

// breakAnalogDeadlock разрывает взаимную блокировку ссылок-аналогов:
// холостое применение (без секции аналогов) первой подходящей записи —
// новая, все цели её ссылок создаются этим же прогоном. Полное состояние
// применит следующий проход; при неудаче вызывающая сторона удаляет
// холостую запись (компенсация). Возвращает очередь (возможно, без
// безнадёжной записи) и признак продвижения.
func (m *Importer) breakAnalogDeadlock(ctx context.Context, queue []*pendingRecord,
	pendingFn service.PendingDesignations, recordIssue func(*pendingRecord, error)) ([]*pendingRecord, bool) {
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
			recordIssue(p, err)
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
	iss := issuef(rec.Line, domain.MsgInternalError, err.Error())
	if de, ok := domain.AsError(err); ok {
		iss.Code = de.Code
		iss.MsgID = de.MsgID
		iss.Args = de.Args
		iss.Message = de.Message
	}
	iss.Record = rec.Input.Name
	return iss
}
