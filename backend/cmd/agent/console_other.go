//go:build !windows

package main

// hideOwnConsole only matters on Windows.
func hideOwnConsole() {}
