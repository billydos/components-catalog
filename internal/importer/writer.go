package importer

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/service"
)

// Экспорт — round-trip формат наполнения (docs/plan/04 §4): тот же вид файла,
// что читает importer, во всех форматах реестра. Порядок детерминирован:
// классы — по снимку каталога, записи — по (kind, designation), секции
// групп — по группам каталога, значения — по sort_order, условия — по коду,
// атрибуты — по каталогу; повторный экспорт даёт байтово тот же файл.

// formatExportNum — канонический текст числа: без экспоненты и потери
// точности (кратчайшая десятичная запись, round-trip точен для float64).
func formatExportNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Export выгружает записи устройств (все классы либо один) в формате
// наполнения: jsonc/yaml — документ с секциями классов, ndjson — по
// строке-обёртке на запись.
func (m *Importer) Export(ctx context.Context, w io.Writer, format Format, kind *domain.Kind) error {
	if !format.Known() {
		return domain.NewErrorf(domain.CodeInvalidImportFile,
			domain.MsgImportFormatUnknown, string(format))
	}
	snap, err := m.app.Snapshot(ctx)
	if err != nil {
		return err
	}
	if kind != nil {
		if _, ok := snap.Kind(*kind); !ok {
			return domain.NewErrorf(domain.CodeValidationFailed,
				domain.MsgKindUnknown, string(*kind))
		}
	}

	root := value{kind: kindObject}
	for _, k := range snap.Kinds {
		if kind != nil && k.Code != *kind {
			continue
		}
		records, err := m.exportKindRecords(ctx, snap, k.Code)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			continue
		}
		root.members = append(root.members, member{name: catalog.KindSection(k.Code), value: array(records...)})
	}

	switch format {
	case FormatNDJSON:
		return writeNDJSONRoot(w, root)
	case FormatYAML:
		return writeYAMLTree(w, root)
	default:
		return writeJSONTree(w, root)
	}
}

// ExportCatalog выгружает каталог (снимок определений) в формате
// наполнения: секция catalog со всеми подразделами.
func (m *Importer) ExportCatalog(ctx context.Context, w io.Writer, format Format) error {
	if !format.Known() {
		return domain.NewErrorf(domain.CodeInvalidImportFile,
			domain.MsgImportFormatUnknown, string(format))
	}
	snap, err := m.app.Snapshot(ctx)
	if err != nil {
		return err
	}
	cat := catalogTree(snap)
	switch format {
	case FormatNDJSON:
		return writeNDJSONCatalog(w, cat)
	case FormatYAML:
		return writeYAMLTree(w, object(pair("catalog", cat)))
	default:
		return writeJSONTree(w, object(pair("catalog", cat)))
	}
}

// exportKindRecords собирает деревья записей класса (пагинация поиска,
// карточка — по стабильному id).
func (m *Importer) exportKindRecords(ctx context.Context, snap *catalog.Snapshot,
	kind domain.Kind) ([]value, error) {
	var out []value
	const pageLimit = 200
	offset := 0
	for {
		page, err := m.app.Services().Devices.Search(ctx, service.SearchQuery{
			Kind: kind, Limit: pageLimit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			card, found, err := m.app.Services().Devices.GetByID(ctx, item.ID)
			if err != nil {
				return nil, err
			}
			if !found { // параллельное удаление — запись уже не выгружается
				continue
			}
			// Разбор обозначения над реестром семейств каталога — как при
			// импорте: явные классификационные поля = хранимые минус
			// продукты парсера (round-trip секции fields).
			p, err := m.app.Services().Designations.ParseForSystem(ctx,
				card.Designation, card.System, card.Kind)
			if err != nil {
				return nil, err
			}
			out = append(out, recordTree(card, &p))
		}
		offset += pageLimit
		if offset >= page.Total {
			return out, nil
		}
	}
}

// recordTree строит дерево записи наполнения из карточки. Поля разбора
// записей восстанавливает разбор обозначения; секция fields содержит
// только явные классификационные поля (хранимые минус продукты парсера).
func recordTree(c *service.Card, parsed *domain.ParsedDesignation) value {
	members := []member{
		pair("name", str(c.Designation)),
		pair("system", str(string(c.System))),
	}
	if parsed != nil {
		if explicit := explicitFields(c, parsed); len(explicit) > 0 {
			fieldPairs := make([]member, 0, len(explicit))
			for _, f := range explicit {
				if f.IsNum {
					fieldPairs = append(fieldPairs, pair(f.Name, num(f.Num)))
					continue
				}
				fieldPairs = append(fieldPairs, pair(f.Name, str(f.Text)))
			}
			members = append(members, pair("fields", object(fieldPairs...)))
		}
	}
	if len(c.Attributes) > 0 {
		attrPairs := make([]member, 0, len(c.Attributes))
		for _, a := range c.Attributes {
			attrPairs = append(attrPairs, pair(a.Code, attrTreeValue(a)))
		}
		members = append(members, pair("attributes", object(attrPairs...)))
	}
	for _, g := range c.Groups {
		if len(g.Values) == 0 {
			continue
		}
		items := make([]value, 0, len(g.Values))
		for _, v := range g.Values {
			items = append(items, valueTree(v))
		}
		members = append(members, pair(g.Section, array(items...)))
	}
	if len(c.Manufacturers) > 0 {
		names := make([]value, 0, len(c.Manufacturers))
		for _, name := range c.Manufacturers {
			names = append(names, str(name))
		}
		members = append(members, pair("manufacturers", array(names...)))
	}
	if len(c.Variants) > 0 {
		variants := make([]value, 0, len(c.Variants))
		for _, v := range c.Variants {
			variantPairs := []member{}
			if v.Label != "" {
				variantPairs = append(variantPairs, pair("label", str(v.Label)))
			}
			for _, g := range v.Groups {
				if len(g.Values) == 0 {
					continue
				}
				items := make([]value, 0, len(g.Values))
				for _, val := range g.Values {
					items = append(items, valueTree(val))
				}
				variantPairs = append(variantPairs, pair(g.Section, array(items...)))
			}
			variants = append(variants, object(variantPairs...))
		}
		members = append(members, pair("variants", array(variants...)))
	}
	if len(c.Analogs) > 0 {
		analogs := make([]value, 0, len(c.Analogs))
		for _, a := range c.Analogs {
			if a.Note == "" {
				analogs = append(analogs, str(a.Designation))
				continue
			}
			analogs = append(analogs, object(
				pair("name", str(a.Designation)),
				pair("note", str(a.Note)),
			))
		}
		members = append(members, pair("analogs", array(analogs...)))
	}
	return object(members...)
}

// explicitFields — явные классификационные поля записи: хранимые поля
// минус продукты разбора обозначения (импортёр запрещает совпадения,
// разность всегда определена).
func explicitFields(c *service.Card, parsed *domain.ParsedDesignation) []domain.Field {
	var out []domain.Field
	for _, f := range c.Fields {
		if _, parsed := parsed.FieldByName(f.Name); parsed {
			continue
		}
		out = append(out, f)
	}
	return out
}

// attrTreeValue — скаляр значения атрибута.
func attrTreeValue(a service.CardAttribute) value {
	switch {
	case a.Text != nil:
		return str(*a.Text)
	case a.Num != nil:
		return num(*a.Num)
	case a.Bool != nil:
		return boolean(*a.Bool)
	}
	return nullValue()
}

// valueTree — объект значения параметра: parameter, value|min|max|text,
// условия — соседние ключи по коду (порядок ключей — как в файле
// наполнения, docs/plan/06-examples.md).
func valueTree(v service.CardValue) value {
	members := []member{pair("parameter", str(v.Parameter))}
	if v.Exact != nil {
		members = append(members, pair("value", num(*v.Exact)))
	}
	if v.Min != nil {
		members = append(members, pair("min", num(*v.Min)))
	}
	if v.Max != nil {
		members = append(members, pair("max", num(*v.Max)))
	}
	if v.Text != nil {
		members = append(members, pair("text", str(*v.Text)))
	}
	for _, c := range v.Conditions {
		members = append(members, pair(c.Condition, num(c.Value)))
	}
	return object(members...)
}

// catalogTree строит дерево секции catalog из снимка (подразделы —
// в порядке catalogSubsections; пустые поля опускаются: чтение
// восстанавливает нулевые значения, is_active пишется всегда явно).
func catalogTree(snap *catalog.Snapshot) value {
	cat := value{kind: kindObject}

	kindRows := make([]value, 0, len(snap.Kinds))
	for _, k := range snap.Kinds {
		kindRows = append(kindRows, object(pair("code", str(string(k.Code)))))
	}
	cat.members = append(cat.members, pair("kinds", array(kindRows...)))

	sysRows := make([]value, 0, len(snap.Systems))
	for _, s := range snap.Systems {
		sysRows = append(sysRows, object(pair("code", str(string(s.Code)))))
	}
	if len(sysRows) > 0 {
		cat.members = append(cat.members, pair("designation_systems", array(sysRows...)))
	}

	skRows := make([]value, 0, len(snap.SystemKinds))
	for _, r := range snap.SystemKinds {
		skRows = append(skRows, object(
			pair("system", str(string(r.System))), pair("kind", str(string(r.Kind)))))
	}
	if len(skRows) > 0 {
		cat.members = append(cat.members, pair("designation_system_kinds", array(skRows...)))
	}

	famRows := make([]value, 0, len(snap.SeriesFamilies))
	for _, f := range snap.SeriesFamilies {
		members := []member{pair("series", str(f.Series)), pair("kind", str(string(f.Kind)))}
		if f.TailSemantic != "" {
			members = append(members, pair("tail_semantic", str(f.TailSemantic)))
		}
		famRows = append(famRows, object(members...))
	}
	if len(famRows) > 0 {
		cat.members = append(cat.members, pair("series_families", array(famRows...)))
	}

	unitRows := make([]value, 0, len(snap.Units))
	for _, u := range snap.Units {
		unitRows = append(unitRows, object(pair("code", str(u.Code))))
	}
	if len(unitRows) > 0 {
		cat.members = append(cat.members, pair("units", array(unitRows...)))
	}

	catRows := make([]value, 0, len(snap.Categories))
	for _, c := range snap.Categories {
		catRows = append(catRows, object(pair("code", str(c.Code))))
	}
	if len(catRows) > 0 {
		cat.members = append(cat.members, pair("categories", array(catRows...)))
	}

	condRows := make([]value, 0, len(snap.Conditions))
	for _, c := range snap.Conditions {
		members := []member{pair("code", str(c.Code))}
		if c.Unit != "" {
			members = append(members, pair("unit", str(c.Unit)))
		}
		if c.AllowNegative {
			members = append(members, pair("allow_negative", boolean(true)))
		}
		condRows = append(condRows, object(members...))
	}
	if len(condRows) > 0 {
		cat.members = append(cat.members, pair("conditions", array(condRows...)))
	}

	groupRows := make([]value, 0, len(snap.Groups))
	for _, g := range snap.Groups {
		members := []member{
			pair("code", str(g.Code)), pair("section", str(g.SectionName)),
		}
		if g.SortOrder != 0 {
			members = append(members, pair("sort_order", num(float64(g.SortOrder))))
		}
		groupRows = append(groupRows, object(members...))
	}
	if len(groupRows) > 0 {
		cat.members = append(cat.members, pair("parameter_groups", array(groupRows...)))
	}

	paramRows := make([]value, 0, len(snap.Parameters))
	for _, p := range snap.Parameters {
		paramRows = append(paramRows, parameterTree(&p))
	}
	if len(paramRows) > 0 {
		cat.members = append(cat.members, pair("parameters", array(paramRows...)))
	}

	attrRows := make([]value, 0, len(snap.Attributes))
	for _, a := range snap.Attributes {
		attrRows = append(attrRows, attributeTree(&a))
	}
	if len(attrRows) > 0 {
		cat.members = append(cat.members, pair("attributes", array(attrRows...)))
	}

	ruleRows := make([]value, 0, len(snap.Rules))
	for _, r := range snap.Rules {
		ruleRows = append(ruleRows, object(pair("code", str(r.Code))))
	}
	if len(ruleRows) > 0 {
		cat.members = append(cat.members, pair("validation_rules", array(ruleRows...)))
	}

	krRows := make([]value, 0, len(snap.KindRules))
	for _, r := range snap.KindRules {
		krRows = append(krRows, object(
			pair("kind", str(string(r.Kind))), pair("rule", str(r.Rule))))
	}
	if len(krRows) > 0 {
		cat.members = append(cat.members, pair("kind_validation_rules", array(krRows...)))
	}
	return cat
}

// parameterTree — строка подраздела parameters.
func parameterTree(p *catalog.ParameterDef) value {
	members := []member{
		pair("code", str(p.Code)), pair("group", str(p.Group)),
	}
	if p.Unit != "" {
		members = append(members, pair("unit", str(p.Unit)))
	}
	members = append(members, pair("value_type", str(string(p.ValueType))))
	if len(p.Kinds) > 0 {
		kinds := make([]value, 0, len(p.Kinds))
		for _, k := range p.Kinds {
			kinds = append(kinds, str(string(k)))
		}
		members = append(members, pair("kinds", array(kinds...)))
	}
	if len(p.EnumValues) > 0 {
		vals := make([]value, 0, len(p.EnumValues))
		for _, e := range p.EnumValues {
			vals = append(vals, str(e))
		}
		members = append(members, pair("enum_values", array(vals...)))
	}
	if len(p.ConditionSets) > 0 {
		sets := make([]value, 0, len(p.ConditionSets))
		for _, set := range p.ConditionSets {
			items := make([]value, 0, len(set.Items))
			for _, it := range set.Items {
				im := []member{pair("condition", str(it.Condition)), pair("mode", str(string(it.Mode)))}
				if it.FixedValue != nil {
					im = append(im, pair("fixed_value", num(*it.FixedValue)))
				}
				items = append(items, object(im...))
			}
			sets = append(sets, object(pair("items", array(items...))))
		}
		members = append(members, pair("condition_sets", array(sets...)))
	}
	if p.Ceiling != nil {
		members = append(members, pair("ceiling", num(*p.Ceiling)))
	}
	if p.AllowNegative {
		members = append(members, pair("allow_negative", boolean(true)))
	}
	if p.ValidationRule != "" {
		members = append(members, pair("rule", str(p.ValidationRule)))
	}
	if p.SortOrder != 0 {
		members = append(members, pair("sort_order", num(float64(p.SortOrder))))
	}
	members = append(members, pair("is_active", boolean(p.Active)))
	return object(members...)
}

// attributeTree — строка подраздела attributes.
func attributeTree(a *catalog.AttributeDef) value {
	members := []member{pair("code", str(a.Code))}
	if a.GroupName != "" {
		members = append(members, pair("group", str(a.GroupName)))
	}
	members = append(members, pair("type", str(string(a.Type))))
	if a.Unit != "" {
		members = append(members, pair("unit", str(a.Unit)))
	}
	if len(a.Kinds) > 0 {
		kinds := make([]value, 0, len(a.Kinds))
		for _, k := range a.Kinds {
			kinds = append(kinds, str(string(k)))
		}
		members = append(members, pair("kinds", array(kinds...)))
	}
	if len(a.EnumValues) > 0 {
		vals := make([]value, 0, len(a.EnumValues))
		for _, e := range a.EnumValues {
			vals = append(vals, str(e))
		}
		members = append(members, pair("enum_values", array(vals...)))
	}
	if a.ValidationRule != "" {
		members = append(members, pair("rule", str(a.ValidationRule)))
	}
	if a.SortOrder != 0 {
		members = append(members, pair("sort_order", num(float64(a.SortOrder))))
	}
	members = append(members, pair("is_active", boolean(a.Active)))
	return object(members...)
}

// Сериализаторы.

// writeJSONTree пишет дерево как JSON с отступами (2 пробела).
func writeJSONTree(w io.Writer, v value) error {
	var b strings.Builder
	emitJSON(&b, v, "")
	_, err := io.WriteString(w, b.String()+"\n")
	return err
}

// emitJSON — отступный JSON: объекты и массивы многострочно.
func emitJSON(b *strings.Builder, v value, indent string) {
	inner := indent + "  "
	switch v.kind {
	case kindObject:
		if len(v.members) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, m := range v.members {
			b.WriteString(inner)
			b.WriteString(jsonScalar(str(m.name)))
			b.WriteString(": ")
			emitJSON(b, m.value, inner)
			if i < len(v.members)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + "}")
	case kindArray:
		if len(v.items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, item := range v.items {
			b.WriteString(inner)
			emitJSON(b, item, inner)
			if i < len(v.items)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + "]")
	default:
		b.WriteString(jsonScalar(v))
	}
}

// writeNDJSONRoot пишет записи построчно: одна строка-обёртка
// {"<класс>": <запись>} на запись.
func writeNDJSONRoot(w io.Writer, root value) error {
	for _, sec := range root.members {
		for _, rec := range sec.value.items {
			if _, err := io.WriteString(w, jsonCompact(object(pair(sec.name, rec)))+"\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeNDJSONCatalog пишет каталог построчно: одна строка-обёртка
// {"catalog": {"<подраздел>": [...]}} на подраздел.
func writeNDJSONCatalog(w io.Writer, cat value) error {
	for _, sub := range cat.members {
		if _, err := io.WriteString(w, jsonCompact(object(
			pair("catalog", object(pair(sub.name, sub.value)))))+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// jsonCompact — компактный JSON одной строки.
func jsonCompact(v value) string {
	var b strings.Builder
	emitJSONCompact(&b, v)
	return b.String()
}

func emitJSONCompact(b *strings.Builder, v value) {
	switch v.kind {
	case kindObject:
		b.WriteByte('{')
		for i, m := range v.members {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(jsonScalar(str(m.name)))
			b.WriteByte(':')
			emitJSONCompact(b, m.value)
		}
		b.WriteByte('}')
	case kindArray:
		b.WriteByte('[')
		for i, item := range v.items {
			if i > 0 {
				b.WriteByte(',')
			}
			emitJSONCompact(b, item)
		}
		b.WriteByte(']')
	default:
		b.WriteString(jsonScalar(v))
	}
}

// writeYAMLTree пишет дерево в блочном YAML: ключи и plain-безопасные
// строки — без кавычек, прочие строки — в двойных кавычках (экранирование
// JSON-подмножества допустимо в YAML), числа/bool/null — как есть.
func writeYAMLTree(w io.Writer, v value) error {
	var b strings.Builder
	emitYAMLValue(&b, v, 0)
	_, err := io.WriteString(w, b.String())
	return err
}

func yamlPad(indent int) string { return strings.Repeat("  ", indent) }

// yamlPlain — допустимость строки как plain-скаляра YAML: без кавычек
// читается обратно строкой (не числом, не bool, не null) совместимыми
// парсерами. Первый символ — буква, цифра или подчёркивание, прочие
// символы — также точка и дефис (внутри); литералы true/false/null и
// слова YAML 1.1 yes/no/on/off/y/n (в любом регистре), записи,
// разбираемые как числа (включая 0x/0b/0o и знак), и nan/inf остаются
// в кавычках.
func yamlPlain(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		case c == '.' || c == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return false
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return false
	}
	switch strings.ToLower(s) {
	case "y", "n", "yes", "no", "on", "off", "true", "false", "null":
		return false
	}
	return true
}

// yamlScalar — скаляр YAML: plain-безопасные строки — без кавычек,
// прочее — JSON-текст (допустим в YAML в двойных кавычках).
func yamlScalar(v value) string {
	if v.kind == kindString && yamlPlain(v.str) {
		return v.str
	}
	return jsonScalar(v)
}

// emitYAMLValue — значение на своих строках с отступом indent
// (объект/массив), скаляр — «значение» без перевода строки.
func emitYAMLValue(b *strings.Builder, v value, indent int) {
	switch v.kind {
	case kindObject:
		for _, m := range v.members {
			b.WriteString(yamlPad(indent))
			emitYAMLMember(b, m, indent)
		}
	case kindArray:
		for _, item := range v.items {
			switch item.kind {
			case kindObject:
				if len(item.members) == 0 {
					b.WriteString(yamlPad(indent) + "- {}\n")
					continue
				}
				for j, m := range item.members {
					if j == 0 {
						b.WriteString(yamlPad(indent) + "- ")
					} else {
						b.WriteString(yamlPad(indent + 1))
					}
					emitYAMLMember(b, m, indent+1)
				}
			case kindArray:
				b.WriteString(yamlPad(indent) + "-\n")
				emitYAMLValue(b, item, indent+1)
			default:
				b.WriteString(yamlPad(indent) + "- " + yamlScalar(item) + "\n")
			}
		}
	default:
		b.WriteString(yamlScalar(v) + "\n")
	}
}

// emitYAMLMember — «ключ:» с продолжением значения.
func emitYAMLMember(b *strings.Builder, m member, indent int) {
	b.WriteString(yamlScalar(str(m.name)))
	b.WriteByte(':')
	switch m.value.kind {
	case kindObject:
		if len(m.value.members) == 0 {
			b.WriteString(" {}\n")
			return
		}
		b.WriteByte('\n')
		emitYAMLValue(b, m.value, indent+1)
	case kindArray:
		if len(m.value.items) == 0 {
			b.WriteString(" []\n")
			return
		}
		b.WriteByte('\n')
		emitYAMLValue(b, m.value, indent+1)
	default:
		b.WriteByte(' ')
		b.WriteString(yamlScalar(m.value))
		b.WriteByte('\n')
	}
}
