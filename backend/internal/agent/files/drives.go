package files

import "github.com/j0x3n/x-console/backend/pkg/protocol"

// DriveEntries turns a GetLogicalDrives bit mask into directory entries
// ("C:\", "D:\", ...). It has no build tag so it is tested on Linux too.
func DriveEntries(mask uint32) []protocol.FileEntry {
	out := []protocol.FileEntry{}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		out = append(out, protocol.FileEntry{Name: root, Path: root, Type: "dir", Mode: "drwxrwxrwx"})
	}
	return out
}
