package drive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
)

// maxTextSave 是在线编辑能保存的最大文本。
const maxTextSave = 10 << 20

var errVersionConflict = httpx.NewError(http.StatusConflict, "version_conflict", "文件在别处改过了")

// etag 把内容的 sha256 写成 HTTP ETag。
func etag(hash string) string { return `"` + hash + `"` }

// etagMatches 判断 If-Match 里有没有当前版本。
func etagMatches(header, hash string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if part == "*" || strings.Trim(part, `"`) == hash {
			return true
		}
	}
	return false
}

// storeBlob 按 sha256 存内容，已有同样内容时不重复写。
func (m *Module) storeBlob(ctx context.Context, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if _, err := m.store.Stat(ctx, blobKey(hash)); err == nil {
		return hash, nil
	} else if !errors.Is(err, files.ErrNotFound) {
		return "", err
	}
	return hash, m.store.Put(ctx, blobKey(hash), bytes.NewReader(data), int64(len(data)))
}

// dropBlob 在没有条目再用这份内容时删掉它。
func (m *Module) dropBlob(ctx context.Context, hash string) {
	if hash == "" {
		return
	}
	count, err := m.q.CountBlobReferences(ctx, hash)
	if err == nil && count == 0 {
		_ = m.store.Delete(ctx, blobKey(hash))
	}
}

func (m *Module) SaveDriveItemContent(w http.ResponseWriter, r *http.Request, id api.ItemId, p api.SaveDriveItemContentParams) {
	ctx := r.Context()
	item, err := m.visibleRow(ctx, id)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.TrashedAt != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTextSave+1))
	if err != nil || len(data) > maxTextSave {
		httpx.Fail(w, r, httpx.Invalid("内容超过 10 MB，不能在线保存"))
		return
	}
	if !utf8.Valid(data) {
		httpx.Fail(w, r, httpx.Invalid("内容不是 UTF-8"))
		return
	}
	if p.IfMatch != nil && !etagMatches(*p.IfMatch, item.Sha256) {
		httpx.Fail(w, r, errVersionConflict)
		return
	}
	hash, err := m.storeBlob(ctx, data)
	if fail(w, r, err) {
		return
	}
	err = m.write(ctx, func(tx *sql.Tx) error {
		// 读和写之间别处可能保存过，这里再比一次。
		query := "UPDATE drive_items SET size=?,sha256=?,updated_at=?,s3_synced_at=NULL,s3_error=NULL WHERE id=? AND trashed_at IS NULL"
		args := []any{len(data), hash, time.Now().UTC(), id}
		if p.IfMatch != nil {
			query += " AND sha256=?"
			args = append(args, item.Sha256)
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errVersionConflict
		}
		return nil
	})
	if err != nil {
		m.dropBlob(ctx, hash)
		m.audit(ctx, "drive.save_text", id, err)
		httpx.Fail(w, r, err)
		return
	}
	if item.Sha256 != hash {
		m.dropBlob(ctx, item.Sha256)
	}
	item, err = m.row(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.event("drive_item.updated", item)
	m.audit(ctx, "drive.save_text", id, nil)
	m.triggerSync()
	w.Header().Set("ETag", etag(item.Sha256))
	httpx.JSON(w, http.StatusOK, m.dto(ctx, item))
}
