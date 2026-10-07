//go:build darwin

// Определение железа на macOS: sysctl (golang.org/x/sys/unix); версия
// ядра — XNU (kern.osrelease).
package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func platformHost() platformInfo {
	p := platformInfo{}
	p.CPUModel, _ = unix.Sysctl("machdep.cpu.brand_string")
	p.MemTotal, _ = unix.SysctlUint64("hw.memsize")
	if ver, err := unix.Sysctl("kern.osproductversion"); err == nil && ver != "" {
		p.OSName = fmt.Sprintf("macOS %s", ver)
	}
	if rel, err := unix.Sysctl("kern.osrelease"); err == nil && rel != "" {
		p.Kernel = fmt.Sprintf("XNU %s", rel)
	}
	return p
}
