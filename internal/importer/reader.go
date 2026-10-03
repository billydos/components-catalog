package importer

import (
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Issue — проблема прогона импорта (Issues-модель —
// docs/plan/04-module-functionality.md §4): корневые проблемы и проблемы
// записей накапливаются за один прогон; жёсткой ошибкой (возвращаемой
// как error) остаётся только синтаксис формата. Сообщение — MsgID
// каталога + аргументы (этап 8.3); Message — канонический en-рендер,
// локализованный рендер и сборка позиции — слой вывода CLI. Тексты —
// контракт CLI.
type Issue struct {
	Record  string // обозначение записи («» — проблема корневого уровня)
	Line    int    // номер строки NDJSON (0 — не определён)
	No      int    // порядковый номер записи в секции документа (0 — не задан)
	Code    domain.Code
	MsgID   domain.MsgID
	Args    []any
	Message string
}

// String — каноническая строка для вывода: позиция и текст.
func (i Issue) String() string {
	msg := i.Message
	switch {
	case i.Record != "" && i.Line > 0:
		return domain.Msgf(domain.MsgImportIssueRecordLine, i.Record, i.Line, msg)
	case i.Record != "":
		return domain.Msgf(domain.MsgImportIssueRecord, i.Record, msg)
	case i.Line > 0:
		return domain.Msgf(domain.MsgImportIssueLine, i.Line, msg)
	case i.No > 0:
		return domain.Msgf(domain.MsgImportIssueNo, i.No, msg)
	}
	return msg
}

// Record — запись наполнения из файла: класс (по ключу секции корня либо
// строке-обёртке NDJSON) и вход сервисного слоя.
type Record struct {
	Kind  domain.Kind
	Input service.DeviceInput
	Line  int // номер строки NDJSON (0 — не определён)
	No    int // порядковый номер записи в секции документа (0 — не задан)
}

// Document — разобранный документ наполнения (jsonc/yaml): записи
// в порядке файла и число записей, отвергнутых при чтении (проблемы —
// в Issues; дерево секции catalog импортёр применяет до записей).
type Document struct {
	Records  []Record
	Rejected int
}

// Читатель файла наполнения: семантика секций и дословные тексты ошибок
// формата — внутри importer (docs/plan/01-architecture.md §2.5). Значения
// (применимость, типы, условия) проверяет движок каталога — единственная
// точка валидации; читатель проверяет только форму файла.
type reader struct {
	snap   *catalog.Snapshot
	issues []Issue
}

// KindSection — ключ секции корня для класса (формат наполнения:
// transistors, diodes, resistors, capacitors — код класса с «s»).
func KindSection(k domain.Kind) string { return string(k) + "s" }

// kindBySection — класс по ключу секции корня (снимок задаёт реестр
// классов; расширение каталога новыми классами расширяет и формат).
func kindBySection(snap *catalog.Snapshot, section string) (domain.Kind, bool) {
	if !strings.HasSuffix(section, "s") {
		return "", false
	}
	k := domain.Kind(strings.TrimSuffix(section, "s"))
	if _, ok := snap.Kind(k); !ok {
		return "", false
	}
	return k, true
}

// rootKeys — допустимые ключи корня для сообщений (catalog + секции классов).
func rootKeys(snap *catalog.Snapshot) string {
	parts := []string{"catalog"}
	for _, k := range snap.Kinds {
		parts = append(parts, KindSection(k.Code))
	}
	return strings.Join(parts, ", ")
}

// sectionKeys — имена секций групп каталога для сообщений.
func sectionKeys(snap *catalog.Snapshot) string {
	parts := make([]string, 0, len(snap.Groups))
	for _, g := range snap.Groups {
		parts = append(parts, g.SectionName)
	}
	return strings.Join(parts, ", ")
}

// systemCodes — коды систем обозначений снимка для сообщений.
func systemCodes(snap *catalog.Snapshot) string {
	parts := make([]string, 0, len(snap.Systems))
	for _, s := range snap.Systems {
		parts = append(parts, string(s.Code))
	}
	return strings.Join(parts, ", ")
}

// ReadDocument разбирает корень документа (jsonc/yaml): секция catalog
// пропускается (импортёр применяет её до записей отдельно), записи
// классов читаются в порядке следования ключей. Проблемы формы
// накапливаются в issues; записи с проблемами не попадают в Document
// (применение частичной записи запрещено).
func ReadDocument(root value, snap *catalog.Snapshot) (doc *Document, issues []Issue) {
	r := &reader{snap: snap}
	doc = &Document{}
	if root.kind != kindObject {
		r.rootFail(domain.MsgImportRootNotObject)
		return doc, r.issues
	}
	for _, m := range root.members {
		if m.name == "catalog" {
			continue
		}
		kind, ok := kindBySection(snap, m.name)
		if !ok {
			r.rootFail(domain.MsgImportRootUnknownKey, m.name, rootKeys(snap))
			continue
		}
		if m.value.kind == kindNull {
			continue
		}
		if m.value.kind != kindArray {
			r.rootFail(domain.MsgImportRootSectionArray, m.name)
			continue
		}
		for i, item := range m.value.items {
			if in, ok := r.record(kind, item, 0, i+1); ok {
				doc.Records = append(doc.Records, Record{Kind: kind, Input: in, No: i + 1})
			} else {
				doc.Rejected++
			}
		}
	}
	return doc, r.issues
}

// rootFail — проблема корневого уровня.
func (r *reader) rootFail(id domain.MsgID, args ...any) {
	r.issues = append(r.issues, issuef(0, id, args...))
}

// rootFailAt — проблема корневого уровня со строкой NDJSON.
func (r *reader) rootFailAt(line int, id domain.MsgID, args ...any) {
	r.issues = append(r.issues, issuef(line, id, args...))
}

// recFail — проблема записи: обозначение (если прочитано) и позиция.
func (r *reader) recFail(rec *Record, id domain.MsgID, args ...any) {
	iss := issuef(rec.Line, id, args...)
	iss.Record = rec.Input.Name
	iss.No = rec.No
	r.issues = append(r.issues, iss)
}

// issuef — проблема без позиционирования записи.
func issuef(line int, id domain.MsgID, args ...any) Issue {
	return Issue{
		Line: line, Code: domain.CodeInvalidImportFile,
		MsgID: id, Args: args, Message: domain.Msgf(id, args...),
	}
}

// record читает одну запись класса kind: строка-обозначение либо объект
// «name» + секции. Возвращает false, если запись отвергнута (проблемы
// уже накоплены).
func (r *reader) record(kind domain.Kind, v value, line, no int) (service.DeviceInput, bool) {
	rec := &Record{Kind: kind, Line: line, No: no}
	if v.kind == kindString {
		rec.Input = service.DeviceInput{Name: strings.TrimSpace(v.str), Kind: kind}
		if rec.Input.Name == "" {
			r.recFail(rec, domain.MsgImportRecordEmptyName)
			return service.DeviceInput{}, false
		}
		return rec.Input, true
	}
	if v.kind != kindObject {
		r.recFail(rec, domain.MsgImportRecordBadValue, describeKindArg(v))
		return service.DeviceInput{}, false
	}
	rec.Input = service.DeviceInput{Kind: kind}

	nameVal, hasName := v.has("name")
	if !hasName || nameVal.kind == kindNull {
		r.recFail(rec, domain.MsgImportRecordNameRequired)
		return service.DeviceInput{}, false
	}
	if nameVal.kind != kindString {
		r.recFail(rec, domain.MsgImportRecordNameString)
		return service.DeviceInput{}, false
	}
	rec.Input.Name = strings.TrimSpace(nameVal.str)
	if rec.Input.Name == "" {
		r.recFail(rec, domain.MsgImportRecordEmptyName)
		return service.DeviceInput{}, false
	}

	// Допустимые ключи: name, system, служебные секции и секции групп
	// каталога (имена секций — из каталога, 05-work-plan.md задача 4.6).
	serviceKeys := map[string]bool{
		"name": true, "system": true, "attributes": true,
		"manufacturers": true, "variants": true, "analogs": true,
	}
	for _, m := range v.members {
		if serviceKeys[m.name] {
			continue
		}
		if _, ok := r.snap.GroupBySection(m.name); ok {
			continue
		}
		r.recFail(rec, domain.MsgImportRecordUnknownField, m.name, sectionKeys(r.snap))
		return service.DeviceInput{}, false
	}

	if sysVal, ok := v.has("system"); ok && sysVal.kind != kindNull {
		if sysVal.kind != kindString {
			r.recFail(rec, domain.MsgImportRecordSystemString)
			return service.DeviceInput{}, false
		}
		sys := domain.System(strings.TrimSpace(sysVal.str))
		if _, known := r.snap.System(sys); !known {
			r.recFail(rec, domain.MsgImportRecordSystemUnknown, string(sys), systemCodes(r.snap))
			return service.DeviceInput{}, false
		}
		rec.Input.System = sys
	}

	if attrVal, ok := v.has("attributes"); ok && attrVal.kind != kindNull {
		attrs, ok := r.attributes(rec, attrVal)
		if !ok {
			return service.DeviceInput{}, false
		}
		rec.Input.Attributes = attrs
	}

	for _, m := range v.members {
		if serviceKeys[m.name] {
			continue
		}
		if _, ok := r.snap.GroupBySection(m.name); !ok {
			continue // уже отвергнуто проверкой ключей
		}
		if m.value.kind == kindNull {
			continue // null — секцию не менять
		}
		if m.value.kind != kindArray {
			r.recFail(rec, domain.MsgImportSectionArray, m.name)
			return service.DeviceInput{}, false
		}
		vals := make([]catalog.ParameterValue, 0, len(m.value.items))
		bad := false
		for i, item := range m.value.items {
			pv, ok := r.parameterValue(rec, m.name, i, item)
			if !ok {
				bad = true
				continue
			}
			vals = append(vals, pv)
		}
		if bad {
			return service.DeviceInput{}, false
		}
		rec.Input.Sections = append(rec.Input.Sections, service.SectionInput{
			Section: m.name, Values: vals,
		})
	}

	if mfrVal, ok := v.has("manufacturers"); ok && mfrVal.kind != kindNull {
		if mfrVal.kind != kindArray {
			r.recFail(rec, domain.MsgImportManufacturersArray)
			return service.DeviceInput{}, false
		}
		names := make([]string, 0, len(mfrVal.items))
		for i, item := range mfrVal.items {
			if item.kind != kindString {
				r.recFail(rec, domain.MsgImportManufacturerItem, i+1)
				return service.DeviceInput{}, false
			}
			names = append(names, strings.TrimSpace(item.str))
		}
		rec.Input.Manufacturers = &names
	}

	if varVal, ok := v.has("variants"); ok && varVal.kind != kindNull {
		if varVal.kind != kindArray {
			r.recFail(rec, domain.MsgImportVariantsArray)
			return service.DeviceInput{}, false
		}
		variants := make([]service.VariantInput, 0, len(varVal.items))
		for i, item := range varVal.items {
			vi, ok := r.variant(rec, i, item)
			if !ok {
				return service.DeviceInput{}, false
			}
			variants = append(variants, vi)
		}
		rec.Input.Variants = &variants
	}

	if anaVal, ok := v.has("analogs"); ok && anaVal.kind != kindNull {
		if anaVal.kind != kindArray {
			r.recFail(rec, domain.MsgImportAnalogsArray)
			return service.DeviceInput{}, false
		}
		analogs := make([]service.AnalogInput, 0, len(anaVal.items))
		for i, item := range anaVal.items {
			a, ok := r.analog(rec, i, item)
			if !ok {
				return service.DeviceInput{}, false
			}
			analogs = append(analogs, a)
		}
		rec.Input.Analogs = &analogs
	}
	return rec.Input, true
}

// attributes читает секцию attributes: объект «код атрибута → значение».
func (r *reader) attributes(rec *Record, v value) ([]catalog.AttributeValue, bool) {
	if v.kind != kindObject {
		r.recFail(rec, domain.MsgImportAttributesObject)
		return nil, false
	}
	attrs := make([]catalog.AttributeValue, 0, len(v.members))
	for _, m := range v.members {
		switch m.value.kind {
		case kindNull:
			continue // null — атрибут не задаётся
		case kindString:
			s := strings.TrimSpace(m.value.str)
			attrs = append(attrs, catalog.AttributeValue{Attribute: m.name, Text: &s})
		case kindNumber:
			f, err := strconv.ParseFloat(m.value.num, 64)
			if err != nil {
				r.recFail(rec, domain.MsgImportAttrNumber, m.name)
				return nil, false
			}
			attrs = append(attrs, catalog.AttributeValue{Attribute: m.name, Num: &f})
		case kindBool:
			b := m.value.boolean
			attrs = append(attrs, catalog.AttributeValue{Attribute: m.name, Bool: &b})
		default:
			r.recFail(rec, domain.MsgImportAttrBadValue, m.name)
			return nil, false
		}
	}
	return attrs, true
}

// parameterValue читает объект значения секции группы: {"parameter": код,
// value|min/max|text, условия — соседние ключи по коду}. Коды параметров
// и условий проверяет движок каталога; читатель — только форму ключей.
func (r *reader) parameterValue(rec *Record, section any, index int, v value) (catalog.ParameterValue, bool) {
	where := domain.MsgArg(domain.MsgImportWhereSection, section, index+1)
	if v.kind != kindObject {
		r.recFail(rec, domain.MsgImportValueObject, where)
		return catalog.ParameterValue{}, false
	}
	pv := catalog.ParameterValue{}
	known := map[string]bool{"parameter": true, "value": true, "min": true, "max": true, "text": true}
	for _, m := range v.members {
		if m.name == "parameter" {
			if m.value.kind != kindString {
				r.recFail(rec, domain.MsgImportValueParameterString, where)
				return catalog.ParameterValue{}, false
			}
			pv.Parameter = strings.TrimSpace(m.value.str)
			continue
		}
		if known[m.name] {
			if f, ok := r.numberKey(rec, where, m); ok {
				switch m.name {
				case "value":
					pv.Exact = &f
				case "min":
					pv.Min = &f
				case "max":
					pv.Max = &f
				}
				continue
			}
			if m.name == "text" && m.value.kind == kindString {
				s := strings.TrimSpace(m.value.str)
				pv.Text = &s
				continue
			}
			if m.name == "text" {
				r.recFail(rec, domain.MsgImportValueTextString, where)
			} else {
				r.recFail(rec, domain.MsgImportValueKeyNumber, where, m.name)
			}
			return catalog.ParameterValue{}, false
		}
		// Прочие ключи — коды условий измерения (число); известность кода
		// и комбинацию условий проверяет движок.
		f, ok := r.numberKey(rec, where, m)
		if !ok {
			r.recFail(rec, domain.MsgImportValueUnknownKey, where, m.name)
			return catalog.ParameterValue{}, false
		}
		pv.Conditions = append(pv.Conditions, catalog.ConditionValue{Condition: m.name, Value: f})
	}
	if pv.Parameter == "" {
		r.recFail(rec, domain.MsgImportValueParameterRequired, where)
		return catalog.ParameterValue{}, false
	}
	return pv, true
}

// numberKey — числовое значение ключа (целые и дробные, текст числа из
// формата без преобразований точности).
func (r *reader) numberKey(rec *Record, where any, m member) (float64, bool) {
	if m.value.kind != kindNumber {
		return 0, false
	}
	f, err := strconv.ParseFloat(m.value.num, 64)
	if err != nil {
		r.recFail(rec, domain.MsgImportValueKeyNumber, where, m.name)
		return 0, false
	}
	return f, true
}

// variant читает исполнение: метка + секции групп (те же, что у записи).
func (r *reader) variant(rec *Record, index int, v value) (service.VariantInput, bool) {
	where := domain.MsgArg(domain.MsgImportWhereVariant, index+1)
	if v.kind != kindObject {
		r.recFail(rec, domain.MsgImportValueObject, where)
		return service.VariantInput{}, false
	}
	vi := service.VariantInput{}
	for _, m := range v.members {
		if m.name == "label" {
			if m.value.kind != kindString {
				r.recFail(rec, domain.MsgImportVariantLabelString, where)
				return service.VariantInput{}, false
			}
			vi.Label = strings.TrimSpace(m.value.str)
			continue
		}
		if _, ok := r.snap.GroupBySection(m.name); !ok {
			r.recFail(rec, domain.MsgImportVariantUnknownField, where, m.name, sectionKeys(r.snap))
			return service.VariantInput{}, false
		}
	}
	for _, m := range v.members {
		if m.name == "label" {
			continue
		}
		if m.value.kind == kindNull {
			continue
		}
		if m.value.kind != kindArray {
			r.recFail(rec, domain.MsgImportVariantSectionArray, where, m.name)
			return service.VariantInput{}, false
		}
		vals := make([]catalog.ParameterValue, 0, len(m.value.items))
		for i, item := range m.value.items {
			pv, ok := r.parameterValue(rec, domain.MsgArg(domain.MsgImportWhereValue, where, i+1), i, item)
			if !ok {
				return service.VariantInput{}, false
			}
			vals = append(vals, pv)
		}
		vi.Sections = append(vi.Sections, service.SectionInput{Section: m.name, Values: vals})
	}
	return vi, true
}

// analog читает ссылку-аналог: строка-обозначение либо объект
// {name, note}.
func (r *reader) analog(rec *Record, index int, v value) (service.AnalogInput, bool) {
	where := domain.MsgArg(domain.MsgImportWhereAnalog, index+1)
	switch v.kind {
	case kindString:
		return service.AnalogInput{Designation: strings.TrimSpace(v.str)}, true
	case kindObject:
		a := service.AnalogInput{}
		nameVal, hasName := v.has("name")
		if !hasName || nameVal.kind != kindString {
			r.recFail(rec, domain.MsgImportAnalogNameRequired, where)
			return service.AnalogInput{}, false
		}
		a.Designation = strings.TrimSpace(nameVal.str)
		for _, m := range v.members {
			if m.name == "name" {
				continue
			}
			if m.name == "note" {
				if m.value.kind != kindString {
					r.recFail(rec, domain.MsgImportAnalogNoteString, where)
					return service.AnalogInput{}, false
				}
				a.Note = strings.TrimSpace(m.value.str)
				continue
			}
			r.recFail(rec, domain.MsgImportAnalogUnknownField, where, m.name)
			return service.AnalogInput{}, false
		}
		return a, true
	}
	r.recFail(rec, domain.MsgImportAnalogItem, where)
	return service.AnalogInput{}, false
}

// ReadRecordLine читает строку-обёртку NDJSON {"<класс>": <запись>} —
// одна JSON-строка, одна сущность: ровно один ключ-класс.
func ReadRecordLine(snap *catalog.Snapshot, v value, line int) (Record, bool, []Issue) {
	r := &reader{snap: snap}
	if v.kind != kindObject {
		r.rootFailAt(line, domain.MsgImportNdjsonLineObject)
		return Record{}, false, r.issues
	}
	if len(v.members) != 1 {
		r.rootFailAt(line, domain.MsgImportNdjsonWrapperSingle)
		return Record{}, false, r.issues
	}
	m := v.members[0]
	if m.name == "catalog" {
		r.rootFailAt(line, domain.MsgImportNdjsonCatalogOrder)
		return Record{}, false, r.issues
	}
	kind, ok := kindBySection(snap, m.name)
	if !ok {
		r.rootFailAt(line, domain.MsgImportNdjsonClassKey,
			m.name, strings.TrimPrefix(rootKeys(snap), "catalog, "))
		return Record{}, false, r.issues
	}
	in, ok := r.record(kind, m.value, line, 0)
	if !ok {
		return Record{}, false, r.issues
	}
	return Record{Kind: kind, Input: in, Line: line}, true, r.issues
}
