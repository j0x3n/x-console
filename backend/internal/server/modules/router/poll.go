package router

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	// downAfter is how long the WAN must be down before a notification.
	downAfter = 2 * time.Minute
	// keepTraffic is how long the minute samples are kept.
	keepTraffic = 30 * 24 * time.Hour
	// maxGap is the longest gap one sample may cover. A longer one (panel
	// restarted, router unreachable) would show as a spike, so it is skipped.
	maxGap = 5 * time.Minute
)

// wanWatch tracks one outage: the WAN interface down, or the router not
// answering at all (from far away a WAN outage looks like that).
type wanWatch struct {
	since    time.Time
	reason   string // "wan" or "unreachable"
	notified bool
	detail   string
	push     bool // the router stopped reporting (B114)
}

// Poll runs once a minute: stores a traffic sample and checks the WAN.
func (m *Module) Poll(ctx context.Context) {
	push, err := m.pushMode(ctx)
	if err != nil {
		m.d.Log.Warn("router config", "err", err)
		return
	}
	if push {
		m.pollPush(ctx, m.now())
		return
	}
	u, err := m.ubus(ctx)
	if errors.Is(err, httpx.ErrIntegrationMissing) {
		return
	}
	if err != nil {
		m.d.Log.Warn("router config", "err", err)
		return
	}
	now := m.now()
	list, err := interfaces(ctx, u)
	if err != nil {
		// Only a failure to reach the router counts as an outage. A refusal
		// (ACL, wrong password) is a setup problem the page shows instead.
		if unreachable(err) {
			m.wanState(ctx, now, false, "unreachable", err.Error())
		} else {
			m.d.Log.Warn("router poll", "err", err)
		}
		return
	}
	wan, ok := pickWAN(list)
	if !ok {
		return // no WAN interface to watch or measure
	}
	if !wan.Up {
		m.wanState(ctx, now, false, "wan", "")
	} else {
		m.wanState(ctx, now, true, "", firstIP(wan))
		m.sampleTraffic(ctx, u, wan.dev(), now)
	}
	if _, err := m.clients(ctx, u, true); err != nil {
		m.d.Log.Debug("router clients", "err", err)
	}
	m.prune(ctx, now)
}

func firstIP(i ifaceInfo) string {
	if len(i.IPv4) > 0 {
		return i.IPv4[0].Address
	}
	return ""
}

// sampleTraffic stores the bytes moved since the previous poll.
func (m *Module) sampleTraffic(ctx context.Context, u *ubus, dev string, now time.Time) {
	if dev == "" {
		return
	}
	rx, tx, err := counters(ctx, u, dev)
	if err != nil {
		m.d.Log.Debug("router counters", "err", err)
		return
	}
	m.addSample(ctx, sample{dev: dev, rx: rx, tx: tx, at: now})
}

// addSample stores the bytes moved since the previous sample and returns the
// average rates over that gap, or nils when there is nothing to compare.
func (m *Module) addSample(ctx context.Context, cur sample) (rxRate, txRate *float64) {
	m.mu.Lock()
	prev := m.poll
	m.poll = cur
	m.mu.Unlock()
	gap := cur.at.Sub(prev.at)
	if prev.at.IsZero() || gap <= 0 || gap > maxGap {
		return nil, nil
	}
	drx, dtx := delta(prev, cur)
	err := m.q.InsertTraffic(ctx, db.InsertTrafficParams{
		At: cur.at.Truncate(time.Minute).Unix(), Seconds: int64(gap.Round(time.Second) / time.Second), Rx: int64(drx), Tx: int64(dtx),
	})
	if err != nil {
		m.d.Log.Warn("router traffic", "err", err)
	}
	rx, tx := float64(drx)/gap.Seconds(), float64(dtx)/gap.Seconds()
	return &rx, &tx
}

// prune drops old samples once an hour.
func (m *Module) prune(ctx context.Context, now time.Time) {
	m.mu.Lock()
	due := now.Sub(m.pruned) >= time.Hour
	if due {
		m.pruned = now
	}
	m.mu.Unlock()
	if !due {
		return
	}
	if err := m.q.PruneTraffic(ctx, now.Add(-keepTraffic).Unix()); err != nil {
		m.d.Log.Warn("router prune", "err", err)
	}
}

// wanState moves the outage tracker and sends the down and back notifications.
func (m *Module) wanState(ctx context.Context, now time.Time, up bool, reason, detail string) {
	m.mu.Lock()
	w := m.watch
	var send *notify.Notification
	switch {
	case up && w.since.IsZero():
	case up:
		if w.notified {
			body := "断了 " + humanDuration(now.Sub(w.since)) + "。"
			if detail != "" {
				body += "WAN 口 IP：" + detail
			}
			send = &notify.Notification{Kind: "router.wan_up", Title: "家里网络恢复了", Body: body}
		}
		w = wanWatch{}
	default:
		if w.since.IsZero() {
			w = wanWatch{since: now, reason: reason, detail: detail}
		}
		if reason == "wan" {
			w.reason = "wan" // the router answered, so it is the WAN
		}
		if !w.notified && now.Sub(w.since) >= downAfter && !now.Before(m.quiet) {
			w.notified = true
			if w.reason == "wan" {
				send = &notify.Notification{Kind: "router.wan_down", Priority: "high", Title: "WAN 口掉线了",
					Body: "已经断了 " + humanDuration(now.Sub(w.since)) + "。路由器能连上，是外网断了。"}
			} else if w.push {
				send = &notify.Notification{Kind: "router.wan_down", Priority: "high", Title: "家里的路由器不上报了",
					Body: "已经 " + humanDuration(now.Sub(w.since)) + "没收到路由器的上报。可能是家里断网、断电，或者路由器上的脚本停了。"}
			} else {
				send = &notify.Notification{Kind: "router.wan_down", Priority: "high", Title: "连不上家里的路由器",
					Body: "已经 " + humanDuration(now.Sub(w.since)) + " 连不上。可能是家里断网、断电，或者转发的代理掉线了。"}
			}
		}
	}
	m.watch = w
	m.mu.Unlock()
	if send == nil {
		return
	}
	send.Link, send.Source = "/router", "router"
	if _, err := m.d.Notify.Send(ctx, *send); err != nil {
		m.d.Log.Warn("router notification failed", "err", err)
	}
}

func humanDuration(d time.Duration) string {
	minutes := int(d.Round(time.Minute) / time.Minute)
	switch {
	case minutes < 1:
		return "不到 1 分钟"
	case minutes < 60:
		return fmt.Sprintf("%d 分钟", minutes)
	case minutes%60 == 0:
		return fmt.Sprintf("%d 小时", minutes/60)
	default:
		return fmt.Sprintf("%d 小时 %d 分钟", minutes/60, minutes%60)
	}
}
