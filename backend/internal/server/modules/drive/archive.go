package drive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

type archiveEntry struct {
	item db.DriveItem
	name string
}

func (m *Module) ArchiveDriveItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.ArchiveRequest
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if len(body.Ids) == 0 || !body.Format.Valid() {
		httpx.Fail(w, r, httpx.Invalid("压缩参数不正确"))
		return
	}
	ext := ".zip"
	mime := "application/zip"
	if body.Format == api.TarGz {
		ext = ".tar.gz"
		mime = "application/gzip"
	}
	name := body.Name
	if !strings.HasSuffix(strings.ToLower(name), ext) {
		name += ext
	}
	if !validName(name) {
		httpx.Fail(w, r, httpx.Invalid("压缩包名称不正确"))
		return
	}
	selected := make(map[int64]bool, len(body.Ids))
	items := make([]db.DriveItem, 0, len(body.Ids))
	var hidden *bool
	for _, id := range body.Ids {
		item, err := m.visibleRow(ctx, id)
		if fail(w, r, err) {
			return
		}
		if item.TrashedAt != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		isHidden := item.Hidden != 0
		if hidden != nil && *hidden != isHidden {
			httpx.Fail(w, r, httpx.NewError(400, "hidden_mismatch", "不能混合压缩隐藏和普通文件"))
			return
		}
		hidden = &isHidden
		if !selected[id] {
			selected[id] = true
			items = append(items, item)
		}
	}
	parent, err := m.archiveParent(ctx, body.ParentId, *hidden)
	if fail(w, r, err) {
		return
	}
	// A selected child is already included by its selected ancestor.
	entries := make([]archiveEntry, 0, len(items))
	for _, item := range items {
		ancestor := item.ParentID
		covered := false
		for ancestor != nil {
			if selected[*ancestor] {
				covered = true
				break
			}
			row, err := m.row(ctx, *ancestor)
			if fail(w, r, err) {
				return
			}
			ancestor = row.ParentID
		}
		if covered {
			continue
		}
		if err := m.collectArchive(ctx, item, item.Name, 0, &entries); fail(w, r, err) {
			return
		}
	}
	var totalBytes int64
	for _, entry := range entries {
		if entry.item.IsDir == 0 {
			totalBytes += entry.item.Size
		}
	}
	targetID := int64(0)
	if parent != nil {
		targetID = *parent
	}
	title := fmt.Sprintf("压缩 %d 项为 %s", len(entries), name)
	start := m.startTask
	if *hidden {
		start, title = m.startHiddenTask, fmt.Sprintf("压缩隐藏空间里的 %d 项", len(entries))
	}
	task := start(api.Archive, title, len(entries), totalBytes, func(jobCtx context.Context, t *driveTask) error {
		resultID, err := m.createArchive(jobCtx, t, entries, name, mime, body.Format, parent, *hidden)
		m.d.Audit.Record(context.WithoutCancel(jobCtx), "drive.archive", strconv.FormatInt(resultID, 10), map[string]any{"items": len(entries), "format": body.Format, "targetId": targetID}, err)
		return err
	}, targetID)
	httpx.JSON(w, http.StatusAccepted, task)
}

func (m *Module) archiveParent(ctx context.Context, id *int64, hidden bool) (*int64, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	if *id < 0 {
		return nil, httpx.NewError(400, "invalid_target", "目标文件夹不可用")
	}
	item, err := m.visibleRow(ctx, *id)
	if err != nil {
		return nil, err
	}
	if item.IsDir == 0 || item.TrashedAt != nil {
		return nil, httpx.NewError(400, "invalid_target", "目标文件夹不可用")
	}
	if (item.Hidden != 0) != hidden {
		return nil, httpx.NewError(400, "hidden_mismatch", "隐藏状态和目标文件夹不一致")
	}
	return &item.ID, nil
}

func (m *Module) collectArchive(ctx context.Context, item db.DriveItem, name string, depth int, out *[]archiveEntry) error {
	if depth > 64 || !validName(item.Name) {
		return httpx.Invalid("文件夹层级或名称不正确")
	}
	*out = append(*out, archiveEntry{item: item, name: name})
	if item.IsDir == 0 {
		return nil
	}
	children, err := m.zipChildren(ctx, item.ID)
	if err != nil {
		return err
	}
	for _, id := range children {
		child, err := m.row(ctx, id)
		if err != nil {
			return err
		}
		if child.TrashedAt != nil || (child.Hidden != 0 && !auth.VaultUnlocked(ctx)) {
			continue
		}
		if err := m.collectArchive(ctx, child, path.Join(name, child.Name), depth+1, out); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) createArchive(ctx context.Context, t *driveTask, entries []archiveEntry, name, mime string, format api.ArchiveRequestFormat, parent *int64, hidden bool) (int64, error) {
	tmp, err := os.CreateTemp(m.tmpDir, "archive-*")
	if err != nil {
		return 0, err
	}
	defer contracts.TrackTemporaryFile(tmp.Name())()
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	output := io.MultiWriter(tmp, hash)
	var writeErr error
	if format == api.Zip {
		zw := zip.NewWriter(output)
		writeErr = m.writeArchiveZip(ctx, t, zw, entries)
		if closeErr := zw.Close(); writeErr == nil {
			writeErr = closeErr
		}
	} else {
		gz := gzip.NewWriter(output)
		tw := tar.NewWriter(gz)
		writeErr = m.writeArchiveTar(ctx, t, tw, entries)
		if closeErr := tw.Close(); writeErr == nil {
			writeErr = closeErr
		}
		if closeErr := gz.Close(); writeErr == nil {
			writeErr = closeErr
		}
	}
	if writeErr != nil {
		tmp.Close()
		return 0, writeErr
	}
	if err := ctx.Err(); err != nil {
		tmp.Close()
		return 0, err
	}
	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	release, err := m.putBlobFile(ctx, digest, tmp.Name(), info.Size())
	if err != nil {
		return 0, err
	}
	defer release()
	committed := false
	defer func() {
		if !committed {
			m.dropBlobLocked(context.WithoutCancel(ctx), digest)
		}
	}()
	var created db.DriveItem
	err = m.write(ctx, func(tx *sql.Tx) error {
		free, err := m.freeName(ctx, tx, parent, hidden, name, 0)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		created, err = db.New(tx).InsertItem(ctx, db.InsertItemParams{ParentID: parent, Name: free, Size: info.Size(), Mime: mime, Sha256: digest, Hidden: intBool(hidden), CreatedAt: now, UpdatedAt: now})
		return err
	})
	if err != nil {
		return 0, err
	}
	committed = true
	m.changeTask(t, true, func(dto *api.DriveTask) { id := created.ID; dto.ResultId = &id })
	m.triggerSync()
	return created.ID, nil
}

func (m *Module) writeArchiveZip(ctx context.Context, t *driveTask, zw *zip.Writer, entries []archiveEntry) error {
	done := 0
	var doneBytes int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		header := &zip.FileHeader{Name: entry.name, Modified: entry.item.UpdatedAt}
		if entry.item.IsDir != 0 {
			header.Name += "/"
			header.Method = zip.Store
			header.SetMode(os.ModeDir | 0o755)
		} else {
			header.Method = zip.Deflate
			header.SetMode(0o644)
		}
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if entry.item.IsDir == 0 {
			if err := m.copyArchiveFile(ctx, writer, entry.item); err != nil {
				return err
			}
			doneBytes += entry.item.Size
		}
		done++
		t.progress(entry.name, done, doneBytes)
	}
	return nil
}

func (m *Module) writeArchiveTar(ctx context.Context, t *driveTask, tw *tar.Writer, entries []archiveEntry) error {
	done := 0
	var doneBytes int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		header := &tar.Header{Name: entry.name, Mode: 0o644, Size: entry.item.Size, ModTime: entry.item.UpdatedAt, Typeflag: tar.TypeReg}
		if entry.item.IsDir != 0 {
			header.Name += "/"
			header.Mode = 0o755
			header.Size = 0
			header.Typeflag = tar.TypeDir
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if entry.item.IsDir == 0 {
			if err := m.copyArchiveFile(ctx, tw, entry.item); err != nil {
				return err
			}
			doneBytes += entry.item.Size
		}
		done++
		t.progress(entry.name, done, doneBytes)
	}
	return nil
}

func (m *Module) copyArchiveFile(ctx context.Context, dst io.Writer, item db.DriveItem) error {
	if item.Sha256 == "" {
		return httpx.NewError(http.StatusBadGateway, "blob_missing", "文件内容不存在")
	}
	src, _, err := m.store.Get(ctx, blobKey(item.Sha256))
	if errors.Is(err, files.ErrNotFound) {
		return httpx.NewError(http.StatusBadGateway, "blob_missing", "文件内容不存在")
	}
	if err != nil {
		return err
	}
	defer src.Close()
	buffer := make([]byte, 32*1024)
	remaining := item.Size
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := int64(len(buffer))
		if remaining < want {
			want = remaining
		}
		n, err := src.Read(buffer[:want])
		if n > 0 {
			written, writeErr := dst.Write(buffer[:n])
			remaining -= int64(written)
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if err != nil {
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
	return nil
}
