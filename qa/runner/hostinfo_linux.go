// Определение железа на Linux: /proc/cpuinfo, /proc/meminfo, os-release,
// версия ядра — /proc/sys/kernel/osrelease (uname -r).
package main

import (
	"os"
	"strconv"
	"strings"
)

func platformHost() platformInfo {
	return platformInfo{
		CPUModel: procValue("/proc/cpuinfo", "model name"),
		MemTotal: meminfoTotal(),
		OSName:   osReleaseName(),
		Kernel:   fileValue("/proc/sys/kernel/osrelease"),
	}
}

// fileValue — содержимое однострочного файла ядра (TrimSpace).
func fileValue(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// procValue — первое значение поля файла вида «ключ: значение» (/proc).
func procValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, found := strings.Cut(line, ":")
		if found && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// meminfoTotal — полная физическая память из MemTotal (/proc/meminfo, кБ).
func meminfoTotal() uint64 {
	fields := strings.Fields(procValue("/proc/meminfo", "MemTotal"))
	if len(fields) == 0 {
		return 0
	}
	kb, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return kb * 1024
}

// osReleaseName — PRETTY_NAME из os-release (freedesktop.org).
func osReleaseName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, found := strings.Cut(line, "=")
		if found && name == "PRETTY_NAME" {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}
