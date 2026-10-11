//go:build !windows

package focusblock

// flushDNS does nothing: Linux resolvers read the hosts file on every lookup.
func flushDNS() {}
