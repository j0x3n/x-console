package drive

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

func (m *Module) nameTaken(ctx context.Context, q *sql.Tx, parent *int64, hidden bool, name string, except int64) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM drive_items WHERE parent_id IS ? AND hidden=? AND name=? AND trashed_at IS NULL AND id<>?", parent, intBool(hidden), name, except).Scan(&count)
	return count > 0, err
}
func (m *Module) freeName(ctx context.Context, q *sql.Tx, parent *int64, hidden bool, name string, except int64) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 0; n < 10000; n++ {
		candidate := name
		if n > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		taken, err := m.nameTaken(ctx, q, parent, hidden, candidate, except)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", httpx.ErrConflict
}
func (m *Module) write(ctx context.Context, fn func(*sql.Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (m *Module) insert(ctx context.Context, parent *int64, name string, isDir bool, size int64, mime, hash string, hidden bool, unique bool) (db.DriveItem, error) {
	var item db.DriveItem
	err := m.write(ctx, func(tx *sql.Tx) error {
		var err error
		if unique {
			name, err = m.freeName(ctx, tx, parent, hidden, name, 0)
		} else {
			var taken bool
			taken, err = m.nameTaken(ctx, tx, parent, hidden, name, 0)
			if taken {
				return httpx.NewError(409, "name_conflict", "同一文件夹已有同名条目")
			}
		}
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		item, err = db.New(tx).InsertItem(ctx, db.InsertItemParams{ParentID: parent, Name: name, IsDir: intBool(isDir), Size: size, Mime: mime, Sha256: hash, Hidden: intBool(hidden), CreatedAt: now, UpdatedAt: now})
		return err
	})
	if err == nil {
		m.event("drive_item.created", item)
		m.audit(ctx, "drive.create", item.ID, nil)
		m.triggerSync()
	}
	return item, err
}

func (m *Module) CreateDriveFolder(w http.ResponseWriter, r *http.Request) {
	var body api.CreateDriveFolderJSONRequestBody
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if !validName(body.Name) {
		httpx.Fail(w, r, httpx.Invalid("文件夹名称不正确"))
		return
	}
	hidden := boolValue(body.Hidden)
	if hidden && !auth.VaultUnlocked(r.Context()) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	parent, err := m.parent(r.Context(), body.ParentId, hidden)
	if fail(w, r, err) {
		return
	}
	item, err := m.insert(r.Context(), parent, body.Name, true, 0, "", "", hidden, false)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, 201, m.dto(r.Context(), item))
}

func (m *Module) ListDriveItems(w http.ResponseWriter, r *http.Request, p api.ListDriveItemsParams) {
	ctx := r.Context()
	hidden := boolValue(p.Hidden)
	trashed := boolValue(p.Trashed)
	if hidden && !auth.VaultUnlocked(ctx) {
		httpx.JSON(w, 200, map[string]any{"items": []api.DriveItem{}})
		return
	}
	var parent *int64
	if p.Parent != nil && *p.Parent > 0 && !trashed && (p.Q == nil || *p.Q == "") {
		var err error
		parent, err = m.parent(ctx, p.Parent, hidden)
		if fail(w, r, err) {
			return
		}
	}
	var args []any
	query := "SELECT " + itemColumns + " FROM drive_items WHERE hidden=?"
	args = append(args, intBool(hidden))
	if trashed {
		query += " AND trashed_at IS NOT NULL AND (parent_id IS NULL OR parent_id NOT IN (SELECT id FROM drive_items WHERE trashed_at IS NOT NULL))"
	} else {
		query += " AND trashed_at IS NULL"
	}
	if p.Q != nil && strings.TrimSpace(*p.Q) != "" {
		query += " AND name LIKE ? ESCAPE '\\'"
		term := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(*p.Q))
		args = append(args, "%"+term+"%")
	} else if !trashed {
		query += " AND parent_id IS ?"
		args = append(args, parent)
	}
	query += " ORDER BY is_dir DESC, name COLLATE NOCASE, id"
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if fail(w, r, err) {
		return
	}
	var found []db.DriveItem
	for rows.Next() {
		var item db.DriveItem
		if item, err = scanItem(rows); err != nil {
			break
		}
		found = append(found, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if fail(w, r, err) {
		return
	}
	items, err := m.dtos(ctx, found)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": items})
}

func (m *Module) GetDriveItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	item, err := m.visibleRow(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	if item.TrashedAt != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	path, err := m.path(r.Context(), item)
	if fail(w, r, err) {
		return
	}
	if path == nil {
		path = []struct {
			Id   int64  `json:"id"`
			Name string `json:"name"`
		}{}
	}
	raw := m.dto(r.Context(), item)
	out := api.DriveItemWithPath{Id: raw.Id, ParentId: raw.ParentId, Name: raw.Name, IsDir: raw.IsDir, Size: raw.Size, Mime: raw.Mime, Hidden: raw.Hidden, RestoreTo: raw.RestoreTo, TrashedAt: raw.TrashedAt, SyncState: api.DriveItemWithPathSyncState(raw.SyncState), SyncError: raw.SyncError, CreatedAt: raw.CreatedAt, UpdatedAt: raw.UpdatedAt, Path: path}
	httpx.JSON(w, 200, out)
}

func (m *Module) descendant(ctx context.Context, tx *sql.Tx, id, parentID int64) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT count(*) FROM subtree WHERE id=?`, id, parentID).Scan(&count)
	return count > 0, err
}

func (m *Module) UpdateDriveItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	ctx := r.Context()
	var body api.UpdateDriveItem
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	item, err := m.visibleRow(ctx, id)
	if fail(w, r, err) {
		return
	}
	if item.TrashedAt != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if body.Name != nil && !validName(*body.Name) {
		httpx.Fail(w, r, httpx.Invalid("名称不正确"))
		return
	}
	if body.Hidden != nil && !auth.VaultUnlocked(ctx) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	hidden := item.Hidden != 0
	parent := item.ParentID
	name := item.Name
	from := item.HiddenFrom
	if body.Name != nil {
		name = *body.Name
	}
	if body.Hidden != nil && *body.Hidden != hidden {
		hidden = *body.Hidden
		if hidden {
			origin := int64(0)
			if parent != nil {
				origin = *parent
			}
			from = &origin
			parent = nil
		} else {
			parent = nil
			if from != nil && *from > 0 {
				p, e := m.row(ctx, *from)
				if e == nil && p.IsDir != 0 && p.Hidden == 0 && p.TrashedAt == nil {
					parent = &p.ID
				}
			}
			from = nil
		}
	} else if body.ParentId != nil {
		parent, err = m.parent(ctx, body.ParentId, hidden)
		if fail(w, r, err) {
			return
		}
	}
	err = m.write(ctx, func(tx *sql.Tx) error {
		if parent != nil {
			var cycle bool
			cycle, err = m.descendant(ctx, tx, id, *parent)
			if err != nil {
				return err
			}
			if cycle {
				return httpx.Invalid("不能移到自己的子文件夹")
			}
		}
		if body.Hidden != nil && *body.Hidden != (item.Hidden != 0) && !hidden {
			name, err = m.freeName(ctx, tx, parent, hidden, name, id)
		} else {
			var taken bool
			taken, err = m.nameTaken(ctx, tx, parent, hidden, name, id)
			if taken {
				return httpx.NewError(409, "name_conflict", "同一文件夹已有同名条目")
			}
		}
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, "UPDATE drive_items SET parent_id=?,name=?,hidden_from=?,updated_at=? WHERE id=?", parent, name, from, now, id)
		if err != nil {
			return err
		}
		if hidden != (item.Hidden != 0) {
			_, err = tx.ExecContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) UPDATE drive_items SET hidden=?,updated_at=? WHERE id IN (SELECT id FROM subtree)`, id, intBool(hidden), now)
		} else if item.IsDir != 0 && (body.Name != nil || body.ParentId != nil) {
			_, err = tx.ExecContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) UPDATE drive_items SET updated_at=? WHERE id IN (SELECT id FROM subtree)`, id, now)
		}
		return err
	})
	if fail(w, r, err) {
		return
	}
	item, err = m.row(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.event("drive_item.updated", item)
	m.audit(ctx, "drive.update", id, nil)
	m.triggerSync()
	httpx.JSON(w, 200, m.dto(ctx, item))
}

func (m *Module) DeleteDriveItem(w http.ResponseWriter, r *http.Request, id api.ItemId, p api.DeleteDriveItemParams) {
	ctx := r.Context()
	permanent := boolValue(p.Permanent)
	if permanent && fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	item, err := m.visibleRow(ctx, id)
	if fail(w, r, err) {
		return
	}
	if item.TrashedAt != nil && !permanent {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if permanent {
		err = m.permanentDelete(ctx, id)
	} else {
		err = m.write(ctx, func(tx *sql.Tx) error {
			now := time.Now().UTC()
			// Items already in the trash keep their own time, so restoring
			// this folder does not bring them back.
			_, err := tx.ExecContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) UPDATE drive_items SET trashed_at=?,updated_at=? WHERE id IN (SELECT id FROM subtree) AND trashed_at IS NULL`, id, now, now)
			return err
		})
	}
	if fail(w, r, err) {
		return
	}
	m.event("drive_item.deleted", item)
	m.audit(ctx, "drive.delete", id, nil)
	httpx.NoContent(w)
}

func (m *Module) RestoreDriveItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	ctx := r.Context()
	item, err := m.visibleRow(ctx, id)
	if fail(w, r, err) {
		return
	}
	if item.TrashedAt == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	parent := item.ParentID
	if parent != nil {
		p, e := m.row(ctx, *parent)
		if e != nil || p.TrashedAt != nil || p.Hidden != item.Hidden {
			parent = nil
		}
	}
	name := item.Name
	err = m.write(ctx, func(tx *sql.Tx) error {
		name, err = m.freeName(ctx, tx, parent, item.Hidden != 0, name, id)
		if err != nil {
			return err
		}
		// Only what was deleted together with this item comes back. A file
		// deleted earlier, then its folder, stays in the trash.
		_, err = tx.ExecContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) UPDATE drive_items SET trashed_at=NULL,updated_at=? WHERE id IN (SELECT id FROM subtree) AND trashed_at=?`, id, time.Now().UTC(), item.TrashedAt)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE drive_items SET parent_id=?,name=? WHERE id=?", parent, name, id)
		return err
	})
	if fail(w, r, err) {
		return
	}
	item, err = m.row(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.event("drive_item.updated", item)
	m.audit(ctx, "drive.restore", id, nil)
	m.triggerSync()
	httpx.JSON(w, 200, m.dto(ctx, item))
}

func (m *Module) GetDriveUsage(w http.ResponseWriter, r *http.Request) {
	var files, bytes, trash int64
	err := m.d.DB.QueryRowContext(r.Context(), "SELECT count(*) FILTER (WHERE is_dir=0 AND trashed_at IS NULL), COALESCE(sum(CASE WHEN is_dir=0 AND trashed_at IS NULL THEN size ELSE 0 END),0), COALESCE(sum(CASE WHEN is_dir=0 AND trashed_at IS NOT NULL THEN size ELSE 0 END),0) FROM drive_items WHERE hidden=0").Scan(&files, &bytes, &trash)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, map[string]int64{"files": files, "bytes": bytes, "trashBytes": trash})
}

func (m *Module) purgeOldTrash(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id FROM drive_items WHERE trashed_at<? AND (parent_id IS NULL OR parent_id NOT IN (SELECT id FROM drive_items WHERE trashed_at IS NOT NULL))", cutoff)
	if err != nil {
		return err
	}
	var ids []int64
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
		if err := m.permanentDelete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) permanentDelete(ctx context.Context, id int64) error {
	var hashes, keys []string
	err := m.write(ctx, func(tx *sql.Tx) error {
		versionRows, err := tx.QueryContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT sha256 FROM drive_file_versions WHERE item_id IN (SELECT id FROM subtree)`, id)
		if err != nil {
			return err
		}
		for versionRows.Next() {
			var hash string
			if err = versionRows.Scan(&hash); err != nil {
				break
			}
			hashes = append(hashes, hash)
		}
		if err == nil {
			err = versionRows.Err()
		}
		versionRows.Close()
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT sha256,s3_key FROM drive_items WHERE id IN (SELECT id FROM subtree) AND is_dir=0`, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var hash string
			var key *string
			if err = rows.Scan(&hash, &key); err != nil {
				break
			}
			hashes = append(hashes, hash)
			if key != nil {
				keys = append(keys, *key)
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		for _, key := range keys {
			if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO drive_s3_deletions(key,created_at) VALUES(?,?)", key, time.Now().UTC()); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) DELETE FROM drive_items WHERE id IN (SELECT id FROM subtree)`, id)
		return err
	})
	if err != nil {
		return err
	}
	for _, hash := range hashes {
		m.dropBlob(ctx, hash)
	}
	m.triggerSync()
	return nil
}
