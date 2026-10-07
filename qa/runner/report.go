// Структурный журнал прогона QA — формат qa-log/1 (грамматика — qa/README.md).
// Пишется построчно с флешем на каждой записи: прерванный прогон оставляет
// читаемый частичный отчёт; одновременно строки дублируются на консоль.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// report — журнал прогона: секции, поля, проверки, заметки.
type report struct {
	f       *os.File
	w       *bufio.Writer
	mirror  io.Writer
	started time.Time
	passed  int
	failed  int
}

// newReport открывает файл отчёта и пишет заголовок формата.
func newReport(path string, mirror io.Writer) (*report, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	r := &report{f: f, w: bufio.NewWriter(f), mirror: mirror, started: time.Now()}
	r.line("# components-catalog QA report")
	r.kv("format", "qa-log/1")
	return r, nil
}

// line пишет строку в файл и на консоль.
func (r *report) line(s string) {
	fmt.Fprintln(r.w, s)
	_ = r.w.Flush()
	if r.mirror != nil {
		fmt.Fprintln(r.mirror, s)
	}
}

// section открывает секцию журнала.
func (r *report) section(name string) {
	r.line("")
	r.line("[" + name + "]")
}

// kv пишет поле «ключ: значение».
func (r *report) kv(key, value string) {
	if value == "" {
		value = "—"
	}
	r.line(fmt.Sprintf("%s: %s", key, value))
}

// note пишет заметку вне нумерации проверок.
func (r *report) note(text string) {
	r.line("note  " + text)
}

// check фиксирует проверку: статус, стабильный идентификатор и описание;
// строки деталей пишутся только при отказе (что именно разошлось).
func (r *report) check(id, desc string, ok bool, details ...string) bool {
	status := "ok   "
	if ok {
		r.passed++
	} else {
		status = "FAIL "
		r.failed++
	}
	r.line(fmt.Sprintf("%s%s — %s", status, id, desc))
	if !ok {
		for _, d := range details {
			if d = strings.TrimSpace(d); d != "" {
				r.line("      | " + strings.ReplaceAll(d, "\n", "\n      | "))
			}
		}
	}
	return ok
}

// envDetails — детали команды для отказа: код выхода и хвост вывода.
func envDetails(res cmdResult, tail int) []string {
	details := []string{fmt.Sprintf("exit=%d", res.code)}
	lines := strings.Split(strings.TrimRight(res.output, "\n"), "\n")
	if len(lines) > tail {
		lines = append([]string{fmt.Sprintf("… (%d строк вывода, последние %d)", len(lines), tail)}, lines[len(lines)-tail:]...)
	}
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			details = append(details, l)
		}
	}
	return details
}

// finish пишет итоговую секцию и закрывает файл; verdict — PASS/FAIL.
func (r *report) finish() string {
	verdict := "PASS"
	if r.failed > 0 {
		verdict = "FAIL"
	}
	r.section("summary")
	r.kv("checks", fmt.Sprintf("%d", r.passed+r.failed))
	r.kv("passed", fmt.Sprintf("%d", r.passed))
	r.kv("failed", fmt.Sprintf("%d", r.failed))
	r.kv("finished", time.Now().Format(time.RFC3339))
	r.kv("duration", time.Since(r.started).Round(time.Second).String())
	r.kv("verdict", verdict)
	_ = r.w.Flush()
	_ = r.f.Close()
	return verdict
}
