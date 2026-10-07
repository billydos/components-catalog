//go:build !linux && !darwin && !windows

// Прочие платформы: только переносимые поля (GOOS/GOARCH/ядра).
package main

func platformHost() platformInfo {
	return platformInfo{}
}
