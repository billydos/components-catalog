package httpapi

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
	"github.com/billydos/components-catalog/internal/service"
)

// Разбор параметров поиска GET /api/v1/components
// (docs/plan/04-module-functionality.md §2): kind, system, q, поля обозначения
// (коды полей разбора — реестр домена), attr.<код>, par.<код>(.min/.max/
// .exact), sort, limit, offset. Неизвестный ключ — ошибка (опечатки
// не молчат).

// queryLimit — разбор limit с потолком (поиск 200, suggest 50).
func queryLimit(values []string, max int) (int, bool, error) {
	if len(values) == 0 {
		return 0, false, nil
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < 1 || n > max {
		return 0, true, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgApiLimitRange, max)
	}
	return n, true, nil
}

// parseSearchQuery разбирает параметры поиска в запрос сервисного слоя;
// применимость фильтров к классу и типы значений проверяет сервис
// (единственная точка валидации — тексты ошибок не дублируются).
func parseSearchQuery(r *http.Request, snap *catalog.Snapshot) (service.SearchQuery, error) {
	values := r.URL.Query()
	q := service.SearchQuery{
		Kind:   domain.Kind(first(values, "kind")),
		System: domain.System(first(values, "system")),
		Query:  first(values, "q"),
	}
	if v, exists, err := queryLimit(values["limit"], service.SearchLimitMax); err != nil {
		return q, err
	} else if exists {
		q.Limit = v
	}
	if vs := values["offset"]; len(vs) > 0 {
		n, err := strconv.Atoi(vs[0])
		if err != nil || n < 0 {
			return q, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgApiOffsetNonNeg)
		}
		q.Offset = n
	}
	if vs := values["sort"]; len(vs) > 0 && strings.TrimSpace(vs[0]) != "" {
		sorts, err := parseSort(vs[0])
		if err != nil {
			return q, err
		}
		q.Sort = sorts
	}

	// Остальные ключи — фильтры; обход по отсортированным именам —
	// детерминированные сообщения об ошибках.
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		v := values[name][0]
		numeric, known := domain.NumericDesignationField(name)
		switch {
		case reservedQueryParam(name):
			// разобраны выше
		case known && !numeric:
			if name == "material" {
				code, okCode := i18n.MaterialCode(v)
				if !okCode {
					return q, domain.NewErrorf(domain.CodeValidationFailed,
						domain.MsgUnknownMaterial, v)
				}
				v = code
			}
			q.Fields = append(q.Fields, service.FieldFilter{Field: name, Text: v})
		case known:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return q, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgApiNumberParam, name)
			}
			q.Fields = append(q.Fields, service.FieldFilter{
				Field: name, Num: f, HasNum: true, Op: service.OpEq,
			})
		case strings.HasPrefix(name, "attr."):
			f, err := parseAttrFilter(snap, strings.TrimPrefix(name, "attr."), v)
			if err != nil {
				return q, err
			}
			q.Attributes = append(q.Attributes, f)
		case strings.HasPrefix(name, "par."):
			f, err := parseParamFilter(strings.TrimPrefix(name, "par."), v)
			if err != nil {
				return q, err
			}
			q.Parameters = append(q.Parameters, f)
		default:
			return q, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgApiQueryParamUnknown, name)
		}
	}
	return q, nil
}

// reservedQueryParam — служебные параметры поиска.
func reservedQueryParam(name string) bool {
	switch name {
	case "kind", "system", "q", "sort", "limit", "offset":
		return true
	}
	return false
}

// parseSort — сортировка: ключи через запятую, «-» — убывание;
// «designation»/«id» либо код поля разбора.
func parseSort(spec string) ([]service.SortField, error) {
	parts := strings.Split(spec, ",")
	out := make([]service.SortField, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part)
		desc := false
		if strings.HasPrefix(key, "-") {
			desc, key = true, key[1:]
		}
		switch {
		case key == "designation":
			out = append(out, service.SortField{Key: key, Desc: desc})
		case key == "id":
			out = append(out, service.SortField{Key: key, Numeric: true, Desc: desc})
		case domain.KnownDesignationField(key):
			numeric, _ := domain.NumericDesignationField(key)
			out = append(out, service.SortField{Key: key, Numeric: numeric, Desc: desc})
		default:
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgApiSortKey, key)
		}
	}
	return out, nil
}

// parseAttrFilter — фильтр атрибута: значение разбирается по типу
// атрибута каталога (текст/число; логические атрибуты в фильтрах не
// поддерживаются хранилищем). Согласованность типа с каталогом и
// применимость к классу проверяет сервис.
func parseAttrFilter(snap *catalog.Snapshot, code, value string) (service.AttributeFilter, error) {
	f := service.AttributeFilter{Attribute: code}
	if def, ok := snap.Attribute(code); ok && def.Type == catalog.AttrBool {
		return f, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgApiBoolAttrFilter, code)
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		f.Num, f.HasNum = n, true
		return f, nil
	}
	f.Text = value
	return f, nil
}

// parseParamFilter — параметрический фильтр: «<код>.min|.max|.exact» —
// границы, «<код>.text» либо «<код>» — текст/enum.
func parseParamFilter(name, value string) (service.ParameterFilter, error) {
	f := service.ParameterFilter{Parameter: name}
	if code, op, split := strings.Cut(name, "."); split {
		switch op {
		case "min", "max", "exact":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return f, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgApiNumberParam, "par."+name)
			}
			f.Parameter = code
			switch op {
			case "min":
				f.Min = &n
			case "max":
				f.Max = &n
			default:
				f.Exact = &n
			}
			return f, nil
		case "text":
			f.Parameter = code
			f.Text = value
			return f, nil
		default:
			// Не операция фильтра — код параметра с точкой (невалиден).
			return f, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgApiQueryParamUnknown, "par."+name)
		}
	}
	f.Text = value
	return f, nil
}

// first — первое значение параметра (пустое — не задан).
func first(values map[string][]string, name string) string {
	if vs := values[name]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}
