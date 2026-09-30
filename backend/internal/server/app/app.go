// Package app wires the server together: shared services, core API, feature
// modules, WebSockets and the static frontend.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/core"
	coreapi "github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/scheduler"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/internal/server/ws"
)

// APIPrefix is where every REST route lives.
const APIPrefix = "/api/v1"

// corePublic are core routes reachable without a session.
var corePublic = []string{"/health", "/auth/status", "/auth/setup", "/auth/setup/skip-totp", "/auth/login", "/agent/pair", "/agent/connect",
	"/agent/install.sh", "/agent/install.ps1", "/agent/uninstall.sh", "/agent/download", "/agent/setup.exe"}

// App is a built server.
type App struct {
	Deps    *module.Deps
	Handler http.Handler
	modules []module.Module
}

// New builds shared services and every module on an open database.
// extra modules are appended after the registered ones; tests use it.
func New(cfg config.Config, conn *sql.DB, extra ...func(*module.Deps) (module.Module, error)) (*App, error) {
	box, err := secrets.NewBox(cfg.MasterKey)
	if err != nil {
		return nil, err
	}
	bus := events.NewBus()
	auditLog := audit.New(conn)
	if err := files.MigrateLegacyLayout(slog.Default(), cfg.DataDir, cfg.FilesDir()); err != nil {
		return nil, fmt.Errorf("move old files: %w", err)
	}
	if err := resetDir(cfg.TmpDir()); err != nil {
		return nil, err
	}
	d := &module.Deps{
		Config:    cfg,
		DB:        conn,
		Log:       slog.Default(),
		Bus:       bus,
		Audit:     auditLog,
		Auth:      auth.NewService(conn, box, auditLog, !cfg.Dev),
		Secrets:   box,
		Settings:  settings.New(conn, box),
		Notify:    notify.New(conn, bus),
		Agents:    agenthub.New(conn, bus, auditLog),
		Scheduler: scheduler.New(cfg.Location),
		Files:     files.NewManager(files.Local{Root: cfg.FilesDir()}),
		Actions:   actions.NewRegistry(),
		Registry:  module.NewRegistry(),
	}
	a := &App{Deps: d}
	for _, build := range dedupe(append(append([]func(*module.Deps) (module.Module, error){}, constructors...), extra...)) {
		m, err := build(d)
		if err != nil {
			return nil, fmt.Errorf("module: %w", err)
		}
		a.modules = append(a.modules, m)
	}
	a.Handler = a.routes()
	return a, nil
}

// resetDir empties dir and makes sure it exists.
func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

// dedupe drops repeated constructors, so tests can pass a module that is
// already registered in modules.go without building it twice.
func dedupe(list []func(*module.Deps) (module.Module, error)) []func(*module.Deps) (module.Module, error) {
	seen := map[uintptr]bool{}
	out := list[:0]
	for _, fn := range list {
		ptr := reflect.ValueOf(fn).Pointer()
		if seen[ptr] {
			continue
		}
		seen[ptr] = true
		out = append(out, fn)
	}
	return out
}

// Start runs module background work and the scheduler.
func (a *App) Start(ctx context.Context) error {
	a.Deps.Scheduler.Every("auth.cleanup", time.Hour, a.Deps.Auth.CleanupExpired)
	for _, m := range a.modules {
		if s, ok := m.(module.Starter); ok {
			if err := s.Start(ctx); err != nil {
				return fmt.Errorf("start %s: %w", m.Name(), err)
			}
		}
	}
	a.Deps.Scheduler.Start()
	return nil
}

// Stop stops background work.
func (a *App) Stop() { a.Deps.Scheduler.Stop() }

func (a *App) routes() http.Handler {
	d := a.Deps
	public := append([]string{}, corePublic...)
	for _, m := range a.modules {
		if p, ok := m.(module.PublicPather); ok {
			public = append(public, p.PublicPaths()...)
		}
	}
	isPublic := func(path string) bool {
		rel := strings.TrimPrefix(path, APIPrefix)
		for _, p := range public {
			if rel == p || strings.HasPrefix(rel, strings.TrimSuffix(p, "/")+"/") {
				return true
			}
		}
		return false
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, httpx.ExposeRequestID, middleware.Recoverer, requestLog)
	r.Route(APIPrefix, func(api chi.Router) {
		api.Use(d.Auth.Middleware(isPublic))
		api.Get("/agent/connect", d.Agents.ServeConnect)
		api.Get("/events", ws.New(d.Bus, d.Agents).ServeHTTP)
		coreapi.HandlerWithOptions(&core.Handlers{Auth: d.Auth, Agents: d.Agents, Notify: d.Notify, Q: db.New(d.DB), Settings: d.Settings, Bus: d.Bus,
			PublicURL: d.Config.PublicURL, AgentsDir: d.Config.AgentsDir},
			coreapi.ChiServerOptions{BaseRouter: api, ErrorHandlerFunc: httpx.BadParam})
		for _, m := range a.modules {
			m.Mount(api)
		}
		api.NotFound(func(w http.ResponseWriter, r *http.Request) { httpx.Fail(w, r, httpx.ErrNotFound) })
	})
	if d.Config.WebDir != "" {
		r.NotFound(spa(d.Config.WebDir))
	}
	return r
}

// spa serves the built frontend and falls back to index.html for client routes.
func spa(dir string) http.HandlerFunc {
	files := http.FileServer(http.Dir(dir))
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	}
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if strings.HasSuffix(r.URL.Path, "/events") || strings.HasSuffix(r.URL.Path, "/agent/connect") {
			return
		}
		slog.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start))
	})
}
