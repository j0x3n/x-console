package router

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// B93: luci-rpc is part of LuCI (rpcd-mod-luci). Routers without it answer
// "Object not found", so the DHCP leases and the hosts seen on the LAN are
// read from whatever the router has. The first source that answers is
// remembered and tried first next time.

const (
	leasesLuci   = "luci"
	leasesOdhcpd = "odhcpd"
	leasesFile   = "file"
	hintsLuci    = "luci"
	hintsARP     = "arp"
)

// errNoClients is returned when no source of leases or hosts answered.
var errNoClients = &httpx.Error{Status: http.StatusBadGateway, Code: "router_error", Message: "路由器上读不到在线设备：没有 luci-rpc，也读不到 DHCP 租约和 ARP 表。装上 luci（rpcd-mod-luci），或者装 rpcd-mod-file 并按设置页的 ACL 允许读 /tmp/dhcp.leases 和 /proc/net/arp，再试一次"}

// ordered puts the remembered source first.
func ordered(all []string, first string) []string {
	out := []string{}
	if first != "" {
		out = append(out, first)
	}
	for _, s := range all {
		if s != first {
			out = append(out, s)
		}
	}
	return out
}

// readLeases returns the DHCP leases. A failure to reach the router is
// returned at once; other failures move on to the next source.
func (m *Module) readLeases(ctx context.Context, u *ubus) ([]dhcpLease, error) {
	m.mu.Lock()
	first := m.leaseSource
	m.mu.Unlock()
	var last error
	for _, src := range ordered([]string{leasesLuci, leasesOdhcpd, leasesFile}, first) {
		var leases []dhcpLease
		var err error
		switch src {
		case leasesLuci:
			var out struct {
				DHCP []dhcpLease `json:"dhcp_leases"`
			}
			err = u.call(ctx, "luci-rpc", "getDHCPLeases", nil, &out)
			leases = out.DHCP
		case leasesOdhcpd:
			leases, err = odhcpdLeases(ctx, u)
		case leasesFile:
			var out struct {
				Data string `json:"data"`
			}
			err = u.call(ctx, "file", "read", map[string]any{"path": "/tmp/dhcp.leases"}, &out)
			leases = parseLeaseFile(out.Data, m.now())
		}
		if err == nil {
			m.mu.Lock()
			m.leaseSource = src
			m.mu.Unlock()
			return leases, nil
		}
		if unreachable(err) {
			return nil, err
		}
		last = err
	}
	return nil, last
}

// odhcpdLeases reads `ubus call dhcp ipv4leases`.
func odhcpdLeases(ctx context.Context, u *ubus) ([]dhcpLease, error) {
	var out struct {
		Device map[string]struct {
			Leases []struct {
				MAC      string `json:"mac"`
				Hostname string `json:"hostname"`
				Address  string `json:"address"`
				Valid    int64  `json:"valid"`
			} `json:"leases"`
		} `json:"device"`
	}
	if err := u.call(ctx, "dhcp", "ipv4leases", nil, &out); err != nil {
		return nil, err
	}
	var leases []dhcpLease
	for _, dev := range out.Device {
		for _, l := range dev.Leases {
			leases = append(leases, dhcpLease{MAC: colonMAC(l.MAC), IP: l.Address, Hostname: l.Hostname, Expires: max(l.Valid, 0)})
		}
	}
	return leases, nil
}

// parseLeaseFile reads dnsmasq's lease file: "<expiry> <mac> <ip> <name> <id>",
// where expiry is a Unix time (0 means never).
func parseLeaseFile(data string, now time.Time) []dhcpLease {
	var leases []dhcpLease
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || strings.Contains(f[1], "*") {
			continue
		}
		l := dhcpLease{MAC: f[1], IP: f[2], Hostname: f[3]}
		if at, err := strconv.ParseInt(f[0], 10, 64); err == nil && at > 0 {
			l.Expires = max(at-now.Unix(), 0)
		}
		leases = append(leases, l)
	}
	return leases
}

// readHints returns the hosts the router has seen: LuCI's host hints, or
// else the ARP table.
func (m *Module) readHints(ctx context.Context, u *ubus) (map[string]hostHint, error) {
	m.mu.Lock()
	first := m.hintSource
	m.mu.Unlock()
	var last error
	for _, src := range ordered([]string{hintsLuci, hintsARP}, first) {
		var hints map[string]hostHint
		var err error
		switch src {
		case hintsLuci:
			err = u.call(ctx, "luci-rpc", "getHostHints", nil, &hints)
		case hintsARP:
			var out struct {
				Data string `json:"data"`
			}
			err = u.call(ctx, "file", "read", map[string]any{"path": "/proc/net/arp"}, &out)
			hints = parseARP(out.Data)
		}
		if err == nil {
			m.mu.Lock()
			m.hintSource = src
			m.mu.Unlock()
			return hints, nil
		}
		if unreachable(err) {
			return nil, err
		}
		last = err
	}
	return nil, last
}

// parseARP reads /proc/net/arp. Only complete entries (flag 0x2) count.
func parseARP(data string) map[string]hostHint {
	hints := map[string]hostHint{}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] == "IP" {
			continue
		}
		flags, err := strconv.ParseInt(strings.TrimPrefix(f[2], "0x"), 16, 64)
		if err != nil || flags&0x2 == 0 || f[3] == "00:00:00:00:00:00" {
			continue
		}
		mac := strings.ToUpper(f[3])
		h := hints[mac]
		h.IPv4 = append(h.IPv4, f[0])
		hints[mac] = h
	}
	return hints
}

// colonMAC turns "aabbcc000002" into "aa:bb:cc:00:00:02".
func colonMAC(mac string) string {
	if len(mac) != 12 || strings.Contains(mac, ":") {
		return mac
	}
	parts := make([]string, 0, 6)
	for i := 0; i < 12; i += 2 {
		parts = append(parts, mac[i:i+2])
	}
	return strings.Join(parts, ":")
}
