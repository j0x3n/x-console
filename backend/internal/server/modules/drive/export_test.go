package drive

import (
	"net/netip"
	"time"
)

// SetAfterBlobPutForTest replaces the hook between storing content and
// inserting its row, and returns a function that restores it.
func SetAfterBlobPutForTest(hook func()) func() {
	old := afterBlobPut
	afterBlobPut = hook
	return func() { afterBlobPut = old }
}

// AllowLocalFetchForTest lets upload_from_url reach any address on the given
// ports (the guard otherwise allows only the public internet on 80 and 443).
func (m *Module) AllowLocalFetchForTest(ports ...string) {
	m.fetchGuard = fetchGuard{
		allowAddr: func(netip.Addr) bool { return true },
		allowPort: func(p string) bool {
			for _, x := range ports {
				if x == p {
					return true
				}
			}
			return false
		},
	}
}

// SetDownloadTimingForTest shortens the stall limit and the wait of an action.
func (m *Module) SetDownloadTimingForTest(idle, wait time.Duration) {
	m.downloadIdle, m.taskWait = idle, wait
}

// PublicAddrForTest exposes the address rules.
func PublicAddrForTest(a netip.Addr) bool { return publicAddr(a) }
