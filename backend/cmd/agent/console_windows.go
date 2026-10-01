//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procShowWindow            = user32.NewProc("ShowWindow")
)

const swHide = 0

// hideOwnConsole hides the black console window when the agent got one of
// its own, which is what happens when Task Scheduler starts it at logon.
// Started from an existing PowerShell or cmd window, other processes share
// the console and it stays visible. The console is hidden, not freed, so
// child programs (wevtutil, PowerShell) keep using it without opening new
// windows.
func hideOwnConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	var pids [4]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptrOf(&pids[0]), uintptr(len(pids)))
	if n != 1 {
		return
	}
	procShowWindow.Call(hwnd, swHide)
}

func uintptrOf(p *uint32) uintptr { return uintptr(unsafe.Pointer(p)) }
