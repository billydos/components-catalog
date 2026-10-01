package importer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Issue — проблема прогона импорта (Issues-модель —
// plan/04-module-functionality.md §4): корневые проблемы и проблемы
// записей накапливаются за один прогон; жёсткой ошибкой (возвращаемой
// как error) остаётся только синтаксис формата. Message — дословный
// текст проблемы; String добавляет позицию. Тексты — контракт CLI.
type Issue struct {
	Record  string // обозначение записи («» — проблема корневого уровня)
	Line    int    // номер строки NDJSON (0 — не определён)
	Label   string // позиция записи в файле («запись №3»), если имя не прочитано
	Code    domain.Code
	Message string
}

// String — строка для вывода: позиция и текст.
func (i Issue) String() string {
	switch {
	case i.Record != "" && i.Line > 0:
		return fmt.Sprintf("запись «%s» (строка %d): %s", i.Record, i.Line, i.Message)
	case i.Record != "":
		return fmt.Sprintf("запись «%s»: %s", i.Record, i.Message)
	case i.Line > 0:
		return fmt.Sprintf("строка %d: %s", i.Line, i.Message)
	case i.Label != "":
		return i.Label + ": " + i.Message
	}
	return i.Message
}

// Record — запись наполнения из файла: класс (по ключу секции корня либо
// строке-обёртке NDJSON) и вход сервисного слоя.
type Record struct {
	Kind  domain.Kind
	Input service.DeviceInput
	Line  int    // номер строки NDJSON (0 — не определён)
	Label string // позиция записи в файле для сообщений («запись №3»)
}

// Document — разобранный документ наполнения (jsonc/yaml): записи
// в порядке файла и число записей, отвергнутых при чтении (проблемы —
// в Issues; дерево секции catalog импортёр применяет до записей).
type Document struct {
	Records  []Record
	Rejected int
}

// Читатель файла наполнения: семантика секций и дословные тексты ошибок
// формата — внутри importer (plan/01-architecture.md §2.5). Значения
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
		r.rootFail("корневой элемент должен быть объектом с ключами-классами и/или \"catalog\"")
		return doc, r.issues
	}
	for _, m := range root.members {
		if m.name == "catalog" {
			continue
		}
		kind, ok := kindBySection(snap, m.name)
		if !ok {
			r.rootFail(fmt.Sprintf(
				"неизвестный ключ корня «%s» (допустимы: %s)", m.name, rootKeys(snap)))
			continue
		}
		if m.value.kind == kindNull {
			continue
		}
		if m.value.kind != kindArray {
			r.rootFail(fmt.Sprintf("секция «%s» должна быть массивом записей", m.name))
			continue
		}
		for i, item := range m.value.items {
			label := fmt.Sprintf("запись №%d", i+1)
			if in, ok := r.record(kind, item, 0, label); ok {
				doc.Records = append(doc.Records, Record{Kind: kind, Input: in, Label: label})
			} else {
				doc.Rejected++
			}
		}
	}
	return doc, r.issues
}

// rootFail — проблема корневого уровня.
func (r *reader) rootFail(msg string) {
	r.issues = append(r.issues, Issue{Code: domain.CodeInvalidImportFile, Message: msg})
}

// recFail — проблема записи: label позиционирует запись, name — обозначение.
func (r *reader) recFail(rec *Record, msg string) {
	r.issues = append(r.issues, Issue{
		Record: rec.Input.Name, Line: rec.Line, Label: rec.Label,
		Code: domain.CodeInvalidImportFile, Message: msg,
	})
}

// record читает одну запись класса kind: строка-обозначение либо объект
// «name» + секции. Возвращает false, если запись отвергнута (проблемы
// уже накоплены).
func (r *reader) record(kind domain.Kind, v value, line int, label string) (service.DeviceInput, bool) {
	rec := &Record{Kind: kind, Line: line, Label: label}
	if v.kind == kindString {
		rec.Input = service.DeviceInput{Name: strings.TrimSpace(v.str), Kind: kind}
		if rec.Input.Name == "" {
			r.recFail(rec, "пустое обозначение записи")
			return service.DeviceInput{}, false
		}
		return rec.Input, true
	}
	if v.kind != kindObject {
		r.recFail(rec, "запись должна быть строкой (обозначение) или объектом, получено: "+describeKind(v))
		return service.DeviceInput{}, false
	}
	rec.Input = service.DeviceInput{Kind: kind}

	nameVal, hasName := v.has("name")
	if !hasName || nameVal.kind == kindNull {
		r.recFail(rec, "обязательный ключ \"name\" — строка с обозначением")
		return service.DeviceInput{}, false
	}
	if nameVal.kind != kindString {
		r.recFail(rec, "\"name\" должно быть строкой с обозначением")
		return service.DeviceInput{}, false
	}
	rec.Input.Name = strings.TrimSpace(nameVal.str)
	if rec.Input.Name == "" {
		r.recFail(rec, "пустое обозначение записи")
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
		r.recFail(rec, fmt.Sprintf(
			"неизвестное поле «%s» (допустимы: name, system, attributes, manufacturers, variants, analogs и секции групп: %s)",
			m.name, sectionKeys(r.snap)))
		return service.DeviceInput{}, false
	}

	if sysVal, ok := v.has("system"); ok && sysVal.kind != kindNull {
		if sysVal.kind != kindString {
			r.recFail(rec, "\"system\" должно быть строкой — код системы обозначений")
			return service.DeviceInput{}, false
		}
		sys := domain.System(strings.TrimSpace(sysVal.str))
		if _, known := r.snap.System(sys); !known {
			r.recFail(rec, fmt.Sprintf(
				"неизвестная система обозначений «%s» (допустимы: %s)", string(sys), systemCodes(r.snap)))
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
			r.recFail(rec, fmt.Sprintf("секция «%s» должна быть массивом объектов", m.name))
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
			r.recFail(rec, "\"manufacturers\" должно быть массивом строк")
			return service.DeviceInput{}, false
		}
		names := make([]string, 0, len(mfrVal.items))
		for i, item := range mfrVal.items {
			if item.kind != kindString {
				r.recFail(rec, fmt.Sprintf("производитель №%d: ожидалась непустая строка", i+1))
				return service.DeviceInput{}, false
			}
			names = append(names, strings.TrimSpace(item.str))
		}
		rec.Input.Manufacturers = &names
	}

	if varVal, ok := v.has("variants"); ok && varVal.kind != kindNull {
		if varVal.kind != kindArray {
			r.recFail(rec, "\"variants\" должно быть массивом объектов исполнений")
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
			r.recFail(rec, "\"analogs\" должно быть массивом (строки либо объекты {name, note})")
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
		r.recFail(rec, "\"attributes\" должно быть объектом «код атрибута: значение»")
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
				r.recFail(rec, fmt.Sprintf("атрибут «%s»: значение должно быть числом", m.name))
				return nil, false
			}
			attrs = append(attrs, catalog.AttributeValue{Attribute: m.name, Num: &f})
		case kindBool:
			b := m.value.boolean
			attrs = append(attrs, catalog.AttributeValue{Attribute: m.name, Bool: &b})
		default:
			r.recFail(rec, fmt.Sprintf(
				"атрибут «%s»: ожидается строка, число, логическое значение или null", m.name))
			return nil, false
		}
	}
	return attrs, true
}

// parameterValue читает объект значения секции группы: {"parameter": код,
// value|min/max|text, условия — соседние ключи по коду}. Коды параметров
// и условий проверяет движок каталога; читатель — только форму ключей.
func (r *reader) parameterValue(rec *Record, section string, index int, v value) (catalog.ParameterValue, bool) {
	where := fmt.Sprintf("секция «%s», значение №%d", section, index+1)
	if v.kind != kindObject {
		r.recFail(rec, fmt.Sprintf("%s: должно быть объектом", where))
		return catalog.ParameterValue{}, false
	}
	pv := catalog.ParameterValue{}
	known := map[string]bool{"parameter": true, "value": true, "min": true, "max": true, "text": true}
	for _, m := range v.members {
		if m.name == "parameter" {
			if m.value.kind != kindString {
				r.recFail(rec, fmt.Sprintf("%s: ключ \"parameter\" должен быть строкой — код параметра", where))
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
				r.recFail(rec, fmt.Sprintf("%s: ключ \"text\" должен быть строкой", where))
			} else {
				r.recFail(rec, fmt.Sprintf("%s: ключ \"%s\" должен быть числом", where, m.name))
			}
			return catalog.ParameterValue{}, false
		}
		// Прочие ключи — коды условий измерения (число); известность кода
		// и комбинацию условий проверяет движок.
		f, ok := r.numberKey(rec, where, m)
		if !ok {
			r.recFail(rec, fmt.Sprintf(
				"%s: ключ «%s» — неизвестное поле (допустимы: parameter, value, min, max, text и коды условий измерения)",
				where, m.name))
			return catalog.ParameterValue{}, false
		}
		pv.Conditions = append(pv.Conditions, catalog.ConditionValue{Condition: m.name, Value: f})
	}
	if pv.Parameter == "" {
		r.recFail(rec, fmt.Sprintf("%s: обязательный ключ \"parameter\" — код параметра", where))
		return catalog.ParameterValue{}, false
	}
	return pv, true
}

// numberKey — числовое значение ключа (целые и дробные, текст числа из
// формата без преобразований точности).
func (r *reader) numberKey(rec *Record, where string, m member) (float64, bool) {
	if m.value.kind != kindNumber {
		return 0, false
	}
	f, err := strconv.ParseFloat(m.value.num, 64)
	if err != nil {
		r.recFail(rec, fmt.Sprintf("%s: ключ \"%s\" должен быть числом", where, m.name))
		return 0, false
	}
	return f, true
}

// variant читает исполнение: метка + секции групп (те же, что у записи).
func (r *reader) variant(rec *Record, index int, v value) (service.VariantInput, bool) {
	where := fmt.Sprintf("исполнение №%d", index+1)
	if v.kind != kindObject {
		r.recFail(rec, fmt.Sprintf("%s: должно быть объектом", where))
		return service.VariantInput{}, false
	}
	vi := service.VariantInput{}
	for _, m := range v.members {
		if m.name == "label" {
			if m.value.kind != kindString {
				r.recFail(rec, fmt.Sprintf("%s: \"label\" должно быть строкой", where))
				return service.VariantInput{}, false
			}
			vi.Label = strings.TrimSpace(m.value.str)
			continue
		}
		if _, ok := r.snap.GroupBySection(m.name); !ok {
			r.recFail(rec, fmt.Sprintf(
				"%s: неизвестное поле «%s» (допустимы: label и секции групп: %s)",
				where, m.name, sectionKeys(r.snap)))
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
			r.recFail(rec, fmt.Sprintf("%s: секция «%s» должна быть массивом объектов", where, m.name))
			return service.VariantInput{}, false
		}
		vals := make([]catalog.ParameterValue, 0, len(m.value.items))
		for i, item := range m.value.items {
			pv, ok := r.parameterValue(rec, where+", значение №"+itoa(i+1), i, item)
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
	where := fmt.Sprintf("аналог №%d", index+1)
	switch v.kind {
	case kindString:
		return service.AnalogInput{Designation: strings.TrimSpace(v.str)}, true
	case kindObject:
		a := service.AnalogInput{}
		nameVal, hasName := v.has("name")
		if !hasName || nameVal.kind != kindString {
			r.recFail(rec, fmt.Sprintf("%s: обязательный ключ \"name\" — обозначение аналога", where))
			return service.AnalogInput{}, false
		}
		a.Designation = strings.TrimSpace(nameVal.str)
		for _, m := range v.members {
			if m.name == "name" {
				continue
			}
			if m.name == "note" {
				if m.value.kind != kindString {
					r.recFail(rec, fmt.Sprintf("%s: \"note\" должно быть строкой", where))
					return service.AnalogInput{}, false
				}
				a.Note = strings.TrimSpace(m.value.str)
				continue
			}
			r.recFail(rec, fmt.Sprintf("%s: неизвестное поле «%s» (допустимы: name, note)", where, m.name))
			return service.AnalogInput{}, false
		}
		return a, true
	}
	r.recFail(rec, fmt.Sprintf("%s: ожидалась строка либо объект с ключом \"name\"", where))
	return service.AnalogInput{}, false
}

// ReadRecordLine читает строку-обёртку NDJSON {"<класс>": <запись>} —
// одна JSON-строка, одна сущность: ровно один ключ-класс.
func ReadRecordLine(snap *catalog.Snapshot, v value, line int) (Record, bool, []Issue) {
	r := &reader{snap: snap}
	if v.kind != kindObject {
		r.rootFailAt(line, "строка должна быть объектом-обёрткой {\"<класс>\": <запись>} либо {\"catalog\": …}")
		return Record{}, false, r.issues
	}
	if len(v.members) != 1 {
		r.rootFailAt(line, "строка-обёртка должна содержать ровно один ключ — класс либо catalog")
		return Record{}, false, r.issues
	}
	m := v.members[0]
	if m.name == "catalog" {
		r.rootFailAt(line, "блок catalog должен предшествовать записям")
		return Record{}, false, r.issues
	}
	kind, ok := kindBySection(snap, m.name)
	if !ok {
		r.rootFailAt(line, fmt.Sprintf(
			"неизвестный ключ-класс «%s» (допустимы: catalog и %s)",
			m.name, strings.TrimPrefix(rootKeys(snap), "catalog, ")))
		return Record{}, false, r.issues
	}
	label := fmt.Sprintf("строка %d", line)
	in, ok := r.record(kind, m.value, line, label)
	if !ok {
		return Record{}, false, r.issues
	}
	return Record{Kind: kind, Input: in, Line: line, Label: label}, true, r.issues
}

// rootFailAt — корневая проблема с номером строки NDJSON.
func (r *reader) rootFailAt(line int, msg string) {
	r.issues = append(r.issues, Issue{Line: line, Code: domain.CodeInvalidImportFile, Message: msg})
}
