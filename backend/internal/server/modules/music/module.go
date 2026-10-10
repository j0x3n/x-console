// Package music is the music library (B146). Songs stay in the drive: this
// module indexes the audio files under the folders the user picked, reads
// their tags, keeps covers and lyrics, and serves playback and playlists.
package music

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

// ServiceKey is where the module registers itself, for tests and for other
// modules that need the library.
const ServiceKey = "music.library"

// FoldersKey is the setting that holds the drive folders to index.
const FoldersKey = "music.folders"

// scanDelay is how long the module waits after the last drive change before
// it scans, so a batch upload causes one scan.
const scanDelay = 3 * time.Second

// Module implements api.ServerInterface.
type Module struct {
	d      *module.Deps
	q      *db.Queries
	store  files.Store // covers
	tmpDir string

	scanReq   chan struct{}
	scanDelay time.Duration
	scanLock  chan struct{} // one slot: held while a scan runs
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), store: d.Files.For("music"), tmpDir: d.Config.TmpDir(),
		scanReq: make(chan struct{}, 1), scanDelay: scanDelay, scanLock: make(chan struct{}, 1),
	}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "music" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start scans once at startup, once a day, and a few seconds after the drive
// changes.
func (m *Module) Start(ctx context.Context) error {
	events, cancel := m.d.Bus.Subscribe("drive_item.", 256)
	m.d.Scheduler.Every("music.reconcile", 24*time.Hour, func(context.Context) error {
		m.requestScan()
		return nil
	})
	m.requestScan()
	go func() {
		defer cancel()
		var due <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-events:
				due = time.After(m.scanDelay)
			case <-m.scanReq:
				due = time.After(0)
			case <-due:
				due = nil
				if err := m.Reconcile(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("music scan failed", "err", err)
				}
			}
		}
	}()
	return nil
}

// requestScan asks the background loop for a scan. It never blocks.
func (m *Module) requestScan() {
	select {
	case m.scanReq <- struct{}{}:
	default:
	}
}

func (m *Module) drive() (contracts.DriveFiles, error) {
	d, ok := module.Lookup[contracts.DriveFiles](m.d.Registry, contracts.DriveFilesKey)
	if !ok {
		return nil, errors.New("云盘模块不可用")
	}
	return d, nil
}

// notFound turns a drive "not found" into the API's 404.
func notFound(err error) error {
	if errors.Is(err, contracts.ErrDriveNotFound) {
		return httpx.ErrNotFound
	}
	return err
}

func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}
