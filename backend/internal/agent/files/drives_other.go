//go:build !windows

package files

import (
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func isDrivesRoot(string) bool { return false }

func rootParent() string { return "" }

func drives() (protocol.FileList, error) {
	return protocol.FileList{}, rpcutil.Unsupported("drive list")
}
