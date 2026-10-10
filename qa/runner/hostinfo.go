// Определение окружения и железа прогона: переносимая часть (stdlib) плюс
// платформенные файлы hostinfo_<os>.go (Linux — /proc, macOS — sysctl,
// Windows — реестр и kernel32). Внешних зависимостей нет: golang.org/x/sys
// уже в графе модуля.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// hostInfo — железо и ОС прогона.
type hostInfo struct {
	Hostname string
	OS       string // GOOS + отображаемое имя ОС
	Arch     string
	Kernel   string // версия ядра ОС (linux — /proc/sys/kernel/osrelease)
	CPU      string // модель процессора
	Cores    int    // логические ядра
	Memory   string // полная физическая память
	Go       string
}

// platformInfo — платформозависимые данные детектора железа
// (заполняется функцией platformHost из hostinfo_<os>.go).
type platformInfo struct {
	CPUModel string
	MemTotal uint64
	OSName   string
	Kernel   string
}

// detectHost собирает окружение: переносимые поля + платформенный детектор.
func detectHost() hostInfo {
	p := platformHost()
	if p.OSName == "" {
		p.OSName = runtime.GOOS
	}
	os := runtime.GOOS
	if p.OSName != os {
		os = fmt.Sprintf("%s (%s)", os, p.OSName)
	}
	return hostInfo{
		Hostname: hostname(),
		OS:       os,
		Arch:     runtime.GOARCH,
		Kernel:   p.Kernel,
		CPU:      p.CPUModel,
		Cores:    runtime.NumCPU(),
		Memory:   formatBytes(p.MemTotal),
		Go:       runtime.Version(),
	}
}

// hostname — имя машины (недоступно — пустая строка).
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// formatBytes — память в двоичных единицах (ГиБ/ТиБ).
func formatBytes(b uint64) string {
	switch {
	case b == 0:
		return ""
	case b >= 1<<40:
		return fmt.Sprintf("%.1f TiB", float64(b)/(1<<40))
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// writeEnvSection — секция [environment] отчёта: железо, ОС, инструментарий,
// модуль и коммит, версии драйверов СУБД из build info.
func writeEnvSection(r *report, root string) {
	r.section("environment")
	h := detectHost()
	r.kv("started", time.Now().Format(time.RFC3339))
	r.kv("host", h.Hostname)
	r.kv("os", h.OS)
	r.kv("arch", h.Arch)
	r.kv("kernel", h.Kernel)
	r.kv("cpu", h.CPU)
	r.kv("cores", fmt.Sprintf("%d", h.Cores))
	r.kv("memory", h.Memory)
	r.kv("go", h.Go)
	mod, commit, dirty := vcsInfo(root)
	r.kv("module", mod)
	r.kv("commit", commit)
	if dirty {
		r.kv("worktree", "dirty")
	}
	for _, d := range driverVersions() {
		r.kv(d[0], d[1])
	}
	r.kv("repo_root", root)
}

// vcsInfo — модуль, коммит и грязнота рабочего дерева: build info, при
// отсутствии штампов vcs (go run) — git; unknown вне репозитория.
func vcsInfo(root string) (module, revision string, dirty bool) {
	module, revision = "unknown", "unknown"
	bi, ok := debug.ReadBuildInfo()
	if ok {
		module = bi.Main.Path
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}
	if revision != "unknown" {
		return
	}
	if res := runCmd(context.Background(), root, cmdTimeout,
		"git", "rev-parse", "HEAD"); res.code == 0 {
		revision = strings.TrimSpace(res.output)
	}
	if res := runCmd(context.Background(), root, cmdTimeout,
		"git", "status", "--porcelain"); res.code == 0 {
		dirty = strings.TrimSpace(res.output) != ""
	}
	return
}

// driverVersions — версии драйверов СУБД из зависимостей модуля.
func driverVersions() [][2]string {
	var out [][2]string
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return out
	}
	for _, dep := range bi.Deps {
		switch dep.Path {
		case "modernc.org/sqlite":
			out = append(out, [2]string{"sqlite_driver", dep.Path + " " + dep.Version})
		case "github.com/jackc/pgx/v5":
			out = append(out, [2]string{"postgres_driver", dep.Path + " " + dep.Version})
		}
	}
	return out
}

// trimSpaceLines — непустые строки вывода (для заметок фазы нагрузочной
// прикидки).
func trimSpaceLines(output string) []string {
	var out []string
	for _, l := range strings.Split(output, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
