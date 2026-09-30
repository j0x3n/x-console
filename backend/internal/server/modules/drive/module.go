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
	out, _ := m.dtos(ctx, []db.DriveItem{item})
	return out[0]
}

// itemColumns selects a full db.DriveItem, in scanItem's order.
const itemColumns = "id,parent_id,name,is_dir,size,mime,sha256,hidden,hidden_from,trashed_at,created_at,updated_at,s3_synced_at,s3_etag,s3_error,s3_key,s3_hash"

func scanItem(s interface{ Scan(...any) error }) (db.DriveItem, error) {
	var i db.DriveItem
	err := s.Scan(&i.ID, &i.ParentID, &i.Name, &i.IsDir, &i.Size, &i.Mime, &i.Sha256, &i.Hidden, &i.HiddenFrom, &i.TrashedAt,
		&i.CreatedAt, &i.UpdatedAt, &i.S3SyncedAt, &i.S3Etag, &i.S3Error, &i.S3Key, &i.S3Hash)
	return i, err
}

// dtos converts a list with a fixed number of queries: the sync settings
// once, the active shares in batches, and each distinct ancestor once.
func (m *Module) dtos(ctx context.Context, items []db.DriveItem) ([]api.DriveItem, error) {
	cfg, _ := m.config(ctx)
	ancestors := map[int64]bool{} // folder id -> it and its ancestors are neither hidden nor trashed
	shareable := make([]bool, len(items))
	var candidates []int64
	for i, item := range items {
		shareable[i] = m.shareableMemo(ctx, item, ancestors)
		if shareable[i] {
			candidates = append(candidates, item.ID)
		}
	}
	shared, err := m.activeShares(ctx, candidates)
	if err != nil {
		return nil, err
	}
	out := make([]api.DriveItem, 0, len(items))
	for i, item := range items {
		d := api.DriveItem{Id: item.ID, ParentId: item.ParentID, Name: item.Name, IsDir: item.IsDir != 0, Size: item.Size, Hidden: item.Hidden != 0, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, TrashedAt: item.TrashedAt, SyncState: "off"}
		isShared := shareable[i] && shared[item.ID]
		d.Shared = &isShared
		if item.IsDir == 0 {
			d.Mime = &item.Mime
		}
		if item.Hidden != 0 && item.ParentID == nil {
			p := m.restorePath(ctx, item.HiddenFrom)
			d.RestoreTo = &p
		}
		if cfg.Enabled && (!d.Hidden || cfg.IncludeHidden) && item.IsDir == 0 {
			d.SyncState = "pending"
			if item.S3Error != nil && *item.S3Error != "" {
				d.SyncState = "failed"
				d.SyncError = item.S3Error
			} else if item.S3SyncedAt != nil && !item.S3SyncedAt.Before(item.UpdatedAt) {
				d.SyncState = "synced"
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// shareableMemo is shareable with the answer for each folder kept in memo,
// so items in the same folder walk its ancestors once.
func (m *Module) shareableMemo(ctx context.Context, item db.DriveItem, memo map[int64]bool) bool {
	if item.Hidden != 0 || item.TrashedAt != nil {
		return false
	}
	var path []int64
	ok := true
	for parent := item.ParentID; parent != nil; {
		if known, seen := memo[*parent]; seen {
			ok = known
			break
		}
		if len(path) >= 64 {
			ok = false
			break
		}
		path = append(path, *parent)
		row, err := m.row(ctx, *parent)
		if err != nil || row.Hidden != 0 || row.TrashedAt != nil {
			ok = false
			break
		}
		parent = row.ParentID
	}
	// Every folder on the walked path shares the answer: above a bad folder
	// nothing was walked, and everything below it is bad too.
	for _, id := range path {
		memo[id] = ok
	}
	return ok
}

// activeShares reports which of ids have a link that still works.
func (m *Module) activeShares(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	now := time.Now().UTC()
	for len(ids) > 0 {
		batch := ids[:min(len(ids), 500)]
		ids = ids[len(batch):]
		args := make([]any, 0, len(batch)+1)
		for _, id := range batch {
			args = append(args, id)
		}
		args = append(args, now)
		rows, err := m.d.DB.QueryContext(ctx, `SELECT DISTINCT item_id FROM drive_shares WHERE item_id IN (?`+strings.Repeat(",?", len(batch)-1)+`)
AND (expires_at IS NULL OR expires_at>?) AND (max_downloads IS NULL OR downloads<max_downloads)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				break
			}
			out[id] = true
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m *Module) audit(ctx context.Context, kind string, id int64, err error) {
	m.d.Audit.Record(ctx, kind, strconv.FormatInt(id, 10), nil, err)
}
