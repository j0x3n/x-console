//go:build !linux && !windows

package svc

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func available() bool { return false }

func list(context.Context) ([]protocol.ServiceInfo, error) {
	return nil, rpcutil.Unsupported("services")
}

func action(context.Context, string, string) error { return rpcutil.Unsupported("services") }

func logs(context.Context, string, int) ([]string, error) {
	return nil, rpcutil.Unsupported("service logs")
}
