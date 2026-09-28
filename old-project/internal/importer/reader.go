package importer

import (
	"fmt"
	"strconv"
	"strings"

	"soviettransistors/internal/domain"
)

// Issue — проблема разбора файла импорта: EntryIndex = 0 — ошибка корневого
// уровня файла, иначе номер записи (с 1). Source = "" — источник не задан.
type Issue struct {
	EntryIndex  int
	Description string
	Source      string
}

// TransistorEntryData — запись справочника: обозначение + необязательные секции.
// nil-секция — «не менять»; пустая непустая-nil секция — очистить (см. Save).
type TransistorEntryData struct {
	Transistor    domain.Transistor
	Attributes    *domain.TransistorAttributes
	Manufacturers []string
	Parameters    []domain.ElectricalParameter
	Ratings       *domain.MaximumRatings
}

type ParseResult struct {
	Entries []TransistorEntryData
	Issues  []Issue
}

func (r *ParseResult) HasErrors() bool { return len(r.Issues) > 0 }

var (
	designationFields         = []string{"material", "subclass", "assembly", "feature", "number", "letters", "modification", "chip"}
	requiredDesignationFields = []string{"material", "subclass", "feature", "number", "letters"}
	detailFields              = []string{"attributes", "parameters", "ratings"}
	// extraFields — ключи jsonc секции attributes: имена полей записи со
	// строчной первой буквой; производится из domain.AttributeFieldNames —
	// единый источник (как в storage), новое поле записи попадает в список
	// автоматически.
	extraFields = buildExtraFields()
	// ratingFields — ключи jsonc секции ratings. Список синхронизирован
	// с domain.RatingFieldNames вручную (правило именования неоднородно:
	// у полей, начинающихся со слова — pulse, temp, — строчная первая буква,
	// у остальных — точные имена полей), поэтому закреплён пин-тестом
	// TestRatingFields_MatchDomain, а чтение — round-trip пин-тестом.
	ratingFields = []string{
		"UkeMax", "UkbMax", "UbeMax", "UkeoMax", "IkMax", "IbMax", "PkMax",
		"IkPulseMax", "PkPulseMax", "pulseDuration", "tempMin", "tempMax", "tempJunctionMax", "Rth",
	}

	nameFormFields        = append([]string{"name"}, detailFields...)
	designationFormFields = append(append([]string{}, designationFields...), detailFields...)
	parameterFields       = buildParameterFields()
)

func buildParameterFields() []string {
	fields := []string{"parameter", "min", "max"}
	for _, key := range domain.AllConditionKeys {
		fields = append(fields, domain.ConditionJsoncKey(key))
	}
	fields = append(fields, "temp")
	return fields
}

// buildExtraFields — ключи jsonc секции attributes из доменных имён полей:
// первая буква строчная (tu, massMax, ...), остальные без изменений.
func buildExtraFields() []string {
	names := domain.AttributeFieldNames()
	fields := make([]string, len(names))
	for i, name := range names {
		fields[i] = strings.ToLower(name[:1]) + name[1:]
	}
	return fields
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// parseRoot разбирает корень дерева значений любого формата: объект с ключом
// "transistors" — массив записей. Все проблемы структуры корня — Issues
// уровня файла (EntryIndex = 0) и накапливаются вместе: пользователь видит
// полный список за один прогон, а не чинит по одной (ANALYSIS-02 B3).
// Жёсткая ошибка (error) остаётся только у фронтендов — синтаксис формата.
func parseRoot(root value) *ParseResult {
	result := &ParseResult{}

	if root.kind != kindObject {
		result.Issues = append(result.Issues, Issue{
			Description: `корневой элемент должен быть объектом вида { "transistors": [ ... ] }`,
		})
		return result
	}
	for _, member := range root.members {
		if member.name != "transistors" {
			result.Issues = append(result.Issues, Issue{
				Description: fmt.Sprintf("неизвестный ключ корневого объекта «%s» (допустим только \"transistors\")", member.name),
			})
		}
	}
	array, ok := root.has("transistors")
	if !ok {
		result.Issues = append(result.Issues, Issue{
			Description: `отсутствует обязательный ключ "transistors"`,
		})
		return result
	}
	if array.kind != kindArray {
		result.Issues = append(result.Issues, Issue{
			Description: `"transistors" должен быть массивом`,
		})
		return result
	}

	for index, item := range array.items {
		readEntry(item, index+1, result)
	}
	return result
}

func readEntry(entry value, index int, result *ParseResult) {
	if entry.kind == kindString {
		if transistor, ok := tryReadName(entry.str, index, result); ok {
			result.Entries = append(result.Entries, TransistorEntryData{Transistor: transistor})
		}
		return
	}
	if entry.kind != kindObject {
		result.Issues = append(result.Issues, Issue{
			EntryIndex:  index,
			Description: fmt.Sprintf("запись должна быть строкой (обозначение) или объектом, получено: %s", describeKind(entry)),
		})
		return
	}

	_, hasName := entry.has("name")

	keyProblems := checkEntryKeys(entry, hasName)
	if len(keyProblems) > 0 {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: strings.Join(keyProblems, "; ")})
		return
	}

	var transistor domain.Transistor
	var ok bool
	if hasName {
		nameProperty, _ := entry.has("name")
		if nameProperty.kind != kindString {
			result.Issues = append(result.Issues, Issue{
				EntryIndex:  index,
				Description: "\"name\" должно быть строкой с обозначением транзистора",
			})
			return
		}
		if transistor, ok = tryReadName(nameProperty.str, index, result); !ok {
			return
		}
	} else {
		if transistor, ok = readDesignationFields(entry, index, result); !ok {
			return
		}
	}

	var attributes *domain.TransistorAttributes
	var manufacturers []string
	issuesBeforeSections := len(result.Issues)
	if attributesElement, present := entry.has("attributes"); present {
		attributes, manufacturers = readAttributes(attributesElement, index, result)
	}

	parameters := readParameters(entry, index, result)
	ratings := readRatings(entry, index, result)

	// ошибка в любой секции — запись не применяется вовсе: иначе в базу попадало бы
	// «голое» обозначение из заведомо ошибочного файла (а при обновлении — остальные
	// секции при отвергнутой). Секция при этом отвергается целиком (см. readParameters).
	if len(result.Issues) > issuesBeforeSections {
		return
	}

	result.Entries = append(result.Entries, TransistorEntryData{
		Transistor:    transistor,
		Attributes:    attributes,
		Manufacturers: manufacturers,
		Parameters:    parameters,
		Ratings:       ratings,
	})
}

func tryReadName(name string, index int, result *ParseResult) (domain.Transistor, bool) {
	transistor, message, ok := domain.TryParseDesignation(name)
	if ok {
		return transistor, true
	}
	result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: message, Source: name})
	return domain.Transistor{}, false
}

// checkEntryKeys — единая для обеих форм объекта-записи проверка допустимых ключей:
// форма "name" допускает "name" и секции деталей, форма явных полей — поля
// обозначения и секции деталей. Поле обозначения рядом с "name" — смешение форм.
func checkEntryKeys(entry value, hasName bool) []string {
	allowed := nameFormFields
	if !hasName {
		allowed = designationFormFields
	}
	allowedKeys := stringSet(allowed)
	designationKeys := stringSet(designationFields)
	var problems []string
	for _, member := range entry.members {
		if allowedKeys[member.name] {
			continue
		}
		if hasName && designationKeys[member.name] {
			problems = append(problems, fmt.Sprintf(
				"нельзя смешивать \"name\" и явные поля обозначения: «%s» задано вместе с \"name\" — обозначение задаётся либо строкой \"name\", либо полями (%s)",
				member.name, strings.Join(designationFields, ", ")))
		} else {
			problems = append(problems, fmt.Sprintf("неизвестное поле «%s» (допустимы: %s)", member.name, strings.Join(allowed, ", ")))
		}
	}
	return problems
}

func readDesignationFields(entry value, index int, result *ParseResult) (domain.Transistor, bool) {
	var problems []string

	for _, required := range requiredDesignationFields {
		if _, ok := entry.has(required); !ok {
			problems = append(problems, fmt.Sprintf("отсутствует обязательное поле \"%s\"", required))
		}
	}

	material := readCharField(entry, "material", domain.IsValidMaterialSymbol, "Г|1, К|2, А|3 или И|4", &problems)
	subclass := readCharField(entry, "subclass", func(c rune) bool { return c == 'Т' || c == 'П' }, "Т или П", &problems)
	assembly := readBoolField(entry, "assembly", &problems)
	feature := readIntField(entry, "feature", func(v int) bool { return v >= 1 && v <= 9 }, "цифра от 1 до 9", &problems)
	number := readIntField(entry, "number", func(v int) bool { return v >= 1 && v <= 999 }, "число от 1 до 999", &problems)
	letters := readLettersField(entry, &problems)
	modification := readOptionalIntField(entry, "modification", func(v int) bool { return v >= 1 && v <= 9 }, "цифра от 1 до 9", &problems)
	chip := readOptionalIntField(entry, "chip", func(v int) bool { return v >= 1 && v <= 6 }, "цифра от 1 до 6", &problems)

	if len(problems) > 0 {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: strings.Join(problems, "; ")})
		return domain.Transistor{}, false
	}

	return domain.Transistor{
		Material:          material,
		Subclass:          subclass,
		IsAssembly:        assembly,
		Feature:           feature,
		DevelopmentNumber: number,
		Letters:           letters,
		Modification:      modification,
		ChipVariant:       chip,
	}, true
}

func readAttributes(element value, index int, result *ParseResult) (*domain.TransistorAttributes, []string) {
	if element.kind == kindNull {
		// null — секция не меняется, равносильно отсутствию
		return nil, nil
	}
	if element.kind != kindObject {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: "\"attributes\" должно быть объектом"})
		return nil, nil
	}
	var problems []string
	extraKeys := stringSet(extraFields)
	for _, member := range element.members {
		if !extraKeys[member.name] && member.name != "manufacturers" {
			problems = append(problems, fmt.Sprintf("неизвестное поле «%s» (допустимы: %s, manufacturers)", member.name, strings.Join(extraFields, ", ")))
		}
	}

	attributes := &domain.TransistorAttributes{
		Structure:         readOptionalTrimmedString(element, "structure", &problems),
		Technology:        readOptionalTrimmedString(element, "technology", &problems),
		Package:           readOptionalTrimmedString(element, "package", &problems),
		PackageMaterial:   readOptionalTrimmedString(element, "packageMaterial", &problems),
		ColorMarking:      readOptionalTrimmedString(element, "colorMarking", &problems),
		Pinout:            readOptionalTrimmedString(element, "pinout", &problems),
		EsdSensitive:      readOptionalBool(element, "esdSensitive", &problems),
		MilitaryGrade:     readOptionalBool(element, "militaryGrade", &problems),
		RadiationHardened: readOptionalBool(element, "radiationHardened", &problems),
		Tu:                readOptionalTrimmedString(element, "tu", &problems),
		Notes:             readOptionalTrimmedString(element, "notes", &problems),
		YearFrom:          readOptionalInt(element, "yearFrom", &problems),
		YearTo:            readOptionalInt(element, "yearTo", &problems),
		MassMax:           readOptionalNumber(element, "massMax", &problems),
		DatasheetUrl:      readOptionalTrimmedString(element, "datasheetUrl", &problems),
	}

	var manufacturers []string
	if manufacturersElement, ok := element.has("manufacturers"); ok {
		if manufacturersElement.kind == kindNull {
			// null — список производителей не меняется
		} else if manufacturersElement.kind != kindArray {
			problems = append(problems, "\"manufacturers\" должно быть массивом строк")
		} else {
			manufacturers = []string{}
			for number, item := range manufacturersElement.items {
				if item.kind != kindString || strings.TrimSpace(item.str) == "" {
					problems = append(problems, fmt.Sprintf("\"manufacturers\" №%d: ожидалось непустое название производителя", number+1))
				} else {
					manufacturers = append(manufacturers, strings.TrimSpace(item.str))
				}
			}
		}
	}

	problems = append(problems, domain.ValidateTransistorAttributes(attributes)...)
	if len(problems) > 0 {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: "атрибуты: " + strings.Join(problems, "; ")})
		return nil, nil
	}
	return attributes, manufacturers
}

func readParameters(entry value, index int, result *ParseResult) []domain.ElectricalParameter {
	array, ok := entry.has("parameters")
	if !ok || array.kind == kindNull {
		// null — секция не меняется, равносильно отсутствию
		return nil
	}
	if array.kind != kindArray {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: "\"parameters\" должно быть массивом объектов"})
		return nil
	}
	parameters := []domain.ElectricalParameter{}
	hasErrors := false
	for number, item := range array.items {
		if parameter, ok := readParameterItem(item, index, number+1, result); ok {
			parameters = append(parameters, parameter)
		} else {
			hasErrors = true
		}
	}
	// при ошибках секция не применяется целиком, чтобы не затереть корректные
	// данные частичным списком
	if hasErrors {
		return nil
	}
	return parameters
}

func readParameterItem(item value, entryIndex, parameterNumber int, result *ParseResult) (domain.ElectricalParameter, bool) {
	if item.kind != kindObject {
		result.Issues = append(result.Issues, Issue{
			EntryIndex:  entryIndex,
			Description: fmt.Sprintf("параметр №%d: должен быть объектом", parameterNumber),
		})
		return domain.ElectricalParameter{}, false
	}

	var problems []string
	parameterKeys := stringSet(parameterFields)
	for _, member := range item.members {
		if !parameterKeys[member.name] {
			problems = append(problems, fmt.Sprintf("неизвестное поле «%s» (допустимы: %s)", member.name, strings.Join(parameterFields, ", ")))
		}
	}

	var kind domain.ParameterKind
	code := ""
	if kindElement, ok := item.has("parameter"); ok && kindElement.kind == kindString {
		code = strings.TrimSpace(kindElement.str)
		if _, ok := domain.TryParameterKindByCode(code); !ok {
			problems = append(problems, fmt.Sprintf("\"parameter\": неизвестный код «%s» (допустимы: %s)", code, domain.ParameterCodesList()))
			code = ""
		} else {
			kind, _ = domain.TryParameterKindByCode(code)
		}
	} else {
		problems = append(problems, fmt.Sprintf("обязательное поле \"parameter\" — код параметра (%s)", domain.ParameterCodesList()))
	}

	parameter := domain.ElectricalParameter{
		Kind:     kind,
		ValueMin: readOptionalNumber(item, "min", &problems),
		ValueMax: readOptionalNumber(item, "max", &problems),
	}
	// чтение условий генерируется из каталога: новый ConditionKey читается
	// автоматически; Temp — вне модели ключей условий (см. domain.AllConditionKeys)
	// и читается последним, чтобы порядок пунктов в сообщениях не менялся
	for _, key := range domain.AllConditionKeys {
		if value := readOptionalNumber(item, domain.ConditionJsoncKey(key), &problems); value != nil {
			parameter = domain.ConditionWithValue(parameter, key, *value)
		}
	}
	parameter.Temp = readOptionalNumber(item, "temp", &problems)

	if code == "" || len(problems) > 0 {
		result.Issues = append(result.Issues, Issue{
			EntryIndex:  entryIndex,
			Description: fmt.Sprintf("параметр №%d: %s", parameterNumber, strings.Join(problems, "; ")),
		})
		return domain.ElectricalParameter{}, false
	}

	if errors := domain.ValidateElectricalParameter(parameter); len(errors) > 0 {
		result.Issues = append(result.Issues, Issue{
			EntryIndex:  entryIndex,
			Description: fmt.Sprintf("параметр №%d (%s): %s", parameterNumber, code, strings.Join(errors, "; ")),
		})
		return domain.ElectricalParameter{}, false
	}
	return parameter, true
}

func readRatings(entry value, index int, result *ParseResult) *domain.MaximumRatings {
	element, ok := entry.has("ratings")
	if !ok || element.kind == kindNull {
		// null — секция не меняется, равносильно отсутствию
		return nil
	}
	if element.kind != kindObject {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: "\"ratings\" должно быть объектом"})
		return nil
	}
	var problems []string
	ratingKeys := stringSet(ratingFields)
	for _, member := range element.members {
		if !ratingKeys[member.name] {
			problems = append(problems, fmt.Sprintf("неизвестное поле «%s» (допустимы: %s)", member.name, strings.Join(ratingFields, ", ")))
		}
	}
	ratings := &domain.MaximumRatings{
		UkeMax:          readOptionalNumber(element, "UkeMax", &problems),
		UkbMax:          readOptionalNumber(element, "UkbMax", &problems),
		UbeMax:          readOptionalNumber(element, "UbeMax", &problems),
		UkeoMax:         readOptionalNumber(element, "UkeoMax", &problems),
		IkMax:           readOptionalNumber(element, "IkMax", &problems),
		IbMax:           readOptionalNumber(element, "IbMax", &problems),
		PkMax:           readOptionalNumber(element, "PkMax", &problems),
		IkPulseMax:      readOptionalNumber(element, "IkPulseMax", &problems),
		PkPulseMax:      readOptionalNumber(element, "PkPulseMax", &problems),
		PulseDuration:   readOptionalNumber(element, "pulseDuration", &problems),
		TempMin:         readOptionalNumber(element, "tempMin", &problems),
		TempMax:         readOptionalNumber(element, "tempMax", &problems),
		TempJunctionMax: readOptionalNumber(element, "tempJunctionMax", &problems),
		Rth:             readOptionalNumber(element, "Rth", &problems),
	}
	problems = append(problems, domain.ValidateMaximumRatings(ratings)...)
	if len(problems) > 0 {
		result.Issues = append(result.Issues, Issue{EntryIndex: index, Description: "предельные данные: " + strings.Join(problems, "; ")})
		return nil
	}
	return ratings
}

func readOptionalTrimmedString(element value, field string, problems *[]string) *string {
	value, ok := element.has(field)
	if !ok || value.kind == kindNull {
		return nil
	}
	if value.kind != kindString {
		*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть строкой", field))
		return nil
	}
	trimmed := strings.TrimSpace(value.str)
	return &trimmed
}

func readOptionalBool(element value, field string, problems *[]string) *bool {
	value, ok := element.has(field)
	if !ok || value.kind == kindNull {
		return nil
	}
	switch value.kind {
	case kindBool:
		return &value.boolean
	default:
		*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть true или false", field))
		return nil
	}
}

func readOptionalInt(element value, field string, problems *[]string) *int {
	value, ok := element.has(field)
	if !ok || value.kind == kindNull {
		return nil
	}
	if number, ok := intValue(value); ok {
		return &number
	}
	*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть целым числом", field))
	return nil
}

func readOptionalNumber(element value, field string, problems *[]string) *float64 {
	value, ok := element.has(field)
	if !ok || value.kind == kindNull {
		return nil
	}
	if value.kind == kindNumber {
		if number, err := strconv.ParseFloat(value.num, 64); err == nil {
			return &number
		}
	}
	*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть числом", field))
	return nil
}

// intValue — целое значение json-числа (текст без дробной части и экспоненты,
// в диапазоне int32), как JsonElement.TryGetInt32.
func intValue(value value) (int, bool) {
	if value.kind != kindNumber {
		return 0, false
	}
	number, err := strconv.ParseInt(value.num, 10, 32)
	if err != nil {
		return 0, false
	}
	return int(number), true
}

func readCharField(entry value, field string, isValid func(rune) bool, expected string, problems *[]string) rune {
	element, ok := entry.has(field)
	if !ok {
		return 0
	}
	var value *string
	if element.kind == kindString {
		value = &element.str
	}
	if value == nil || len([]rune(*value)) != 1 || !isValid([]rune(*value)[0]) {
		got := "не строка"
		if value != nil {
			got = *value
		}
		*problems = append(*problems, fmt.Sprintf("\"%s\": ожидалось %s, получено «%s»", field, expected, got))
		return 0
	}
	return []rune(*value)[0]
}

func readBoolField(entry value, field string, problems *[]string) bool {
	element, ok := entry.has(field)
	if !ok || element.kind == kindNull {
		return false
	}
	if element.kind == kindBool {
		return element.boolean
	}
	*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть true или false", field))
	return false
}

func readIntField(entry value, field string, isValid func(int) bool, expected string, problems *[]string) int {
	element, ok := entry.has(field)
	if !ok {
		return 0
	}
	value, ok := intValue(element)
	if !ok {
		*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть целым числом (%s)", field, expected))
		return 0
	}
	if !isValid(value) {
		*problems = append(*problems, fmt.Sprintf("\"%s\": %s, получено %d", field, expected, value))
		return 0
	}
	return value
}

func readOptionalIntField(entry value, field string, isValid func(int) bool, expected string, problems *[]string) *int {
	element, ok := entry.has(field)
	if !ok || element.kind == kindNull {
		return nil
	}
	value, ok := intValue(element)
	if !ok {
		*problems = append(*problems, fmt.Sprintf("\"%s\" должно быть целым числом или null (%s)", field, expected))
		return nil
	}
	if !isValid(value) {
		*problems = append(*problems, fmt.Sprintf("\"%s\": %s, получено %d", field, expected, value))
		return nil
	}
	return &value
}

func readLettersField(entry value, problems *[]string) string {
	element, ok := entry.has("letters")
	if !ok {
		return ""
	}
	var value *string
	if element.kind == kindString {
		value = &element.str
	}
	if value == nil || !domain.IsUpperLetters(*value) {
		got := "не строка"
		if value != nil {
			got = *value
		}
		*problems = append(*problems, fmt.Sprintf("\"letters\": одна или две заглавные русские буквы, получено «%s»", got))
		return ""
	}
	return *value
}

func describeKind(value value) string {
	switch value.kind {
	case kindNumber:
		return "число"
	case kindBool:
		if value.boolean {
			return "true"
		}
		return "false"
	case kindNull:
		return "null"
	case kindArray:
		return "массив"
	case kindString:
		return "String"
	case kindObject:
		return "Object"
	}
	return "неизвестный тип"
}
