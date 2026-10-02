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

func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer, ok := PeerIP(r.RemoteAddr)
	if !ok {
		return ""
	}
	allowed := false
	for _, prefix := range trusted {
		if prefix.Contains(peer) {
			allowed = true
			break
		}
	}
	if allowed {
		for _, value := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
			addr, err := netip.ParseAddr(strings.TrimSpace(value))
			if err == nil && addr.Zone() == "" {
				return addr.Unmap().String()
			}
		}
	}
	return peer.String()
}
