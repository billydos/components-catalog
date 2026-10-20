package service

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// buildCards собирает карточки записей по списку стабильных id одним
// пакетным чтением дочерних таблиц в общей транзакции чтения (один
// снимок данных — конкурентная запись не даёт карточку со смешанным
// состоянием): поля разбора, атрибуты, значения параметров по группам
// каталога, исполнения (матрица номиналов), производители, исходящие
// и встречные ссылки-аналоги (D8). Порядок результата — по порядку id
// входа; параллельно удалённые записи пропускаются.
func (s *DeviceService) buildCards(ctx context.Context, snap *catalog.Snapshot, ids []int64) ([]*Card, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	tx, err := s.app.db.BeginReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck — откат транзакции чтения
	devs, err := tx.FindDevicesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	fields, err := tx.LoadFieldsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	attrRows, err := tx.LoadAttributesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	values, err := tx.LoadValuesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	variants, err := tx.LoadVariantsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	manufacturers, err := tx.LoadManufacturersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	outgoing, err := tx.LoadOutgoingAnalogsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	backlinks, err := tx.LoadBacklinksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	synonyms, err := tx.LoadSynonymsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*storage.DeviceRow, len(devs))
	for _, dev := range devs {
		byID[dev.ID] = dev
	}
	// Группы типов: соседние записи по базовому обозначению (поле base)
	// и их данные типа в целом — мини-матрица различий.
	bases := typeBases(ids, byID, fields)
	siblings, err := tx.FindTypeSiblings(ctx, bases)
	if err != nil {
		return nil, err
	}
	var sibValues map[int64][]storage.ValueRow
	if sibIDs := siblingIDs(byID, siblings); len(sibIDs) > 0 {
		sibValues, err = tx.LoadValuesByIDs(ctx, sibIDs)
		if err != nil {
			return nil, err
		}
	}
	cards := make([]*Card, 0, len(ids))
	for _, id := range ids {
		dev, ok := byID[id]
		if !ok {
			continue // параллельное удаление — запись уже не собирается
		}
		cards = append(cards, assembleCard(snap, dev, fields[id], attrRows[id],
			values[id], variants[id], manufacturers[id], outgoing[id], backlinks[id], synonyms[id]))
	}
	for _, c := range cards {
		attachTypeGroup(snap, c, fields[c.ID], values[c.ID], siblings, sibValues)
	}
	return cards, nil
}

// typeBases — базовые обозначения (поле base) собираемых записей без
// повторов, в порядке входа.
func typeBases(ids []int64, byID map[int64]*storage.DeviceRow, fields map[int64][]domain.Field) []string {
	seen := make(map[string]bool, len(ids))
	var out []string
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			continue
		}
		if base, ok := baseOf(fields[id]); ok && !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
	}
	return out
}

// siblingIDs — идентификаторы соседних записей групп типов (без самих
// собираемых записей — их значения уже прочитаны).
func siblingIDs(byID map[int64]*storage.DeviceRow, siblings map[string][]storage.TypeSiblingRow) []int64 {
	seen := make(map[int64]bool)
	var out []int64
	for _, rows := range siblings {
		for _, r := range rows {
			if _, requested := byID[r.ID]; requested || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r.ID)
		}
	}
	return out
}

// baseOf — базовое обозначение (тип) из хранимых полей записи.
func baseOf(fields []domain.Field) (string, bool) {
	for _, f := range fields {
		if f.Name == domain.FieldBase && !f.IsNum {
			return f.Text, true
		}
	}
	return "", false
}

// typeMember — член группы типа: обозначение и данные типа в целом.
type typeMember struct {
	designation string
	values      map[string]CardValue
}

// typeLevelValues — значения параметров типа в целом (variant_id NULL)
// с единицами каталога; неизвестные каталогу параметры пропускаются.
func typeLevelValues(snap *catalog.Snapshot, values []storage.ValueRow) map[string]CardValue {
	out := make(map[string]CardValue)
	for _, vr := range values {
		if vr.VariantID != nil {
			continue
		}
		def, ok := snap.Parameter(vr.Value.Parameter)
		if !ok {
			continue
		}
		out[vr.Value.Parameter] = CardValue{
			Parameter:  vr.Value.Parameter,
			Unit:       def.Unit,
			Exact:      vr.Value.Exact,
			Min:        vr.Value.Min,
			Max:        vr.Value.Max,
			Text:       vr.Value.Text,
			Conditions: condsFromStorage(vr.Value.Conditions),
		}
	}
	return out
}

// attachTypeGroup собирает блок «группа типа» карточки: соседние записи
// того же класса с тем же базовым обозначением и мини-матрица различий
// данных типа в целом (в матрицу входят только различающиеся параметры;
// порядок записей — по обозначению, параметров — каталог). Без соседей
// блока нет.
func attachTypeGroup(snap *catalog.Snapshot, c *Card,
	fields []domain.Field, values []storage.ValueRow,
	siblings map[string][]storage.TypeSiblingRow, sibValues map[int64][]storage.ValueRow) {
	base, ok := baseOf(fields)
	if !ok {
		return
	}
	members := []typeMember{{designation: c.Designation, values: typeLevelValues(snap, values)}}
	sib := make([]CardTypeSibling, 0, len(siblings[base]))
	for _, r := range siblings[base] {
		if r.Kind != string(c.Kind) || r.ID == c.ID {
			continue
		}
		sib = append(sib, CardTypeSibling{
			ID: r.ID, System: domain.System(r.System), Designation: r.Designation,
		})
		members = append(members, typeMember{
			designation: r.Designation, values: typeLevelValues(snap, sibValues[r.ID]),
		})
	}
	if len(sib) == 0 {
		return
	}
	// Порядок соседей — по обозначению в Go: коллация СУБД (sqlite BINARY,
	// postgres — базы) не влияет на ответ между диалектами.
	slices.SortStableFunc(sib, func(a, b CardTypeSibling) int { return strings.Compare(a.Designation, b.Designation) })
	sort.SliceStable(members, func(i, j int) bool { return members[i].designation < members[j].designation })
	c.TypeGroup = &CardTypeGroup{Base: base, Siblings: sib}
	for _, p := range snap.Parameters {
		first, hasFirst := members[0].values[p.Code]
		differ := false
		for _, m := range members[1:] {
			v, has := m.values[p.Code]
			if has != hasFirst || (has && !cardValueEqual(first, v)) {
				differ = true
				break
			}
		}
		if !differ {
			continue
		}
		diff := CardTypeDiff{Parameter: p.Code, Unit: p.Unit}
		for _, m := range members {
			if v, has := m.values[p.Code]; has {
				diff.Values = append(diff.Values, CardTypeValue{Designation: m.designation, Value: v})
			}
		}
		c.TypeGroup.Diffs = append(c.TypeGroup.Diffs, diff)
	}
}

// cardValueEqual — побитовое равенство значений (числа — разряд REAL,
// текст и условия — точно; условия сравниваются канонически по коду).
func cardValueEqual(a, b CardValue) bool {
	if !numPtrEqual(a.Exact, b.Exact) || !numPtrEqual(a.Min, b.Min) ||
		!numPtrEqual(a.Max, b.Max) || !strPtrEqual(a.Text, b.Text) {
		return false
	}
	if len(a.Conditions) != len(b.Conditions) {
		return false
	}
	if len(a.Conditions) == 0 {
		return true
	}
	ac := append([]catalog.ConditionValue(nil), a.Conditions...)
	bc := append([]catalog.ConditionValue(nil), b.Conditions...)
	byCode := func(s []catalog.ConditionValue) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].Condition < s[j].Condition })
	}
	byCode(ac)
	byCode(bc)
	for i := range ac {
		if ac[i].Condition != bc[i].Condition || ac[i].Value != bc[i].Value {
			return false
		}
	}
	return true
}

// assembleCard собирает карточку из считанных строк (без обращений к БД).
func assembleCard(snap *catalog.Snapshot, dev *storage.DeviceRow,
	fields []domain.Field, attrRows []storage.AttrRow, values []storage.ValueRow,
	variants []storage.VariantRow, manufacturers []string,
	outgoing []storage.AnalogRow, backlinks []storage.BacklinkRow,
	synonyms []storage.SynonymRow) *Card {
	card := &Card{
		ID:            dev.ID,
		Kind:          dev.Kind,
		System:        dev.System,
		Designation:   dev.Designation,
		Fields:        fields,
		Manufacturers: manufacturers,
	}
	// Атрибуты — в порядке каталога.
	byAttr := make(map[string]storage.AttrRow, len(attrRows))
	for _, a := range attrRows {
		byAttr[a.Attribute] = a
	}
	for _, def := range snap.Attributes {
		row, ok := byAttr[def.Code]
		if !ok {
			continue
		}
		card.Attributes = append(card.Attributes, CardAttribute{
			Code: def.Code, Text: row.Text, Num: row.Num, Bool: row.Bool,
		})
	}

	// Значения типа в целом и исполнений — по группам каталога
	// (предгруппировка — один проход по значениям).
	grouped := groupCardValues(snap, values)
	card.Groups = cardGroups(snap, grouped, nil)
	for _, v := range variants {
		vID := v.ID
		card.Variants = append(card.Variants, CardVariant{
			Label:  v.Label,
			Groups: cardGroups(snap, grouped, &vID),
		})
	}

	for _, a := range outgoing {
		card.Analogs = append(card.Analogs, CardLink{
			Kind: dev.Kind, System: a.System, Designation: a.Designation, Note: a.Note,
		})
	}
	for _, b := range backlinks {
		card.Backlinks = append(card.Backlinks, CardLink{
			Kind: b.Kind, System: b.System, Designation: b.Designation, Note: b.Note,
		})
	}
	// Исторические названия — порядок sort_order (канонический —
	// по обозначению).
	slices.SortStableFunc(synonyms, func(a, b storage.SynonymRow) int {
		return strings.Compare(a.Designation, b.Designation)
	})
	for _, s := range synonyms {
		card.Synonyms = append(card.Synonyms, CardSynonym{
			Designation: s.Designation, Note: s.Note, YearFrom: s.YearFrom, YearTo: s.YearTo,
		})
	}
	return card
}

// variantKey — ключ предгруппировки значений: данные типа в целом
// (set = false) либо значения исполнения с идентификатором id.
type variantKey struct {
	id  int64
	set bool
}

func variantKeyOf(v *int64) variantKey {
	if v == nil {
		return variantKey{}
	}
	return variantKey{id: *v, set: true}
}

// groupCardValues группирует значения по (исполнение, группа каталога)
// одним проходом; группа — из определения параметра, неизвестные каталогу
// параметры пропускаются. Порядок значений внутри группы — порядок строк.
func groupCardValues(snap *catalog.Snapshot, values []storage.ValueRow) map[variantKey]map[string][]CardValue {
	out := make(map[variantKey]map[string][]CardValue)
	for _, vr := range values {
		def, ok := snap.Parameter(vr.Value.Parameter)
		if !ok {
			continue
		}
		key := variantKeyOf(vr.VariantID)
		byGroup := out[key]
		if byGroup == nil {
			byGroup = make(map[string][]CardValue)
			out[key] = byGroup
		}
		byGroup[def.Group] = append(byGroup[def.Group], CardValue{
			Parameter:  vr.Value.Parameter,
			Unit:       def.Unit,
			Exact:      vr.Value.Exact,
			Min:        vr.Value.Min,
			Max:        vr.Value.Max,
			Text:       vr.Value.Text,
			Conditions: condsFromStorage(vr.Value.Conditions),
		})
	}
	return out
}

// cardGroups собирает значения параметров по группам каталога из
// предгруппировки (порядок групп и параметров — каталог; значения —
// порядок строк). variantID = nil — данные типа в целом, иначе — значения
// исполнения.
func cardGroups(snap *catalog.Snapshot, grouped map[variantKey]map[string][]CardValue, variantID *int64) []CardGroup {
	byGroup := grouped[variantKeyOf(variantID)]
	if len(byGroup) == 0 {
		return nil
	}
	var groups []CardGroup
	for _, g := range snap.Groups {
		vals := byGroup[g.Code]
		if len(vals) == 0 {
			continue
		}
		groups = append(groups, CardGroup{
			Code: g.Code, Section: g.SectionName, Values: vals,
		})
	}
	return groups
}

func condsFromStorage(conds []storage.Cond) []catalog.ConditionValue {
	if len(conds) == 0 {
		return nil
	}
	out := make([]catalog.ConditionValue, 0, len(conds))
	for _, c := range conds {
		out = append(out, catalog.ConditionValue{Condition: c.Code, Value: c.Value})
	}
	return out
}

// Find ищет запись по точному обозначению; порядок разрешения — точный
// ключ записи, затем историческое название-синоним (план 03 §9:
// обозначение-синоним не является записью, попадание помечается
// MatchedBySynonym), затем подсказки (равнозначная по материалу запись
// полупроводников gost — Г/1, К/2, физически равнозначные символы,
// записи раздельны; сосед по типу). Разбор обозначения для подсказок
// выполняется после разрешения: точное попадание и синоним не требуют
// разбираемой грамматики (записи other).
func (s *DeviceService) Find(ctx context.Context, kind domain.Kind, designation string) (FindResult, error) {
	card, found, err := s.Get(ctx, kind, designation)
	if err != nil {
		return FindResult{}, err
	}
	if found {
		return FindResult{Found: card}, nil
	}
	canonical, err := domain.Canonicalize(designation)
	if err != nil {
		return FindResult{}, err
	}
	res, err := s.findBySynonym(ctx, kind, canonical)
	if err != nil || res.Found != nil {
		return res, err
	}
	p, err := s.app.designations.ParseForSystem(ctx, designation, "", kind)
	if err != nil {
		return FindResult{}, err
	}
	if res.Suggestion, err = s.suggestEquivalent(ctx, p); err != nil {
		return FindResult{}, err
	}
	if res.Suggestion == nil {
		if res.TypeSuggestion, err = s.suggestTypeSibling(ctx, p); err != nil {
			return FindResult{}, err
		}
	}
	return res, nil
}

// findBySynonym ищет запись по историческому названию: владелец синонима
// читается по каноническому обозначению (kind == "" — любой класс),
// карточка собирается пакетным чтением.
func (s *DeviceService) findBySynonym(ctx context.Context, kind domain.Kind, canonical string) (FindResult, error) {
	owner, err := s.app.db.FindSynonymOwner(ctx, kind, canonical)
	if err != nil {
		return FindResult{}, err
	}
	if owner == nil {
		return FindResult{}, nil
	}
	snap, err := s.app.cache.Snapshot(ctx)
	if err != nil {
		return FindResult{}, err
	}
	cards, err := s.buildCards(ctx, snap, []int64{owner.DeviceID})
	if err != nil {
		return FindResult{}, err
	}
	if len(cards) == 0 {
		return FindResult{}, nil // параллельное удаление владельца синонима
	}
	return FindResult{Found: cards[0], MatchedBySynonym: canonical}, nil
}

// suggestEquivalent ищет равнозначную по материалу запись: замена первого
// символа обозначения парным (КТ312 ↔ 2Т312) — только gost-полупроводники.
func (s *DeviceService) suggestEquivalent(ctx context.Context, p domain.ParsedDesignation) (*Card, error) {
	if p.System != domain.SystemGost {
		return nil, nil
	}
	if p.Kind != domain.KindTransistor && p.Kind != domain.KindDiode {
		return nil, nil
	}
	runes := []rune(p.Designation)
	if len(runes) < 2 {
		return nil, nil
	}
	pair, ok := domain.GostEquivalentSymbol(runes[0])
	if !ok {
		return nil, nil
	}
	runes[0] = pair
	card, found, err := s.Get(ctx, p.Kind, string(runes))
	if err != nil || !found {
		return nil, err
	}
	return card, nil
}

// suggestTypeSibling подсказывает запись группы типа: обозначение не
// найдено, но тип (базовое обозначение) существует с другой буквой
// классификации либо хвостовым элементом (КТ315А → КТ315Б); кандидат —
// первый по обозначению.
func (s *DeviceService) suggestTypeSibling(ctx context.Context, p domain.ParsedDesignation) (*Card, error) {
	base, ok := p.FieldByName(domain.FieldBase)
	if !ok {
		return nil, nil
	}
	siblings, err := s.app.db.FindTypeSiblings(ctx, []string{base.Text})
	if err != nil {
		return nil, err
	}
	for _, r := range siblings[base.Text] {
		if r.Kind != string(p.Kind) || r.Designation == p.Designation {
			continue
		}
		card, found, err := s.Get(ctx, p.Kind, r.Designation)
		if err != nil {
			return nil, err
		}
		if !found {
			continue // сосед удалён между запросом и чтением — следующий
		}
		return card, nil
	}
	return nil, nil
}
