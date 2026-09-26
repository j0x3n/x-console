//go:build windows

package power

import (
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	powrprof            = windows.NewLazySystemDLL("powrprof.dll")
	procLockWorkStation = user32.NewProc("LockWorkStation")
	procSetSuspendState = powrprof.NewProc("SetSuspendState")
)

func available() bool { return true }

func doPower(action string) error {
	switch action {
	case protocol.PowerLock:
		if r, _, err := procLockWorkStation.Call(); r == 0 {
			return rpcutil.Failed("lock: %v", err)
		}
		return nil
	case protocol.PowerSleep:
		if err := procSetSuspendState.Find(); err != nil {
			return rpcutil.Unsupported("sleep")
		}
		// Suspend after answering, otherwise the reply never leaves the machine.
		go func() {
			time.Sleep(time.Second)
			procSetSuspendState.Call(0, 0, 0)
		}()
		return nil
	case protocol.PowerShutdown, protocol.PowerRestart:
		cmd := exec.Command("shutdown.exe", ShutdownArgs(action)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out, err := cmd.CombinedOutput(); err != nil {
			return rpcutil.Failed("shutdown: %v %s", err, out)
		}
		return nil
	}
	return rpcutil.BadParams("unknown action")
}

// open uses the shell's "open" verb, like double-clicking in Explorer. It
// handles programs, documents and URLs.
func open(target, args string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return rpcutil.BadParams("invalid target")
	}
	var argp *uint16
	if args != "" {
		if argp, err = windows.UTF16PtrFromString(args); err != nil {
			return rpcutil.BadParams("invalid args")
		}
	}
	if err := windows.ShellExecute(0, verb, file, argp, nil, windows.SW_SHOWNORMAL); err != nil {
		return rpcutil.FromOS(err)
	}
	return nil
}
