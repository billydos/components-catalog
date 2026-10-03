package i18n

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Language — язык отображаемых строк (D9). Канонический язык модуля — en.
type Language string

// Реестр языков бандлов: en — канонический (полнота обязательна), ru —
// полная локаль. Расширение — новым кодом здесь и бандлом.
const (
	En Language = "en"
	Ru Language = "ru"
)

// languageOrder — стабильный порядок реестра.
var languageOrder = []Language{En, Ru}

// bundles — реестр бандлов по языкам.
var bundles = map[Language]map[string]string{
	En: bundleEn,
	Ru: bundleRu,
}

// Languages возвращает языки бандлов в стабильном порядке.
func Languages() []Language {
	return slices.Clone(languageOrder)
}

// IsValid сообщает, есть ли бандл для языка.
func (l Language) IsValid() bool {
	return slices.Contains(languageOrder, l)
}

// String возвращает код языка.
func (l Language) String() string {
	return string(l)
}

// ParseLanguage разбирает тег языка: точное совпадение кода либо базовый
// подтег («en-US» → en); регистр не важен. Второе значение — успех.
func ParseLanguage(tag string) (Language, bool) {
	base, _, _ := strings.Cut(strings.TrimSpace(tag), "-")
	switch base = strings.ToLower(base); Language(base) {
	case En:
		return En, true
	case Ru:
		return Ru, true
	}
	return "", false
}

// Negotiate выбирает язык по заголовку Accept-Language: теги с q-значениями
// по убыванию приоритета, совпадение по базовому подтегу; неподдерживаемые
// теги пропускаются, при пустом/непонятном заголовке — канонический en.
func Negotiate(header string) Language {
	type candidate struct {
		tag string
		q   float64
	}
	var list []candidate
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag, params, _ := strings.Cut(part, ";")
		c := candidate{tag: strings.TrimSpace(tag), q: 1}
		if v, ok := strings.CutPrefix(params, "q="); ok {
			if q, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				c.q = q
			}
		}
		if c.tag != "" && c.tag != "*" {
			list = append(list, c)
		}
	}
	slices.SortStableFunc(list, func(a, b candidate) int {
		switch {
		case a.q > b.q:
			return -1
		case a.q < b.q:
			return 1
		}
		return 0
	})
	for _, c := range list {
		if l, ok := ParseLanguage(c.tag); ok {
			return l
		}
	}
	return En
}

// resolve возвращает строку по ключу: запрошенный язык → en (fallback) →
// fallback (как правило сам код сущности — расширения каталога данными без
// записи в бандлах отображаются кодом).
func resolve(l Language, key, fallback string) string {
	if !l.IsValid() {
		l = En
	}
	if s, ok := bundles[l][key]; ok && s != "" {
		return s
	}
	if l != En {
		if s, ok := bundles[En][key]; ok && s != "" {
			return s
		}
	}
	return fallback
}

// HasString сообщает, есть ли непустая строка по ключу в языке (для тестов
// полноты бандлов; без fallback).
func HasString(l Language, key string) bool {
	s, ok := bundles[l][key]
	return ok && s != ""
}

// Keys перечисляет ключи бандла языка (для тестов полноты).
func Keys(l Language) []string {
	out := make([]string, 0, len(bundles[l]))
	for k := range bundles[l] {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func mustKnown(l Language) {
	if !l.IsValid() {
		panic(fmt.Sprintf("i18n: неизвестный язык %q", string(l)))
	}
}

// KindName возвращает отображаемое название класса приборов.
func KindName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "kind."+code, code)
}

// SystemName возвращает короткое имя системы обозначений.
func SystemName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "system."+code, code)
}

// SystemDescription возвращает пояснение системы обозначений.
func SystemDescription(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "system."+code+".description", "")
}

// FamilyName возвращает расшифровку семейства системы series.
func FamilyName(l Language, series string) string {
	mustKnown(l)
	return resolve(l, "family."+series, series)
}

// UnitName возвращает отображаемое название единицы.
func UnitName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "unit."+code+".name", code)
}

// UnitSymbol возвращает символ единицы для вывода значений.
func UnitSymbol(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "unit."+code+".symbol", code)
}

// ConditionName возвращает отображаемое название условия измерения.
func ConditionName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "condition."+code, code)
}

// GroupName возвращает отображаемое название группы параметров.
func GroupName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "group."+code, code)
}

// ParameterName возвращает отображаемое название параметра.
func ParameterName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "param."+code, code)
}

// AttributeName возвращает отображаемое название атрибута.
func AttributeName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "attr."+code, code)
}

// RuleDescription возвращает описание именованного правила валидации.
func RuleDescription(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "rule."+code, code)
}

// DesignationField возвращает отображаемое имя поля разбора обозначения.
func DesignationField(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "field."+code, code)
}

// MaterialName возвращает отображаемое название материала полупроводника
// по стабильному коду словаря (D9: значения словаря — коды).
func MaterialName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "material."+code, code)
}

// MaterialCode канонизирует вход значения материала: код словаря либо
// отображаемое название любой локали (кремний/silicon) → стабильный код.
// Для фильтров поиска на краю (CLI --material, REST material=).
func MaterialCode(codeOrDisplay string) (string, bool) {
	if codeOrDisplay == "" {
		return "", false
	}
	if HasString(En, "material."+codeOrDisplay) {
		return codeOrDisplay, true
	}
	for _, l := range languageOrder {
		for key, val := range bundles[l] {
			if strings.HasPrefix(key, "material.") && val == codeOrDisplay {
				return strings.TrimPrefix(key, "material."), true
			}
		}
	}
	return "", false
}

// SubclassName возвращает отображаемое название подкласса прибора
// по стабильному коду словаря (D9: значения словаря — коды).
func SubclassName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "subclass."+code, code)
}

// SubclassCode канонизирует вход значения подкласса: код словаря либо
// отображаемое название любой локали → стабильный код. Для фильтров
// поиска на краю (CLI --subclass, REST subclass=).
func SubclassCode(codeOrDisplay string) (string, bool) {
	if codeOrDisplay == "" {
		return "", false
	}
	if HasString(En, "subclass."+codeOrDisplay) {
		return codeOrDisplay, true
	}
	for _, l := range languageOrder {
		for key, val := range bundles[l] {
			if strings.HasPrefix(key, "subclass.") && val == codeOrDisplay {
				return strings.TrimPrefix(key, "subclass."), true
			}
		}
	}
	return "", false
}

// AdjustmentName возвращает отображаемое название способа подстройки
// по стабильному коду словаря (D9: значения словаря — коды).
func AdjustmentName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "adjustment."+code, code)
}

// AdjustmentCode канонизирует вход значения подстройки: код словаря либо
// отображаемое название любой локали → стабильный код. Для фильтров
// поиска на краю (CLI --adjustment, REST adjustment=).
func AdjustmentCode(codeOrDisplay string) (string, bool) {
	if codeOrDisplay == "" {
		return "", false
	}
	if HasString(En, "adjustment."+codeOrDisplay) {
		return codeOrDisplay, true
	}
	for _, l := range languageOrder {
		for key, val := range bundles[l] {
			if strings.HasPrefix(key, "adjustment.") && val == codeOrDisplay {
				return strings.TrimPrefix(key, "adjustment."), true
			}
		}
	}
	return "", false
}

// CategoryName возвращает отображаемое название категории по коду
// каталожного словаря; расширения каталога данными без записи в бандле
// отображаются кодом (D9).
func CategoryName(l Language, code string) string {
	mustKnown(l)
	return resolve(l, "category."+code, code)
}

// CategoryCode канонизирует вход значения категории по кодам словаря
// каталога (данные, не бандлы): точный код либо отображаемое название
// любой локали бандлов → код. Для фильтров поиска на краю (CLI --category,
// REST category=).
func CategoryCode(codeOrDisplay string, codes []string) (string, bool) {
	if codeOrDisplay == "" {
		return "", false
	}
	for _, code := range codes {
		if code == codeOrDisplay {
			return code, true
		}
	}
	for _, l := range languageOrder {
		for key, val := range bundles[l] {
			if strings.HasPrefix(key, "category.") && val == codeOrDisplay {
				code := strings.TrimPrefix(key, "category.")
				for _, known := range codes {
					if known == code {
						return code, true
					}
				}
			}
		}
	}
	return "", false
}
