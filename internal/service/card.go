package service

import (
	"context"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/storage"
)

// buildCard собирает карточку записи: поля разбора, атрибуты, значения
// параметров по группам каталога, исполнения (матрица номиналов),
// производители, исходящие и встречные ссылки-аналоги (D8).
func (s *DeviceService) buildCard(ctx context.Context, snap *catalog.Snapshot, dev *storage.DeviceRow) (*Card, error) {
	fields, err := s.app.db.LoadFields(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	attrRows, err := s.app.db.LoadAttributes(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	values, err := s.app.db.LoadValues(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	variants, err := s.app.db.LoadVariants(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	manufacturers, err := s.app.db.LoadManufacturers(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	outgoing, err := s.app.db.LoadOutgoingAnalogs(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	backlinks, err := s.app.db.LoadBacklinks(ctx, dev.ID)
	if err != nil {
		return nil, err
	}

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

	// Значения типа в целом и исполнений — по группам каталога.
	card.Groups = cardGroups(snap, values, nil)
	for _, v := range variants {
		vID := v.ID
		card.Variants = append(card.Variants, CardVariant{
			Label:  v.Label,
			Groups: cardGroups(snap, values, &vID),
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
	return card, nil
}

// cardGroups собирает значения параметров по группам каталога (порядок
// групп и параметров — каталог; значения — порядок строк). variantID = nil —
// данные типа в целом, иначе — значения исполнения.
func cardGroups(snap *catalog.Snapshot, values []storage.ValueRow, variantID *int64) []CardGroup {
	var groups []CardGroup
	for _, g := range snap.Groups {
		var vals []CardValue
		for _, vr := range values {
			if !sameVariant(vr.VariantID, variantID) {
				continue
			}
			def, ok := snap.Parameter(vr.Value.Parameter)
			if !ok || def.Group != g.Code {
				continue
			}
			vals = append(vals, CardValue{
				Parameter:  vr.Value.Parameter,
				Unit:       def.Unit,
				Exact:      vr.Value.Exact,
				Min:        vr.Value.Min,
				Max:        vr.Value.Max,
				Text:       vr.Value.Text,
				Conditions: condsFromStorage(vr.Value.Conditions),
			})
		}
		if len(vals) == 0 {
			continue
		}
		groups = append(groups, CardGroup{
			Code: g.Code, Section: g.SectionName, Values: vals,
		})
	}
	return groups
}

func sameVariant(a *int64, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
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
