//go:build linux

package syslog

import (
	"os"
	"os/exec"
)

// Tests change these.
var (
	journalctlName = "journalctl"
	textLogPaths   = []string{"/var/log/syslog", "/var/log/messages"}
)

// detect picks the journal when journalctl exists, or a syslog file (Unraid
// has /var/log/syslog and no journal, B64).
func detect() backend {
	if _, err := exec.LookPath(journalctlName); err == nil {
		return newJournal()
	}
	for _, path := range textLogPaths {
		if _, err := os.Stat(path); err == nil {
			return &textLog{path: path}
		}
	}
	return nil
}
