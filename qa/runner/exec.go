// Помощники запуска внешних команд: go, собранные catalogctl/restsrv, gofmt.
// Логика прогона целиком в Go — никаких sh/bat/cmd (регламент qa/README.md).
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Таймауты фаз: сборка/статика и запросы сценарных прогонов.
const (
	buildTimeout   = 10 * time.Minute
	staticTimeout  = 30 * time.Minute
	cmdTimeout     = 2 * time.Minute
	loadTimeout    = 40 * time.Minute
	requestTimeout = 15 * time.Second
)

// cmdResult — итог внешней команды: код выхода (отрицательный — снята по
// таймауту/сигналу либо не запущена) и объединённый вывод stdout+stderr
// (ошибки запуска/таймаута дописываются в вывод последней строкой —
// так причина попадает в детали отказа отчёта).
type cmdResult struct {
	code   int
	output string
}

// runCmd выполняет команду в каталоге dir, объединяя stdout и stderr.
func runCmd(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) cmdResult {
	return runCmdEnv(ctx, dir, timeout, nil, name, args...)
}

// runCmdEnv — как runCmd с явным окружением дочернего процесса
// (nil — наследовать текущее).
func runCmdEnv(ctx context.Context, dir string, timeout time.Duration, env []string, name string, args ...string) cmdResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	res := cmdResult{code: exitCode(cmd.Run()), output: buf.String()}
	if ctx.Err() != nil {
		res.code = -1
		res.output += "\n[" + ctx.Err().Error() + "]"
	}
	return res
}

// runCmdOut — как runCmd, но stdout перенаправляется в файл outPath
// (экспорт catalogctl).
func runCmdOut(ctx context.Context, dir, outPath string, timeout time.Duration, name string, args ...string) cmdResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	f, err := os.Create(outPath)
	if err != nil {
		return cmdResult{code: -1, output: "[" + err.Error() + "]"}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = f
	var buf bytes.Buffer
	cmd.Stderr = &buf
	runErr := cmd.Run()
	res := cmdResult{code: exitCode(runErr), output: buf.String()}
	if res.code < 0 && runErr != nil {
		res.output += "\n[" + runErr.Error() + "]"
	}
	if err := f.Close(); err != nil && res.code == 0 {
		res.code, res.output = -1, res.output+"\n[close: "+err.Error()+"]"
	}
	return res
}

// envWithout — окружение без указанных переменных (для запуска дочерних
// процессов с перекрытым значением: дубликаты записей неоднозначны).
func envWithout(env []string, names ...string) []string {
	skip := make(map[string]bool, len(names))
	for _, n := range names {
		skip[n] = true
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !skip[name] {
			out = append(out, kv)
		}
	}
	return out
}

// exitCode — код выхода ошибки запуска (0 — успех, -1 — сигнал/таймаут).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// exeSuffix — расширение исполняемого файла текущей платформы.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// buildBinaries собирает catalogctl и restsrv во временный каталог прогона.
// Возвращает пути бинарников; ok=false — сборка не прошла (детали — в отчёте).
func buildBinaries(ctx context.Context, r *report, root, tmp string) (ctl, srv string, ok bool) {
	r.section("phase: build")
	ctl = filepath.Join(tmp, "catalogctl"+exeSuffix())
	srv = filepath.Join(tmp, "restsrv"+exeSuffix())
	builds := []struct {
		id, pkg, out string
	}{
		{"build/catalogctl", "./cmd/catalogctl", ctl},
		{"build/restsrv", "./qa/restsrv", srv},
	}
	ok = true
	for _, b := range builds {
		res := runCmd(ctx, root, buildTimeout, "go", "build", "-o", b.out, b.pkg)
		if !r.check(b.id, "go build "+b.pkg, res.code == 0, envDetails(res, 20)...) {
			ok = false
		}
	}
	return
}

// staticPhase — статические проверки репозитория: go build/vet, gofmt,
// go test (полный контур AGENTS.md «Проверка»). `go test` запускается без
// CATALOG_QA_SCALE: TestLoad — прерогатива выделенной фазы load, наследование
// переменной гоняло бы прикидку дважды. При postgres-ноге go test идёт
// с -p 1: одноразовая база CATALOG_TEST_POSTGRES_DSN одна на все пакеты,
// параллельные пакеты конфликтуют на DROP/CREATE таблиц (deadlock 40P01,
// гонка создания системных типов 23505); пакеты последовательны — тесты
// внутри пакета и так последовательны.
func staticPhase(ctx context.Context, r *report, root string, opts runOptions) {
	r.section("phase: static")
	res := runCmd(ctx, root, staticTimeout, "go", "build", "./...")
	r.check("static/build", "go build ./...", res.code == 0, envDetails(res, 20)...)
	res = runCmd(ctx, root, staticTimeout, "go", "vet", "./...")
	r.check("static/vet", "go vet ./...", res.code == 0, envDetails(res, 30)...)
	gofmtCheck(ctx, r, root)
	testArgs := []string{"test"}
	if opts.postgresDSN != "" {
		testArgs = append(testArgs, "-p", "1")
	}
	testArgs = append(testArgs, "./...")
	res = runCmdEnv(ctx, root, staticTimeout,
		envWithout(os.Environ(), "CATALOG_QA_SCALE"),
		"go", testArgs...)
	r.check("static/test", "go "+strings.Join(testArgs, " "),
		res.code == 0, envDetails(res, 60)...)
}

// gofmtCheck — gofmt -l . обязан быть пуст (путь к gofmt — из GOROOT).
func gofmtCheck(ctx context.Context, r *report, root string) {
	bin := filepath.Join(runtime.GOROOT(), "bin", "gofmt"+exeSuffix())
	if _, err := os.Stat(bin); err != nil {
		bin = "gofmt"
	}
	res := runCmd(ctx, root, cmdTimeout, bin, "-l", ".")
	files := trimSpaceLines(res.output)
	r.check("static/gofmt", "gofmt -l . — без изменений", res.code == 0 && len(files) == 0,
		envDetails(res, 30)...)
}

// loadPhase — нагрузочная прикидка (qa/load_test.go): запуск боевым
// `go test` с масштабом; строки измерений (p50/p95, скорость импорта)
// попадают в отчёт заметками.
// -count=1 отключает кэш результатов go test: прикидка — измерение,
// кэшированная реплика с теми же переменными окружения выдаёт старые
// результаты измерений за нулевое время (база одноразовая — повторного
// прогона нет).
func loadPhase(ctx context.Context, r *report, root string, opts runOptions) {
	r.section("phase: load")
	r.kv("scale", strconv.Itoa(opts.scale))
	ctx, cancel := context.WithTimeout(ctx, loadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./qa", "-run", "TestLoad",
		"-count=1", "-v", "-timeout", "30m")
	cmd.Dir = root
	// Наследованное CATALOG_QA_SCALE/CATALOG_QA_DIALECT заменяется явными
	// значениями прогона (дубликаты записей окружения неоднозначны);
	// -dialect переносится и на нагрузочную прикидку.
	env := append(envWithout(os.Environ(), "CATALOG_QA_SCALE", "CATALOG_QA_DIALECT"),
		"CATALOG_QA_SCALE="+strconv.Itoa(opts.scale))
	if opts.dialect != "" {
		env = append(env, "CATALOG_QA_DIALECT="+opts.dialect)
	}
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	runErr := cmd.Run()
	code := exitCode(runErr)
	if ctx.Err() != nil {
		code = -1
	}
	res := cmdResult{code: code, output: buf.String()}
	if code < 0 && runErr != nil {
		res.output += "\n[" + runErr.Error() + "]"
	}
	for _, l := range trimSpaceLines(res.output) {
		if strings.Contains(l, "p50=") || strings.Contains(l, "диалект=") {
			r.note(l)
		}
	}
	r.check("load/run", "go test ./qa -run TestLoad (масштаб "+strconv.Itoa(opts.scale)+")",
		res.code == 0, envDetails(res, 40)...)
}
