package importer

import (
	"strings"

	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Секция sources (план 03 §13, D13): корневая — определения источников
// (импортёр применяет их до записей, upsert по коду), запись — ссылки
// на коды реестра с опциональной заметкой места («таблица 5.3», «с.
// 214»). Здесь только форма: типы ключей и известность полей;
// значения (charset кода, словарь типов, границы года, повторы)
// проверяет сервисный слой — единая точка валидации.

// sourceDefKeys — поля определения источника (для сообщений).
const sourceDefKeys = "code, title, type, authors, editors, year, publisher, edition, isbn, urls"

// ReadSourcesSection читает корневую секцию sources — массив
// определений источников. Значение null/отсутствие — пустой вход;
// строки NDJSON ({"sources": […]}) читаются тем же читателем и
// сливаются конкатенацией массивов (importNDJSON).
func ReadSourcesSection(v value) ([]service.SourceInput, []Issue) {
	r := &reader{}
	if v.kind == kindNull {
		return nil, nil
	}
	if v.kind != kindArray {
		r.rootFail(domain.MsgImportSourcesArray)
		return nil, r.issues
	}
	out := make([]service.SourceInput, 0, len(v.items))
	for i, item := range v.items {
		def, ok := readSourceDef(r, i+1, item)
		if !ok {
			return nil, r.issues
		}
		out = append(out, def)
	}
	return out, r.issues
}

// readSourceDef — одно определение источника: объект с полями
// code/title/type (строки), year (целое), publisher/edition/isbn
// (строки), authors/editors/urls (массивы строк); прочие ключи —
// ошибка, незаданные поля — не заданы.
func readSourceDef(r *reader, no int, v value) (service.SourceInput, bool) {
	if v.kind != kindObject {
		r.rootFail(domain.MsgImportSourcesObject, no)
		return service.SourceInput{}, false
	}
	var def service.SourceInput
	where := domain.MsgArg(domain.MsgImportWhereSource, no)
	for _, m := range v.members {
		switch m.name {
		case "code", "title", "type", "publisher", "edition", "isbn":
			if m.value.kind == kindNull {
				continue
			}
			if m.value.kind != kindString {
				r.rootFail(domain.MsgImportSourcesFieldString, where, m.name)
				return service.SourceInput{}, false
			}
			s := strings.TrimSpace(m.value.str)
			switch m.name {
			case "code":
				def.Code = s
			case "title":
				def.Title = s
			case "type":
				def.Type = s
			case "publisher":
				def.Publisher = s
			case "edition":
				def.Edition = s
			case "isbn":
				// Канонизация: пробелы и дефисы не несут данных,
				// ISBN хранится и сравнивается нормализованным.
				def.ISBN = canonicalISBN(s)
			}
		case "authors", "editors", "urls":
			if m.value.kind == kindNull {
				continue
			}
			if m.value.kind != kindArray {
				r.rootFail(domain.MsgImportSourcesFieldStrings, where, m.name)
				return service.SourceInput{}, false
			}
			list := make([]string, 0, len(m.value.items))
			for _, item := range m.value.items {
				if item.kind != kindString {
					r.rootFail(domain.MsgImportSourcesItemString, where, m.name)
					return service.SourceInput{}, false
				}
				list = append(list, strings.TrimSpace(item.str))
			}
			switch m.name {
			case "authors":
				def.Authors = list
			case "editors":
				def.Editors = list
			case "urls":
				def.URLs = list
			}
		case "year":
			if m.value.kind == kindNull {
				continue
			}
			if m.value.kind != kindNumber {
				r.rootFail(domain.MsgImportSourcesFieldInteger, where)
				return service.SourceInput{}, false
			}
			y, ok := intFromNumber(m.value.num)
			if !ok {
				r.rootFail(domain.MsgImportSourcesFieldInteger, where)
				return service.SourceInput{}, false
			}
			def.Year = &y
		default:
			r.rootFail(domain.MsgImportSourcesUnknownField, where, m.name, sourceDefKeys)
			return service.SourceInput{}, false
		}
	}
	if def.Code == "" {
		r.rootFail(domain.MsgImportSourcesFieldRequired, where, "code")
		return service.SourceInput{}, false
	}
	if def.Title == "" {
		r.rootFail(domain.MsgImportSourcesFieldRequired, where, "title")
		return service.SourceInput{}, false
	}
	if def.Type == "" {
		r.rootFail(domain.MsgImportSourcesFieldRequired, where, "type")
		return service.SourceInput{}, false
	}
	return def, true
}

// canonicalISBN убирает пробелы и дефисы ISBN (чек-сумма не проверяется
// — форматы изданий различаются; нормализация даёт стабильное
// сравнение round-trip).
func canonicalISBN(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '-':
			return -1
		}
		return r
	}, s)
}

// recordSources читает секцию sources записи: массив строк-кодов
// либо объектов {code, note}.
func (r *reader) recordSources(rec *Record, v value) ([]service.SourceRefInput, bool) {
	if v.kind != kindArray {
		r.recFail(rec, domain.MsgImportRecordSourcesArray)
		return nil, false
	}
	out := make([]service.SourceRefInput, 0, len(v.items))
	for i, item := range v.items {
		where := domain.MsgArg(domain.MsgImportWhereSource, i+1)
		switch item.kind {
		case kindString:
			code := strings.TrimSpace(item.str)
			if code == "" {
				r.recFail(rec, domain.MsgImportRecordSourceCodeRequired, where)
				return nil, false
			}
			out = append(out, service.SourceRefInput{Code: code})
		case kindObject:
			var ref service.SourceRefInput
			for _, m := range item.members {
				switch m.name {
				case "code":
					if m.value.kind != kindString {
						r.recFail(rec, domain.MsgImportRecordSourceCodeRequired, where)
						return nil, false
					}
					ref.Code = strings.TrimSpace(m.value.str)
				case "note":
					if m.value.kind == kindNull {
						continue
					}
					if m.value.kind != kindString {
						r.recFail(rec, domain.MsgImportRecordSourceNoteString, where)
						return nil, false
					}
					ref.Note = strings.TrimSpace(m.value.str)
				default:
					r.recFail(rec, domain.MsgImportRecordSourceUnknownField, where, m.name)
					return nil, false
				}
			}
			if ref.Code == "" {
				r.recFail(rec, domain.MsgImportRecordSourceCodeRequired, where)
				return nil, false
			}
			out = append(out, ref)
		default:
			r.recFail(rec, domain.MsgImportRecordSourceItem, where)
			return nil, false
		}
	}
	return out, true
}
