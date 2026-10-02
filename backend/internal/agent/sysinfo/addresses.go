package sysinfo

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

var observedAddress struct {
	sync.RWMutex
	ip string
}

func publicAddress(addr netip.Addr) bool {
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
		return false
	}
	for _, raw := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		prefix, _ := netip.ParsePrefix(raw)
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func Addresses() []protocol.HostAddress {
	out := []protocol.HostAddress{}
	seen := map[string]bool{}
	add := func(addr netip.Addr) {
		addr = addr.Unmap()
		if !addr.IsValid() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() || addr.IsMulticast() {
			return
		}
		ip := addr.String()
		if seen[ip] {
			return
		}
		seen[ip] = true
		family := "v6"
		if addr.Is4() {
			family = "v4"
		}
		out = append(out, protocol.HostAddress{IP: ip, Family: family, Public: publicAddress(addr)})
	}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, value := range addresses {
			prefix, err := netip.ParsePrefix(value.String())
			if err == nil {
				add(prefix.Addr())
			}
		}
	}
	observedAddress.RLock()
	ip := observedAddress.ip
	observedAddress.RUnlock()
	if addr, err := netip.ParseAddr(ip); err == nil {
		add(addr)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Public != out[j].Public {
			return out[i].Public
		}
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		return out[i].IP < out[j].IP
	})
	return out
}

func RegisterAddresses(c *conn.Client) {
	c.OnConnect(func(ctx context.Context) {
		refreshObserved(ctx, c.Server, c.Token)
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refreshObserved(ctx, c.Server, c.Token)
			}
		}
	})
}

func refreshObserved(ctx context.Context, server, token string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(server, "/")+"/api/v1/agent/whoami", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var out struct {
		IP string `json:"ip"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out) != nil {
		return
	}
	addr, err := netip.ParseAddr(out.IP)
	if err != nil || addr.Zone() != "" {
		return
	}
	observedAddress.Lock()
	observedAddress.ip = addr.Unmap().String()
	observedAddress.Unlock()
}
