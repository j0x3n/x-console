// Package journal keeps a diary and a per-day timeline of what happened in the
// other modules (B118). Modules offer contracts.ActivitySource; the journal
// collects from them on a schedule and stores the result, so history survives
// the caches and tables it came from. See docs/specs/B118.md.
package journal

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	// ServiceKey is where the module registers itself, for tests.
	ServiceKey = "journal.module"

	maxDiary      = 20000
	maxTitle      = 200
	maxDetail     = 500
	maxPerSource  = 2000
	freshFor      = 30 * time.Second
	recentDays    = 3  // days the scheduled job and the page refresh re-read
	backfillDays  = 90 // read once at the first start
	backfillChunk = 30
	maxSearchHits = 100
	backfillKey   = "journal.backfilled"
)

// kindOrder is the order of the counts in a day.
var kindOrder = []api.JournalKind{
	api.Commit, api.Pr, api.Card, api.Task, api.Focus, api.Habit, api.Workout,
	api.Event, api.Alert, api.Screen, api.Link, api.Note,
}

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time

	// collectMu makes one collection run at a time.
	collectMu sync.Mutex
	// mu guards lastCollect.
	mu          sync.Mutex
	lastCollect time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	// Calendar and host alerts already have contracts, so the journal reads
	// them itself instead of asking those modules for more code.
	module.Provide[contracts.ActivitySource](d.Registry, contracts.ActivitySourcePrefix+"calendar", calendarSource{m})
	module.Provide[contracts.ActivitySource](d.Registry, contracts.ActivitySourcePrefix+"hosts", hostAlertSource{m})
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "journal" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the collection and fills in the first 90 days once.
func (m *Module) Start(ctx context.Context) error {
	m.d.Scheduler.Every("journal.collect", 10*time.Minute, func(ctx context.Context) error {
		m.collectRecent(ctx)
		m.d.Bus.Publish("journal.collected", map[string]any{})
		return nil
	})
	go func() {
		var done bool
		if err := m.d.Settings.Get(ctx, backfillKey, &done); err != nil && !errors.Is(err, settings.ErrNotSet) {
			return
		}
		if done {
			return
		}
		if err := m.backfill(ctx); err != nil {
			m.d.Log.Warn("journal backfill", "err", err)
			return
		}
		_ = m.d.Settings.Set(ctx, backfillKey, true)
		m.d.Bus.Publish("journal.collected", map[string]any{})
	}()
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) loc() *time.Location {
	if l := m.d.Scheduler.Location(); l != nil {
		return l
	}
	return time.Local
}

const dayLayout = "2006-01-02"

func (m *Module) dayOf(t time.Time) string { return t.In(m.loc()).Format(dayLayout) }

// startOfDay is local midnight of the day t falls on.
func (m *Module) startOfDay(t time.Time) time.Time {
	t = t.In(m.loc())
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, m.loc())
}

func (m *Module) parseDay(s string) (time.Time, error) {
	t, err := time.ParseInLocation(dayLayout, s, m.loc())
	if err != nil {
		return time.Time{}, httpx.Invalid("日期要写成 YYYY-MM-DD")
	}
	return t, nil
}

// ---------- collection ----------

type namedSource struct {
	name string
	src  contracts.ActivitySource
}

func (m *Module) sources() []namedSource {
	var out []namedSource
	for key, src := range module.All[contracts.ActivitySource](m.d.Registry) {
		if name, ok := strings.CutPrefix(key, contracts.ActivitySourcePrefix); ok && name != "" {
			out = append(out, namedSource{name, src})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// collect reads every source for [from, until) and replaces what is stored for
// that range. A source that fails keeps its old items.
func (m *Module) collect(ctx context.Context, from, until time.Time) {
	m.collectMu.Lock()
	defer m.collectMu.Unlock()
	for _, s := range m.sources() {
		if ctx.Err() != nil {
			return
		}
		m.collectOne(ctx, s, from, until)
	}
}

func (m *Module) collectOne(ctx context.Context, s namedSource, from, until time.Time) {
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	items, err := s.src.Activity(sctx, from, until)
	if err != nil {
		m.d.Log.Warn("journal source", "source", s.name, "err", err)
		return
	}
	if len(items) > maxPerSource {
		items = items[:maxPerSource]
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)
	if err := q.DeleteSourceItems(ctx, db.DeleteSourceItemsParams{Source: s.name, At: from.UTC(), At_2: until.UTC()}); err != nil {
		m.d.Log.Warn("journal clear", "source", s.name, "err", err)
		return
	}
	for _, it := range items {
		title := clip(it.Title, maxTitle)
		if it.Ref == "" || it.Kind == "" || it.Module == "" || title == "" || it.At.Before(from) || !it.At.Before(until) {
			continue
		}
		err := q.UpsertItem(ctx, db.UpsertItemParams{
			Day: m.dayOf(it.At), At: it.At.UTC(), Module: it.Module, Source: s.name, Ref: it.Ref, Kind: it.Kind,
			Title: title, Detail: clip(it.Detail, maxDetail), Link: it.Link, Minutes: int64(it.Minutes),
		})
		if err != nil {
			m.d.Log.Warn("journal save", "source", s.name, "err", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		m.d.Log.Warn("journal commit", "source", s.name, "err", err)
	}
}

// collectRecent reads from the day before yesterday to the end of today.
func (m *Module) collectRecent(ctx context.Context) {
	today := m.startOfDay(m.now())
	m.collect(ctx, today.AddDate(0, 0, -(recentDays-1)), today.AddDate(0, 0, 1))
	m.mu.Lock()
	m.lastCollect = m.now()
	m.mu.Unlock()
}

func (m *Module) backfill(ctx context.Context) error {
	today := m.startOfDay(m.now())
	for end := today.AddDate(0, 0, 1); end.After(today.AddDate(0, 0, -backfillDays)); end = end.AddDate(0, 0, -backfillChunk) {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.collect(ctx, end.AddDate(0, 0, -backfillChunk), end)
	}
	return nil
}

// ensureFresh reads the recent days again unless that happened a moment ago.
func (m *Module) ensureFresh(ctx context.Context) {
	m.mu.Lock()
	recent := m.now().Sub(m.lastCollect) < freshFor
	m.mu.Unlock()
	if !recent {
		m.collectRecent(ctx)
	}
}

// ---------- hidden modules ----------

// visibility says whether items of a module may be shown to the caller now.
func (m *Module) visibility(ctx context.Context) func(module string) bool {
	h, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	cache := map[string]bool{}
	return func(name string) bool {
		if h == nil {
			return true
		}
		if v, ok := cache[name]; ok {
			return v
		}
		v := !h.Hidden(ctx, name)
		cache[name] = v
		return v
	}
}
