package drive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

// driveFiles is the drive's face for other modules (B146 music). It never
// shows hidden or trashed items, so the vault lock state does not matter.
type driveFiles struct{ m *Module }

var _ contracts.DriveFiles = driveFiles{}

func (f driveFiles) active(ctx context.Context, id int64) (db.DriveItem, error) {
	item, err := f.m.row(ctx, id)
	if errors.Is(err, httpx.ErrNotFound) {
		return item, contracts.ErrDriveNotFound
	}
	if err != nil {
		return item, err
	}
	if item.Hidden != 0 || item.TrashedAt != nil {
		return item, contracts.ErrDriveNotFound
	}
	return item, nil
}

func (f driveFiles) toFile(ctx context.Context, item db.DriveItem) contracts.DriveFile {
	out := contracts.DriveFile{ID: item.ID, Name: item.Name, Size: item.Size, Mime: item.Mime, SHA256: item.Sha256, UpdatedAt: item.UpdatedAt}
	if item.ParentID != nil {
		out.ParentID = *item.ParentID
		if p, err := f.m.row(ctx, *item.ParentID); err == nil {
			out.ParentName = p.Name
		}
	}
	return out
}

func (f driveFiles) File(ctx context.Context, id int64) (contracts.DriveFile, error) {
	item, err := f.active(ctx, id)
	if err != nil {
		return contracts.DriveFile{}, err
	}
	if item.IsDir != 0 {
		return contracts.DriveFile{}, contracts.ErrDriveNotFound
	}
	return f.toFile(ctx, item), nil
}

func (f driveFiles) Folder(ctx context.Context, id int64) (contracts.DriveFolder, error) {
	item, err := f.active(ctx, id)
	if err != nil {
		return contracts.DriveFolder{}, err
	}
	if item.IsDir == 0 {
		return contracts.DriveFolder{}, contracts.ErrDriveNotFound
	}
	return contracts.DriveFolder{ID: item.ID, Name: item.Name, Path: f.m.pathString(ctx, item)}, nil
}

func (f driveFiles) Open(ctx context.Context, id int64) (io.ReadSeekCloser, contracts.DriveFile, error) {
	file, err := f.File(ctx, id)
	if err != nil {
		return nil, file, err
	}
	rc, _, err := files.OpenSeeker(ctx, f.m.store, blobKey(file.SHA256))
	if errors.Is(err, files.ErrNotFound) {
		err = contracts.ErrDriveNotFound
	}
	return rc, file, err
}

func (f driveFiles) ListFiles(ctx context.Context, folderIDs []int64, exts []string) ([]contracts.DriveFile, error) {
	if len(folderIDs) == 0 {
		return nil, nil
	}
	want := map[string]bool{}
	for _, e := range exts {
		want[strings.ToLower(e)] = true
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(folderIDs)), ",")
	args := make([]any, len(folderIDs))
	for i, id := range folderIDs {
		args[i] = id
	}
	rows, err := f.m.d.DB.QueryContext(ctx, `WITH RECURSIVE tree(id) AS (
  SELECT id FROM drive_items WHERE id IN (`+marks+`) AND is_dir=1 AND hidden=0 AND trashed_at IS NULL
  UNION
  SELECT c.id FROM drive_items c JOIN tree t ON c.parent_id=t.id WHERE c.is_dir=1 AND c.hidden=0 AND c.trashed_at IS NULL
)
SELECT f.id, COALESCE(f.parent_id,0), COALESCE(p.name,''), f.name, f.size, f.mime, f.sha256, f.updated_at
FROM drive_items f JOIN tree t ON f.parent_id=t.id LEFT JOIN drive_items p ON p.id=f.parent_id
WHERE f.is_dir=0 AND f.hidden=0 AND f.trashed_at IS NULL ORDER BY f.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.DriveFile
	for rows.Next() {
		var file contracts.DriveFile
		var updated time.Time
		if err := rows.Scan(&file.ID, &file.ParentID, &file.ParentName, &file.Name, &file.Size, &file.Mime, &file.SHA256, &updated); err != nil {
			return nil, err
		}
		file.UpdatedAt = updated
		if len(want) > 0 && !want[strings.ToLower(filepath.Ext(file.Name))] {
			continue
		}
		out = append(out, file)
	}
	return out, rows.Err()
}

// pathString is the "/a/b/name" path of an item.
func (m *Module) pathString(ctx context.Context, item db.DriveItem) string {
	ancestors, err := m.path(ctx, item)
	if err != nil {
		return "/" + item.Name
	}
	names := make([]string, 0, len(ancestors)+1)
	for _, p := range ancestors {
		names = append(names, p.Name)
	}
	return "/" + strings.Join(append(names, item.Name), "/")
}

// ReplaceContent implements contracts.DriveFiles.
func (f driveFiles) ReplaceContent(ctx context.Context, id int64, path, expectSHA256 string) (contracts.DriveFile, error) {
	m := f.m
	ctx = auth.WithoutVault(ctx)
	src, err := os.Open(path)
	if err != nil {
		return contracts.DriveFile{}, err
	}
	defer src.Close()
	h := sha256.New()
	size, err := io.Copy(h, src)
	if err != nil {
		return contracts.DriveFile{}, err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	release, err := m.putBlobFile(ctx, hash, path, size)
	if err != nil {
		return contracts.DriveFile{}, err
	}
	var oldHash string
	err = m.write(ctx, func(tx *sql.Tx) error {
		current, err := db.New(tx).GetItem(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return contracts.ErrDriveNotFound
		}
		if err != nil {
			return err
		}
		if current.IsDir != 0 || current.Hidden != 0 || current.TrashedAt != nil {
			return contracts.ErrDriveNotFound
		}
		if expectSHA256 != "" && current.Sha256 != expectSHA256 {
			return contracts.ErrDriveChanged
		}
		oldHash = current.Sha256
		_, err = tx.ExecContext(ctx, "UPDATE drive_items SET size=?,sha256=?,updated_at=?,s3_synced_at=NULL WHERE id=?", size, hash, time.Now().UTC(), id)
		return err
	})
	if err != nil {
		m.dropBlobLocked(context.WithoutCancel(ctx), hash)
	}
	release()
	if err != nil {
		return contracts.DriveFile{}, err
	}
	if oldHash != hash {
		m.dropBlob(ctx, oldHash)
	}
	item, err := m.row(ctx, id)
	if err != nil {
		return contracts.DriveFile{}, err
	}
	m.event("drive_item.updated", item)
	m.audit(ctx, "drive.replace_content", id, nil)
	m.triggerSync()
	return f.toFile(ctx, item), nil
}
