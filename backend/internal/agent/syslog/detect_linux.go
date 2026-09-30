//go:build linux

package syslog

import (
	"os"
	"os/exec"
)

// detect picks the journal when journalctl exists, or a syslog file.
func detect() backend {
	if _, err := exec.LookPath("journalctl"); err == nil {
		return newJournal()
	}
	for _, path := range []string{"/var/log/syslog", "/var/log/messages"} {
		if _, err := os.Stat(path); err == nil {
			return &textLog{path: path}
		}
	}
	return nil
}
