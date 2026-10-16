package service

import (
	"context"

	"github.com/billydos/components-catalog/internal/domain"
)

// DesignationService — обозначения: разбор с автодетектом и автодополнение
// по префиксу (docs/plan/04-module-functionality.md §1.1).
type DesignationService struct {
	app *App
}

// Parse разбирает обозначение с автодетектом класса и системы: строгие
// грамматики (gost/ost/pro/jedec/jis) — кодом, порядок попыток фиксирован
// планом 03 §2.4; системы без разбора (other) автодетекта не имеют.
func (s *DesignationService) Parse(ctx context.Context, text string) (domain.ParsedDesignation, error) {
	return domain.ParseDesignation(text)
}

// ParseForSystem разбирает обозначение с явной системой и необязательным
// классом (переопределяют автодетект; класс обязателен для other) —
// CLI parse --system/--kind.
func (s *DesignationService) ParseForSystem(ctx context.Context, text string,
	system domain.System, kind domain.Kind) (domain.ParsedDesignation, error) {
	return domain.ParseDesignationForSystem(text, system, kind)
}

// ParseWithCandidates разбирает обозначение (автодетект либо явная система
// с необязательным классом) и перечисляет всех кандидатов строгих систем
// в фиксированном порядке автодетекта — REST GET /api/v1/parse: перечень
// принявших обозначение систем заменяет числовой «конфиденс» строгих
// грамматик (первый кандидат при автодетекте совпадает с основным
// разбором). Явная система авторитетна: когда основной разбор удался,
// а перечисление кандидатов — нет (other вне строгих грамматик),
// кандидаты пусты.
func (s *DesignationService) ParseWithCandidates(ctx context.Context, text string,
	system domain.System, kind domain.Kind) (domain.ParsedDesignation, []domain.ParsedDesignation, error) {
	p, err := domain.ParseDesignationForSystem(text, system, kind)
	if err != nil {
		return domain.ParsedDesignation{}, nil, err
	}
	candidates, err := domain.ParseDesignationCandidates(text, kind)
	if err != nil {
		return p, nil, nil
	}
	return p, candidates, nil
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
			return nil, domain.NewErrorf(domain.CodeValidationFailed, domain.MsgKindUnknown, string(kind))
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
