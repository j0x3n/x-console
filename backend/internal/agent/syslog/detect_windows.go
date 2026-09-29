//go:build windows

package syslog

func detect() backend { return newWevt() }
