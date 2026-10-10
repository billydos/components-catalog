package service

import (
	"math"
	"sort"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// Явные поля записи (секция fields формата наполнения;
// docs/plan/03-data-model.md §2.5). Валидация — после разбора обозначения
// (известен класс и система записи): допустимы только классификационные
// поля, применимые к классу и не устанавливаемые парсером системы самосто-
// ятельно; для записей other (разбора нет) — дополнительно грамматические
// поля series/dev_number/letters (бывшие продукты системы series, D10).
// Значения — коды словарей (материалы/подклассы/подстройки — domain;
// категории — каталожный словарь снимка). Возвращает нормализованный
// набор (отсортирован по имени поля — каноническое сравнение состояний
// и round-trip экспорт).

// validateExplicitFields проверяет и нормализует набор явных полей.
func validateExplicitFields(p domain.ParsedDesignation, fields []domain.Field,
	snap *catalog.Snapshot) ([]domain.Field, error) {
	seen := make(map[string]bool, len(fields))
	out := make([]domain.Field, 0, len(fields))
	for _, f := range fields {
		otherField := p.System == domain.SystemOther && domain.OtherFieldKnown(f.Name)
		if !classificationField(f.Name) && !otherField {
			// Известное грамматическое поле, устанавливаемое парсером
			// системы записи, точнее диагностируется как продукт разбора.
			if domain.KnownDesignationField(f.Name) && domain.ParserFieldKnown(p.System, p.Kind, f.Name) {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgSvcFieldParserOwned, f.Name, string(p.System))
			}
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcFieldUnknown, f.Name)
		}
		if seen[f.Name] {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcFieldDuplicate, f.Name)
		}
		seen[f.Name] = true
		if !otherField && !domain.ClassificationFieldAppliesTo(f.Name, p.Kind) {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcFieldNotApplicable, f.Name, string(p.Kind))
		}
		if domain.ParserFieldKnown(p.System, p.Kind, f.Name) {
			return nil, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcFieldParserOwned, f.Name, string(p.System))
		}
		if otherField {
			norm, err := normalizeOtherField(f)
			if err != nil {
				return nil, err
			}
			out = append(out, norm)
			continue
		}
		switch f.Name {
		case "material":
			if _, ok := domain.MaterialByCode(f.Text); !ok {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgUnknownMaterial, f.Text)
			}
		case "subclass":
			if _, ok := domain.SubclassByCode(f.Text); !ok {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgUnknownSubclass, f.Text,
					strings.Join(domain.SubclassCodes(), ", "))
			}
		case "adjustment":
			if _, ok := domain.AdjustmentByCode(f.Text); !ok {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgUnknownAdjust, f.Text,
					strings.Join(domain.AdjustmentCodes(), ", "))
			}
		case "category":
			if _, ok := snap.Category(f.Text); !ok {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgUnknownCategory, f.Text,
					strings.Join(snap.CategoryCodes(), ", "))
			}
		case "assembly":
			if !f.IsNum || (f.Num != 0 && f.Num != 1) {
				return nil, domain.NewErrorf(domain.CodeValidationFailed,
					domain.MsgSvcFieldAssemblyRange)
			}
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// normalizeOtherField проверяет и канонизирует грамматическое поле
// записи other: series/letters — непустой текст в верхнем регистре
// (каноническая форма, как у продуктов разбора), dev_number —
// положительное целое.
func normalizeOtherField(f domain.Field) (domain.Field, error) {
	switch f.Name {
	case "series", "letters":
		s := strings.ToUpper(strings.TrimSpace(f.Text))
		if f.IsNum || s == "" {
			return domain.Field{}, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcFieldTextExpected, f.Name)
		}
		return domain.TextField(f.Name, s), nil
	case "dev_number":
		if !f.IsNum || f.Num <= 0 || f.Num != math.Trunc(f.Num) {
			return domain.Field{}, domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgSvcFieldDevNumber)
		}
		return domain.NumField(f.Name, f.Num), nil
	}
	return f, nil
}

// classificationField сообщает, входит ли имя в реестр классификационных
// полей (domain.ClassificationFieldCodes).
func classificationField(name string) bool {
	for _, code := range domain.ClassificationFieldCodes {
		if name == code {
			return true
		}
	}
	return false
}

// explicitFieldsOf вычисляет явные классификационные поля из хранимых:
// поля записи минус продукты разбора обозначения (импортёр запрещает
// совпадения, разность всегда определена; порядок — хранимый, поле
// ORDER BY field).
func explicitFieldsOf(stored, parsed []domain.Field) []domain.Field {
	var out []domain.Field
	for _, f := range stored {
		parsedAlready := false
		for _, pf := range parsed {
			if pf.Name == f.Name {
				parsedAlready = true
				break
			}
		}
		if parsedAlready {
			continue
		}
		out = append(out, f)
	}
	return out
}

// fieldNamesUnion — имена полей двух наборов без повторов (список удаления
// при замене явных полей).
func fieldNamesUnion(a, b []domain.Field) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, f := range a {
		if !seen[f.Name] {
			seen[f.Name] = true
			out = append(out, f.Name)
		}
	}
	for _, f := range b {
		if !seen[f.Name] {
			seen[f.Name] = true
			out = append(out, f.Name)
		}
	}
	return out
}
