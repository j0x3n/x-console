package drive

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

type shareRate struct {
	count int
	until time.Time
}
type shareFailure struct {
	count       int
	lockedUntil time.Time
	updated     time.Time
}

var (
	errPublicShare       = httpx.NewError(404, "share_not_found", "分享链接不存在")
	errShareCodeRequired = httpx.NewError(401, "share_code_required", "请输入密码")
	errShareLimit        = httpx.NewError(410, "share_limit_reached", "下载次数已用完")
)

func (m *Module) PublicPaths() []string { return []string{"/public/shares"} }

func publicClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func publicHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "no-store")
}

func (m *Module) shareRateLimit(w http.ResponseWriter, r *http.Request) bool {
	ip := publicClientIP(r)
	now := time.Now().UTC()
	m.shareMu.Lock()
	entry := m.shareHits[ip]
	if !entry.until.After(now) {
		entry = shareRate{until: now.Add(time.Minute)}
	}
	entry.count++
	m.shareHits[ip] = entry
	m.shareMu.Unlock()
	if entry.count <= 60 {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(entry.until).Seconds())+1)))
	httpx.Fail(w, r, httpx.ErrTooManyRequests)
	return true
}

func (m *Module) pruneShareRates(context.Context) error {
	now := time.Now().UTC()
	m.shareMu.Lock()
	defer m.shareMu.Unlock()
	for ip, entry := range m.shareHits {
		if !entry.until.After(now) {
			delete(m.shareHits, ip)
		}
	}
	for key, at := range m.shareFetches {
		if now.Sub(at) >= shareFetchWindow {
			delete(m.shareFetches, key)
		}
	}
	for key, entry := range m.shareFails {
		if entry.updated.Before(now.Add(-10*time.Minute)) && !entry.lockedUntil.After(now) {
			delete(m.shareFails, key)
		}
	}
	return nil
}

func (m *Module) resolvePublicShare(ctx context.Context, token string) (db.DriveShare, db.DriveItem, error) {
	var share db.DriveShare
	err := m.d.DB.QueryRowContext(ctx, "SELECT id,item_id,token,code_sealed,expires_at,max_downloads,visits,downloads,created_at,last_access_at FROM drive_shares WHERE token=?", token).Scan(&share.ID, &share.ItemID, &share.Token, &share.CodeSealed, &share.ExpiresAt, &share.MaxDownloads, &share.Visits, &share.Downloads, &share.CreatedAt, &share.LastAccessAt)
	if errors.Is(err, sql.ErrNoRows) {
		return share, db.DriveItem{}, errPublicShare
	}
	if err != nil {
		return share, db.DriveItem{}, err
	}
	item, err := m.row(ctx, share.ItemID)
	if err != nil || !m.shareable(ctx, item) || (share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now().UTC())) {
		return share, db.DriveItem{}, errPublicShare
	}
	return share, item, nil
}

func (m *Module) shareMAC(id int64, expires int64, token string) []byte {
	key := hmac.New(sha256.New, m.d.Config.MasterKey)
	key.Write([]byte("x-console/share-access/v1"))
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(strconv.FormatInt(id, 10)))
	mac.Write([]byte("."))
	mac.Write([]byte(strconv.FormatInt(expires, 10)))
	mac.Write([]byte("."))
	mac.Write([]byte(token))
	return mac.Sum(nil)
}

func (m *Module) shareAccessToken(share db.DriveShare) (string, time.Time) {
	expiry := time.Now().UTC().Add(time.Hour)
	seconds := expiry.Unix()
	signed := strconv.FormatInt(share.ID, 10) + "." + strconv.FormatInt(seconds, 10)
	return signed + "." + base64.RawURLEncoding.EncodeToString(m.shareMAC(share.ID, seconds, share.Token)), expiry
}

func (m *Module) validShareAccess(share db.DriveShare, access *string) bool {
	if access == nil {
		return false
	}
	parts := strings.Split(*access, ".")
	if len(parts) != 3 {
		return false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id != share.ID {
		return false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || expires <= time.Now().UTC().Unix() {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, m.shareMAC(id, expires, share.Token)) == 1
}

func (m *Module) publicShare(w http.ResponseWriter, r *http.Request, token string, access *string) (db.DriveShare, db.DriveItem, bool) {
	publicHeaders(w)
	if m.shareRateLimit(w, r) {
		return db.DriveShare{}, db.DriveItem{}, false
	}
	share, item, err := m.resolvePublicShare(r.Context(), token)
	if fail(w, r, err) {
		return share, item, false
	}
	if share.CodeSealed != nil && !m.validShareAccess(share, access) {
		httpx.Fail(w, r, errShareCodeRequired)
		return share, item, false
	}
	return share, item, true
}

func (m *Module) UnlockPublicShare(w http.ResponseWriter, r *http.Request, token api.ShareToken) {
	publicHeaders(w)
	if m.shareRateLimit(w, r) {
		return
	}
	share, _, err := m.resolvePublicShare(r.Context(), token)
	if fail(w, r, err) {
		return
	}
	var body api.UnlockPublicShareJSONBody
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if share.CodeSealed != nil {
		key := fmt.Sprintf("%d:%s", share.ID, publicClientIP(r))
		now := time.Now().UTC()
		m.shareMu.Lock()
		failed := m.shareFails[key]
		if failed.lockedUntil.After(now) {
			m.shareMu.Unlock()
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(failed.lockedUntil).Seconds())+1)))
			httpx.Fail(w, r, httpx.NewError(429, "share_locked", "试错太多次，10 分钟后再试"))
			return
		}
		m.shareMu.Unlock()
		plain, err := m.d.Secrets.Open(*share.CodeSealed)
		if fail(w, r, err) {
			return
		}
		if subtle.ConstantTimeCompare([]byte(plain), []byte(body.Code)) != 1 {
			m.shareMu.Lock()
			failed = m.shareFails[key]
			if failed.updated.Before(now.Add(-10 * time.Minute)) {
				failed.count = 0
			}
			failed.count++
			failed.updated = now
			if failed.count >= 5 {
				failed.lockedUntil = now.Add(10 * time.Minute)
			}
			m.shareFails[key] = failed
			m.shareMu.Unlock()
			if failed.count >= 5 {
				w.Header().Set("Retry-After", "600")
				httpx.Fail(w, r, httpx.NewError(429, "share_locked", "试错太多次，10 分钟后再试"))
			} else {
				httpx.Fail(w, r, httpx.NewError(403, "share_code_wrong", fmt.Sprintf("密码不对，还能试 %d 次", 5-failed.count)))
			}
			return
		}
		m.shareMu.Lock()
		delete(m.shareFails, key)
		m.shareMu.Unlock()
	}
	access, expires := m.shareAccessToken(share)
	httpx.JSON(w, http.StatusOK, map[string]any{"access": access, "expiresAt": expires})
}

func (m *Module) GetPublicShare(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.GetPublicShareParams) {
	share, item, ok := m.publicShare(w, r, token, params.T)
	if !ok {
		return
	}
	info := api.PublicShare{Name: item.Name, IsDir: item.IsDir != 0, Size: item.Size, UpdatedAt: &item.UpdatedAt, ExpiresAt: share.ExpiresAt}
	if item.IsDir != 0 {
		err := m.d.DB.QueryRowContext(r.Context(), `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? AND trashed_at IS NULL AND hidden=0 UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id WHERE d.trashed_at IS NULL AND d.hidden=0) SELECT COALESCE(sum(size),0) FROM drive_items WHERE id IN (SELECT id FROM subtree) AND is_dir=0`, item.ID).Scan(&info.Size)
		if fail(w, r, err) {
			return
		}
	} else {
		info.Mime = &item.Mime
	}
	if share.MaxDownloads != nil {
		remaining := int(*share.MaxDownloads - share.Downloads)
		if remaining < 0 {
			remaining = 0
		}
		info.DownloadsLeft = &remaining
	}
	_, err := m.d.DB.ExecContext(r.Context(), "UPDATE drive_shares SET visits=visits+1,last_access_at=? WHERE id=?", time.Now().UTC(), share.ID)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, info)
}

func (m *Module) scopedShareItem(ctx context.Context, root db.DriveItem, id int64) (db.DriveItem, []db.DriveItem, error) {
	if id == 0 {
		id = root.ID
	}
	var reversed []db.DriveItem
	for depth := 0; depth < 64; depth++ {
		item, err := m.row(ctx, id)
		if err != nil || item.Hidden != 0 || item.TrashedAt != nil {
			return db.DriveItem{}, nil, errPublicShare
		}
		if item.ID == root.ID {
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			if len(reversed) == 0 {
				return item, reversed, nil
			}
			last := reversed[len(reversed)-1]
			return last, reversed, nil
		}
		if root.IsDir == 0 || item.ParentID == nil {
			return db.DriveItem{}, nil, errPublicShare
		}
		reversed = append(reversed, item)
		id = *item.ParentID
	}
	return db.DriveItem{}, nil, errPublicShare
}

func (m *Module) ListPublicShareItems(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.ListPublicShareItemsParams) {
	_, root, ok := m.publicShare(w, r, token, params.T)
	if !ok {
		return
	}
	if root.IsDir == 0 {
		httpx.Fail(w, r, errPublicShare)
		return
	}
	id := root.ID
	if params.Folder != nil {
		id = *params.Folder
	}
	folder, pathItems, err := m.scopedShareItem(r.Context(), root, id)
	if fail(w, r, err) {
		return
	}
	if folder.IsDir == 0 {
		httpx.Fail(w, r, errPublicShare)
		return
	}
	rows, err := m.d.DB.QueryContext(r.Context(), "SELECT "+itemColumns+" FROM drive_items WHERE parent_id=? AND hidden=0 AND trashed_at IS NULL ORDER BY is_dir DESC,name,id", folder.ID)
	if fail(w, r, err) {
		return
	}
	defer rows.Close()
	items := []api.PublicShareItem{}
	for rows.Next() {
		var item db.DriveItem
		if item, err = scanItem(rows); err != nil {
			break
		}
		entry := api.PublicShareItem{Id: item.ID, Name: item.Name, IsDir: item.IsDir != 0, Size: item.Size, UpdatedAt: item.UpdatedAt}
		if item.IsDir == 0 {
			entry.Mime = &item.Mime
			entry.Thumbnail = shareThumbnailType(item.Mime)
		}
		items = append(items, entry)
	}
	if err == nil {
		err = rows.Err()
	}
	if fail(w, r, err) {
		return
	}
	path := []map[string]any{}
	for _, item := range pathItems {
		path = append(path, map[string]any{"id": item.ID, "name": item.Name})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "path": path})
}

const shareFetchWindow = time.Hour

func startsShareDownload(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "bytes=") {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(value, "bytes="), ",") {
		start, _, ok := strings.Cut(strings.TrimSpace(part), "-")
		if ok && start != "" {
			n, err := strconv.ParseInt(strings.TrimSpace(start), 10, 64)
			if err == nil && n == 0 {
				return true
			}
		}
	}
	return false
}
func (m *Module) countedFetch(key string) bool {
	m.shareMu.Lock()
	defer m.shareMu.Unlock()
	at, ok := m.shareFetches[key]
	return ok && time.Since(at) < shareFetchWindow
}

func (m *Module) auditShareDownload(ctx context.Context, shareID, itemID int64, ip string, err error) {
	ctx = audit.WithActor(ctx, "share:"+strconv.FormatInt(shareID, 10))
	m.d.Audit.Record(ctx, "drive.share.download", strconv.FormatInt(itemID, 10), map[string]any{"ip": ip, "fileId": itemID}, err)
}

func (m *Module) GetPublicShareContent(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.GetPublicShareContentParams) {
	share, root, ok := m.publicShare(w, r, token, params.T)
	if !ok {
		return
	}
	id := root.ID
	if root.IsDir != 0 {
		if params.Item == nil {
			httpx.Fail(w, r, errPublicShare)
			return
		}
		id = *params.Item
	} else if params.Item != nil && *params.Item != id {
		httpx.Fail(w, r, errPublicShare)
		return
	}
	item, _, err := m.scopedShareItem(r.Context(), root, id)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.Sha256 == "" {
		httpx.Fail(w, r, errPublicShare)
		return
	}
	stream, _, err := files.OpenSeeker(r.Context(), m.store, blobKey(item.Sha256))
	if errors.Is(err, files.ErrNotFound) {
		err = errPublicShare
	}
	if fail(w, r, err) {
		return
	}
	defer stream.Close()
	preview := boolValue(params.Preview) || boolValue(params.Inline)
	ip := publicClientIP(r)
	fetchKey := fmt.Sprintf("%d:%d:%s:%s", share.ID, item.ID, ip, r.UserAgent())
	if share.MaxDownloads != nil && share.Downloads >= *share.MaxDownloads && (preview || !m.countedFetch(fetchKey)) {
		httpx.Fail(w, r, errShareLimit)
		return
	}
	disposition := "attachment"
	if preview {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": item.Name}))
	w.Header().Set("Content-Type", item.Mime)
	w.Header().Set("ETag", etag(item.Sha256))
	var writer http.ResponseWriter = w
	if !preview && r.Method == http.MethodGet {
		writer = &shareDownloadWriter{ResponseWriter: w, request: r, count: func(status int) error {
			if status == http.StatusPartialContent && !startsShareDownload(r.Header.Get("Range")) {
				return nil
			}
			err := m.recordShareDownload(r.Context(), share, item, r, fetchKey)
			if err == nil {
				m.auditShareDownload(r.Context(), share.ID, item.ID, ip, nil)
			}
			return err
		}}
	}
	http.ServeContent(writer, r, item.Name, item.UpdatedAt, stream)
}

func (m *Module) DownloadPublicShareZip(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.DownloadPublicShareZipParams) {
	share, root, ok := m.publicShare(w, r, token, params.T)
	if !ok {
		return
	}
	if share.MaxDownloads != nil && share.Downloads >= *share.MaxDownloads {
		httpx.Fail(w, r, errShareLimit)
		return
	}
	if root.IsDir == 0 {
		httpx.Fail(w, r, errPublicShare)
		return
	}
	if r.Method == http.MethodGet {
		if err := m.recordShareDownload(r.Context(), share, root, r, ""); fail(w, r, err) {
			return
		}
		m.auditShareDownload(r.Context(), share.ID, root.ID, publicClientIP(r), nil)
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": root.Name + ".zip"}))
	w.WriteHeader(http.StatusOK)
	zw := zip.NewWriter(w)
	if err := m.writeZipItem(auth.WithoutVault(r.Context()), zw, root, root.Name, 0); err != nil {
		slog.Error("public share zip failed", "share", share.ID, "error", err)
		panic(http.ErrAbortHandler)
	}
	if err := zw.Close(); err != nil {
		slog.Error("public share zip close failed", "share", share.ID, "error", err)
		panic(http.ErrAbortHandler)
	}
}
