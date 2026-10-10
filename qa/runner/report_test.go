package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// newTestReport — отчёт во временный файл с зеркалом строк в буфер.
func newTestReport(t *testing.T) (*report, *bytes.Buffer) {
	t.Helper()
	var mirror bytes.Buffer
	r, err := newReport(filepath.Join(t.TempDir(), "qa.log"), &mirror)
	if err != nil {
		t.Fatalf("newReport: %v", err)
	}
	t.Cleanup(func() { _ = r.f.Close() })
	return r, &mirror
}

// Отменённый прогон: прерванная отменой проверка — заметка «отменён», а не
// FAIL и не счётчик; в summary появляется поле cancelled: yes, вердикт
// без реальных отказов — PASS (код выхода 130 выбирает main).
func TestReportCancelledCheckSkipsFail(t *testing.T) {
	r, mirror := newTestReport(t)
	cancelled := false
	r.onCancel(func() bool { return cancelled })
	if !r.check("a/pass", "успешная до отмены", true) {
		t.Fatal("успешная проверка отмечена неуспешной")
	}
	cancelled = true
	if r.check("a/cmd", "прервана отменой", false, "exit=-1") {
		t.Fatal("отменённая проверка отмечена успешной")
	}
	if r.passed != 1 || r.failed != 0 {
		t.Fatalf("счётчики: passed=%d failed=%d (отменённая не считается проверкой)", r.passed, r.failed)
	}
	if verdict := r.finish(); verdict != "PASS" {
		t.Fatalf("вердикт отменённого без реальных отказов: %s", verdict)
	}
	s := mirror.String()
	if !strings.Contains(s, "note  a/cmd — прервана отменой: отменён") {
		t.Errorf("нет заметки отмены:\n%s", s)
	}
	if strings.Contains(s, "FAIL") {
		t.Errorf("ложный FAIL в отменённом прогоне:\n%s", s)
	}
	if !strings.Contains(s, "cancelled: yes") {
		t.Errorf("нет поля cancelled в summary:\n%s", s)
	}
}

// Отмена после последней проверки: поле cancelled проявляется в summary
// опросом источника отмены (isCancelled).
func TestReportCancelWithoutLateCheck(t *testing.T) {
	r, mirror := newTestReport(t)
	cancelled := false
	r.onCancel(func() bool { return cancelled })
	r.check("a/pass", "успешная", true)
	cancelled = true
	if !r.isCancelled() {
		t.Fatal("isCancelled не видит отмену источника")
	}
	r.finish()
	if !strings.Contains(mirror.String(), "cancelled: yes") {
		t.Errorf("нет поля cancelled в summary:\n%s", mirror.String())
	}
}

// Обычный прогон: отказы считаются как раньше, поля cancelled нет.
func TestReportFailCountUnchanged(t *testing.T) {
	r, mirror := newTestReport(t)
	r.check("a/fail", "отказ", false, "exit=1")
	if r.failed != 1 || r.passed != 0 {
		t.Fatalf("счётчики: passed=%d failed=%d", r.passed, r.failed)
	}
	if verdict := r.finish(); verdict != "FAIL" {
		t.Fatalf("вердикт: %s", verdict)
	}
	if s := mirror.String(); strings.Contains(s, "cancelled") {
		t.Errorf("поле cancelled в не отменённом прогоне:\n%s", s)
	}
}
