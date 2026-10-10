//go:build !windows

package main

// setupConsole only matters on Windows, where the agent has no console.
func setupConsole() {}
