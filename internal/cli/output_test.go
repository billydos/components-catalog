package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/billydos/components-catalog/internal/cli"
	"github.com/billydos/components-catalog/internal/domain"
	"github.com/billydos/components-catalog/internal/i18n"
)

// Префиксы вывода ошибок CLI — строки каталога сообщений (этап 8.3):
// канонический en и ru-бандл.
func TestErrorPrefixes(t *testing.T) {
	if got := i18n.Message(i18n.En, "cli_err_prefix"); got != "Error: " {
		t.Errorf("en префикс ожидаемых ошибок изменился: %q", got)
	}
	if got := i18n.Message(i18n.Ru, "cli_err_prefix"); got != "Ошибка: " {
		t.Errorf("ru префикс ожидаемых ошибок изменился: %q", got)
	}
	if got := i18n.Message(i18n.En, "cli_unexpected_prefix"); got != "Unexpected error: " {
		t.Errorf("en префикс непредвиденных ошибок изменился: %q", got)
	}
	if got := i18n.Message(i18n.Ru, "cli_unexpected_prefix"); got != "Непредвиденная ошибка: " {
		t.Errorf("ru префикс непредвиденных ошибок изменился: %q", got)
	}
}

func TestPrintError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"ожидаемая",
			domain.NewErrorf(domain.CodeNotFound, domain.MsgCliRecordNotFound, "КТ315"),
			"Error: record «КТ315» not found\n",
		},
		{
			"завёрнутая ожидаемая",
			fmt.Errorf("import: %w", domain.NewErrorf(domain.CodeInvalidImportFile, domain.MsgImportDuplicateKey, "x", 1)),
			"Error: import: duplicate key «x» (line 1)\n",
		},
		{
			"непредвиденная",
			errors.New("сбой"),
			"Unexpected error: сбой\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			cli.PrintError(&b, i18n.En, tc.err)
			if b.String() != tc.want {
				t.Fatalf("вывод изменился:\n got:  %q\n want: %q", b.String(), tc.want)
			}
		})
	}
}

func TestPrintErrorNil(t *testing.T) {
	var b bytes.Buffer
	cli.PrintError(&b, i18n.En, nil)
	if b.Len() != 0 {
		t.Fatalf("при nil ошибки вывод должен быть пуст: %q", b.String())
	}
}
