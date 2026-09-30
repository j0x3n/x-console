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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
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

type blobLock struct {
	mu    sync.Mutex
	users int
}

// lockBlob serializes work on one content hash. Content is shared by
// hash, so an upload that finds it already stored must insert its row before
// a delete counts the references, or the delete removes content the new row
// needs. Writers hold the lock from storing the content until their row is
// committed; deleters hold it while they count and delete. Take it before
// m.mu (m.write), never inside.
func (m *Module) lockBlob(hash string) func() {
	m.blobMu.Lock()
	l := m.blobLocks[hash]
	if l == nil {
		l = &blobLock{}
		m.blobLocks[hash] = l
	}
	l.users++
	m.blobMu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		m.blobMu.Lock()
		l.users--
		if l.users == 0 {
			delete(m.blobLocks, hash)
		}
		m.blobMu.Unlock()
	}
}

// afterBlobPut runs between storing content and inserting its row. Tests
// use it to hold an upload there.
var afterBlobPut = func() {}

// storeBlob 按 sha256 存内容，已有同样内容时不重复写。成功时持有这份内容的锁，
// 用它的条目提交以后调用 release。
func (m *Module) storeBlob(ctx context.Context, data []byte) (hash string, release func(), err error) {
	sum := sha256.Sum256(data)
	hash = hex.EncodeToString(sum[:])
	release = m.lockBlob(hash)
	if _, err = m.store.Stat(ctx, blobKey(hash)); err == nil {
		afterBlobPut()
		return hash, release, nil
	} else if !errors.Is(err, files.ErrNotFound) {
		release()
		return "", nil, err
	}
	if err = m.store.Put(ctx, blobKey(hash), bytes.NewReader(data), int64(len(data))); err != nil {
		release()
		return "", nil, err
	}
	afterBlobPut()
	return hash, release, nil
}

// dropBlob 在没有条目再用这份内容时删掉它。
func (m *Module) dropBlob(ctx context.Context, hash string) {
	if hash == "" {
		return
	}
	release := m.lockBlob(hash)
	defer release()
	m.dropBlobLocked(ctx, hash)
}

// dropBlobLocked is dropBlob for a caller that holds the hash's lock.
func (m *Module) dropBlobLocked(ctx context.Context, hash string) {
	count, err := m.q.CountBlobReferences(ctx, hash)
	if err == nil && count == 0 {
		_ = m.store.Delete(ctx, blobKey(hash))
		_ = m.store.Delete(ctx, thumbnailKey(hash))
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
	hash, release, err := m.storeBlob(ctx, data)
	if fail(w, r, err) {
		return
	}
	err = m.write(ctx, func(tx *sql.Tx) error {
		// 读和写之间别处可能保存过，这里再比一次。
		current, err := db.New(tx).GetItem(ctx, id)
		if err != nil {
			return err
		}
		if current.TrashedAt != nil {
			return httpx.ErrNotFound
		}
		if p.IfMatch != nil && current.Sha256 != item.Sha256 {
			return errVersionConflict
		}
		if err := m.recordVersion(ctx, tx, current, hash); err != nil {
			return err
		}
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
		m.dropBlobLocked(ctx, hash)
	}
	release()
	if err != nil {
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
