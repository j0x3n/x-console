//go:build windows

package syslog

import "golang.org/x/sys/windows"

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procGetACP             = kernel32.NewProc("GetACP")
)

// outputCodePage is the code page console programs like wevtutil write in.
func outputCodePage() uint32 {
	if cp, _, _ := procGetConsoleOutputCP.Call(); cp != 0 {
		return uint32(cp)
	}
	cp, _, _ := procGetACP.Call()
	return uint32(cp)
}
