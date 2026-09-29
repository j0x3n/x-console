package drive

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// recordVersion stores the content that is about to be replaced. Call it in
// the same transaction as the replacement, after reading the current row.
func (m *Module) recordVersion(ctx context.Context, tx *sql.Tx, item db.DriveItem, nextHash string) error {
	if item.IsDir != 0 || item.Sha256 == "" || item.Sha256 == nextHash {
		return nil
	}
	var latest string
	err := tx.QueryRowContext(ctx, "SELECT sha256 FROM drive_file_versions WHERE item_id=? ORDER BY created_at DESC,id DESC LIMIT 1", item.ID).Scan(&latest)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && latest == item.Sha256 {
		return nil
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO drive_file_versions(item_id,size,sha256,created_at) VALUES(?,?,?,?)", item.ID, item.Size, item.Sha256, time.Now().UTC())
	return err
}

func (m *Module) versionItem(ctx context.Context, id int64) (db.DriveItem, error) {
	item, err := m.visibleRow(ctx, id)
	if err != nil {
		return item, err
	}
	if item.TrashedAt != nil {
		return item, httpx.ErrNotFound
	}
	if item.IsDir != 0 {
		return item, httpx.Invalid("文件夹没有历史版本")
	}
	return item, nil
}

func (m *Module) ListDriveVersions(w http.ResponseWriter, r *http.Request, itemID api.ItemId) {
	ctx := r.Context()
	if _, err := m.versionItem(ctx, itemID); fail(w, r, err) {
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,size,sha256,created_at FROM drive_file_versions WHERE item_id=? ORDER BY created_at DESC,id DESC", itemID)
	if fail(w, r, err) {
		return
	}
	defer rows.Close()
	items := []api.DriveVersion{}
	for rows.Next() {
		var item api.DriveVersion
		if err = rows.Scan(&item.Id, &item.Size, &item.Sha256, &item.CreatedAt); err != nil {
			break
		}
		items = append(items, item)
	}
	if err == nil {
		err = rows.Err()
	}
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (m *Module) GetDriveVersionContent(w http.ResponseWriter, r *http.Request, itemID api.ItemId, versionID api.VersionId) {
	ctx := r.Context()
	item, err := m.versionItem(ctx, itemID)
	if fail(w, r, err) {
		return
	}
	var hash string
	var created time.Time
	err = m.d.DB.QueryRowContext(ctx, "SELECT sha256,created_at FROM drive_file_versions WHERE id=? AND item_id=?", versionID, itemID).Scan(&hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	stream, _, err := files.OpenSeeker(ctx, m.store, blobKey(hash))
	if errors.Is(err, files.ErrNotFound) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", item.Mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("ETag", etag(hash))
	http.ServeContent(w, r, item.Name, created, stream)
}

func (m *Module) RestoreDriveVersion(w http.ResponseWriter, r *http.Request, itemID api.ItemId, versionID api.VersionId) {
	ctx := r.Context()
	if _, err := m.versionItem(ctx, itemID); fail(w, r, err) {
		return
	}
	err := m.write(ctx, func(tx *sql.Tx) error {
		item, err := db.New(tx).GetItem(ctx, itemID)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if item.TrashedAt != nil || item.IsDir != 0 {
			return httpx.ErrNotFound
		}
		var hash string
		var size int64
		err = tx.QueryRowContext(ctx, "SELECT sha256,size FROM drive_file_versions WHERE id=? AND item_id=?", versionID, itemID).Scan(&hash, &size)
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := m.recordVersion(ctx, tx, item, hash); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE drive_items SET sha256=?,size=?,updated_at=?,s3_synced_at=NULL,s3_error=NULL WHERE id=?", hash, size, time.Now().UTC(), itemID)
		return err
	})
	if err != nil {
		m.audit(ctx, "drive.version.restore", itemID, err)
	}
	if fail(w, r, err) {
		return
	}
	item, err := m.row(ctx, itemID)
	if fail(w, r, err) {
		return
	}
	m.event("drive_item.updated", item)
	m.audit(ctx, "drive.version.restore", itemID, nil)
	m.triggerSync()
	w.Header().Set("ETag", etag(item.Sha256))
	httpx.JSON(w, http.StatusOK, m.dto(ctx, item))
}

const versionSettingsKey = "drive.versions"

var defaultVersionSettings = api.DriveVersionSettings{KeepCount: 50, KeepDays: 30}

func (m *Module) versionSettings(ctx context.Context) (api.DriveVersionSettings, error) {
	value := defaultVersionSettings
	err := m.d.Settings.Get(ctx, versionSettingsKey, &value)
	if errors.Is(err, settings.ErrNotSet) {
		return defaultVersionSettings, nil
	}
	return value, err
}

func (m *Module) GetDriveVersionSettings(w http.ResponseWriter, r *http.Request) {
	value, err := m.versionSettings(r.Context())
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}

func (m *Module) PutDriveVersionSettings(w http.ResponseWriter, r *http.Request) {
	var value api.DriveVersionSettings
	if fail(w, r, httpx.Decode(r, &value)) {
		return
	}
	if value.KeepCount < 1 || value.KeepCount > 500 || value.KeepDays < 1 || value.KeepDays > 3650 {
		httpx.Fail(w, r, httpx.Invalid("历史版本保留数量或天数不正确"))
		return
	}
	if fail(w, r, m.d.Settings.Set(r.Context(), versionSettingsKey, value)) {
		return
	}
	httpx.JSON(w, http.StatusOK, value)
	base := m.taskBase
	if base == nil {
		base = context.Background()
	}
	go func() { _ = m.pruneVersions(base) }()
}

func (m *Module) pruneVersions(ctx context.Context) error {
	config, err := m.versionSettings(ctx)
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -config.KeepDays)
	var hashes []string
	err = m.write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id,sha256 FROM (SELECT id,sha256,created_at,ROW_NUMBER() OVER (PARTITION BY item_id ORDER BY created_at DESC,id DESC) AS rank FROM drive_file_versions) WHERE rank>? OR created_at<?`, config.KeepCount, cutoff)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			var hash string
			if err = rows.Scan(&id, &hash); err != nil {
				break
			}
			ids = append(ids, id)
			hashes = append(hashes, hash)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, "DELETE FROM drive_file_versions WHERE id=?", id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, hash := range hashes {
		m.dropBlob(ctx, hash)
	}
	return nil
}
