//go:build !windows

package syslog

// outputCodePage is only known on Windows.
func outputCodePage() uint32 { return 0 }
