//go:build !linux && !windows

package pty

import (
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func available() bool { return false }

func start(protocol.PTYOpenParams) (terminal, error) {
	return nil, rpcutil.Unsupported("terminal")
}
