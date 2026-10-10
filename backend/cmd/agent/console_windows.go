//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// The Windows agent is built as a GUI program (-H=windowsgui), so Windows
// never creates a console window for it, however it is started (task
// scheduler, double click, a shell). The cost is that it has no standard
// output of its own. setupConsole fixes that:
//   - started from a shell, it attaches to the shell's console, so "pair" and
//     "version" still print there;
//   - started by the task scheduler or Explorer, it writes the log to a file.
//
// Child programs have to be started with CREATE_NO_WINDOW, see package
// nowindow.

const (
	attachParentProcess = ^uint32(0)
	maxLogSize          = 1 << 20
)

var procAttachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

func setupConsole() {
	needOut := !validStdHandle(windows.STD_OUTPUT_HANDLE)
	needErr := !validStdHandle(windows.STD_ERROR_HANDLE)
	if !needOut && !needErr {
		return
	}
	if ok, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); ok != 0 {
		if needOut {
			if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
				os.Stdout = f
			}
		}
		if needErr {
			if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
				os.Stderr = f
			}
		}
		return
	}
	f := openLogFile()
	if f == nil {
		return
	}
	if needOut {
		os.Stdout = f
	}
	if needErr {
		os.Stderr = f
		// Go's own crash output goes to the process error handle.
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	}
}

func validStdHandle(id uint32) bool {
	h, err := windows.GetStdHandle(id)
	return err == nil && h != 0 && h != windows.InvalidHandle
}

// openLogFile opens %LOCALAPPDATA%\x-console-agent\agent.log. A log over 1 MB
// is moved to agent.log.old first, so the file never grows without limit.
func openLogFile() *os.File {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return nil
	}
	dir := filepath.Join(base, "x-console-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	path := filepath.Join(dir, "agent.log")
	if st, err := os.Stat(path); err == nil && st.Size() > maxLogSize {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
}
