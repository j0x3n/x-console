package reminders

// B164: notifications by state. A category set to "working" goes out at once
// while the user works at a computer. Otherwise it waits in the digest queue
// (hold_for = working) until:
//   - the state turns to working: the computer in use shows them as system
//     notifications, or the channels get one merged message when it cannot;
//   - the phone's panel claims them (POST /notify/digest/claim);
//   - a day has passed: one merged message through the channels.
// Sleeping also counts as quiet hours when the setting allows it.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

const (
	heldMaxAge     = 24 * time.Hour // then they go out through the channels
	heldShowSingly = 3              // up to this many pop up one by one
)

// meState is the user's state, empty when the me module is missing.
func (m *Module) meState() contracts.MeState {
	if me, ok := module.Lookup[contracts.Me](m.d.Registry, contracts.MeKey); ok {
		return me.Current()
	}
	return contracts.MeState{}
}

// isQuiet reports quiet hours: the fixed period, or sleeping (B164).
func (m *Module) isQuiet(ctx context.Context, now time.Time) bool {
	q := m.quietHours(ctx)
	if inQuietHours(q, now, m.d.Config.Location) {
		return true
	}
	return q.whenAsleep() && m.meState().State == contracts.MeSleeping
}

// followMe sends what waited for the computer when the state turns to
// working.
func (m *Module) followMe(ctx context.Context) {
	me, ok := module.Lookup[contracts.Me](m.d.Registry, contracts.MeKey)
	if !ok {
		return
	}
	ch, cancel := me.Subscribe()
	prev := me.Current().State
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case st, ok := <-ch:
				if !ok {
					return
				}
				if st.State == contracts.MeWorking && prev != contracts.MeWorking {
					if err := m.deliverHeld(ctx, st.Host, time.Now()); err != nil {
						m.d.Log.Warn("send held notifications", "err", err)
					}
				}
				prev = st.State
			}
		}
	}()
}

// deliverHeld sends the "working" queue on return to the computer host.
func (m *Module) deliverHeld(ctx context.Context, host string, now time.Time) error {
	m.digestMu.Lock()
	defer m.digestMu.Unlock()
	items, err := m.q.ListDigestItems(ctx, modeWorking)
	if err != nil || len(items) == 0 {
		return err
	}
	if !m.showOnHost(ctx, host, items) {
		if m.isQuiet(ctx, now) {
			return nil // the next return or the day limit sends them
		}
		msg := buildDigest(items, now, m.d.Config.Location, m.publicLink)
		if ok, failed := m.d.Notify.DeliverNow(ctx, notify.Stored{CreatedAt: now.UTC(), Notification: msg}); ok == 0 && failed > 0 {
			return fmt.Errorf("every channel failed")
		}
	}
	return m.q.DeleteDigestItemsUpTo(ctx, db.DeleteDigestItemsUpToParams{ID: items[len(items)-1].ID, HoldFor: modeWorking})
}

// showOnHost pops items up on the computer as system notifications: one by
// one up to three, else one summary. It reports false when the computer
// cannot show them.
func (m *Module) showOnHost(ctx context.Context, host string, items []db.NotifyDigestQueue) bool {
	if host == "" || m.d.Agents == nil {
		return false
	}
	agent, err := m.d.Agents.Get(ctx, host)
	if err != nil || !agent.Online || !agent.Has("notify.show") {
		return false
	}
	type show struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	var shows []show
	if len(items) <= heldShowSingly {
		for _, it := range items {
			shows = append(shows, show{it.Title, it.Body})
		}
	} else {
		titles := make([]string, 0, heldShowSingly)
		for _, it := range items[:heldShowSingly] {
			titles = append(titles, "· "+strings.TrimSpace(it.Title))
		}
		shows = append(shows, show{fmt.Sprintf("你不在时有 %d 条通知", len(items)), strings.Join(titles, "\n") + "\n在面板的通知中心查看全部"})
	}
	for _, s := range shows {
		callCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := m.d.Agents.Call(callCtx, host, "notify.show", s, nil)
		cancel()
		if err != nil {
			m.d.Log.Warn("show held notification", "host", host, "err", err)
			return false
		}
	}
	return true
}

// flushStaleHeld sends the "working" queue through the channels once its
// oldest item has waited a day, so it never waits forever.
func (m *Module) flushStaleHeld(ctx context.Context, now time.Time) error {
	at, err := m.q.OldestDigestItemAt(ctx, modeWorking)
	if err != nil || now.Sub(at) < heldMaxAge {
		return nil // sql.ErrNoRows: nothing waits
	}
	m.digestMu.Lock()
	defer m.digestMu.Unlock()
	items, err := m.q.ListDigestItems(ctx, modeWorking)
	if err != nil || len(items) == 0 {
		return err
	}
	msg := buildDigest(items, now, m.d.Config.Location, m.publicLink)
	if ok, failed := m.d.Notify.DeliverNow(ctx, notify.Stored{CreatedAt: now.UTC(), Notification: msg}); ok == 0 && failed > 0 {
		return fmt.Errorf("every channel failed")
	}
	return m.q.DeleteDigestItemsUpTo(ctx, db.DeleteDigestItemsUpToParams{ID: items[len(items)-1].ID, HoldFor: modeWorking})
}

// ClaimHeldNotifications implements api.ServerInterface: the phone's panel
// takes the "working" queue and shows it itself.
func (m *Module) ClaimHeldNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m.digestMu.Lock()
	items, err := m.q.ListDigestItems(ctx, modeWorking)
	if err == nil && len(items) > 0 {
		err = m.q.DeleteDigestItemsUpTo(ctx, db.DeleteDigestItemsUpToParams{ID: items[len(items)-1].ID, HoldFor: modeWorking})
	}
	m.digestMu.Unlock()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.HeldNotification, 0, len(items))
	for _, it := range items {
		out = append(out, api.HeldNotification{Id: it.NotificationID, Kind: it.Kind, Title: it.Title, Body: it.Body, Link: it.Link, CreatedAt: it.CreatedAt})
	}
	if len(out) > 0 {
		m.d.Bus.Publish("notify.digest_sent", map[string]any{"count": len(out), "claimed": true})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}
