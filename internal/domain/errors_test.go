package domain_test

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/billydos/components-catalog/internal/domain"
)

// Реестр кодов ошибок — контракт: новый код добавляется осознанно,
// вместе со строкой этого теста.
func TestKnownCodes(t *testing.T) {
	want := []domain.Code{
		"already_exists",
		"attribute_not_applicable",
		"database_not_initialized",
		"designation_mismatch",
		"internal_error",
		"invalid_designation",
		"invalid_import_file",
		"kind_ambiguous",
		"kind_not_supported",
		"method_not_allowed",
		"not_found",
		"parameter_not_applicable",
		"power_suffix_not_supported",
		"schema_version_mismatch",
		"unknown_attribute",
		"unknown_condition",
		"unknown_parameter",
		"validation_failed",
	}
	got := domain.KnownCodes()
	if !slices.Equal(got, want) {
		t.Fatalf("реестр кодов изменился:\n got:  %v\n want: %v", got, want)
	}
}

func TestCodeFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for _, c := range domain.KnownCodes() {
		if !pattern.MatchString(string(c)) {
			t.Errorf("код %q не соответствует snake_case", string(c))
		}
		if !c.IsKnown() {
			t.Errorf("код %q отсутствует в реестре", string(c))
		}
	}
	if domain.Code("no_such_code").IsKnown() {
		t.Error("незарегистрированный код ошибочно признан известным")
	}
}

// Дословные тексты сообщений — контракт (docs/plan/01-architecture.md §2.1,
// docs/plan/02-database.md §5, docs/plan/03-data-model.md §2.4).
func TestContractMessages(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"schema_version_mismatch",
			domain.SchemaVersionMismatch(3, 7),
			"database was created by another module version (3 ≠ 7); recreate it: delete the file/database and run import",
		},
		{
			"database_not_initialized",
			domain.DatabaseNotInitialized(),
			"database is not initialized or is not a module database; run init (CLI) or EnsureCreated",
		},
		{
			"kind_not_supported",
			domain.KindNotSupported(),
			"designation belongs to a kind not supported by the module",
		},
		{
			"kind_ambiguous",
			domain.KindAmbiguous(),
			"device kind is not determined unambiguously by the designation; specify the kind explicitly",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("дословный текст нарушен:\n got:  %s\n want: %s", got, tc.want)
			}
		})
	}
}

func TestAsError(t *testing.T) {
	base := domain.NewErrorf(domain.CodeInvalidDesignation, domain.MsgEmptyDesignation)
	if got, ok := domain.AsError(base); !ok || got != base {
		t.Fatal("ожидаемая ошибка не распознана")
	}
	wrapped := fmt.Errorf("импорт: %w", base)
	if got, ok := domain.AsError(wrapped); !ok || got != base {
		t.Fatal("завёрнутая ожидаемая ошибка не распознана")
	}
	if _, ok := domain.AsError(errors.New("прочая")); ok {
		t.Fatal("прочая ошибка ошибочно распознана как ожидаемая")
	}
	if _, ok := domain.AsError(nil); ok {
		t.Fatal("nil ошибочно распознан как ожидаемая ошибка")
	}
}

func TestErrorReturnsMessageOnly(t *testing.T) {
	e := domain.NewErrorf(domain.CodeNotFound, domain.MsgCliRecordNotFound, "КТ315")
	if got := e.Error(); got != "record «КТ315» not found" {
		t.Fatalf("Error() должен возвращать только текст сообщения: %q", got)
	}
	if e.Code != domain.CodeNotFound {
		t.Fatalf("код не сохранён: %q", string(e.Code))
	}
}
