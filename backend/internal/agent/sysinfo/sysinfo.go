// Package sysinfo answers protocol.MethodPing and MethodSystemInfo.
package sysinfo

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Info collects basic facts. The metrics package (M2/M3) replaces the fields
// that need gopsutil.
var Info = func() protocol.SystemInfo {
	host, _ := os.Hostname()
	return protocol.SystemInfo{Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, CPUCores: runtime.NumCPU()}
}

// Ping answers MethodPing.
func Ping(ctx context.Context, _ json.RawMessage) (any, error) {
	return protocol.Pong{Time: time.Now().UTC().Format(time.RFC3339)}, nil
}

// SystemInfo answers MethodSystemInfo.
func SystemInfo(ctx context.Context, _ json.RawMessage) (any, error) {
	info := Info()
	info.Addresses = Addresses()
	return info, nil
}
