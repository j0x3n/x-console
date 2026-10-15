package router

// B228: the panel remembers every device it has seen, tells online from
// offline on each report, and sends the notifications. B232 adds the one for
// a device it has never seen. The state lives in memory and is written to
// router_devices when it changes, so a restart of the panel does not turn
// every device into a new arrival.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	// wiredOfflineAfter is how long a device that is not on Wi-Fi may stay out
	// of sight before it counts as gone. The neighbor table reacts slowly to a
	// pulled cable.
	wiredOfflineAfter = 5 * time.Minute
	// wifiGrace is how long a device that left the Wi-Fi list may still show in
	// the neighbor table before it counts as moved to a cable. The kernel keeps
	// the entry of a phone that just left REACHABLE for up to 45 seconds, and
	// that must not hide the offline.
	wifiGrace = 60 * time.Second
	// devicePersistEvery bounds the writes of last_seen. Reports can come every
	// 3 seconds and the table has dozens of devices.
	devicePersistEvery = time.Minute
	// forgetDevicesAfter is how long a device without a note may stay away
	// before it is dropped.
	forgetDevicesAfter = 30 * 24 * time.Hour
	maxNoteRunes       = 40
)

const (
	connWifi  = "wifi"
	connWired = "wired"
)

// devState is one device. It mirrors a row of router_devices plus what the last
// report said.
type devState struct {
	mac       string
	note      string
	notify    bool
	cooldown  *int // minutes, nil follows the default
	conn      string
	wifiIface string
	name      string // host name from the DHCP lease
	ip        string

	online       bool
	onlineSince  time.Time
	offlineSince time.Time
	firstSeen    time.Time
	lastSeen     time.Time

	notifiedAt     time.Time
	notifiedOnline bool

	// Only in memory.
	ipv6         []string
	leaseExpires time.Time
	wifiLostAt   time.Time // left the Wi-Fi list but is still in the neighbor table
	savedAt      time.Time // when the row was last written
	rate         devRate   // B229
}

// devRate is the speed from the last report. B229 fills it.
type devRate struct {
	rx, tx float64
	ok     bool
}

// deviceStore is the in-memory copy of router_devices.
type deviceStore struct {
	mu     sync.Mutex
	loaded bool
	byMAC  map[string]*devState
	pruned time.Time
}

func (d *devState) displayName() string {
	switch {
	case d.note != "":
		return d.note
	case d.name != "":
		return d.name
	}
	return d.mac
}

func fromRow(r db.RouterDevice) *devState {
	d := &devState{
		mac: r.Mac, note: r.Note, notify: r.Notify != 0, conn: r.Conn, wifiIface: r.WifiIface, name: r.LastName, ip: r.LastIp,
		online: r.Online != 0, firstSeen: time.Unix(r.FirstSeen, 0), lastSeen: time.Unix(r.LastSeen, 0), savedAt: time.Unix(r.LastSeen, 0),
	}
	if r.CooldownMinutes != nil {
		c := int(*r.CooldownMinutes)
		d.cooldown = &c
	}
	if r.OnlineSince != nil {
		d.onlineSince = time.Unix(*r.OnlineSince, 0)
	}
	if r.OfflineSince != nil {
		d.offlineSince = time.Unix(*r.OfflineSince, 0)
	}
	if r.NotifiedAt != nil {
		d.notifiedAt = time.Unix(*r.NotifiedAt, 0)
	}
	if r.NotifiedOnline != nil {
		d.notifiedOnline = *r.NotifiedOnline != 0
	}
	return d
}

func unixPtr(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	v := t.Unix()
	return &v
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// params is the row to write for d.
func (d *devState) params() db.UpsertRouterDeviceParams {
	p := db.UpsertRouterDeviceParams{
		Mac: d.mac, Note: d.note, Notify: boolInt(d.notify), Conn: d.conn, WifiIface: d.wifiIface, LastName: d.name, LastIp: d.ip,
		Online: boolInt(d.online), OnlineSince: unixPtr(d.onlineSince), OfflineSince: unixPtr(d.offlineSince),
		FirstSeen: d.firstSeen.Unix(), LastSeen: d.lastSeen.Unix(), NotifiedAt: unixPtr(d.notifiedAt),
	}
	if d.cooldown != nil {
		c := int64(*d.cooldown)
		p.CooldownMinutes = &c
	}
	if !d.notifiedAt.IsZero() {
		v := boolInt(d.notifiedOnline)
		p.NotifiedOnline = &v
	}
	return p
}

// load reads the table once. The caller holds s.mu.
func (m *Module) loadDevices(ctx context.Context) error {
	s := &m.devs
	if s.loaded {
		return nil
	}
	rows, err := m.q.ListRouterDevices(ctx)
	if err != nil {
		return err
	}
	s.byMAC = make(map[string]*devState, len(rows))
	for _, r := range rows {
		s.byMAC[r.Mac] = fromRow(r)
	}
	s.loaded = true
	return nil
}

// saveDevices writes the rows of ds in one transaction.
func (m *Module) saveDevices(ctx context.Context, ds []*devState, now time.Time) {
	if len(ds) == 0 {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err == nil {
		q := m.q.WithTx(tx)
		for _, d := range ds {
			if err = q.UpsertRouterDevice(ctx, d.params()); err != nil {
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		m.d.Log.Warn("router devices save", "err", err)
		return
	}
	for _, d := range ds {
		d.savedAt = now
	}
}

// seen is what one report says about one MAC.
type seen struct {
	wifiIface string
	reach     bool // on the network now
	ipv4      []neighIP
	ipv6      []string
}

type neighIP struct {
	ip    string
	reach bool
}

// observe joins the sections of a report by MAC. wanDevs are the devices of
// the WAN interfaces: their neighbors are the provider's, not devices of the
// home.
func observe(rep report, wanDevs map[string]bool) map[string]*seen {
	out := map[string]*seen{}
	get := func(mac string) *seen {
		s := out[mac]
		if s == nil {
			s = &seen{}
			out[mac] = s
		}
		return s
	}
	for mac, iface := range rep.wifi {
		s := get(mac)
		s.wifiIface, s.reach = iface, true
	}
	entries := rep.neigh
	if !rep.hasNeigh {
		entries = parseARPEntries(rep.arp)
		for i := range entries {
			entries[i].state = "REACHABLE" // an ARP entry says the host answered, and that is all the old script knows
		}
	}
	for _, e := range entries {
		if wanDevs[e.dev] {
			continue
		}
		s := get(e.mac)
		ok := e.reachable()
		s.reach = s.reach || ok
		switch {
		case e.ip.Is4():
			s.ipv4 = append(s.ipv4, neighIP{ip: e.ip.String(), reach: ok})
		case e.ip.Is6() && !e.ip.IsLinkLocalUnicast() && !slices.Contains(s.ipv6, e.ip.String()):
			s.ipv6 = append(s.ipv6, e.ip.String())
		}
	}
	return out
}

// pickIP chooses the address to show: the leased one when the neighbor table
// agrees or has nothing, else one that answered, else any.
func pickIP(s *seen, lease string) string {
	if s == nil || len(s.ipv4) == 0 {
		return lease
	}
	for _, c := range s.ipv4 {
		if c.ip == lease {
			return lease
		}
	}
	for _, c := range s.ipv4 {
		if c.reach {
			return c.ip
		}
	}
	return s.ipv4[0].ip
}

// deviceUpdate is what a report leaves for the pages.
type deviceUpdate struct {
	clients                       []api.RouterClient
	online, wifi, wired, knownAll int
}

// updateDevices takes one report into the device table: who is online, who
// came or left, which notifications are due.
func (m *Module) updateDevices(ctx context.Context, rep report, wanDevs map[string]bool, leaseNow, now time.Time) deviceUpdate {
	opts, err := m.options(ctx)
	if err != nil {
		m.d.Log.Warn("router options", "err", err)
		opts = defaultOptions()
	}
	var send []notify.Notification
	s := &m.devs
	s.mu.Lock()
	if err := m.loadDevices(ctx); err != nil {
		s.mu.Unlock()
		m.d.Log.Warn("router devices load", "err", err)
		return deviceUpdate{}
	}
	obs := observe(rep, wanDevs)
	leases := map[string]dhcpLease{}
	for _, l := range parseLeaseFile(rep.leases, leaseNow) {
		if mac := deviceMAC(l.MAC); mac != "" {
			leases[mac] = l
		}
	}

	// Everything with a sign of life, and everything already known.
	macs := make([]string, 0, len(obs)+len(s.byMAC))
	for mac := range obs {
		macs = append(macs, mac)
	}
	for mac := range s.byMAC {
		if _, ok := obs[mac]; !ok {
			macs = append(macs, mac)
		}
	}
	slices.Sort(macs)

	var save []*devState
	for _, mac := range macs {
		o := obs[mac]
		d := s.byMAC[mac]
		present := o != nil && o.reach
		if d == nil {
			if !present {
				continue // seen in the neighbor table only as STALE, not worth a row
			}
			d = &devState{mac: mac, firstSeen: now, lastSeen: now}
			s.byMAC[mac] = d
			m.deviceArrived(ctx, d, o, leases[mac], rep.hasWifi, now, opts, &send)
			save = append(save, d)
			continue
		}
		before := *d
		flip := evaluateDevice(d, o, present, rep.hasWifi, now)
		if present {
			m.fillDevice(d, o, leases[mac], now)
		}
		switch flip {
		case flipOnline, flipOffline:
			m.deviceFlipped(ctx, d, flip == flipOnline, now, opts, &send)
		default:
			m.notifyDevice(ctx, d, false, now, opts, &send)
		}
		if flip != flipNone || d.changedFrom(before) || (present && now.Sub(d.savedAt) >= devicePersistEvery) {
			save = append(save, d)
		}
	}
	m.saveDevices(ctx, save, now)

	var up deviceUpdate
	up.knownAll = len(s.byMAC)
	for _, d := range s.byMAC {
		if !d.online {
			continue
		}
		up.online++
		switch d.conn {
		case connWifi:
			up.wifi++
		case connWired:
			up.wired++
		}
		up.clients = append(up.clients, d.client())
	}
	s.mu.Unlock()
	sortClients(up.clients)
	m.sendNotifications(ctx, send)
	return up
}

// changedFrom says whether something the table stores differs from before.
func (d *devState) changedFrom(b devState) bool {
	return d.conn != b.conn || d.wifiIface != b.wifiIface || d.name != b.name || d.ip != b.ip
}

type flip int

const (
	flipNone flip = iota
	flipOnline
	flipOffline
)

// evaluateDevice moves d to the state the report says. present means the
// report shows the device on the network. o is nil for a device the report
// does not mention.
//
// Wi-Fi: online while it is in the association list, offline in the report
// where it is gone. Cable: online while the neighbor table has it, offline
// after wiredOfflineAfter without a sign. A device that left the Wi-Fi list
// but is still in the neighbor table stays online for wifiGrace. If it is
// still there after that, it moved to a cable and no offline is recorded.
func evaluateDevice(d *devState, o *seen, present, hasWifi bool, now time.Time) flip {
	wasOnline := d.online
	onWifi := hasWifi && o != nil && o.wifiIface != ""
	switch {
	case onWifi:
		d.conn, d.wifiIface, d.wifiLostAt = connWifi, o.wifiIface, time.Time{}
	case present:
		if wasOnline && d.conn == connWifi && hasWifi {
			if d.wifiLostAt.IsZero() {
				d.wifiLostAt = now
			}
			if now.Sub(d.wifiLostAt) >= wifiGrace {
				d.conn, d.wifiIface, d.wifiLostAt = connWired, "", time.Time{}
			}
		} else if hasWifi {
			d.conn, d.wifiIface = connWired, ""
		}
	}
	if present {
		d.lastSeen = now
		if !wasOnline {
			d.online, d.onlineSince, d.wifiLostAt = true, now, time.Time{}
			return flipOnline
		}
		return flipNone
	}
	if !wasOnline {
		return flipNone
	}
	switch {
	case d.conn == connWifi && hasWifi:
		d.online, d.offlineSince, d.wifiLostAt = false, now, time.Time{}
		return flipOffline
	case now.Sub(d.lastSeen) >= wiredOfflineAfter:
		d.online, d.offlineSince, d.wifiLostAt = false, d.lastSeen, time.Time{}
		return flipOffline
	}
	return flipNone
}

// fillDevice copies the address, name and lease of a present device.
func (m *Module) fillDevice(d *devState, o *seen, lease dhcpLease, now time.Time) {
	if ip := pickIP(o, lease.IP); ip != "" {
		d.ip = ip
	}
	if lease.Hostname != "" && lease.Hostname != "*" {
		d.name = lease.Hostname
	}
	d.ipv6 = nil
	if o != nil {
		d.ipv6 = slices.Clone(o.ipv6)
	}
	d.leaseExpires = time.Time{}
	if lease.Expires > 0 {
		d.leaseExpires = now.Add(time.Duration(lease.Expires) * time.Second).Truncate(time.Second)
	}
}

// deviceArrived handles a MAC the table does not have yet. B232 adds the
// notification for it.
func (m *Module) deviceArrived(ctx context.Context, d *devState, o *seen, lease dhcpLease, hasWifi bool, now time.Time, opts options, send *[]notify.Notification) {
	d.online, d.onlineSince = true, now
	switch {
	case hasWifi && o.wifiIface != "":
		d.conn, d.wifiIface = connWifi, o.wifiIface
	case hasWifi:
		d.conn = connWired
	}
	m.fillDevice(d, o, lease, now)
}

// deviceFlipped records the change and decides whether to notify.
func (m *Module) deviceFlipped(ctx context.Context, d *devState, online bool, now time.Time, opts options, send *[]notify.Notification) {
	kind, title := "device_offline", d.displayName()+" 离线了"
	if online {
		kind, title = "device_online", d.displayName()+" 上线了"
	}
	m.addEvent(ctx, now, kind, d.mac, title, deviceBody(d))
	m.notifyDevice(ctx, d, true, now, opts, send)
}

// deviceBody is the text under the title: address, how it is connected, and
// how long it was away or online.
func deviceBody(d *devState) string {
	var parts []string
	if d.ip != "" {
		parts = append(parts, "IP "+d.ip)
	}
	switch d.conn {
	case connWifi:
		if d.wifiIface != "" {
			parts = append(parts, "Wi-Fi（"+d.wifiIface+"）")
		} else {
			parts = append(parts, "Wi-Fi")
		}
	case connWired:
		parts = append(parts, "有线")
	}
	switch {
	case d.online && !d.offlineSince.IsZero() && d.onlineSince.After(d.offlineSince):
		parts = append(parts, "离开了 "+humanDuration(d.onlineSince.Sub(d.offlineSince)))
	case !d.online && !d.onlineSince.IsZero() && d.offlineSince.After(d.onlineSince):
		parts = append(parts, "这次在线了 "+humanDuration(d.offlineSince.Sub(d.onlineSince)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "。") + "。"
}

// cooldownFor is the minutes d must wait before it is announced again.
func cooldownFor(d *devState, o options) time.Duration {
	c := o.Cooldown
	if d.cooldown != nil {
		c = *d.cooldown
	}
	return time.Duration(c) * time.Minute
}

// notifyDevice queues the online or offline notification of d when one is due.
// changed says d flipped in this very report.
//
// A device that was announced less than its cool-down ago is not announced
// again, the flip is only in the event list. When the cool-down is over and
// the device is not in the state the last notification told, one more
// notification says where it is now and how many times it changed.
func (m *Module) notifyDevice(ctx context.Context, d *devState, changed bool, now time.Time, o options, send *[]notify.Notification) {
	if !o.DeviceNotify || !d.notify {
		return
	}
	if d.notifiedAt.IsZero() {
		if !changed {
			return
		}
	} else if now.Sub(d.notifiedAt) < cooldownFor(d, o) || d.online == d.notifiedOnline {
		return
	}
	n := notify.Notification{Link: "/router?device=" + d.mac, Source: "router", Scope: "device:" + d.mac}
	flips := 1
	if !d.notifiedAt.IsZero() {
		c, err := m.q.CountRouterDeviceFlips(ctx, db.CountRouterDeviceFlipsParams{Mac: &d.mac, At: d.notifiedAt.Unix() + 1})
		if err != nil {
			m.d.Log.Warn("router device flips", "err", err)
		} else {
			flips = int(c)
		}
	}
	state, kind := "上线了", "router.device_online"
	if !d.online {
		state, kind = "离线了", "router.device_offline"
	}
	n.Kind = kind
	if flips <= 1 {
		n.Title = d.displayName() + " " + state
		n.Body = deviceBody(d)
	} else {
		n.Title = d.displayName() + " 现在" + state
		n.Body = "过去 " + humanDuration(now.Sub(d.notifiedAt)) + "上下线 " + strconv.Itoa(flips) + " 次。" + deviceBody(d)
	}
	d.notifiedAt, d.notifiedOnline = now, d.online
	*send = append(*send, n)
}

// sendNotifications delivers the queued notifications. It runs without the
// device lock.
func (m *Module) sendNotifications(ctx context.Context, list []notify.Notification) {
	for _, n := range list {
		if _, err := m.d.Notify.Send(ctx, n); err != nil {
			m.d.Log.Warn("router notification failed", "err", err)
		}
	}
}

// pollDevices runs once a minute, also while the router is quiet: a
// notification held back by the cool-down is sent when the time is over.
func (m *Module) pollDevices(ctx context.Context, now time.Time) {
	s := &m.devs
	s.mu.Lock()
	if !s.loaded {
		s.mu.Unlock()
		return
	}
	opts, err := m.options(ctx)
	if err != nil {
		s.mu.Unlock()
		return
	}
	var send []notify.Notification
	var save []*devState
	for _, d := range s.byMAC {
		before := d.notifiedAt
		m.notifyDevice(ctx, d, false, now, opts, &send)
		if !d.notifiedAt.Equal(before) {
			save = append(save, d)
		}
	}
	m.saveDevices(ctx, save, now)
	s.mu.Unlock()
	m.sendNotifications(ctx, send)
}

// pruneDevices drops devices that have been away for 30 days and have no
// note, once a day.
func (m *Module) pruneDevices(ctx context.Context, now time.Time) {
	s := &m.devs
	s.mu.Lock()
	if now.Sub(s.pruned) < 24*time.Hour {
		s.mu.Unlock()
		return
	}
	s.pruned = now
	cutoff := now.Add(-forgetDevicesAfter)
	if s.loaded {
		for mac, d := range s.byMAC {
			if !d.online && d.note == "" && d.lastSeen.Before(cutoff) {
				delete(s.byMAC, mac)
			}
		}
	}
	s.mu.Unlock()
	if err := m.q.PruneRouterDevices(ctx, cutoff.Unix()); err != nil {
		m.d.Log.Warn("router prune devices", "err", err)
	}
	if err := m.q.PruneOrphanDeviceTraffic(ctx); err != nil {
		m.d.Log.Warn("router prune device traffic", "err", err)
	}
}

// forgetNotified clears the notification history of every device.
func (m *Module) forgetNotified(ctx context.Context) {
	s := &m.devs
	s.mu.Lock()
	for _, d := range s.byMAC {
		d.notifiedAt, d.notifiedOnline = time.Time{}, false
	}
	s.mu.Unlock()
	if err := m.q.ResetRouterNotified(ctx); err != nil {
		m.d.Log.Warn("router reset notified", "err", err)
	}
}

// addEvent writes one line of the event list.
func (m *Module) addEvent(ctx context.Context, at time.Time, kind, mac, title, detail string) {
	var macPtr *string
	if mac != "" {
		macPtr = &mac
	}
	err := m.q.InsertRouterEvent(context.WithoutCancel(ctx), db.InsertRouterEventParams{At: at.Unix(), Kind: kind, Mac: macPtr, Title: title, Detail: detail})
	if err != nil {
		m.d.Log.Warn("router event", "kind", kind, "err", err)
	}
}

// client is the device as GET /router/clients shows it.
func (d *devState) client() api.RouterClient {
	c := api.RouterClient{Mac: d.mac}
	if d.name != "" {
		name := d.name
		c.Name = &name
	}
	if d.note != "" {
		note := d.note
		c.Note = &note
	}
	if d.ip != "" {
		ip := d.ip
		c.Ip = &ip
	}
	if len(d.ipv6) > 0 {
		v6 := slices.Clone(d.ipv6)
		c.Ipv6 = &v6
	}
	if !d.leaseExpires.IsZero() {
		at := d.leaseExpires.UTC()
		c.LeaseExpiresAt = &at
	}
	since := d.onlineSince.UTC().Truncate(time.Second)
	c.OnlineSince = &since
	return c
}

// ---- HTTP ----

// view is the device as GET /router/devices shows it.
func (m *Module) deviceView(d *devState) api.RouterDevice {
	out := api.RouterDevice{
		Mac: d.mac, Note: d.note, DisplayName: d.displayName(), Online: d.online, Notify: d.notify,
		FirstSeen: d.firstSeen.UTC(), LastSeen: d.lastSeen.UTC(),
	}
	if d.name != "" {
		name := d.name
		out.Name = &name
	}
	if d.ip != "" {
		ip := d.ip
		out.Ip = &ip
	}
	if len(d.ipv6) > 0 && d.online {
		v6 := slices.Clone(d.ipv6)
		out.Ipv6 = &v6
	}
	switch d.conn {
	case connWifi:
		c := api.RouterDeviceConn(connWifi)
		out.Conn = &c
	case connWired:
		c := api.RouterDeviceConn(connWired)
		out.Conn = &c
	}
	if d.conn == connWifi && d.wifiIface != "" {
		w := d.wifiIface
		out.WifiIface = &w
	}
	if d.online {
		since := d.onlineSince.UTC()
		out.OnlineSince = &since
		if !d.leaseExpires.IsZero() {
			at := d.leaseExpires.UTC()
			out.LeaseExpiresAt = &at
		}
	}
	if d.cooldown != nil {
		c := *d.cooldown
		out.CooldownMinutes = &c
	}
	return out
}

// ListRouterDevices lists every device the panel remembers.
func (m *Module) ListRouterDevices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := m.requirePush(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s := &m.devs
	s.mu.Lock()
	if err := m.loadDevices(ctx); err != nil {
		s.mu.Unlock()
		httpx.Fail(w, r, err)
		return
	}
	list := make([]*devState, 0, len(s.byMAC))
	for _, d := range s.byMAC {
		list = append(list, d)
	}
	slices.SortFunc(list, compareDevices)
	items := make([]api.RouterDevice, 0, len(list))
	for _, d := range list {
		items = append(items, m.deviceView(d))
	}
	s.mu.Unlock()
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "trafficAvailable": false})
}

// compareDevices puts the online devices first, by IP. The others follow, the
// last seen first.
func compareDevices(a, b *devState) int {
	if a.online != b.online {
		if a.online {
			return -1
		}
		return 1
	}
	if a.online {
		aa, aerr := netip.ParseAddr(a.ip)
		ba, berr := netip.ParseAddr(b.ip)
		switch {
		case aerr == nil && berr == nil:
			if c := aa.Compare(ba); c != 0 {
				return c
			}
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		}
		return strings.Compare(a.displayName(), b.displayName())
	}
	if c := b.lastSeen.Compare(a.lastSeen); c != 0 {
		return c
	}
	return strings.Compare(a.mac, b.mac)
}

// requirePush is the 409 the device pages get in the other modes.
func (m *Module) requirePush(ctx context.Context) error {
	push, err := m.pushMode(ctx)
	if err != nil {
		return err
	}
	if !push {
		return httpx.NewError(http.StatusConflict, "router_not_push", "要路由器主动上报才有设备列表。在设置里改成主动上报")
	}
	return nil
}

// decodeAll reads a JSON body like httpx.Decode and also insists that every
// field in keys is there. A missing field would otherwise read as the zero
// value, and "notify": false is not what an absent notify means.
func decodeAll(w http.ResponseWriter, r *http.Request, v any, keys ...string) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return httpx.Invalid("请求体读不出来或者太大")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return httpx.Invalid("请求体格式不正确")
	}
	for _, k := range keys {
		if _, ok := fields[k]; !ok {
			return httpx.Invalid("缺少字段 " + k)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.Invalid("请求体格式不正确: " + err.Error())
	}
	return nil
}

// macParam reads the MAC of a path.
func macParam(raw string) (string, error) {
	mac := deviceMAC(raw)
	if mac == "" {
		return "", httpx.Invalid("MAC 格式不对")
	}
	return mac, nil
}

// UpdateRouterDevice changes the note, the notification switch and the
// cool-down of one device.
func (m *Module) UpdateRouterDevice(w http.ResponseWriter, r *http.Request, rawMAC string) {
	ctx := r.Context()
	mac, err := macParam(rawMAC)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.RouterDeviceUpdate
	if err := decodeAll(w, r, &in, "note", "notify", "cooldownMinutes"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > maxNoteRunes {
		httpx.Fail(w, r, httpx.Invalid("备注最多 40 个字"))
		return
	}
	if strings.ContainsFunc(note, func(c rune) bool { return c < ' ' || c == 0x7f }) {
		httpx.Fail(w, r, httpx.Invalid("备注里不能有换行和控制字符"))
		return
	}
	if in.CooldownMinutes != nil && !slices.Contains(cooldownChoices, *in.CooldownMinutes) {
		httpx.Fail(w, r, httpx.Invalid("再次通知间隔只能是 0、5、15、30、60 分钟，或者跟随默认"))
		return
	}
	s := &m.devs
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := m.loadDevices(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d := s.byMAC[mac]
	if d == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	next := *d
	next.note, next.notify = note, in.Notify
	next.cooldown = nil
	if in.CooldownMinutes != nil {
		c := *in.CooldownMinutes
		next.cooldown = &c
	}
	if in.Notify && !d.notify {
		next.notifiedAt, next.notifiedOnline = time.Time{}, false // old history of a switch that was off
	}
	p := db.UpdateRouterDeviceSettingsParams{Note: next.note, Notify: boolInt(next.notify), Mac: mac}
	if next.cooldown != nil {
		c := int64(*next.cooldown)
		p.CooldownMinutes = &c
	}
	p.NotifiedAt = unixPtr(next.notifiedAt)
	if !next.notifiedAt.IsZero() {
		v := boolInt(next.notifiedOnline)
		p.NotifiedOnline = &v
	}
	err = m.q.UpdateRouterDeviceSettings(ctx, p)
	m.d.Audit.Record(ctx, "router.device.update", mac, map[string]any{"note": next.note, "notify": next.notify, "cooldownMinutes": in.CooldownMinutes}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	*d = next
	httpx.JSON(w, http.StatusOK, m.deviceView(d))
}

// DeleteRouterDevice forgets a device. It is new again when it comes back.
func (m *Module) DeleteRouterDevice(w http.ResponseWriter, r *http.Request, rawMAC string) {
	ctx := r.Context()
	mac, err := macParam(rawMAC)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s := &m.devs
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := m.loadDevices(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if s.byMAC[mac] == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	err = m.forgetDevice(ctx, mac)
	m.d.Audit.Record(ctx, "router.device.forget", mac, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	delete(s.byMAC, mac)
	w.WriteHeader(http.StatusNoContent)
}

// forgetDevice deletes the rows of one device.
func (m *Module) forgetDevice(ctx context.Context, mac string) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	q := m.q.WithTx(tx)
	if err = q.DeleteRouterDeviceTraffic(ctx, mac); err == nil {
		err = q.DeleteRouterDevice(ctx, mac)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
