//go:build !windows

package power

import "github.com/j0x3n/x-console/backend/internal/agent/rpcutil"

func available() bool { return false }

func doPower(string) error { return rpcutil.Unsupported("power actions") }

func open(string, string) error { return rpcutil.Unsupported("opening programs") }
