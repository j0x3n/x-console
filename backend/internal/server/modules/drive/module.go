package drive

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

type Module struct {
	d          *module.Deps
	q          *db.Queries
	store      files.Store // the drive's own part of the site's file store
	tmpDir     string      // uploads are received here before they get a name
	mu         sync.Mutex
	runMu      sync.Mutex
	syncMu     sync.Mutex
	syncStatus api.S3Status
	syncReq    chan struct{}
	tasksMu    sync.Mutex
	tasks      map[string]*driveTask
	taskSlots  chan struct{}
	taskBase   context.Context
	taskNow    func() time.Time
	shareMu    sync.Mutex
	shareHits  map[string]shareRate
	shareFails map[string]shareFailure
	blobMu     sync.Mutex
	blobLocks  map[string]*blobLock // see lockBlob
}

var _ api.ServerInterface = (*Module)(nil)
var _ module.Starter = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	if err := os.MkdirAll(d.Config.TmpDir(), 0700); err != nil {
		return nil, err
	}
	m := &Module{d: d, q: db.New(d.DB), store: d.Files.For("drive"), tmpDir: d.Config.TmpDir(), syncReq: make(chan struct{}, 1), syncStatus: api.S3Status{State: "off"}, tasks: make(map[string]*driveTask), taskSlots: make(chan struct{}, 2), taskNow: time.Now, shareHits: make(map[string]shareRate), shareFails: make(map[string]shareFailure), blobLocks: make(map[string]*blobLock)}
	m.registerActions()
	return m, nil
}
func (m *Module) Name() string { return "drive" }
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}
func (m *Module) Start(ctx context.Context) error {
	m.taskBase = ctx
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.syncReq:
				_ = m.syncAll(ctx)
			}
		}
	}()
	m.d.Scheduler.Every("drive.sync", 10*time.Minute, m.syncAll)
	m.d.Scheduler.Every("drive.purge", 24*time.Hour, m.purgeOldTrash)
	m.d.Scheduler.Every("drive.tasks.cleanup", time.Minute, m.pruneTasks)
	m.d.Scheduler.Every("drive.versions.prune", 24*time.Hour, m.pruneVersions)
	m.d.Scheduler.Every("drive.shares.prune", 24*time.Hour, m.pruneShares)
	m.d.Scheduler.Every("drive.shares.rate_cleanup", time.Minute, m.pruneShareRates)
	return nil
}

// blobKey and thumbnailKey are the keys of a file's content and its preview
// in the drive's store.
func blobKey(hash string) string      { return "blobs/" + hash[:2] + "/" + hash }
func thumbnailKey(hash string) string { return "thumbnails/" + hash + ".jpg" }

func (m *Module) row(ctx context.Context, id int64) (db.DriveItem, error) {
	if id <= 0 {
		return db.DriveItem{}, httpx.ErrNotFound
	}
	item, err := m.q.GetItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return item, httpx.ErrNotFound
	}
	return item, err
}
func (m *Module) visibleRow(ctx context.Context, id int64) (db.DriveItem, error) {
	item, err := m.row(ctx, id)
	if err != nil {
		return item, err
	}
	if item.Hidden != 0 && !auth.VaultUnlocked(ctx) {
		return item, httpx.ErrNotFound
	}
	return item, nil
}
func (m *Module) parent(ctx context.Context, id *int64, hidden bool) (*int64, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	p, err := m.visibleRow(ctx, *id)
	if err != nil {
		return nil, err
	}
	if p.IsDir == 0 || p.TrashedAt != nil || (p.Hidden != 0) != hidden {
		return nil, httpx.Invalid("目标文件夹不可用")
	}
	return &p.ID, nil
}
func validName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && !strings.ContainsAny(name, "/\\\x00\r\n")
}
func boolValue(p *bool) bool { return p != nil && *p }
func intBool(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func fail(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	httpx.Fail(w, r, err)
	return true
}

func (m *Module) event(kind string, item db.DriveItem) {
	if item.Hidden != 0 {
		m.d.Bus.Publish(kind, map[string]any{"id": item.ID, "hidden": true})
		return
	}
	m.d.Bus.Publish(kind, m.dto(context.Background(), item))
}

// path returns the ancestor names, skipping deleted ancestors only for restore hints.
func (m *Module) path(ctx context.Context, item db.DriveItem) ([]struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}, error) {
	var out []struct {
		Id   int64  `json:"id"`
		Name string `json:"name"`
	}
	seen := map[int64]bool{}
	for item.ParentID != nil {
		if seen[*item.ParentID] {
			return nil, httpx.Invalid("文件夹存在循环")
		}
		seen[*item.ParentID] = true
		p, err := m.row(ctx, *item.ParentID)
		if err != nil {
			return nil, err
		}
		out = append(out, struct {
			Id   int64  `json:"id"`
			Name string `json:"name"`
		}{p.ID, p.Name})
		item = p
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (m *Module) restorePath(ctx context.Context, from *int64) string {
	if from == nil || *from == 0 {
		return "/"
	}
	item, err := m.row(ctx, *from)
	if err != nil {
		return "/"
	}
	ancestors, err := m.path(ctx, item)
	if err != nil {
		return "/"
	}
	names := make([]string, 0, len(ancestors)+1)
	for _, p := range ancestors {
		names = append(names, p.Name)
	}
	return "/" + strings.Join(append(names, item.Name), "/")
}

func (m *Module) dto(ctx context.Context, item db.DriveItem) api.DriveItem {
	out := api.DriveItem{Id: item.ID, ParentId: item.ParentID, Name: item.Name, IsDir: item.IsDir != 0, Size: item.Size, Hidden: item.Hidden != 0, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, TrashedAt: item.TrashedAt, SyncState: "off"}
	shared := false
	if m.shareable(ctx, item) {
		var count int
		if err := m.d.DB.QueryRowContext(ctx, `SELECT count(*) FROM drive_shares WHERE item_id=? AND (expires_at IS NULL OR expires_at>?) AND (max_downloads IS NULL OR downloads<max_downloads)`, item.ID, time.Now().UTC()).Scan(&count); err == nil {
			shared = count > 0
		}
	}
	out.Shared = &shared
	if item.IsDir == 0 {
		out.Mime = &item.Mime
	}
	if item.Hidden != 0 && item.ParentID == nil {
		p := m.restorePath(ctx, item.HiddenFrom)
		out.RestoreTo = &p
	}
	cfg, _ := m.config(ctx)
	if cfg.Enabled && (!out.Hidden || cfg.IncludeHidden) && item.IsDir == 0 {
		out.SyncState = "pending"
		if item.S3Error != nil && *item.S3Error != "" {
			out.SyncState = "failed"
			out.SyncError = item.S3Error
		} else if item.S3SyncedAt != nil && !item.S3SyncedAt.Before(item.UpdatedAt) {
			out.SyncState = "synced"
		}
	}
	return out
}

func (m *Module) audit(ctx context.Context, kind string, id int64, err error) {
	m.d.Audit.Record(ctx, kind, strconv.FormatInt(id, 10), nil, err)
}
