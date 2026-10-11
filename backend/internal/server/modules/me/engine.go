package me

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/db"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// dedupWindow: the same event reported twice within this time counts once.
// The sleep focus mode and the bedtime of the Health app both fire at about
// the same time, and Home Assistant may say the same thing as the phone.
const dedupWindow = 2 * time.Minute

// hostInfo is one computer that can report idle time.
type hostInfo struct {
	ID     string
	Name   string
	Online bool
	State  string         // active idle locked unknown offline
	Idle   *time.Duration // since the last key or mouse, when known
}

// realHosts lists the agents that can report idle time, with the state the
// hosts module worked out.
func (m *Module) realHosts(ctx context.Context) []hostInfo {
	agents, err := m.d.Agents.List(ctx)
	if err != nil {
		return nil
	}
	presence, havePresence := module.Lookup[contracts.Presence](m.d.Registry, contracts.PresenceKey)
	var out []hostInfo
	for _, a := range agents {
		if !a.Has(protocol.CapPresence) {
			continue
		}
		h := hostInfo{ID: a.ID, Name: a.Name, Online: a.Online, State: "offline"}
		if havePresence {
			p := presence.State(a.ID)
			h.State, h.Online = p.State, p.Online
			if p.IdleSeconds != nil {
				idle := time.Duration(*p.IdleSeconds) * time.Second
				h.Idle = &idle
			}
		}
		out = append(out, h)
	}
	return out
}

// workHosts are the computers that count as work computers: the chosen ones,
// or all when none was chosen.
func (c config) workHosts(all []hostInfo) []hostInfo {
	if len(c.WorkHosts) == 0 {
		return all
	}
	var out []hostInfo
	for _, h := range all {
		if slices.Contains(c.WorkHosts, h.ID) {
			out = append(out, h)
		}
	}
	return out
}

// loadSchedule reads the daily schedule from the habits module. The
// guess is off when it cannot be read.
func (m *Module) loadSchedule(ctx context.Context) *schedule {
	p, ok := module.Lookup[contracts.Schedule](m.d.Registry, contracts.ScheduleKey)
	if !ok {
		return nil
	}
	rules, err := p.Rules(ctx)
	if err != nil {
		return nil
	}
	wake, ok1 := parseClock(rules.WakeTime)
	sleep, ok2 := parseClock(rules.SleepTime)
	if !ok1 || !ok2 {
		return nil
	}
	loc, err := time.LoadLocation(rules.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return &schedule{Wake: wake, Sleep: sleep, Loc: loc}
}

// ensureLoaded reads what survives a restart: the settings, a manual state,
// the open state row and the times the decision remembers.
func (m *Module) ensureLoaded(ctx context.Context) error {
	if m.loaded {
		return nil
	}
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return err
	}
	manual, err := m.loadManual(ctx)
	if err != nil {
		return err
	}
	now := m.nowLocked()
	open, err := m.q.OpenMeState(ctx)
	switch {
	case err == nil:
		m.cur = &open
		if open.State == contracts.MeWorking {
			m.lastActive = now
		}
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if m.lastActive.IsZero() {
		end, err := m.q.LastMeStateEnd(ctx, contracts.MeWorking)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if end != nil {
			m.lastActive = *end
		}
	}
	sleeps, err := m.q.RecentMeSleeps(ctx)
	if err != nil {
		return err
	}
	for _, s := range sleeps {
		if s.EndedAt != nil && s.EndedAt.Sub(s.StartedAt) >= minSleep {
			m.lastWoke = *s.EndedAt
			break
		}
	}
	m.cfg, m.manual, m.loaded = cfg, manual, true
	return nil
}

// recompute decides the state now and records a change.
func (m *Module) recompute(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.recomputeLocked(ctx); err != nil {
		m.d.Log.Warn("me: work out the state", "err", err)
	}
}

func (m *Module) recomputeLocked(ctx context.Context) error {
	if err := m.ensureLoaded(ctx); err != nil {
		return err
	}
	now := m.nowLocked()
	if m.manual != nil && !m.manual.Until.IsZero() && !now.Before(m.manual.Until) {
		if err := m.d.Settings.Delete(ctx, manualKey); err != nil {
			return err
		}
		m.manual = nil
	}
	hosts := m.cfg.workHosts(m.listHosts(ctx))
	active := false
	var lastInput time.Time
	for _, h := range hosts {
		if h.State != "active" {
			continue
		}
		active = true
		if h.Idle != nil {
			if t := now.Add(-*h.Idle); t.After(lastInput) {
				lastInput = t
			}
		}
	}
	if active {
		m.lastActive = now
	}
	sleep, err := m.latestSleep(ctx)
	if err != nil {
		return err
	}
	// Using the computer after the sleep began ends the sleep. Input from
	// before it (a few minutes of idle time still counting as "in use" when
	// the user just got into bed) does not.
	if active && sleep.Event == evSleepStart && lastInput.After(sleep.At) {
		if sleep, err = m.insertReport(ctx, evSleepEnd, srcPC, now, now); err != nil {
			return err
		}
	}
	place, err := m.latestPlace(ctx)
	if err != nil {
		return err
	}
	var sched *schedule
	if m.cfg.GuessSleep {
		sched = m.loadSchedule(ctx)
	}
	d := decide(input{
		Now: now, Manual: m.manual, PCActive: active, LastActive: m.lastActive,
		Sleep: sleep, Place: place, GuessSleep: m.cfg.GuessSleep, Schedule: sched, LastWoke: m.lastWoke,
	})
	return m.apply(ctx, now, d)
}

// latestSleep and latestPlace read the newest report of each kind. Reports
// are ordered by the time of the event, so the newest event wins however late
// it arrived.
func (m *Module) latestSleep(ctx context.Context) (report, error) {
	row, err := m.q.LatestMeSleepReport(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return report{}, nil
	}
	return report{Event: row.Event, Source: row.Source, At: row.At}, err
}

func (m *Module) latestPlace(ctx context.Context) (report, error) {
	row, err := m.q.LatestMePlaceReport(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return report{}, nil
	}
	return report{Event: row.Event, Source: row.Source, At: row.At}, err
}

func (m *Module) insertReport(ctx context.Context, event, source string, at, now time.Time) (report, error) {
	row, err := m.q.InsertMeReport(ctx, db.InsertMeReportParams{Event: event, Source: source, At: at.UTC(), CreatedAt: now.UTC()})
	return report{Event: row.Event, Source: row.Source, At: row.At}, err
}

// apply records a change of state, tells the others and, when a sleep of
// three hours or more ended, tells them the user woke up.
func (m *Module) apply(ctx context.Context, now time.Time, d decision) error {
	guessed, manual := b2i(d.Guessed), b2i(d.Manual)
	if m.cur == nil || m.cur.State != d.State {
		started := now
		// A sleep the phone reported began when it said so, not when this
		// code noticed (the work computer may still have counted as in use).
		if d.State == contracts.MeSleeping && !d.SleptAt.IsZero() && d.SleptAt.Before(now) && (m.cur == nil || !d.SleptAt.Before(m.cur.StartedAt)) {
			started = d.SleptAt
		}
		old := m.cur
		if old != nil {
			end := started.UTC()
			if err := m.q.CloseMeState(ctx, db.CloseMeStateParams{EndedAt: &end, ID: old.ID}); err != nil {
				return err
			}
		}
		row, err := m.q.InsertMeState(ctx, db.InsertMeStateParams{State: d.State, Source: d.Source, Guessed: guessed, Manual: manual, StartedAt: started.UTC()})
		if err != nil {
			return err
		}
		m.cur = &row
		if old != nil && old.State == contracts.MeSleeping && now.Sub(old.StartedAt) >= minSleep {
			m.lastWoke = now
			m.d.Bus.Publish("me.woke", contracts.MeWoke{SleptAt: old.StartedAt, WokeAt: now, Guessed: old.Guessed != 0})
		}
	} else if m.cur.Source != d.Source || m.cur.Guessed != guessed || m.cur.Manual != manual {
		if err := m.q.UpdateMeStateFlags(ctx, db.UpdateMeStateFlagsParams{Source: d.Source, Guessed: guessed, Manual: manual, ID: m.cur.ID}); err != nil {
			return err
		}
		m.cur.Source, m.cur.Guessed, m.cur.Manual = d.Source, guessed, manual
	}
	view := api.MeState{State: api.MeStateName(d.State), Since: m.cur.StartedAt, Guessed: d.Guessed, Manual: d.Manual, Source: d.Source, AtHome: d.AtHome}
	if d.Manual && m.manual != nil && !m.manual.Until.IsZero() {
		until := m.manual.Until
		view.ManualUntil = &until
	}
	if !sameView(m.view, view) {
		m.view = view
		m.d.Bus.Publish("me.state", view)
		c := toContract(view)
		for ch := range m.subs {
			select {
			case ch <- c:
			default:
			}
		}
	}
	return nil
}

func sameView(a, b api.MeState) bool {
	return a.State == b.State && a.Since.Equal(b.Since) && a.Guessed == b.Guessed && a.Manual == b.Manual && a.Source == b.Source &&
		((a.AtHome == nil) == (b.AtHome == nil)) && (a.AtHome == nil || *a.AtHome == *b.AtHome) &&
		((a.ManualUntil == nil) == (b.ManualUntil == nil)) && (a.ManualUntil == nil || a.ManualUntil.Equal(*b.ManualUntil))
}

func b2i(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

// ingest stores one reported event and decides again. It reports false for a
// repeat of an event that was reported a moment ago.
func (m *Module) ingest(ctx context.Context, event, source string, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureLoaded(ctx); err != nil {
		return false, err
	}
	now := m.nowLocked()
	if at.IsZero() {
		at = now
	}
	at = at.Truncate(time.Second)
	n, err := m.q.CountMeReportsNear(ctx, db.CountMeReportsNearParams{Event: event, FromAt: at.Add(-dedupWindow).UTC(), ToAt: at.Add(dedupWindow).UTC()})
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if _, err := m.insertReport(ctx, event, source, at, now); err != nil {
		return false, err
	}
	return true, m.recomputeLocked(ctx)
}
