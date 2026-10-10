//go:build windows

package screentime

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	getForeground    = user32.NewProc("GetForegroundWindow")
	getWindowTextW   = user32.NewProc("GetWindowTextW")
	getWindowThread  = user32.NewProc("GetWindowThreadProcessId")
	processQueryInfo = uint32(0x1000) // PROCESS_QUERY_LIMITED_INFORMATION
)

// Available reports whether this system can read the foreground window.
func Available() bool { return true }

// foreground returns the program and title of the window the user is in.
// It fails when no window has the focus, or in a session that has no desktop.
func foreground() (Window, bool) {
	hwnd, _, _ := getForeground.Call()
	if hwnd == 0 {
		return Window{}, false
	}
	var pid uint32
	getWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return Window{}, false
	}
	app := imageName(pid)
	if app == "" {
		return Window{}, false
	}
	var title [512]uint16
	n, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	return Window{App: app, Title: windows.UTF16ToString(title[:n])}, true
}

func imageName(pid uint32) string {
	h, err := windows.OpenProcess(processQueryInfo, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH*2)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:size]))
}
