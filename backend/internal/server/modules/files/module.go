package files

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	filestore "github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/files/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/maintenance"
)

type Module struct {
	d   *module.Deps
	now func() time.Time
}

var _ api.ServerInterface = (*Module)(nil)
var _ contracts.Files = (*Module)(nil)
var _ module.Starter = (*Module)(nil)

func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, now: func() time.Time { return time.Now().UTC() }}
	module.Provide[contracts.Files](d.Registry, contracts.FilesKey, m)
	module.Provide[contracts.Cleaner](d.Registry, contracts.MaintenanceCleanerPrefix+"uploads", maintenance.AttachmentCleaner{Deps: d, Now: m.now})
	for _, scope := range []string{"projects", "calendar", "reminders", "coding"} {
		module.Provide[contracts.StorageReporter](d.Registry, contracts.MaintenanceStoragePrefix+"uploads."+scope, maintenance.StoreReporter{Store: m.store(scope), Registry: d.Registry, Key: "uploads." + scope, Label: "公共上传 " + scope, Module: scope, Prefix: "uploads"})
	}
	return m, nil
}
func (m *Module) Name() string { return "files" }
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}
func (m *Module) Start(context.Context) error {
	m.d.Scheduler.Every("files.cleanup", 24*time.Hour, m.cleanup)
	return nil
}
func (m *Module) store(scope string) filestore.Store { return m.d.Files.For(scope) }
func key(id int64) string                            { return "uploads/" + strconv.FormatInt(id, 10) }

type row struct {
	id                int64
	scope, name, mime string
	size              int64
}

func (m *Module) file(ctx context.Context, id int64) (row, error) {
	var x row
	err := m.d.DB.QueryRowContext(ctx, "SELECT id,scope,name,mime,size FROM uploaded_files WHERE id=?", id).Scan(&x.id, &x.scope, &x.name, &x.mime, &x.size)
	if err == sql.ErrNoRows {
		return x, httpx.ErrNotFound
	}
	return x, err
}
func (m *Module) delete(ctx context.Context, id int64) error {
	x, err := m.file(ctx, id)
	if err != nil {
		return err
	}
	if err = m.store(x.scope).Delete(ctx, key(id)); err != nil {
		return err
	}
	if err = m.store(x.scope).Delete(ctx, key(id)+".thumb.jpg"); err != nil {
		return err
	}
	_, err = m.d.DB.ExecContext(ctx, "DELETE FROM uploaded_files WHERE id=?", id)
	return err
}

var embeddedFile = regexp.MustCompile(`/api/v1/files/([0-9]+)\b`)

func (m *Module) Claim(ctx context.Context, kind string, ownerID int64, markdown string) error {
	if ownerID <= 0 {
		return httpx.Invalid("文件归属无效")
	}
	scope := map[string]string{"project": "projects", "issue": "projects", "comment": "projects", "calendar": "calendar", "reminder": "reminders", "coding": "coding"}[kind]
	if scope == "" {
		return httpx.Invalid("文件归属类型无效")
	}
	seen := map[int64]bool{}
	for _, match := range embeddedFile.FindAllStringSubmatch(markdown, -1) {
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err = m.d.DB.ExecContext(ctx, "UPDATE uploaded_files SET owner_kind=?,owner_id=? WHERE id=? AND scope=? AND owner_kind IS NULL", kind, ownerID, id, scope); err != nil {
			return err
		}
	}
	return nil
}
func (m *Module) DeleteOwned(ctx context.Context, kind string, ownerID int64) error {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id FROM uploaded_files WHERE owner_kind=? AND owner_id=?", kind, ownerID)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = m.delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (m *Module) cleanup(ctx context.Context) error {
	fn := func(ctx context.Context) error {
		ctx = contracts.IgnoreHidden(ctx)
		cleaner := maintenance.AttachmentCleaner{Deps: m.d, Now: m.now}
		items, err := cleaner.Scan(ctx)
		if err != nil {
			return err
		}
		ids := []string{}
		for _, item := range items {
			if item.Kind == "unclaimed_uploads" {
				ids = append(ids, item.ID)
			}
		}
		_, err = cleaner.Clean(ctx, ids)
		return err
	}
	if storage, ok := module.Lookup[contracts.MaintenanceStorage](m.d.Registry, contracts.MaintenanceStorageKey); ok {
		return storage.WithCleanup(ctx, fn)
	}
	return fn(ctx)
}
func validScope(scope string) bool {
	return strings.Contains("|projects|calendar|reminders|coding|", "|"+scope+"|") && scope != ""
}
