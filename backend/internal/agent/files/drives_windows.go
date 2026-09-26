//go:build windows

package files

import (
	"golang.org/x/sys/windows"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// On Windows "/" is a virtual directory holding the drives.
func isDrivesRoot(path string) bool { return path == "/" || path == `\` }

func rootParent() string { return "/" }

func drives() (protocol.FileList, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return protocol.FileList{}, rpcutil.FromOS(err)
	}
	return protocol.FileList{Path: "/", Sep: `\`, Entries: DriveEntries(mask)}, nil
}
