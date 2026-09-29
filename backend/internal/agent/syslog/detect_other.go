//go:build !linux && !windows

package syslog

func detect() backend { return nil }
