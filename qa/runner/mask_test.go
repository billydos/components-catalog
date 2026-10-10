package main

import (
	"strings"
	"testing"

	"github.com/billydos/components-catalog/internal/storage"
)

// Произвольный вывод с URL-формой DSN маскируется: пароль не остаётся
// в строке отчёта ни полностью, ни частично, остальной текст не меняется.
func TestMaskSecretsURLFormInArbitraryOutput(t *testing.T) {
	out := maskSecrets(
		"failed to connect to `postgres://qa:password@db.local:5432/catalog?sslmode=disable`: timeout",
		nil)
	if strings.Contains(out, "password") {
		t.Errorf("пароль DSN не маскирован: %s", out)
	}
	if !strings.Contains(out, "postgres://qa:***@db.local:5432/catalog?sslmode=disable") {
		t.Errorf("userinfo не заменён маской: %s", out)
	}
}

// Пароль, содержащий «@», выделяется по последнему «@» authority — как в
// storage.maskURLDSN.
func TestMaskSecretsURLFormSpecialPassword(t *testing.T) {
	out := maskSecrets("connect postgres://qa:p@ss@db.local/catalog failed", nil)
	if strings.Contains(out, "p@ss") {
		t.Errorf("пароль с «@» не замаскирован: %s", out)
	}
	if !strings.Contains(out, "postgres://qa:***@db.local/catalog") {
		t.Errorf("userinfo не заменён маской: %s", out)
	}
}

// Точное вхождение известного DSN заменяется маскированной формой целиком —
// включая ключевую форму, не распознаваемую по «схеме://».
func TestMaskSecretsKnownKeywordDSN(t *testing.T) {
	dsn := "host=db.local user=qa password=s3cret dbname=catalog"
	out := maskSecrets("connect "+dsn+" failed", []string{dsn})
	if strings.Contains(out, "s3cret") {
		t.Errorf("пароль ключевой формы не маскирован: %s", out)
	}
	if want := "connect " + storage.MaskDSN(dsn) + " failed"; out != want {
		t.Errorf("DSN заменён неточно:\n получено: %s\n ожидалось: %s", out, want)
	}
}

// URL без пароля и текст без DSN не меняются.
func TestMaskSecretsLeavesPlainURLs(t *testing.T) {
	for _, s := range []string{
		"",
		"GET /components?kind=transistor — 200",
		"https://example.com/path?q=1",
		"postgres://qa@db.local/catalog",
		"postgres://db.local/catalog",
	} {
		if out := maskSecrets(s, nil); out != s {
			t.Errorf("строка без пароля изменена: %q → %q", s, out)
		}
	}
}

// Маскировка тождественна повторному применению: уже замаскированный
// фрагмент («user:***@») не меняется.
func TestMaskSecretsIdempotent(t *testing.T) {
	dsn := "postgres://qa:password@db.local/catalog"
	once := maskSecrets("connect "+dsn+" failed", []string{dsn})
	if twice := maskSecrets(once, []string{dsn}); twice != once {
		t.Errorf("повторная маскировка изменила строку:\n %s\n %s", once, twice)
	}
}

// Граница отчёта: детали отказа с DSN в выводе команды маскируются в
// журнале (файл и консольное зеркало) — основной путь утечки секрета
// в закоммичаемый отчёт.
func TestReportMasksSecretsInDetails(t *testing.T) {
	r, mirror := newTestReport(t)
	r.setSecrets("postgres://qa:pw@db.local/catalog")
	r.check("a/leak", "вывод команды со строкой подключения", false,
		"exit=1", "connect postgres://qa:pw@db.local/catalog failed")
	s := mirror.String()
	if strings.Contains(s, ":pw@") {
		t.Errorf("пароль DSN в деталях отчёта:\n%s", s)
	}
	if !strings.Contains(s, "postgres://qa:***@db.local/catalog") {
		t.Errorf("нет маскированной формы DSN:\n%s", s)
	}
}
