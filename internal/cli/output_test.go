package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/billydos/components-catalog/internal/cli"
	"github.com/billydos/components-catalog/internal/domain"
)

// Префиксы вывода ошибок CLI — дословный контракт
// (docs/plan/04-module-functionality.md §3).
func TestErrorPrefixes(t *testing.T) {
	if cli.ErrorPrefix != "Ошибка: " {
		t.Errorf("префикс ожидаемых ошибок изменился: %q", cli.ErrorPrefix)
	}
	if cli.UnexpectedErrorPrefix != "Непредвиденная ошибка: " {
		t.Errorf("префикс непредвиденных ошибок изменился: %q", cli.UnexpectedErrorPrefix)
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
			domain.NewError(domain.CodeNotFound, "запись не найдена"),
			"Ошибка: запись не найдена\n",
		},
		{
			"завёрнутая ожидаемая",
			fmt.Errorf("импорт: %w", domain.NewError(domain.CodeInvalidImportFile, "файл не разобран")),
			"Ошибка: импорт: файл не разобран\n",
		},
		{
			"непредвиденная",
			errors.New("сбой"),
			"Непредвиденная ошибка: сбой\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			cli.PrintError(&b, tc.err)
			if b.String() != tc.want {
				t.Fatalf("вывод изменился:\n got:  %q\n want: %q", b.String(), tc.want)
			}
		})
	}
}

func TestPrintErrorNil(t *testing.T) {
	var b bytes.Buffer
	cli.PrintError(&b, nil)
	if b.Len() != 0 {
		t.Fatalf("при nil ошибки вывод должен быть пуст: %q", b.String())
	}
}
