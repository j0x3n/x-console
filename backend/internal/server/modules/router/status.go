package router

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
)

// The ubus replies, only the fields used here.

type boardInfo struct {
	Hostname string `json:"hostname"`
	Model    string `json:"model"`
	Release  struct {
		Distribution string `json:"distribution"`
		Version      string `json:"version"`
		Description  string `json:"description"`
	} `json:"release"`
}

type systemInfo struct {
	Uptime int64    `json:"uptime"`
	Load   []uint64 `json:"load"`
	Memory struct {
		Total     int64 `json:"total"`
		Free      int64 `json:"free"`
		Buffered  int64 `json:"buffered"`
		Cached    int64 `json:"cached"`
		Available int64 `json:"available"`
	} `json:"memory"`
}

type ifaceAddr struct {
	Address string `json:"address"`
	Mask    int    `json:"mask"`
}

type ifaceInfo struct {
	Interface string      `json:"interface"`
	Up        bool        `json:"up"`
	Uptime    int64       `json:"uptime"`
	Proto     string      `json:"proto"`
	Device    string      `json:"device"`
	L3Device  string      `json:"l3_device"`
	IPv4      []ifaceAddr `json:"ipv4-address"`
	IPv6      []ifaceAddr `json:"ipv6-address"`
}

func (i ifaceInfo) dev() string {
	if i.L3Device != "" {
		return i.L3Device
	}
	return i.Device
}

func (i ifaceInfo) toAPI() api.RouterInterface {
	out := api.RouterInterface{Name: i.Interface, Up: i.Up, Ipv4: []string{}, Ipv6: []string{}}
	if i.Proto != "" {
		out.Proto = &i.Proto
	}
	if d := i.dev(); d != "" {
		out.Device = &d
	}
	if i.Up {
		up := i.Uptime
		out.UptimeSeconds = &up
	}
	for _, a := range i.IPv4 {
		out.Ipv4 = append(out.Ipv4, fmt.Sprintf("%s/%d", a.Address, a.Mask))
	}
	for _, a := range i.IPv6 {
		out.Ipv6 = append(out.Ipv6, fmt.Sprintf("%s/%d", a.Address, a.Mask))
	}
	return out
}

// interfaces lists the logical interfaces, without loopback.
func interfaces(ctx context.Context, u *ubus) ([]ifaceInfo, error) {
	var dump struct {
		Interface []ifaceInfo `json:"interface"`
	}
	if err := u.call(ctx, "network.interface", "dump", nil, &dump); err != nil {
		return nil, err
	}
	out := dump.Interface[:0]
	for _, i := range dump.Interface {
		if i.Interface != "loopback" {
			out = append(out, i)
		}
	}
	return out, nil
}

// pickWAN finds the WAN interface: "wan", else the first one named like
// wan* that is not IPv6 only.
func pickWAN(list []ifaceInfo) (ifaceInfo, bool) {
	for _, i := range list {
		if i.Interface == "wan" {
			return i, true
		}
	}
	for _, i := range list {
		if strings.HasPrefix(i.Interface, "wan") && !strings.HasSuffix(i.Interface, "6") {
			return i, true
		}
	}
	return ifaceInfo{}, false
}

// sample is one reading of the WAN counters.
type sample struct {
	dev    string
	rx, tx uint64
	at     time.Time
}

// counters reads the byte counters of one network device.
func counters(ctx context.Context, u *ubus, dev string) (uint64, uint64, error) {
	var st struct {
		Statistics struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"statistics"`
	}
	if err := u.call(ctx, "network.device", "status", map[string]string{"name": dev}, &st); err != nil {
		return 0, 0, err
	}
	return st.Statistics.RxBytes, st.Statistics.TxBytes, nil
}

// delta is the bytes moved since prev. A counter that went down was reset
// (router reboot, PPPoE redial), so the new value is all new traffic.
func delta(prev, cur sample) (rx, tx uint64) {
	rx, tx = cur.rx, cur.tx
	if prev.dev == cur.dev && cur.rx >= prev.rx {
		rx = cur.rx - prev.rx
	}
	if prev.dev == cur.dev && cur.tx >= prev.tx {
		tx = cur.tx - prev.tx
	}
	return rx, tx
}

// status reads everything GET /router/status shows.
func (m *Module) status(ctx context.Context, u *ubus) (api.RouterStatus, error) {
	var board boardInfo
	if err := u.call(ctx, "system", "board", nil, &board); err != nil {
		return api.RouterStatus{}, err
	}
	var info systemInfo
	if err := u.call(ctx, "system", "info", nil, &info); err != nil {
		return api.RouterStatus{}, err
	}
	list, err := interfaces(ctx, u)
	if err != nil {
		return api.RouterStatus{}, err
	}
	now := m.now()
	out := api.RouterStatus{
		Hostname: board.Hostname, Model: board.Model, Firmware: board.Release.Description,
		UptimeSeconds: info.Uptime, Interfaces: []api.RouterInterface{}, CheckedAt: now.UTC(),
	}
	if out.Firmware == "" {
		out.Firmware = strings.TrimSpace(board.Release.Distribution + " " + board.Release.Version)
	}
	if info.Memory.Total > 0 {
		total, avail := info.Memory.Total, info.Memory.Available
		if avail == 0 {
			avail = info.Memory.Free + info.Memory.Buffered + info.Memory.Cached
		}
		out.MemoryTotal, out.MemoryAvailable = &total, &avail
	}
	if len(info.Load) == 3 {
		load := make([]float64, 3)
		for i, v := range info.Load {
			load[i] = float64(v) / 65536
		}
		out.Load = &load
	}
	for _, i := range list {
		out.Interfaces = append(out.Interfaces, i.toAPI())
	}
	if wan, ok := pickWAN(list); ok {
		w := wan.toAPI()
		out.Wan = &w
		if dev := wan.dev(); dev != "" && wan.Up {
			if rx, tx, err := counters(ctx, u, dev); err == nil {
				cur := sample{dev: dev, rx: rx, tx: tx, at: now}
				m.mu.Lock()
				prev := m.live
				m.live = cur
				m.mu.Unlock()
				if dt := cur.at.Sub(prev.at).Seconds(); prev.dev == dev && dt >= 1 && dt <= 600 {
					drx, dtx := delta(prev, cur)
					rxRate, txRate := float64(drx)/dt, float64(dtx)/dt
					out.RxRate, out.TxRate = &rxRate, &txRate
				}
			}
		}
	}
	if clients, err := m.clients(ctx, u, false); err == nil {
		out.ClientCount = len(clients)
	}
	return out, nil
}

type dhcpLease struct {
	Expires  int64  `json:"expires"`
	Hostname string `json:"hostname"`
	MAC      string `json:"macaddr"`
	IP       string `json:"ipaddr"`
}

type hostHint struct {
	Name    string   `json:"name"`
	IPv4    []string `json:"ipaddrs"`
	IPv6    []string `json:"ip6addrs"`
	Legacy4 string   `json:"ipv4"` // older LuCI
	Legacy6 string   `json:"ipv6"`
}

// clientsCache keeps the merged list for a short while: the status page
// asks every few seconds and each read is two ubus calls.
type clientsCache struct {
	at    time.Time
	items []api.RouterClient
}

const clientsTTL = 20 * time.Second

// clients merges DHCP leases and host hints. fresh skips the cache.
func (m *Module) clients(ctx context.Context, u *ubus, fresh bool) ([]api.RouterClient, error) {
	now := m.now()
	m.mu.Lock()
	if c := m.cached; c != nil && !fresh && now.Sub(c.at) < clientsTTL {
		items := c.items
		m.mu.Unlock()
		return items, nil
	}
	m.mu.Unlock()
	// B93: without luci-rpc, fall back to odhcpd, the lease file and the ARP
	// table. Either half is enough to list devices.
	leases, lerr := m.readLeases(ctx, u)
	if lerr != nil && unreachable(lerr) {
		return nil, lerr
	}
	hints, herr := m.readHints(ctx, u)
	if herr != nil && unreachable(herr) {
		return nil, herr
	}
	if lerr != nil && herr != nil {
		m.d.Log.Warn("router: no client source", "leases", lerr, "hosts", herr)
		return nil, errNoClients
	}
	byMAC := map[string]*api.RouterClient{}
	get := func(mac string) *api.RouterClient {
		mac = strings.ToUpper(strings.TrimSpace(mac))
		if mac == "" {
			return nil
		}
		c, ok := byMAC[mac]
		if !ok {
			c = &api.RouterClient{Mac: mac}
			byMAC[mac] = c
		}
		return c
	}
	for _, l := range leases {
		c := get(l.MAC)
		if c == nil {
			continue
		}
		if l.IP != "" {
			ip := l.IP
			c.Ip = &ip
		}
		if l.Hostname != "" && l.Hostname != "*" {
			name := l.Hostname
			c.Name = &name
		}
		if l.Expires > 0 {
			at := now.Add(time.Duration(l.Expires) * time.Second).UTC().Truncate(time.Second)
			c.LeaseExpiresAt = &at
		}
	}
	for mac, h := range hints {
		v4, v6 := h.IPv4, h.IPv6
		if len(v4) == 0 && h.Legacy4 != "" {
			v4 = []string{h.Legacy4}
		}
		if len(v6) == 0 && h.Legacy6 != "" {
			v6 = []string{h.Legacy6}
		}
		if len(v4) == 0 && len(v6) == 0 {
			continue // a name from /etc/ethers with no address seen
		}
		c := get(mac)
		if c == nil {
			continue
		}
		if c.Ip == nil && len(v4) > 0 {
			ip := v4[0]
			c.Ip = &ip
		}
		if c.Name == nil && h.Name != "" {
			name := h.Name
			c.Name = &name
		}
		if len(v6) > 0 {
			list := slices.Clone(v6)
			c.Ipv6 = &list
		}
	}
	items := make([]api.RouterClient, 0, len(byMAC))
	m.mu.Lock()
	seen := make(map[string]time.Time, len(byMAC))
	for mac, c := range byMAC {
		since, ok := m.seen[mac]
		if !ok {
			since = now.UTC().Truncate(time.Second)
		}
		seen[mac] = since
		c.OnlineSince = &since
		items = append(items, *c)
	}
	m.seen = seen
	sortClients(items)
	m.cached = &clientsCache{at: now, items: items}
	m.mu.Unlock()
	return items, nil
}

// sortClients orders by IPv4 address, devices without one last by name.
func sortClients(items []api.RouterClient) {
	addr := func(c api.RouterClient) (netip.Addr, bool) {
		if c.Ip == nil {
			return netip.Addr{}, false
		}
		a, err := netip.ParseAddr(*c.Ip)
		return a, err == nil
	}
	name := func(c api.RouterClient) string {
		if c.Name != nil {
			return *c.Name
		}
		return c.Mac
	}
	slices.SortFunc(items, func(a, b api.RouterClient) int {
		aa, aok := addr(a)
		ba, bok := addr(b)
		switch {
		case aok && bok:
			if c := aa.Compare(ba); c != 0 {
				return c
			}
		case aok:
			return -1
		case bok:
			return 1
		}
		return strings.Compare(name(a), name(b))
	})
}
