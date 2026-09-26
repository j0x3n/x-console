//go:build !windows

package clipboard

import "github.com/j0x3n/x-console/backend/internal/agent/rpcutil"

func available() bool { return false }

func get() (string, error) { return "", rpcutil.Unsupported("clipboard") }

func set(string) error { return rpcutil.Unsupported("clipboard") }
