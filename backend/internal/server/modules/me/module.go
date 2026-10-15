// Package me works out what the user is doing now: working at a computer,
// away from it, asleep, out of the house or at home (B162). The signals are
// the idle time of the work computers, and reports from an iPhone shortcut or
// Home Assistant for sleep and for coming and leaving home. Other modules read
// the state through contracts.Me. See docs/specs/B162-B164.md.
package me

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/db"
)

const (
	keepStatesFor  = 180 * 24 * time.Hour
	keepReportsFor = 30 * 24 * time.Hour
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "me.module"

// Module implements api.ServerInterface and contracts.Me.
type Module struct {
	d *module.Deps
	q *db.Queries

	// mu guards the fields below. Every recompute holds it, so reports, the
	// minute tick and presence changes never decide at the same time.
	mu         sync.Mutex
	nowFn      func() time.Time
	loaded     bool
	cfg        config
	manual     *manualSetting
	cur        *db.MeState // the open row of me_states
	lastActive time.Time   // the last time a work computer was in use
	lastWoke   time.Time   // when the last sleep of three hours or more ended
	phoneSeen  time.Time   // B164: the last POST /me/seen from a phone
	view       api.MeState
	subs       map[chan contracts.MeState]struct{}
	// listHosts lists the computers that report idle time. Tests replace it.
	listHosts func(ctx context.Context) []hostInfo

	haMu    sync.Mutex
	haLast  map[string]string // last state of the Home Assistant entities
	tokenMu sync.Mutex        // guards the report token setting
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ contracts.Me        = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
	_ module.PublicPather = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), nowFn: time.Now, subs: map[chan contracts.MeState]struct{}{}, haLast: map[string]string{}}
	m.listHosts = m.realHosts
	m.registerActions()
	module.Provide[contracts.Me](d.Registry, contracts.MeKey, m)
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "me" }

// Mount implements module.Module. The report route is public (it checks its
// own token), and a public prefix also covers /me/report/token and the other
// methods of /me/report, so the guard below sends those back to the normal
// session and CSRF checks.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam, Middlewares: []api.MiddlewareFunc{sessionGuard}})
}

// PublicPaths implements module.PublicPather.
func (m *Module) PublicPaths() []string { return []string{reportPath} }

// sessionGuard asks for a login session (and the CSRF header on writes) on
// every route except the token-checked report itself.
func sessionGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1"+reportPath {
			next.ServeHTTP(w, r)
			return
		}
		if auth.FromContext(r.Context()) == nil {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(auth.CSRFHeader) != "x-console" {
			httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "csrf", "缺少 X-Requested-With 请求头"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start follows the computers and Home Assistant, and decides once a minute.
func (m *Module) Start(ctx context.Context) error {
	m.recompute(ctx)
	m.d.Scheduler.Every("me.tick", time.Minute, func(ctx context.Context) error {
		m.recompute(ctx)
		return nil
	})
	m.d.Scheduler.Every("me.cleanup", time.Hour, m.cleanup)
	presence, cancelPresence := m.d.Bus.Subscribe("host.presence", 64)
	go func() {
		defer cancelPresence()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-presence:
				if !ok {
					return
				}
				m.recompute(ctx)
			}
		}
	}()
	m.followHA(ctx)
	return nil
}

func (m *Module) now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nowFn().Truncate(time.Second)
}

// nowLocked is now for code that already holds mu.
func (m *Module) nowLocked() time.Time { return m.nowFn().Truncate(time.Second) }

func (m *Module) cleanup(ctx context.Context) error {
	now := m.now()
	before := now.Add(-keepStatesFor).UTC()
	if err := m.q.DeleteMeStatesBefore(ctx, &before); err != nil {
		return err
	}
	return m.q.DeleteMeReportsBefore(ctx, now.Add(-keepReportsFor).UTC())
}

// Current implements contracts.Me.
func (m *Module) Current() contracts.MeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return toContract(m.view)
}

// Subscribe implements contracts.Me.
func (m *Module) Subscribe() (<-chan contracts.MeState, func()) {
	ch := make(chan contracts.MeState, 16)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.subs, ch)
			close(ch)
			m.mu.Unlock()
		})
	}
}

func toContract(v api.MeState) contracts.MeState {
	return contracts.MeState{State: string(v.State), Since: v.Since, Guessed: v.Guessed, Manual: v.Manual, Source: v.Source, AtHome: v.AtHome,
		PhoneActive: v.PhoneActive != nil && *v.PhoneActive, Host: deref(v.Host)}
}
