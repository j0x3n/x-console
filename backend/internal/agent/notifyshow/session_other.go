//go:build !windows

package notifyshow

import (
	"os/exec"
	"runtime"
)

func Available() bool {
	name := "notify-send"
	if runtime.GOOS == "darwin" {
		name = "osascript"
	}
	_, err := exec.LookPath(name)
	return (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && err == nil
}

func prepareCommand(*exec.Cmd) {}
