package httpx

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	out := []netip.Prefix{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, e := netip.ParseAddr(value)
			if e != nil {
				return nil, fmt.Errorf("可信代理地址无效: %s", value)
			}
			addr = addr.Unmap()
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

func PeerIP(remote string) (netip.Addr, bool) {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	addr, err := netip.ParseAddr(strings.Trim(remote, "[]"))
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// ClientIP returns the address of the client. When the peer is a trusted
// proxy it walks X-Forwarded-For from the right and stops at the first hop
// that is not trusted, so a client cannot pick its own address by sending
// the header itself.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer, ok := PeerIP(r.RemoteAddr)
	if !ok {
		return ""
	}
	isTrusted := func(addr netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(addr) {
				return true
			}
		}
		return false
	}
	client := peer
	if !isTrusted(peer) {
		return client.String()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil || addr.Zone() != "" {
			break
		}
		client = addr.Unmap()
		if !isTrusted(client) {
			break
		}
	}
	return client.String()
}
