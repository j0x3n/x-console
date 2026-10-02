package maintenance

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type Module struct {
	d          *module.Deps
	mu         sync.Mutex
	ctx        context.Context
	busy       bool
	scan       Job
	cleanup    Job
	snapshot   map[string][]contracts.CleanupItem
	consumed   map[string]bool
	started    time.Time
	points     []Point
	usage      []contracts.StorageUsage
	usageAt    time.Time
	usageMu    sync.Mutex
	sampleMu   sync.Mutex
	cpuAt      time.Time
	cpuSeconds float64
}

func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, ctx: context.Background(), started: time.Now().UTC(), snapshot: map[string][]contracts.CleanupItem{}, consumed: map[string]bool{}, scan: Job{State: "idle", Groups: []Group{}}, cleanup: Job{State: "idle", Groups: []Group{}}}
	module.Provide[contracts.Cleaner](d.Registry, contracts.MaintenanceCleanerPrefix+"records", recordCleaner{d.DB})
	module.Provide[contracts.Cleaner](d.Registry, contracts.MaintenanceCleanerPrefix+"tmp", temporaryCleaner{d.Config.TmpDir(), d.Registry})
	return m, nil
}

func (m *Module) Name() string { return "maintenance" }

func (m *Module) Mount(r chi.Router) {
	r.Get("/maintenance/overview", m.overviewHandler)
	r.Get("/maintenance/metrics", func(w http.ResponseWriter, r *http.Request) { httpx.JSON(w, http.StatusOK, m.metrics()) })
	r.Post("/maintenance/vacuum", m.vacuumHandler)
	r.Post("/maintenance/scan", m.scanHandler)
	r.Get("/maintenance/scan", func(w http.ResponseWriter, r *http.Request) { httpx.JSON(w, http.StatusOK, m.job(r.Context(), false)) })
	r.Post("/maintenance/cleanup", m.cleanupHandler)
	r.Get("/maintenance/cleanup", func(w http.ResponseWriter, r *http.Request) { httpx.JSON(w, http.StatusOK, m.job(r.Context(), true)) })
}

func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	m.sample(ctx)
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.sample(ctx)
			}
		}
	}()
	return nil
}

func (m *Module) hidden(ctx context.Context, name string) bool {
	if name == "public_uploads" {
		return m.hidden(ctx, "projects") || m.hidden(ctx, "calendar") || m.hidden(ctx, "reminders") || m.hidden(ctx, "coding")
	}
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	return ok && contracts.BackendBlocked(ctx, h, name, "/"+name)
}
