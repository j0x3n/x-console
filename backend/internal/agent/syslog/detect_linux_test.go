//go:build linux

package syslog

import (
	"os"
	"path/filepath"
	"testing"
)

// B64: Unraid has no journald. The agent reads /var/log/syslog instead.
func TestDetectFallsBackToSyslogFile(t *testing.T) {
	oldPaths, oldJournal := textLogPaths, journalctlName
	t.Cleanup(func() { textLogPaths, journalctlName = oldPaths, oldJournal })
	dir := t.TempDir()
	path := filepath.Join(dir, "syslog")
	if err := os.WriteFile(path, []byte("Oct  1 10:00:00 Tower kernel: md: array started\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	journalctlName = "journalctl-not-installed"
	textLogPaths = []string{filepath.Join(dir, "missing"), path}
	b, ok := detect().(*textLog)
	if !ok || b.path != path {
		t.Fatalf("detect: %#v", detect())
	}
	textLogPaths = []string{filepath.Join(dir, "missing")}
	if detect() != nil {
		t.Fatal("nothing to read should give no backend")
	}
}
