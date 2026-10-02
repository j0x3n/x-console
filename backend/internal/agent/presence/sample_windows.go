package presence

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

var presenceUser32 = windows.NewLazySystemDLL("user32.dll")
var getLastInput = presenceUser32.NewProc("GetLastInputInfo")
var tickCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")
var openInputDesktop = presenceUser32.NewProc("OpenInputDesktop")
var closeDesktop = presenceUser32.NewProc("CloseDesktop")
var desktopName = presenceUser32.NewProc("GetUserObjectInformationW")

type lastInput struct {
	Size uint32
	Time uint32
}

func Get(context.Context) Sample {
	var session uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session) != nil || session == 0 {
		return Sample{}
	}
	return sampleUserSession(session, func() (int64, bool) {
		input := lastInput{Size: uint32(unsafe.Sizeof(lastInput{}))}
		result, _, _ := getLastInput.Call(uintptr(unsafe.Pointer(&input)))
		if result == 0 {
			return 0, false
		}
		ticks, _, _ := tickCount.Call()
		return int64(uint32(ticks)-input.Time) / 1000, true
	}, windowsLocked)
}

func windowsLocked() *bool {
	desktop, _, callErr := openInputDesktop.Call(0, 0, 1)
	if desktop == 0 {
		if callErr == windows.ERROR_ACCESS_DENIED {
			return new(true)
		}
		return nil
	}
	defer closeDesktop.Call(desktop)
	var name [256]uint16
	var needed uint32
	result, _, _ := desktopName.Call(desktop, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed)))
	if result != 0 {
		return new(windows.UTF16ToString(name[:]) != "Default")
	}
	return nil
}
