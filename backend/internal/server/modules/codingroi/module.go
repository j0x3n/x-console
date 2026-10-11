// Package codingroi is the cost report of AI coding (B129): for every coding
// task of a week or a month it adds up the cost, the time the AI ran, the
// time the user spent reviewing and whether the work was merged, and groups
// the result by repository, tool or agent. It only reads other modules'
// tables. The one thing it records itself is whether a task's pull request
// was merged, because the coding module does not keep that.
// See docs/specs/B129.md.
package codingroi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/codingroi/api"
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "codingroi.module"

// syncEvery is how often pull requests are checked for a merge.
const syncEvery = 30 * time.Minute

// Module implements api.ServerInterface.
type Module struct {
	d   *module.Deps
	now func() time.Time
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, now: func() time.Time { return time.Now().UTC() }}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "codingroi" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start follows merged pull requests and checks the GitHub cache.
func (m *Module) Start(ctx context.Context) error {
	events, cancel := m.d.Bus.Subscribe("coding_task.merged", 64)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				var p struct {
					TaskID int64  `json:"taskId"`
					PRURL  string `json:"prUrl"`
				}
				raw, err := json.Marshal(ev.Data)
				if err != nil || json.Unmarshal(raw, &p) != nil || p.TaskID == 0 {
					continue
				}
				if err := m.recordOutcome(ctx, p.TaskID, "merged", m.now(), "webhook", p.PRURL); err != nil {
					m.d.Log.Warn("codingroi: record merge", "task", p.TaskID, "err", err)
				}
			}
		}
	}()
	m.d.Scheduler.Every("codingroi.sync", syncEvery, m.syncPulls)
	return nil
}

// GetCodingROIReport implements GET /codingroi/report.
func (m *Module) GetCodingROIReport(w http.ResponseWriter, r *http.Request, params api.GetCodingROIReportParams) {
	q, err := parseQuery(m.location(), string(deref(params.Period)), deref(params.Date), string(deref(params.GroupBy)), m.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rep, err := m.report(r.Context(), q)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

// ExportCodingROIReport implements GET /codingroi/report.csv.
func (m *Module) ExportCodingROIReport(w http.ResponseWriter, r *http.Request, params api.ExportCodingROIReportParams) {
	q, err := parseQuery(m.location(), string(deref(params.Period)), deref(params.Date), string(deref(params.GroupBy)), m.now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rep, err := m.report(r.Context(), q)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="coding-roi-`+rep.From+`-`+rep.To+`.csv"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := writeCSV(w, rep, m.location()); err != nil {
		m.d.Log.Warn("codingroi: write csv", "err", err)
	}
}

func (m *Module) location() *time.Location {
	if loc := m.d.Scheduler.Location(); loc != nil {
		return loc
	}
	return time.Local
}

// screenHidden reports whether the screen time module is hidden from the
// caller, so the review time must not be shown.
func (m *Module) screenHidden(ctx context.Context) bool {
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	return ok && h.Hidden(ctx, "screentime")
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
