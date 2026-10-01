//go:build linux

package svc

import (
	"path/filepath"
	"testing"
)

// B64: without systemd (Unraid) the agent does not report the services capability.
func TestNoSystemdNoServices(t *testing.T) {
	old := systemdDir
	t.Cleanup(func() { systemdDir = old })
	systemdDir = filepath.Join(t.TempDir(), "missing")
	if Available() {
		t.Fatal("services reported without systemd")
	}
}
