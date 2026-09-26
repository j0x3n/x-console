// Package calendar is the calendar part of M11: ICS subscriptions and
// read-only CalDAV calendars, synced every 15 minutes. Repeating events are
// stored once and expanded when queried, in the user's time zone.
//
// It provides contracts.Calendar (used by the daily brief) and the
// calendar.events action.
package calendar

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/db"
)

const (
	kindICS    = "ics"
	kindCalDAV = "caldav"
)

// Module implements api.ServerInterface and contracts.Calendar.
type Module struct {
	d    *module.Deps
	q    *db.Queries
	http *http.Client

	syncMu sync.Mutex // one sync at a time
	ctx    context.Context
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.Calendar  = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), http: &http.Client{Timeout: 30 * time.Second}, ctx: context.Background()}
	module.Provide[contracts.Calendar](d.Registry, contracts.CalendarKey, m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "calendar" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the periodic sync.
func (m *Module) Start(ctx context.Context) error {
	m.ctx = ctx
	m.d.Scheduler.Every("calendar.sync", syncInterval, m.syncAll)
	return nil
}

// syncLater refreshes a calendar in the background, for example right after
// it was added.
func (m *Module) syncLater(id int64) {
	ctx := m.ctx
	go func() {
		if _, err := m.syncOne(ctx, id); err != nil && ctx.Err() == nil {
			m.d.Log.Warn("calendar sync", "calendar", id, "err", err)
		}
	}()
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}

// Events implements contracts.Calendar.
func (m *Module) Events(ctx context.Context, from, to time.Time) ([]contracts.CalendarEvent, error) {
	occ, err := m.occurrences(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.CalendarEvent, 0, len(occ))
	for _, o := range occ {
		out = append(out, contracts.CalendarEvent{
			Title: o.row.Title, Start: o.start, End: o.end, AllDay: o.row.AllDay == 1,
			Location: o.row.Location, Calendar: o.row.CalendarName,
		})
	}
	return out, nil
}

// occurrences loads the candidate rows for a range and expands them.
func (m *Module) occurrences(ctx context.Context, from, to time.Time) ([]occurrence, error) {
	if !to.After(from) {
		return nil, httpx.Invalid("结束时间要晚于开始时间")
	}
	if to.Sub(from) > maxRange {
		return nil, httpx.Invalid("时间范围最长 100 天")
	}
	// Stored times are wall clock in the event's zone. Any zone is within
	// 14 hours of UTC, so a day of slack on both sides is enough.
	rows, err := m.q.ListEventCandidates(ctx, db.ListEventCandidatesParams{
		Since: wallOf(from.UTC()).AddDate(0, 0, -1),
		Until: wallOf(to.UTC()).AddDate(0, 0, 1),
	})
	if err != nil {
		return nil, err
	}
	return expand(rows, from, to, m.d.Config.Location), nil
}

// maxRange bounds GET /calendar/events.
const maxRange = 100 * 24 * time.Hour
