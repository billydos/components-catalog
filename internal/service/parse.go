package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/billydos/components-catalog/internal/catalog"
	"github.com/billydos/components-catalog/internal/domain"
)

// DesignationService — обозначения: разбор с автодетектом и автодополнение
// по префиксу (plan/04-module-functionality.md §1.1).
type DesignationService struct {
	app *App
}

// Parse разбирает обозначение с автодетектом класса и системы. Семейства
// системы series разрешаются по реестру каталога (данные series_families,
// расширяемые импортом), а не только по стартовому реестру домена.
func (s *DesignationService) Parse(ctx context.Context, text string) (domain.ParsedDesignation, error) {
	return s.parse(ctx, text, "", "")
}

// ParseForSystem разбирает обозначение с явной системой и необязательным
// классом (переопределяют автодетект; класс обязателен для other) над
// расширенным реестром семейств каталога — CLI parse --system/--kind.
func (s *DesignationService) ParseForSystem(ctx context.Context, text string,
	system domain.System, kind domain.Kind) (domain.ParsedDesignation, error) {
	return s.parse(ctx, text, system, kind)
}

// parse — разбор с необязательными явными системой и классом
// (переопределяют автодетект; класс обязателен для other).
func (s *DesignationService) parse(ctx context.Context, text string, system domain.System, kind domain.Kind) (domain.ParsedDesignation, error) {
	p, err := domain.ParseDesignationForSystem(text, system, kind)
	if err == nil {
		return p, nil
	}
	// Расширенный реестр семейств каталога: автодетект либо явная система
	// series со неизвестным стартовому реестру семейством.
	de, ok := domain.AsError(err)
	if !ok || de.Code != domain.CodeInvalidDesignation {
		return p, err
	}
	if system != "" && system != domain.SystemSeries {
		return p, err
	}
	snap, serr := s.app.cache.Snapshot(ctx)
	if serr != nil {
		return p, serr
	}
	canonical, cerr := domain.Canonicalize(text)
	if cerr != nil {
		return p, err
	}
	registry := seriesRegistry(snap)
	for _, f := range registry {
		if strings.HasPrefix(canonical, f.Series) {
			return domain.ParseSeriesWithRegistry(canonical, registry, kind)
		}
	}
	return p, err
}

// seriesRegistry строит реестр семейств домена из данных каталога.
func seriesRegistry(snap *catalog.Snapshot) []domain.SeriesFamily {
	out := make([]domain.SeriesFamily, 0, len(snap.SeriesFamilies))
	for _, f := range snap.SeriesFamilies {
		out = append(out, domain.SeriesFamily{
			Series: f.Series,
			Kind:   f.Kind,
			Name:   f.Name,
			Power:  f.TailSemantic == catalog.TailSemanticPower,
		})
	}
	return out
}

// Suggest возвращает обозначения по префиксу (канонизация префикса;
// подстрочный регистр не важен — хранятся канонические строки).
func (s *DesignationService) Suggest(ctx context.Context, prefix string, kind domain.Kind, limit int) ([]Suggestion, error) {
	canonical, err := domain.Canonicalize(prefix)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = SuggestDefaultLimit
	}
	if limit > SuggestLimitMax {
		limit = SuggestLimitMax
	}
	k := ""
	if kind != "" {
		snap, err := s.app.cache.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		if _, ok := snap.Kind(kind); !ok {
			return nil, domain.NewError(domain.CodeValidationFailed,
				fmt.Sprintf("неизвестный класс приборов «%s»", string(kind)))
		}
		k = string(kind)
	}
	rows, err := s.app.db.SuggestPrefix(ctx, canonical, k, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, 0, len(rows))
	for _, r := range rows {
		out = append(out, Suggestion{
			Kind: domain.Kind(r.Kind), System: domain.System(r.System), Designation: r.Designation,
		})
	}
	return out, nil
}
