package backup

import (
	"context"
	"io"
	"mime"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
)

var (
	errBadID   = httpx.Invalid("备份名称不正确")
	errMissing = httpx.NewError(http.StatusNotFound, "not_found", "找不到这份备份")
)

// ListBackups is GET /backups.
func (m *Module) ListBackups(w http.ResponseWriter, r *http.Request) {
	all, err := m.entries(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.Backup, 0, len(all))
	for _, e := range all {
		items = append(items, e.toAPI())
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// DownloadBackup is GET /backups/{backupId}/download. Packages in S3 are read
// through the server, so the bucket address is never shown.
func (m *Module) DownloadBackup(w http.ResponseWriter, r *http.Request, id api.BackupId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := m.find(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, info, err := files.OpenSeeker(ctx, e.store, e.key)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer f.Close()
	m.d.Audit.Record(ctx, "backup.download", id, nil, nil)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": id}))
	http.ServeContent(w, r, id, info.ModTime, f)
}

// DeleteBackup is DELETE /backups/{backupId}.
func (m *Module) DeleteBackup(w http.ResponseWriter, r *http.Request, id api.BackupId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := m.find(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = e.remove(ctx)
	m.d.Audit.Record(ctx, "backup.delete", id, map[string]any{"location": e.location}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("backup.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

// UploadBackup is POST /backups/upload. The file goes straight to disk and is
// checked after it arrived.
func (m *Module) UploadBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("请用表单上传文件"))
		return
	}
	name := "uploaded-" + m.now().In(m.d.Config.Location).Format("20060102-150405") + ".tar.gz"
	stored := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			httpx.Fail(w, r, httpx.Invalid("上传的内容不完整"))
			return
		}
		if part.FormName() != "file" {
			continue
		}
		if err := m.local.Put(ctx, name, part, -1); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		stored = true
		break
	}
	if !stored {
		httpx.Fail(w, r, httpx.Invalid("没有收到文件"))
		return
	}
	man, err := m.checkStored(ctx, name)
	if err != nil {
		_ = m.local.Delete(ctx, name)
		m.d.Audit.Record(ctx, "backup.upload", name, nil, err)
		httpx.Fail(w, r, httpx.Invalid(err.Error()))
		return
	}
	if err := writeSidecar(ctx, m.local, name, sidecar{manifest: man, Kind: kindUploaded}); err != nil {
		_ = m.local.Delete(ctx, name)
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "backup.upload", name, nil, nil)
	e, err := m.find(ctx, name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("backup.created", map[string]any{"id": name})
	httpx.JSON(w, http.StatusCreated, e.toAPI())
}

// checkStored opens a local package and reads its manifest.
func (m *Module) checkStored(ctx context.Context, name string) (manifest, error) {
	rc, _, err := m.local.Get(ctx, name)
	if err != nil {
		return manifest{}, err
	}
	defer rc.Close()
	ar, err := openArchive(rc)
	if err != nil {
		return manifest{}, err
	}
	defer ar.Close()
	return ar.readManifest()
}
