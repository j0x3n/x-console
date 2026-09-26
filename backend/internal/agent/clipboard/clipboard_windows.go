//go:build windows

package clipboard

import (
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procIsClipboardFormatAvail = user32.NewProc("IsClipboardFormatAvailable")
	procGlobalAlloc            = kernel32.NewProc("GlobalAlloc")
	procGlobalFree             = kernel32.NewProc("GlobalFree")
	procGlobalLock             = kernel32.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32.NewProc("GlobalUnlock")
)

func available() bool { return true }

// pointer converts a memory handle returned by the Win32 API.
func pointer(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// open retries for a second because other programs hold the clipboard briefly.
func open() error {
	deadline := time.Now().Add(time.Second)
	for {
		r, _, err := procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return rpcutil.Failed("open clipboard: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func get() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := open(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()
	if r, _, _ := procIsClipboardFormatAvail.Call(cfUnicodeText); r == 0 {
		return "", nil
	}
	h, _, err := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", rpcutil.Failed("read clipboard: %v", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return "", rpcutil.Failed("lock clipboard data: %v", err)
	}
	text := windows.UTF16PtrToString((*uint16)(pointer(p)))
	procGlobalUnlock.Call(h)
	return text, nil
}

func set(text string) error {
	data, err := windows.UTF16FromString(text)
	if err != nil {
		return rpcutil.BadParams("%v", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := open(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return rpcutil.Failed("empty clipboard: %v", err)
	}
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)*2))
	if h == 0 {
		return rpcutil.Failed("allocate: %v", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return rpcutil.Failed("lock: %v", err)
	}
	copy(unsafe.Slice((*uint16)(pointer(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return rpcutil.Failed("write clipboard: %v", err)
	}
	return nil
}
