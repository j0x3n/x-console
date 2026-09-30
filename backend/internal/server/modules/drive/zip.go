package drive

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

// DownloadDriveZip writes a ZIP directly to the response. Only top-level ids
// are validated before the response starts; a later storage error aborts it.
func (m *Module) DownloadDriveZip(w http.ResponseWriter, r *http.Request, params api.DownloadDriveZipParams) {
	ctx := r.Context()
	if len(params.Ids) == 0 {
		httpx.Fail(w, r, httpx.Invalid("请选择要下载的文件"))
		return
	}
	items := make([]db.DriveItem, 0, len(params.Ids))
	seen := make(map[int64]bool, len(params.Ids))
	for _, id := range params.Ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		item, err := m.visibleRow(ctx, id)
		if fail(w, r, err) {
			return
		}
		if item.TrashedAt != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		if !validName(item.Name) {
			httpx.Fail(w, r, httpx.Invalid("文件名不正确"))
			return
		}
		items = append(items, item)
	}
	name := "下载.zip"
	if len(items) == 1 {
		name = items[0].Name + ".zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	zw := zip.NewWriter(w)
	var streamErr error
	defer func() {
		m.d.Audit.Record(context.WithoutCancel(ctx), "drive.zip", "", map[string]any{"items": len(items)}, streamErr)
	}()
	for _, item := range items {
		if streamErr = m.writeZipItem(ctx, zw, item, item.Name, 0); streamErr != nil {
			slog.Error("drive zip failed", "error", streamErr)
			panic(http.ErrAbortHandler)
		}
	}
	if streamErr = zw.Close(); streamErr != nil {
		slog.Error("drive zip close failed", "error", streamErr)
		panic(http.ErrAbortHandler)
	}
}

func (m *Module) writeZipItem(ctx context.Context, zw *zip.Writer, item db.DriveItem, name string, depth int) error {
	if depth > 64 || !validName(item.Name) {
		return httpx.Invalid("文件夹层级或名称不正确")
	}
	if item.IsDir != 0 {
		header := &zip.FileHeader{Name: name + "/", Method: zip.Store, Modified: item.UpdatedAt}
		header.SetMode(os.ModeDir | 0o755)
		if _, err := zw.CreateHeader(header); err != nil {
			return err
		}
		ids, err := m.zipChildren(ctx, item.ID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			child, err := m.row(ctx, id)
			if err != nil {
				return err
			}
			if child.TrashedAt != nil || (child.Hidden != 0 && !auth.VaultUnlocked(ctx)) {
				continue
			}
			if err := m.writeZipItem(ctx, zw, child, path.Join(name, child.Name), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: item.UpdatedAt}
	header.SetMode(0o644)
	out, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	if item.Sha256 == "" {
		return httpx.NewError(http.StatusBadGateway, "blob_missing", "文件内容不存在")
	}
	in, _, err := m.store.Get(ctx, blobKey(item.Sha256))
	if errors.Is(err, files.ErrNotFound) {
		return httpx.NewError(http.StatusBadGateway, "blob_missing", "文件内容不存在")
	}
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := in.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (m *Module) zipChildren(ctx context.Context, parentID int64) ([]int64, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT id FROM drive_items WHERE parent_id=? AND trashed_at IS NULL ORDER BY is_dir DESC, name, id`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
