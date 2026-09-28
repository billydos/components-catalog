package importer

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"soviettransistors/internal/domain"
)

// Тесты семантики чтения записей (формы записи, секции, отвержение ошибочных)
// идут напрямую на дереве value — без участия фронтендов форматов. Фронтенды
// покрываются format_jsonc_test.go и format_yaml_test.go, выбор формата по
// расширению файла — format_test.go.

// Конструкторы дерева value для тестов читателя.
func str(s string) value       { return value{kind: kindString, str: s} }
func num(text string) value    { return value{kind: kindNumber, num: text} }
func boolean(b bool) value     { return value{kind: kindBool, boolean: b} }
func null() value              { return value{kind: kindNull} }
func arr(items ...value) value { return value{kind: kindArray, items: items} }
func obj(ms ...member) value   { return value{kind: kindObject, members: ms} }

// Проблемы структуры корня — Issues уровня файла (EntryIndex = 0) и
// накапливаются вместе: файл с опечаткой в "transistors" и лишними ключами
// даёт полный список проблем за один прогон (ANALYSIS-02 B3).
func TestParseRoot_RootProblems(t *testing.T) {
	cases := []struct {
		name   string
		root   value
		issues []string
	}{
		{
			"корень не объект",
			arr(num("1"), num("2")),
			[]string{`корневой элемент должен быть объектом вида { "transistors": [ ... ] }`},
		},
		{
			"нет ключа transistors",
			obj(),
			[]string{`отсутствует обязательный ключ "transistors"`},
		},
		{
			"transistors не массив",
			obj(member{"transistors", obj()}),
			[]string{`"transistors" должен быть массивом`},
		},
		{
			"опечатка в transistors и лишние ключи",
			obj(member{"transistor", arr()}, member{"extra", num("2")}),
			[]string{
				`неизвестный ключ корневого объекта «transistor» (допустим только "transistors")`,
				`неизвестный ключ корневого объекта «extra» (допустим только "transistors")`,
				`отсутствует обязательный ключ "transistors"`,
			},
		},
		{
			"transistors не массив и лишний ключ",
			obj(member{"extra", num("1")}, member{"transistors", num("5")}),
			[]string{
				`неизвестный ключ корневого объекта «extra» (допустим только "transistors")`,
				`"transistors" должен быть массивом`,
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := parseRoot(testCase.root)
			if len(result.Entries) != 0 {
				t.Errorf("entries = %+v, ожидалось отсутствие записей", result.Entries)
			}
			if len(result.Issues) != len(testCase.issues) {
				t.Fatalf("issues = %+v, ожидалось %d проблем", result.Issues, len(testCase.issues))
			}
			for i, want := range testCase.issues {
				if got := result.Issues[i].Description; got != want {
					t.Errorf("текст проблемы %d:\n%q\nожидалось:\n%q", i+1, got, want)
				}
				if result.Issues[i].EntryIndex != 0 {
					t.Errorf("EntryIndex проблемы %d = %d, ожидался 0 (уровень файла)", i+1, result.Issues[i].EntryIndex)
				}
			}
		})
	}
}

func TestParseRoot_UnknownRootKey_ProducesIssue(t *testing.T) {
	result := parseRoot(obj(member{"extra", num("1")}, member{"transistors", arr()}))
	if !result.HasErrors() {
		t.Fatal("ожидалась проблема")
	}
	want := "неизвестный ключ корневого объекта «extra» (допустим только \"transistors\")"
	if got := result.Issues[0].Description; got != want {
		t.Errorf("текст проблемы = %q, ожидалось %q", got, want)
	}
}

func TestParseRoot_StringEntry_AndKindDescription(t *testing.T) {
	result := parseRoot(obj(member{"transistors", arr(str("КТ315Б"), num("42"))}))
	if len(result.Entries) != 1 || result.Entries[0].Transistor.Name() != "КТ315Б" {
		t.Fatalf("entries = %+v", result.Entries)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	want := "запись должна быть строкой (обозначение) или объектом, получено: число"
	if got := result.Issues[0].Description; got != want {
		t.Errorf("текст проблемы = %q, ожидалось %q", got, want)
	}
}

func TestParseRoot_MixedNameAndDesignationFields(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(member{"name", str("КТ315Б")}, member{"letters", str("Б")}),
		)},
	)
	result := parseRoot(root)
	if len(result.Entries) != 0 {
		t.Error("запись со смешением форм не должна быть принята")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	want := "нельзя смешивать \"name\" и явные поля обозначения: «letters» задано вместе с \"name\" — обозначение задаётся либо строкой \"name\", либо полями (material, subclass, assembly, feature, number, letters, modification, chip)"
	if got := result.Issues[0].Description; got != want {
		t.Errorf("текст проблемы:\n%q\nожидалось:\n%q", got, want)
	}
}

func TestParseRoot_DesignationFormEntry(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(
				member{"material", str("Г")}, member{"subclass", str("Т")},
				member{"assembly", boolean(false)},
				member{"feature", num("1")}, member{"number", num("15")}, member{"letters", str("А")},
				member{"ratings", obj(member{"IkMax", num("50")}, member{"tempMin", num("-40")}, member{"tempMax", num("55")})},
			),
		)},
	)
	result := parseRoot(root)
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	entry := result.Entries[0]
	if entry.Transistor.Name() != "ГТ115А" {
		t.Errorf("обозначение = %q", entry.Transistor.Name())
	}
	if entry.Ratings == nil || entry.Ratings.IkMax == nil || *entry.Ratings.IkMax != 50 {
		t.Errorf("ratings = %+v", entry.Ratings)
	}
}

func TestParseRoot_SectionError_RejectsWholeEntry(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(
				member{"name", str("КТ315Б")},
				member{"parameters", arr(obj(member{"parameter", str("h21e")}, member{"min", num("50")}))},
			),
		)},
	)
	result := parseRoot(root)
	if len(result.Entries) != 0 {
		t.Error("запись с ошибкой в секции должна быть отвергнута целиком")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	if !contains(result.Issues[0].Description, "условия —") {
		t.Errorf("текст проблемы = %q", result.Issues[0].Description)
	}
}

func TestParseRoot_EmptyParameterArray_ClearsSection(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(member{"name", str("КТ315Б")}, member{"parameters", arr()}),
		)},
	)
	result := parseRoot(root)
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	parameters := result.Entries[0].Parameters
	if parameters == nil {
		t.Error("пустой массив должен давать непустую-nil пустую секцию (очистка)")
	}
	if len(parameters) != 0 {
		t.Errorf("parameters = %+v", parameters)
	}
}

func TestParseRoot_ManufacturersNull_DoesNotChange(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(
				member{"name", str("КТ315Б")},
				member{"attributes", obj(member{"manufacturers", null()}, member{"structure", str("npn")})},
			),
		)},
	)
	result := parseRoot(root)
	entry := result.Entries[0]
	if entry.Attributes == nil || entry.Attributes.Structure == nil || *entry.Attributes.Structure != "npn" {
		t.Fatalf("attributes = %+v", entry.Attributes)
	}
	if entry.Manufacturers != nil {
		t.Errorf("manufacturers = %v, ожидалось nil", entry.Manufacturers)
	}
}

func TestParseRoot_UnknownParameterCode(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(
				member{"name", str("КТ315Б")},
				member{"parameters", arr(
					obj(member{"parameter", str("h21")}, member{"min", num("50")}, member{"Uke", num("10")}, member{"Ik", num("1")}),
				)},
			),
		)},
	)
	result := parseRoot(root)
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	if !contains(result.Issues[0].Description, "неизвестный код «h21»") {
		t.Errorf("текст проблемы = %q", result.Issues[0].Description)
	}
}

func TestParseRoot_RatingsValidation(t *testing.T) {
	root := obj(
		member{"transistors", arr(
			obj(member{"name", str("КТ315Б")}, member{"ratings", obj(member{"IkPulseMax", num("100")})}),
		)},
	)
	result := parseRoot(root)
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	if !contains(result.Issues[0].Description, "длительность импульса (pulseDuration, мкс) обязательна") {
		t.Errorf("текст проблемы = %q", result.Issues[0].Description)
	}
}

func contains(haystack, fragment string) bool {
	return strings.Contains(haystack, fragment)
}

// Пин-тест A2: каждый ключ domain.AllConditionKeys (+ temp) обязан проходить
// путём «ключ в файле → значение в структуре ElectricalParameter». Тест
// самонастраивается по каталогу: для ключа берётся первый параметр с вариантом
// условий, допускающим ключ, и собирается корректный элемент файла. При
// добавлении ключа в каталог без чтения (или без использующей его спецификации)
// тест падает, пока цепочка не замкнётся.
var conditionSampleText = map[domain.ConditionKey]string{
	domain.CondUke:  "10",
	domain.CondUkb:  "5",
	domain.CondUeb:  "5",
	domain.CondIk:   "1",
	domain.CondIe:   "10",
	domain.CondIb:   "50",
	domain.CondFreq: "1.8",
	domain.CondRg:   "500",
	domain.CondRbe:  "100",
}

// conditionExample — первый параметр каталога с вариантом, где ключ условия
// обязателен или необязателен.
func conditionExample(key domain.ConditionKey) (domain.ParameterInfo, domain.ConditionVariant, bool) {
	for _, info := range domain.ParameterCatalog {
		for _, variant := range info.Conditions.Variants {
			if variant.IsRequired(key) || variant.IsOptional(key) {
				return info, variant, true
			}
		}
	}
	return domain.ParameterInfo{}, domain.ConditionVariant{}, false
}

func TestParseRoot_AllConditionKeys_ReadIntoParameter(t *testing.T) {
	for _, key := range domain.AllConditionKeys {
		t.Run(string(key), func(t *testing.T) {
			info, variant, ok := conditionExample(key)
			if !ok {
				t.Fatalf("в каталоге нет параметра, допускающего условие %s", key)
			}
			item := obj(member{"parameter", str(info.Code)})
			switch info.Direction {
			case domain.AtLeast, domain.AtLeastOrRange:
				item.members = append(item.members, member{"min", num("50")})
			case domain.AtMost:
				item.members = append(item.members, member{"max", num("1")})
			}
			for _, required := range variant.RequiredKeys() {
				item.members = append(item.members, member{domain.ConditionJsoncKey(required), num(conditionSampleText[required])})
			}
			if variant.IsOptional(key) {
				item.members = append(item.members, member{domain.ConditionJsoncKey(key), num(conditionSampleText[key])})
			}

			root := obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, member{"parameters", arr(item)}),
			)})
			result := parseRoot(root)
			if result.HasErrors() {
				t.Fatalf("ожидалось отсутствие проблем, получено: %+v", result.Issues)
			}
			if len(result.Entries) != 1 || len(result.Entries[0].Parameters) != 1 {
				t.Fatalf("entries = %+v", result.Entries)
			}
			want, err := strconv.ParseFloat(conditionSampleText[key], 64)
			if err != nil {
				t.Fatalf("тестовое значение %s: %v", key, err)
			}
			value := domain.ConditionValueOf(result.Entries[0].Parameters[0], key)
			if value == nil || *value != want {
				t.Errorf("условие %s = %v, ожидалось %v", key, value, want)
			}
		})
	}

	t.Run("temp", func(t *testing.T) {
		root := obj(member{"transistors", arr(
			obj(member{"name", str("КТ315Б")},
				member{"parameters", arr(
					obj(member{"parameter", str("h21e")}, member{"min", num("50")},
						member{"Uke", num("10")}, member{"Ik", num("1")}, member{"temp", num("25")}),
				)}),
		)})
		result := parseRoot(root)
		if result.HasErrors() || len(result.Entries) != 1 || len(result.Entries[0].Parameters) != 1 {
			t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
		}
		temp := result.Entries[0].Parameters[0].Temp
		if temp == nil || *temp != 25 {
			t.Errorf("temp = %v, ожидалось 25", temp)
		}
	})
}

// Пин-тесты B1 (ANALYSIS-02): список extraFields производится из
// domain.AttributeFieldNames, ratingFields синхронизирован с
// domain.RatingFieldNames вручную (закреплён пином соответствия), а
// построение структур — явные литералы в readAttributes/readRatings.
// Round-trip тесты собирают объект секции со всеми ключами списка и
// валидными значениями: после разбора каждое поле структуры обязано быть
// не nil. Поле, добавленное в домен без попадания в список или без чтения
// в importer'е, — «допустимый» ключ с молча теряемым значением — роняет
// соответствующий пин, пока цепочка не замкнётся.

// attributeJsoncKey — ключ jsonc секции attributes: имя поля записи со
// строчной первой буквой (та же схема, что у buildExtraFields).
func attributeJsoncKey(fieldName string) string {
	return strings.ToLower(fieldName[:1]) + fieldName[1:]
}

// attributeSampleValue — валидное значение поля по типу; перекрытия — поля,
// чьи значения связаны доменной валидацией (годы, масса).
var attributeSampleValues = map[string]value{
	"YearFrom": num("1975"),
	"YearTo":   num("1990"),
	"MassMax":  num("0.85"),
}

func attributeSampleValue(field reflect.StructField) value {
	if sample, ok := attributeSampleValues[field.Name]; ok {
		return sample
	}
	switch field.Type.Elem().Kind() {
	case reflect.Bool:
		return boolean(true)
	case reflect.Int:
		return num("1")
	default:
		return str("значение")
	}
}

func TestParseRoot_AllAttributeFields_ReadIntoStructure(t *testing.T) {
	structType := reflect.TypeOf(domain.TransistorAttributes{})
	members := make([]member, 0, structType.NumField())
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.Type.Kind() != reflect.Ptr {
			t.Fatalf("поле %s — не указатель: необязательные поля записи — только указатели", field.Name)
		}
		members = append(members, member{attributeJsoncKey(field.Name), attributeSampleValue(field)})
	}
	root := obj(member{"transistors", arr(
		obj(member{"name", str("КТ315Б")}, member{"attributes", obj(members...)}),
	)})
	result := parseRoot(root)
	if result.HasErrors() {
		t.Fatalf("ожидалось отсутствие проблем, получено: %+v", result.Issues)
	}
	attributes := result.Entries[0].Attributes
	if attributes == nil {
		t.Fatal("attributes = nil, ожидалась прочитанная секция")
	}
	structValue := reflect.ValueOf(*attributes)
	for i := 0; i < structType.NumField(); i++ {
		if structValue.Field(i).IsNil() {
			t.Errorf("поле %s не прочитано из секции: ключ «%s» допустим, но readAttributes его не читает",
				structType.Field(i).Name, attributeJsoncKey(structType.Field(i).Name))
		}
	}
}

// TestRatingFields_MatchDomain — пин соответствия ручного списка ratingFields
// доменным полям: та же длина, тот же порядок, имена совпадают без учёта
// регистра первой буквы. Поле, добавленное в domain.MaximumRatings без
// обновления списка, роняет пин — пока список не замкнётся.
func TestRatingFields_MatchDomain(t *testing.T) {
	names := domain.RatingFieldNames()
	if len(ratingFields) != len(names) {
		t.Fatalf("ratingFields = %v, доменные поля: %v — списки разошлись", ratingFields, names)
	}
	for i, key := range ratingFields {
		if !strings.EqualFold(key, names[i]) {
			t.Errorf("ratingFields[%d] = %q, доменное поле %q — списки разошлись", i, key, names[i])
		}
	}
}

// ratingSampleValues — валидные значения ключей секции ratings (все поля
// числовые); перекрытия — температуры, связанные требованием Tмин < Tмакс.
var ratingSampleValues = map[string]value{
	"tempMin": num("-40"),
	"tempMax": num("55"),
}

func TestParseRoot_AllRatingFields_ReadIntoStructure(t *testing.T) {
	structType := reflect.TypeOf(domain.MaximumRatings{})
	members := make([]member, 0, len(ratingFields))
	for _, key := range ratingFields {
		sample := num("10")
		if override, ok := ratingSampleValues[key]; ok {
			sample = override
		}
		members = append(members, member{key, sample})
	}
	root := obj(member{"transistors", arr(
		obj(member{"name", str("КТ315Б")}, member{"ratings", obj(members...)}),
	)})
	result := parseRoot(root)
	if result.HasErrors() {
		t.Fatalf("ожидалось отсутствие проблем, получено: %+v", result.Issues)
	}
	ratings := result.Entries[0].Ratings
	if ratings == nil {
		t.Fatal("ratings = nil, ожидалась прочитанная секция")
	}
	structValue := reflect.ValueOf(*ratings)
	for i := 0; i < structType.NumField(); i++ {
		if structType.Field(i).Type.Kind() != reflect.Ptr {
			t.Fatalf("поле %s — не указатель: необязательные поля предельных данных — только указатели", structType.Field(i).Name)
		}
		if structValue.Field(i).IsNil() {
			t.Errorf("поле %s не прочитано из секции: ключ «%s» допустим, но readRatings его не читает",
				structType.Field(i).Name, ratingFields[i])
		}
	}
}

func TestParseRoot_InvalidDesignation(t *testing.T) {
	result := parseRoot(obj(member{"transistors", arr(str("ХТ315А"))}))
	if len(result.Entries) != 0 || len(result.Issues) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	if issue := result.Issues[0]; issue.EntryIndex != 1 || issue.Source != "ХТ315А" ||
		!contains(issue.Description, "позиция 1: ожидался тип материала") {
		t.Errorf("issue = %+v", issue)
	}

	result = parseRoot(obj(
		member{"transistors", arr(
			obj(member{"name", str("КТ315")}),
		)},
	))
	if len(result.Entries) != 0 || len(result.Issues) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	if issue := result.Issues[0]; issue.Source != "КТ315" || !contains(issue.Description, "буква классификации") {
		t.Errorf("issue = %+v", issue)
	}
}

func TestParseRoot_NameFieldNotString(t *testing.T) {
	result := parseRoot(obj(
		member{"transistors", arr(
			obj(member{"name", num("5")}),
		)},
	))
	if len(result.Entries) != 0 || len(result.Issues) != 1 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	want := `"name" должно быть строкой с обозначением транзистора`
	if got := result.Issues[0].Description; got != want {
		t.Errorf("текст проблемы = %q, ожидалось %q", got, want)
	}
}

func TestParseRoot_NonObjectEntryKinds(t *testing.T) {
	result := parseRoot(obj(member{"transistors", arr(boolean(true), null(), arr())}))
	if len(result.Entries) != 0 || len(result.Issues) != 3 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	for index, want := range []string{"true", "null", "массив"} {
		if got := result.Issues[index].Description; !contains(got, "получено: "+want) {
			t.Errorf("текст проблемы %d = %q, ожидалось «получено: %s»", index+1, got, want)
		}
	}
}

// designationEntry — запись в форме явных полей обозначения с валидным базовым
// набором (ГТ115А); переданные члены заменяют одноимённые или добавляются.
func designationEntry(ms ...member) value {
	base := []member{
		{"material", str("Г")}, {"subclass", str("Т")}, {"assembly", boolean(false)},
		{"feature", num("1")}, {"number", num("15")}, {"letters", str("А")},
	}
	for _, m := range ms {
		replaced := false
		for i := range base {
			if base[i].name == m.name {
				base[i] = m
				replaced = true
			}
		}
		if !replaced {
			base = append(base, m)
		}
	}
	return value{kind: kindObject, members: base}
}

func TestParseRoot_DesignationFieldValidation(t *testing.T) {
	cases := []struct {
		name  string
		entry value
		want  string
	}{
		{"material не строка", designationEntry(member{"material", num("1")}), `"material": ожидалось Г|1, К|2, А|3 или И|4, получено «не строка»`},
		{"material неверный символ", designationEntry(member{"material", str("Х")}), `"material": ожидалось Г|1, К|2, А|3 или И|4, получено «Х»`},
		{"material больше одного символа", designationEntry(member{"material", str("ГТ")}), `получено «ГТ»`},
		{"subclass неверный символ", designationEntry(member{"subclass", str("М")}), `"subclass": ожидалось Т или П, получено «М»`},
		{"assembly не логическое значение", designationEntry(member{"assembly", num("1")}), `"assembly" должно быть true или false`},
		{"feature не целое число", designationEntry(member{"feature", str("1")}), `"feature" должно быть целым числом (цифра от 1 до 9)`},
		{"feature вне диапазона", designationEntry(member{"feature", num("0")}), `"feature": цифра от 1 до 9, получено 0`},
		{"number дробное", designationEntry(member{"number", num("1.5")}), `"number" должно быть целым числом (число от 1 до 999)`},
		{"number вне диапазона", designationEntry(member{"number", num("1000")}), `"number": число от 1 до 999, получено 1000`},
		{"letters строчные", designationEntry(member{"letters", str("а")}), `"letters": одна или две заглавные русские буквы, получено «а»`},
		{"letters не строка", designationEntry(member{"letters", num("1")}), `получено «не строка»`},
		{"modification дробное", designationEntry(member{"modification", num("1.5")}), `"modification" должно быть целым числом или null (цифра от 1 до 9)`},
		{"modification вне диапазона", designationEntry(member{"modification", num("10")}), `"modification": цифра от 1 до 9, получено 10`},
		{"chip вне диапазона", designationEntry(member{"chip", num("7")}), `"chip": цифра от 1 до 6, получено 7`},
		{"неизвестное поле записи", designationEntry(member{"foo", num("1")}), "неизвестное поле «foo»"},
		{
			"отсутствует material",
			obj(member{"subclass", str("Т")}, member{"assembly", boolean(false)},
				member{"feature", num("1")}, member{"number", num("15")}, member{"letters", str("А")}),
			`отсутствует обязательное поле "material"`,
		},
		{
			"отсутствуют feature и number",
			obj(member{"material", str("Г")}, member{"subclass", str("Т")},
				member{"assembly", boolean(false)}, member{"letters", str("А")}),
			`отсутствует обязательное поле "feature"`,
		},
		{
			"отсутствует letters",
			obj(member{"material", str("Г")}, member{"subclass", str("Т")}, member{"assembly", boolean(false)},
				member{"feature", num("1")}, member{"number", num("15")}),
			`отсутствует обязательное поле "letters"`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := parseRoot(obj(member{"transistors", arr(testCase.entry)}))
			if len(result.Entries) != 0 {
				t.Error("запись с ошибкой в поле обозначения должна быть отвергнута")
			}
			if len(result.Issues) != 1 {
				t.Fatalf("issues = %+v", result.Issues)
			}
			if !contains(result.Issues[0].Description, testCase.want) {
				t.Errorf("текст проблемы:\n%q\nожидалось вхождение:\n%q", result.Issues[0].Description, testCase.want)
			}
		})
	}
}

func TestParseRoot_DesignationForm_OptionalFields(t *testing.T) {
	result := parseRoot(obj(member{"transistors", arr(
		designationEntry(member{"assembly", null()}, member{"modification", num("1")}, member{"chip", num("2")}),
		designationEntry(member{"assembly", boolean(true)}, member{"modification", null()}, member{"chip", null()}),
	)}))
	if result.HasErrors() || len(result.Entries) != 2 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}
	first, second := result.Entries[0].Transistor, result.Entries[1].Transistor
	if first.Name() != "ГТ115А1-2" || first.IsAssembly ||
		first.Modification == nil || *first.Modification != 1 || first.ChipVariant == nil || *first.ChipVariant != 2 {
		t.Errorf("первая запись = %+v (%q)", first, first.Name())
	}
	if second.Name() != "ГТС115А" || !second.IsAssembly || second.Modification != nil || second.ChipVariant != nil {
		t.Errorf("вторая запись = %+v (%q)", second, second.Name())
	}
}

func TestParseRoot_Attributes_FullValidSet(t *testing.T) {
	root := obj(member{"transistors", arr(
		obj(
			member{"name", str("КТ315Б")},
			member{"attributes", obj(
				member{"structure", str("npn")},
				member{"technology", str("планарная")},
				member{"package", str("TO-92")},
				member{"packageMaterial", str("пластик")},
				member{"colorMarking", str("жёлтая точка")},
				member{"pinout", str("КБЭ")},
				member{"esdSensitive", boolean(true)},
				member{"militaryGrade", boolean(false)},
				member{"radiationHardened", boolean(true)},
				member{"tu", str("ТУ 11.365.001-71")},
				member{"notes", str("универсальный маломощный")},
				member{"yearFrom", num("1975")},
				member{"yearTo", num("1990")},
				member{"massMax", num("0.85")},
				member{"datasheetUrl", str("https://example.com/kt315.pdf")},
				member{"manufacturers", arr(str("Рязанский завод"), str(" завод «Экран» "))},
			)},
		),
		obj(
			member{"name", str("ГТ115А")},
			member{"attributes", obj(
				member{"structure", null()},
				member{"esdSensitive", null()},
				member{"yearFrom", null()},
				member{"massMax", null()},
			)},
		),
	)})
	result := parseRoot(root)
	if result.HasErrors() || len(result.Entries) != 2 {
		t.Fatalf("entries = %+v, issues = %+v", result.Entries, result.Issues)
	}

	attributes := result.Entries[0].Attributes
	if attributes == nil {
		t.Fatal("attributes = nil, ожидалась прочитанная секция")
	}
	checkString := func(field *string, want string, name string) {
		if field == nil || *field != want {
			t.Errorf("%s = %v, ожидалось %q", name, field, want)
		}
	}
	checkString(attributes.Structure, "npn", "structure")
	checkString(attributes.Technology, "планарная", "technology")
	checkString(attributes.Package, "TO-92", "package")
	checkString(attributes.PackageMaterial, "пластик", "packageMaterial")
	checkString(attributes.ColorMarking, "жёлтая точка", "colorMarking")
	checkString(attributes.Pinout, "КБЭ", "pinout")
	checkString(attributes.Tu, "ТУ 11.365.001-71", "tu")
	checkString(attributes.Notes, "универсальный маломощный", "notes")
	checkString(attributes.DatasheetUrl, "https://example.com/kt315.pdf", "datasheetUrl")
	checkBool := func(field *bool, want bool, name string) {
		if field == nil || *field != want {
			t.Errorf("%s = %v, ожидалось %v", name, field, want)
		}
	}
	checkBool(attributes.EsdSensitive, true, "esdSensitive")
	checkBool(attributes.MilitaryGrade, false, "militaryGrade")
	checkBool(attributes.RadiationHardened, true, "radiationHardened")
	checkInt := func(field *int, want int, name string) {
		if field == nil || *field != want {
			t.Errorf("%s = %v, ожидалось %d", name, field, want)
		}
	}
	checkInt(attributes.YearFrom, 1975, "yearFrom")
	checkInt(attributes.YearTo, 1990, "yearTo")
	if attributes.MassMax == nil || *attributes.MassMax != 0.85 {
		t.Errorf("massMax = %v, ожидалось 0.85", attributes.MassMax)
	}
	manufacturers := result.Entries[0].Manufacturers
	if len(manufacturers) != 2 || manufacturers[0] != "Рязанский завод" || manufacturers[1] != "завод «Экран»" {
		t.Errorf("manufacturers = %v (названия должны обрезаться по пробелам)", manufacturers)
	}

	// null в необязательных полях — «не задано»: секция читается без ошибок
	empty := result.Entries[1].Attributes
	if empty == nil || empty.Structure != nil || empty.EsdSensitive != nil ||
		empty.YearFrom != nil || empty.MassMax != nil {
		t.Errorf("attributes второй записи = %+v, ожидались nil-поля", empty)
	}
	if result.Entries[1].Manufacturers != nil {
		t.Errorf("manufacturers = %v, ожидалось nil", result.Entries[1].Manufacturers)
	}
}

func TestParseRoot_Attributes_Problems(t *testing.T) {
	cases := []struct {
		name       string
		attributes value
		want       string
	}{
		{"секция не объект", num("5"), `"attributes" должно быть объектом`},
		{"неизвестное поле", obj(member{"foo", num("1")}), "неизвестное поле «foo»"},
		{"текстовое поле не строка", obj(member{"structure", num("1")}), `"structure" должно быть строкой`},
		{"логическое поле не bool", obj(member{"esdSensitive", num("1")}), `"esdSensitive" должно быть true или false`},
		{"год не целое число", obj(member{"yearFrom", str("1975")}), `"yearFrom" должно быть целым числом`},
		{"пустое значение атрибута", obj(member{"structure", str("   ")}), "структура: пустое значение"},
		{"год начала вне диапазона", obj(member{"yearFrom", num("1900")}), "год начала выпуска должен быть от 1949 до 2100, получено 1900"},
		{"годы в обратном порядке", obj(member{"yearFrom", num("1980")}, member{"yearTo", num("1970")}), "год начала выпуска (1980) должен быть меньше года окончания (1970)"},
		{"масса неположительная", obj(member{"massMax", num("0")}), "масса «не более» должна быть положительной"},
		{"manufacturers не массив", obj(member{"manufacturers", str("завод")}), `"manufacturers" должно быть массивом строк`},
		{"manufacturers невалидный элемент", obj(member{"manufacturers", arr(num("5"))}), `"manufacturers" №1: ожидалось непустое название производителя`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, member{"attributes", testCase.attributes}),
			)})
			result := parseRoot(root)
			if len(result.Entries) != 0 {
				t.Error("запись с ошибкой в атрибутах должна быть отвергнута целиком")
			}
			if len(result.Issues) != 1 {
				t.Fatalf("issues = %+v", result.Issues)
			}
			if !contains(result.Issues[0].Description, testCase.want) {
				t.Errorf("текст проблемы:\n%q\nожидалось вхождение:\n%q", result.Issues[0].Description, testCase.want)
			}
		})
	}
}

// null у целой секции — «не менять», равносильно отсутствию: запись валидна,
// все секции читаются как nil (см. ANALYSIS-02 B2, вариант 2).
func TestParseRoot_NullSections_SameAsAbsent(t *testing.T) {
	assertBareEntry := func(t *testing.T, result *ParseResult) {
		t.Helper()
		if len(result.Issues) != 0 {
			t.Fatalf("issues = %+v, ожидалось отсутствие проблем", result.Issues)
		}
		if len(result.Entries) != 1 {
			t.Fatalf("entries = %+v, ожидалась одна запись", result.Entries)
		}
		entry := result.Entries[0]
		if entry.Attributes != nil || entry.Manufacturers != nil ||
			entry.Parameters != nil || entry.Ratings != nil {
			t.Errorf("секции = attributes:%v manufacturers:%v parameters:%v ratings:%v, ожидались nil",
				entry.Attributes, entry.Manufacturers, entry.Parameters, entry.Ratings)
		}
	}
	for _, section := range []member{
		{"attributes", null()},
		{"parameters", null()},
		{"ratings", null()},
	} {
		t.Run(section.name, func(t *testing.T) {
			root := obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, section),
			)})
			result := parseRoot(root)
			assertBareEntry(t, result)
		})
	}
	t.Run("все три null-секции сразу", func(t *testing.T) {
		root := obj(member{"transistors", arr(
			obj(member{"name", str("КТ315Б")},
				member{"attributes", null()},
				member{"parameters", null()},
				member{"ratings", null()}),
		)})
		result := parseRoot(root)
		assertBareEntry(t, result)
	})
}

func TestParseRoot_ParameterAndRatingProblems(t *testing.T) {
	cases := []struct {
		name string
		root value
		want string
	}{
		{
			"секция параметров не массив",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, member{"parameters", num("5")}),
			)}),
			`"parameters" должно быть массивом объектов`,
		},
		{
			"элемент параметра не объект",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, member{"parameters", arr(num("5"))}),
			)}),
			"параметр №1: должен быть объектом",
		},
		{
			"отсутствует код параметра",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")},
					member{"parameters", arr(
						obj(member{"min", num("50")}),
					)}),
			)}),
			`обязательное поле "parameter"`,
		},
		{
			// закрепляет текущий текст: «parameter» не строка — то же сообщение,
			// что и при отсутствии поля (ANALYSIS-02 C6)
			"код параметра не строка",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")},
					member{"parameters", arr(
						obj(member{"parameter", num("5")}),
					)}),
			)}),
			`обязательное поле "parameter"`,
		},
		{
			"неизвестное поле параметра",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, member{"parameters", arr(
					obj(member{"parameter", str("h21e")}, member{"min", num("50")},
						member{"Uke", num("10")}, member{"Ik", num("1")}, member{"foo", num("1")}),
				)}),
			)}),
			"неизвестное поле «foo»",
		},
		{
			"секция предельных данных не объект",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")}, member{"ratings", num("5")}),
			)}),
			`"ratings" должно быть объектом`,
		},
		{
			"неизвестное поле предельных данных",
			obj(member{"transistors", arr(
				obj(member{"name", str("КТ315Б")},
					member{"ratings", obj(
						member{"foo", num("1")},
					)}),
			)}),
			"предельные данные: неизвестное поле «foo»",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := parseRoot(testCase.root)
			if len(result.Entries) != 0 {
				t.Error("запись с ошибкой в секции должна быть отвергнута целиком")
			}
			if len(result.Issues) != 1 {
				t.Fatalf("issues = %+v", result.Issues)
			}
			if !contains(result.Issues[0].Description, testCase.want) {
				t.Errorf("текст проблемы:\n%q\nожидалось вхождение:\n%q", result.Issues[0].Description, testCase.want)
			}
		})
	}
}
