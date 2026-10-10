// Package screentime reports which program the user had in front of them,
// one sample per minute (B116). Only Windows desktops declare the capability.
// See docs/specs/B116.md.
package screentime

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/presence"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// sampleEvery is how often the foreground window is looked at.
	sampleEvery = 15 * time.Second
	// idleLimit is how long without keyboard or mouse still counts as being
	// at the computer (reading, watching a video).
	idleLimit = 5 * time.Minute
	// maxApp and maxTitle cap what is sent.
	maxApp   = 120
	maxTitle = 300
)

// Window is what is in front of the user now.
type Window struct {
	App   string
	Title string
}

// Reading is one look at the computer.
type Reading struct {
	// Active is false when the screen is locked, the user has been away for
	// too long or the state is not known.
	Active bool
	Window Window
	OK     bool // Window is filled in
}

// minute collects the readings of one calendar minute.
type minute struct {
	at     int64
	counts map[string]int
	titles map[string]string
	order  []string
}

// Aggregator turns readings into one sample per minute: the program that was
// in front most often while the user was there. A minute with no reading of
// an active user produces nothing.
type Aggregator struct {
	cur *minute
}

// Add records a reading taken at t. When t is in a later minute than the
// readings so far, it returns the finished minute.
func (a *Aggregator) Add(t time.Time, r Reading) (protocol.ScreenSample, bool) {
	m := t.Unix() / 60
	var out protocol.ScreenSample
	var done bool
	if a.cur != nil && a.cur.at != m {
		out, done = a.cur.result()
		a.cur = nil
	}
	if !r.Active || !r.OK || r.Window.App == "" {
		return out, done
	}
	if a.cur == nil {
		a.cur = &minute{at: m, counts: map[string]int{}, titles: map[string]string{}}
	}
	app := r.Window.App
	if _, seen := a.cur.counts[app]; !seen {
		a.cur.order = append(a.cur.order, app)
	}
	a.cur.counts[app]++
	a.cur.titles[app] = r.Window.Title
	return out, done
}

// result picks the program with the most readings. Ties go to the one seen
// first. The title is the last one seen for that program.
func (m *minute) result() (protocol.ScreenSample, bool) {
	best, bestN := "", 0
	for _, app := range m.order {
		if m.counts[app] > bestN {
			best, bestN = app, m.counts[app]
		}
	}
	if best == "" {
		return protocol.ScreenSample{}, false
	}
	return protocol.ScreenSample{Minute: m.at, App: clip(best, maxApp), Title: clip(m.titles[best], maxTitle)}, true
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// Register starts reporting when this system can read the foreground window.
func Register(c *conn.Client) {
	if !Available() {
		return
	}
	register(c, readNow, time.Now, sampleEvery)
}

func readNow(ctx context.Context) Reading {
	p := presence.Get(ctx)
	if !p.Known || (p.Locked != nil && *p.Locked) || time.Duration(p.IdleSeconds)*time.Second >= idleLimit {
		return Reading{}
	}
	w, ok := foreground()
	return Reading{Active: true, Window: w, OK: ok}
}

func register(c *conn.Client, read func(context.Context) Reading, now func() time.Time, every time.Duration) {
	c.OnConnect(func(ctx context.Context) {
		var agg Aggregator
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s, ok := agg.Add(now(), read(ctx)); ok {
					_ = c.Emit(ctx, protocol.EventScreenSample, s)
				}
			}
		}
	})
}
