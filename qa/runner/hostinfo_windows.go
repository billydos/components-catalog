//go:build windows

// Определение железа на Windows: реестр (модель CPU, издание ОС, номер
// сборки ядра NT) и GlobalMemoryStatusEx из kernel32.
package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func platformHost() platformInfo {
	p := platformInfo{
		CPUModel: registryString(
			`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString"),
		OSName: windowsEdition(),
		Kernel: ntKernelBuild(),
	}
	p.MemTotal = globalMemoryTotal()
	return p
}

// registryString — строковое значение реестра HKLM.
func registryString(path, value string) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close() //nolint:errcheck — закрытие ключа только для чтения
	s, _, err := k.GetStringValue(value)
	if err != nil {
		return ""
	}
	return s
}

// windowsEdition — издание ОС из CurrentVersion (ProductName + DisplayVersion).
func windowsEdition() string {
	const key = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	name := registryString(key, "ProductName")
	ver := registryString(key, "DisplayVersion")
	switch {
	case name == "":
		return ""
	case ver == "":
		return name
	default:
		return fmt.Sprintf("%s %s", name, ver)
	}
}

// ntKernelBuild — номер сборки ядра NT: CurrentBuildNumber (+ UBR при
// наличии), например «NT 26100.1742».
func ntKernelBuild() string {
	const key = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	build := registryString(key, "CurrentBuildNumber")
	if build == "" {
		return ""
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, key, registry.QUERY_VALUE); err == nil {
		defer k.Close() //nolint:errcheck — закрытие ключа только для чтения
		if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
			return fmt.Sprintf("NT %s.%d", build, ubr)
		}
	}
	return "NT " + build
}

// memoryStatusEx — MEMORYSTATUSEX (GlobalMemoryStatusEx, kernel32).
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// globalMemoryTotal — полная физическая память.
func globalMemoryTotal() uint64 {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	stat := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	_, _, err := proc.Call(uintptr(unsafe.Pointer(&stat)))
	if errno, ok := err.(syscall.Errno); ok && errno != 0 {
		return 0
	}
	return stat.TotalPhys
}
