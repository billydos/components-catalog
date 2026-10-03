package importer

import (
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/seed"
)

// Читатель файла наполнения: семантика секций (nil/пусто/задано),
// формы записей, дословные тексты проблем. Снимок — стартовый каталог
// (сиды), как у чистой базы.

func testSnapshot(t *testing.T) *catalog.Snapshot {
	t.Helper()
	snap, probs := catalog.ApplyCatalog(nil, seed.Catalog())
	if len(probs) > 0 {
		t.Fatalf("стартовый каталог невалиден: %v", probs[0])
	}
	return snap
}

func parseDoc(t *testing.T, text string) (value, *catalog.Snapshot) {
	t.Helper()
	v, err := parseJSONC([]byte(text))
	if err != nil {
		t.Fatalf("разбор jsonc: %v", err)
	}
	return v, testSnapshot(t)
}

func issueMessages(issues []Issue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.String())
	}
	return out
}

// Секция fields: форма значений (коды словарей — строками, assembly —
// числом), null-члены пропускаются, неизвестные ключи — дословные тексты.
func TestReadDocumentClassificationFields(t *testing.T) {
	root, snap := parseDoc(t, `{
		"transistors": [
			{
				"name": "MJE340", "system": "other",
				"fields": { "material": "si", "subclass": "bjt", "category": "high_voltage", "assembly": 0 }
			},
			{ "name": "MJE350", "system": "other", "fields": { "material": null, "subclass": "bjt" } },
			{ "name": "MJE360", "system": "other", "fields": [] },
			{ "name": "MJE370", "system": "other", "fields": { "power": 5 } },
			{ "name": "MJE380", "system": "other", "fields": { "material": 5 } },
			{ "name": "MJE390", "system": "other", "fields": { "assembly": "нет" } }
		]
	}`)
	doc, issues := ReadDocument(root, snap)
	if len(doc.Records) != 2 || doc.Rejected != 4 {
		t.Fatalf("записи: %d, rejected: %d: %v", len(doc.Records), doc.Rejected, issueMessages(issues))
	}
	f := doc.Records[0].Input.Fields
	if f == nil || len(*f) != 4 {
		t.Fatalf("fields: %v", f)
	}
	byName := map[string]domain.Field{}
	for _, field := range *f {
		byName[field.Name] = field
	}
	if byName["material"].Text != "si" || byName["subclass"].Text != "bjt" ||
		byName["category"].Text != "high_voltage" {
		t.Fatalf("текстовые поля: %v", *f)
	}
	if !byName["assembly"].IsNum || byName["assembly"].Num != 0 {
		t.Fatalf("assembly: %v", byName["assembly"])
	}
	f2 := doc.Records[1].Input.Fields
	if f2 == nil || len(*f2) != 1 || (*f2)[0].Text != "bjt" {
		t.Fatalf("null-член: %v", f2)
	}
	got := issueMessages(issues)
	want := []string{
		"record «MJE360»: the fields section must be an object «field code → value»",
		"record «MJE370»: unknown classification field «power» (allowed: material, subclass, adjustment, category, assembly)",
		"record «MJE380»: value of classification field «material» must be a string with a dictionary code",
		"record «MJE390»: value of classification field assembly must be a number 0 or 1",
	}
	for _, w := range want {
		if !containsMessage(got, w) {
			t.Errorf("проблема отсутствует: %q (есть: %v)", w, got)
		}
	}
}

func TestReadDocumentRecordForms(t *testing.T) {
	root, snap := parseDoc(t, `{
		"transistors": [
			"МП39",
			{ "name": "КТ315Б", "system": "gost" }
		]
	}`)
	doc, issues := ReadDocument(root, snap)
	if len(issues) != 0 {
		t.Fatalf("проблемы не ожидались: %v", issueMessages(issues))
	}
	if len(doc.Records) != 2 {
		t.Fatalf("записей: %d, ожидалось 2", len(doc.Records))
	}
	if doc.Records[0].Input.Name != "МП39" || doc.Records[0].Kind != domain.KindTransistor {
		t.Fatalf("string-запись: %+v", doc.Records[0].Input)
	}
	if doc.Records[1].Input.System != domain.SystemGost {
		t.Fatalf("system записи: %q", doc.Records[1].Input.System)
	}
}

func TestReadDocumentSectionSemantics(t *testing.T) {
	root, snap := parseDoc(t, `{
		"resistors": [
			{
				"name": "С2-33Н",
				"attributes": { "technology": "металлодиэлектрические" },
				"parameters": [
					{ "parameter": "Rnom", "min": 1, "max": 10000000 },
					{ "parameter": "TKS", "max": 500 }
				],
				"ratings": null,
				"dimensions": [],
				"manufacturers": [],
				"analogs": null
			}
		]
	}`)
	doc, issues := ReadDocument(root, snap)
	if len(issues) != 0 {
		t.Fatalf("проблемы не ожидались: %v", issueMessages(issues))
	}
	in := doc.Records[0].Input

	if len(in.Attributes) != 1 || in.Attributes[0].Attribute != "technology" {
		t.Fatalf("атрибуты: %+v", in.Attributes)
	}
	// ratings: null — секция не меняется: в Sections её нет.
	var sections []string
	for _, s := range in.Sections {
		sections = append(sections, s.Section)
	}
	if strings.Join(sections, ",") != "parameters,dimensions" {
		t.Fatalf("секции: %v (null-секция должна отсутствовать)", sections)
	}
	// dimensions: [] — очистка: секция задана с пустым списком.
	var dims []catalog.ParameterValue
	for _, s := range in.Sections {
		if s.Section == "dimensions" {
			dims = s.Values
		}
	}
	if dims == nil || len(dims) != 0 {
		t.Fatalf("dimensions должен быть задан пустым: %v", dims)
	}
	// manufacturers: [] — очистка (не nil).
	if in.Manufacturers == nil || len(*in.Manufacturers) != 0 {
		t.Fatalf("manufacturers должен быть задан пустым: %v", in.Manufacturers)
	}
	// analogs: null — не менять.
	if in.Analogs != nil {
		t.Fatalf("analogs должен быть nil: %v", in.Analogs)
	}
}

func TestReadDocumentValueConditions(t *testing.T) {
	root, snap := parseDoc(t, `{
		"transistors": [
			{
				"name": "КТ315Б",
				"parameters": [
					{ "parameter": "h21e", "min": 50, "max": 350, "Uke": 10, "Ik": 1 }
				]
			}
		]
	}`)
	doc, issues := ReadDocument(root, snap)
	if len(issues) != 0 {
		t.Fatalf("проблемы не ожидались: %v", issueMessages(issues))
	}
	vals := doc.Records[0].Input.Sections[0].Values
	if len(vals) != 1 || vals[0].Parameter != "h21e" {
		t.Fatalf("значение: %+v", vals)
	}
	if vals[0].Min == nil || *vals[0].Min != 50 || vals[0].Max == nil || *vals[0].Max != 350 {
		t.Fatalf("границы: %+v", vals[0])
	}
	if len(vals[0].Conditions) != 2 {
		t.Fatalf("условия: %+v", vals[0].Conditions)
	}
	if vals[0].Conditions[0].Condition != "Uke" || vals[0].Conditions[0].Value != 10 {
		t.Fatalf("условие Uke: %+v", vals[0].Conditions[0])
	}
}

func TestReadDocumentVariantsAndAnalogs(t *testing.T) {
	root, snap := parseDoc(t, `{
		"capacitors": [
			{
				"name": "К50-35",
				"variants": [
					{
						"label": "160 В",
						"parameters": [ { "parameter": "Unom", "value": 160 } ],
						"dimensions": [ { "parameter": "massMax", "value": 1.5 } ]
					}
				]
			}
		],
		"diodes": [
			{
				"name": "Д226",
				"analogs": [ "1N4007", { "name": "UF4007", "note": "быстрый" } ]
			}
		]
	}`)
	doc, issues := ReadDocument(root, snap)
	if len(issues) != 0 {
		t.Fatalf("проблемы не ожидались: %v", issueMessages(issues))
	}
	cap := doc.Records[0].Input
	if cap.Variants == nil || len(*cap.Variants) != 1 {
		t.Fatalf("варианты: %v", cap.Variants)
	}
	v := (*cap.Variants)[0]
	if v.Label != "160 В" || len(v.Sections) != 2 {
		t.Fatalf("вариант: %+v", v)
	}
	dio := doc.Records[1].Input
	if dio.Analogs == nil || len(*dio.Analogs) != 2 {
		t.Fatalf("аналоги: %v", dio.Analogs)
	}
	analogs := *dio.Analogs
	if analogs[0].Designation != "1N4007" || analogs[0].Note != "" {
		t.Fatalf("аналог-string: %+v", analogs[0])
	}
	if analogs[1].Designation != "UF4007" || analogs[1].Note != "быстрый" {
		t.Fatalf("аналог-object: %+v", analogs[1])
	}
}

func TestReadDocumentIssuesVerbatimTexts(t *testing.T) {
	root, snap := parseDoc(t, `{
		"widgets": [],
		"transistors": [
			{ "nane": "КТ315Б" },
			{ "name": "КТ315Б", "system": "star" },
			{ "name": "КТ315Б", "voltparams": [] },
			{ "name": "КТ315Б", "parameters": [ { "parameter": "h21e", "min": "много" } ] },
			42
		]
	}`)
	doc, issues := ReadDocument(root, snap)
	if len(doc.Records) != 0 || doc.Rejected != 5 {
		t.Fatalf("записи: %d, rejected: %d (ожидалось 0/5)", len(doc.Records), doc.Rejected)
	}
	if len(issues) != 6 {
		t.Fatalf("проблем: %d, ожидалось 6: %v", len(issues), issueMessages(issues))
	}
	want := []string{
		"unknown root key «widgets» (allowed: catalog, transistors, diodes, resistors, capacitors)",
		"record no. 1: mandatory key \"name\" — a string with the designation",
		"record «КТ315Б»: unknown designation system «star» (allowed: gost, ost, pro, jedec, jis, series, other)",
		"record «КТ315Б»: unknown field «voltparams» (allowed: name, system, fields, attributes, manufacturers, variants, analogs and group sections: parameters, ratings, dimensions)",
		"record «КТ315Б»: section «parameters», value no. 1: key \"min\" must be a number",
		"record no. 5: a record must be a string (designation) or an object, got: number",
	}
	got := issueMessages(issues)
	// порядок проблем: корневая — при обходе ключей корня, записи — по порядку.
	if got[0] != want[0] {
		t.Errorf("проблема 1: %q", got[0])
	}
	for _, w := range want[1:] {
		if !containsMessage(got, w) {
			t.Errorf("проблема отсутствует: %q (есть: %v)", w, got)
		}
	}
}

func containsMessage(messages []string, want string) bool {
	for _, m := range messages {
		if m == want {
			return true
		}
	}
	return false
}

func TestReadRecordLineWrapperRules(t *testing.T) {
	snap := testSnapshot(t)

	rec, ok, issues := ReadRecordLine(snap, mustParseLine(t, `{"transistors": {"name": "МП39"}}`), 3)
	if !ok || len(issues) != 0 {
		t.Fatalf("string-обёртка: ok=%v issues=%v", ok, issueMessages(issues))
	}
	if rec.Line != 3 || rec.Kind != domain.KindTransistor || rec.Input.Name != "МП39" {
		t.Fatalf("запись: %+v", rec)
	}

	_, ok, issues = ReadRecordLine(snap, mustParseLine(t, `{"transistors": {"name": "A"}, "diodes": {"name": "B"}}`), 4)
	if ok || len(issues) != 1 || issues[0].String() != "line 4: the wrapper line must contain exactly one key — a kind or catalog" {
		t.Fatalf("два ключа: ok=%v issues=%v", ok, issueMessages(issues))
	}

	_, ok, issues = ReadRecordLine(snap, mustParseLine(t, `42`), 5)
	if ok || len(issues) != 1 || issues[0].String() != "line 5: the line must be a wrapper object {\"<kind>\": <record>} or {\"catalog\": …}" {
		t.Fatalf("не object: ok=%v issues=%v", ok, issueMessages(issues))
	}
}

func mustParseLine(t *testing.T, text string) value {
	t.Helper()
	v, err := parseLineJSON([]byte(text), 1)
	if err != nil {
		t.Fatalf("разбор строки: %v", err)
	}
	return v
}

func TestReadCatalogSectionMapping(t *testing.T) {
	v, err := parseJSONC([]byte(`{
		"catalog": {
			"units": [ { "code": "kV" } ],
			"conditions": [ { "code": "Ub", "unit": "V", "allow_negative": true } ],
			"parameter_groups": [ { "code": "env", "section": "environment" } ],
			"parameters": [ {
				"code": "vib", "group": "env", "value_type": "text"
			} ],
			"attributes": [ { "code": "coating", "type": "enum", "enum_values": ["лак", "эмаль"] } ],
			"kind_validation_rules": [ { "kind": "capacitor", "rule": "cap_variant_matrix" } ]
		}
	}`))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	catVal, _ := v.has("catalog")
	in, issues := ReadCatalogSection(catVal)
	if len(issues) != 0 {
		t.Fatalf("проблемы не ожидались: %v", issueMessages(issues))
	}
	if len(in.Units) != 1 || in.Units[0].Code != "kV" {
		t.Fatalf("units: %+v", in.Units)
	}
	if len(in.Conditions) != 1 || !in.Conditions[0].AllowNegative {
		t.Fatalf("conditions: %+v", in.Conditions)
	}
	if len(in.Groups) != 1 || in.Groups[0].SectionName != "environment" {
		t.Fatalf("groups: %+v", in.Groups)
	}
	if len(in.Parameters) != 1 || in.Parameters[0].ValueType != catalog.ValueText || !in.Parameters[0].Active {
		t.Fatalf("parameters: %+v", in.Parameters)
	}
	if len(in.Attributes) != 1 || len(in.Attributes[0].EnumValues) != 2 {
		t.Fatalf("attributes: %+v", in.Attributes)
	}
	if len(in.KindRules) != 1 || in.KindRules[0].Rule != "cap_variant_matrix" {
		t.Fatalf("kind_validation_rules: %+v", in.KindRules)
	}
}

func TestReadCatalogSectionConditionSets(t *testing.T) {
	v, err := parseJSONC([]byte(`{ "catalog": { "parameters": [ {
		"code": "Ck", "group": "electrical",
		"value_type": "at_most",
		"condition_sets": [ { "items": [
			{ "condition": "Ukb", "mode": "required" },
			{ "condition": "freq", "mode": "required", "fixed_value": 0.1 }
		] } ]
	} ] } }`))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	catVal, _ := v.has("catalog")
	in, issues := ReadCatalogSection(catVal)
	if len(issues) != 0 {
		t.Fatalf("проблемы не ожидались: %v", issueMessages(issues))
	}
	sets := in.Parameters[0].ConditionSets
	if len(sets) != 1 || sets[0].No != 1 || len(sets[0].Items) != 2 {
		t.Fatalf("наборы условий: %+v", sets)
	}
	if sets[0].Items[1].FixedValue == nil || *sets[0].Items[1].FixedValue != 0.1 {
		t.Fatalf("fixed_value: %+v", sets[0].Items[1])
	}
}

func TestReadCatalogSectionUnknownSubsection(t *testing.T) {
	v, err := parseJSONC([]byte(`{ "catalog": { "paraneters": [] } }`))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	catVal, _ := v.has("catalog")
	_, issues := ReadCatalogSection(catVal)
	if len(issues) != 1 {
		t.Fatalf("проблем: %d", len(issues))
	}
	want := "catalog: unknown catalog subsection «paraneters» (allowed: kinds, designation_systems, designation_system_kinds, series_families, units, categories, conditions, parameter_groups, parameters, attributes, validation_rules, kind_validation_rules)"
	if issues[0].String() != want {
		t.Fatalf("текст: %q", issues[0].String())
	}
}

func TestReadCatalogSectionUnknownField(t *testing.T) {
	v, err := parseJSONC([]byte(`{ "catalog": { "units": [ { "code": "kV", "simbol": "кВ" } ] } }`))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	catVal, _ := v.has("catalog")
	_, issues := ReadCatalogSection(catVal)
	if len(issues) != 1 {
		t.Fatalf("проблем: %d", len(issues))
	}
	want := "catalog: section units, «kV»: unknown field «simbol» (allowed: code)"
	if issues[0].String() != want {
		t.Fatalf("текст: %q", issues[0].String())
	}
}
