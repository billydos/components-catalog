package service

import (
	"context"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// buildCards собирает карточки записей по списку стабильных id одним
// пакетным чтением дочерних таблиц (по SELECT с device_id IN на таблицу —
// экспорт полной базы без запросов на каждую запись): поля разбора,
// атрибуты, значения параметров по группам каталога, исполнения (матрица
// номиналов), производители, исходящие и встречные ссылки-аналоги (D8).
// Порядок результата — по порядку id входа; параллельно удалённые записи
// пропускаются.
func (s *DeviceService) buildCards(ctx context.Context, snap *catalog.Snapshot, ids []int64) ([]*Card, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	devs, err := s.app.db.FindDevicesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	fields, err := s.app.db.LoadFieldsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	attrRows, err := s.app.db.LoadAttributesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	values, err := s.app.db.LoadValuesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	variants, err := s.app.db.LoadVariantsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	manufacturers, err := s.app.db.LoadManufacturersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	outgoing, err := s.app.db.LoadOutgoingAnalogsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	backlinks, err := s.app.db.LoadBacklinksByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*storage.DeviceRow, len(devs))
	for _, dev := range devs {
		byID[dev.ID] = dev
	}
	cards := make([]*Card, 0, len(ids))
	for _, id := range ids {
		dev, ok := byID[id]
		if !ok {
			continue // параллельное удаление — запись уже не собирается
		}
		cards = append(cards, assembleCard(snap, dev, fields[id], attrRows[id],
			values[id], variants[id], manufacturers[id], outgoing[id], backlinks[id]))
	}
	return cards, nil
}

// assembleCard собирает карточку из считанных строк (без обращений к БД).
func assembleCard(snap *catalog.Snapshot, dev *storage.DeviceRow,
	fields []domain.Field, attrRows []storage.AttrRow, values []storage.ValueRow,
	variants []storage.VariantRow, manufacturers []string,
	outgoing []storage.AnalogRow, backlinks []storage.BacklinkRow) *Card {
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

// SourceKind не используется: класс источника хранится прямо в BacklinkRow.

// Find ищет запись по точному обозначению; при отсутствии совпадения у
// полупроводников gost подсказывает равнозначную по материалу запись
// (Г/1, К/2 — физически равнозначные символы, записи раздельны:
// docs/plan/01-architecture.md §2.1).
func (s *DeviceService) Find(ctx context.Context, kind domain.Kind, designation string) (FindResult, error) {
	p, err := s.app.designations.parse(ctx, designation, "", kind)
	if err != nil {
		return FindResult{}, err
	}
	card, found, err := s.Get(ctx, p.Kind, p.Designation)
	if err != nil {
		return FindResult{}, err
	}
	if found {
		return FindResult{Found: card}, nil
	}
	var res FindResult
	res.Suggestion, err = s.suggestEquivalent(ctx, p)
	if err != nil {
		return FindResult{}, err
	}
	return res, nil
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
