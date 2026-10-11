package drive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

// One-time links (B151). An AI client with a shell gets a short-lived address
// to PUT a file to, or GET a file from, because the MCP request body is too
// small for a song. The token is shown once and stored as a SHA-256 hash; a
// link works once and ends after 10 minutes. Anything wrong with a token
// (unknown, ended, used) is the same 404.

const (
	linkTTL          = 10 * time.Minute
	defaultMaxUpload = int64(1) << 30
	maxUploadKey     = "drive.mcp_max_upload"
)

var errLinkGone = httpx.NewError(http.StatusNotFound, "link_not_found", "链接不存在或已失效")

func newLinkToken() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, linkHash(token), nil
}

func linkHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// maxUpload is the biggest file the MCP uploads accept.
func (m *Module) maxUpload() int64 {
	var v int64
	if err := m.d.Settings.Get(context.Background(), maxUploadKey, &v); err == nil && v > 0 {
		return v
	}
	return defaultMaxUpload
}

// PublicPaths gains the two link routes. They check their own token.
func (m *Module) linkPaths() []string {
	return []string{"/drive/upload-links/", "/drive/download-links/"}
}

// createUploadLink records a link for one file in one folder.
func (m *Module) createUploadLink(ctx context.Context, parent *int64, name, onConflict string, size int64, by string) (token string, expires time.Time, max int64, err error) {
	max = m.maxUpload()
	if size > max {
		return "", expires, max, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("文件太大，上限是 %d MB", max>>20))
	}
	hash := ""
	if token, hash, err = newLinkToken(); err != nil {
		return "", expires, max, err
	}
	now := time.Now().UTC()
	expires = now.Add(linkTTL)
	_, err = m.d.DB.ExecContext(ctx, "INSERT INTO drive_upload_links(token_hash,parent_id,name,on_conflict,max_size,expires_at,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)",
		hash, parent, name, onConflict, max, expires, by, now)
	return token, expires, max, err
}

// PutDriveUploadLink receives the file for an upload link.
func (m *Module) PutDriveUploadLink(w http.ResponseWriter, r *http.Request, token string) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")
	if m.shareRateLimit(w, r) {
		return
	}
	var link struct {
		id               int64
		parent           *int64
		name, onConflict string
		maxSize          int64
	}
	var expires time.Time
	var used *time.Time
	err := m.d.DB.QueryRowContext(ctx, "SELECT id,parent_id,name,on_conflict,max_size,expires_at,used_at FROM drive_upload_links WHERE token_hash=?", linkHash(token)).
		Scan(&link.id, &link.parent, &link.name, &link.onConflict, &link.maxSize, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (used != nil || !expires.After(time.Now().UTC()))) {
		httpx.Fail(w, r, errLinkGone)
		return
	}
	if fail(w, r, err) {
		return
	}
	// The link is spent from here on, whatever happens to the upload.
	res, err := m.d.DB.ExecContext(ctx, "UPDATE drive_upload_links SET used_at=? WHERE id=? AND used_at IS NULL", time.Now().UTC(), link.id)
	if fail(w, r, err) {
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.Fail(w, r, errLinkGone)
		return
	}
	if r.ContentLength > link.maxSize {
		httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文件超过创建链接时给的大小"))
		return
	}
	item, err := m.storeStream(ctx, http.MaxBytesReader(w, r.Body, link.maxSize), link.parent, link.name, link.onConflict)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			err = httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文件超过创建链接时给的大小")
		}
		httpx.Fail(w, r, err)
		return
	}
	m.audit(ctx, "drive.upload_link", item.ID, nil)
	httpx.JSON(w, http.StatusCreated, m.dto(ctx, item))
}

// storeStream writes a stream to the drive as a new file.
func (m *Module) storeStream(ctx context.Context, body io.Reader, parent *int64, name, onConflict string) (db.DriveItem, error) {
	tmp, err := os.CreateTemp(m.tmpDir, "upload-*")
	if err != nil {
		return db.DriveItem{}, err
	}
	defer os.Remove(tmp.Name())
	defer contracts.TrackTemporaryFile(tmp.Name())()
	h := sha256.New()
	sniff := make([]byte, 0, 512)
	n, err := io.Copy(io.MultiWriter(tmp, h, &headBuffer{buf: &sniff}), body)
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return db.DriveItem{}, err
	}
	if n == 0 {
		return db.DriveItem{}, httpx.Invalid("文件是空的")
	}
	digest := hex.EncodeToString(h.Sum(nil))
	release, err := m.putBlobFile(ctx, digest, tmp.Name(), n)
	if err != nil {
		return db.DriveItem{}, err
	}
	defer release()
	item, err := m.insert(ctx, parent, name, false, n, http.DetectContentType(sniff), digest, false, onConflict == "rename")
	if err != nil {
		m.dropBlobLocked(context.Background(), digest)
	}
	return item, err
}

// headBuffer keeps the first 512 bytes written to it, for content sniffing.
type headBuffer struct{ buf *[]byte }

func (h *headBuffer) Write(p []byte) (int, error) {
	if room := 512 - len(*h.buf); room > 0 {
		*h.buf = append(*h.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

// createDownloadLink records a link for one file.
func (m *Module) createDownloadLink(ctx context.Context, itemID int64, by string) (token string, expires time.Time, err error) {
	hash := ""
	if token, hash, err = newLinkToken(); err != nil {
		return "", expires, err
	}
	now := time.Now().UTC()
	expires = now.Add(linkTTL)
	_, err = m.d.DB.ExecContext(ctx, "INSERT INTO drive_download_links(token_hash,item_id,expires_at,created_by,created_at) VALUES(?,?,?,?,?)", hash, itemID, expires, by, now)
	return token, expires, err
}

// GetDriveDownloadLink sends the file of a download link.
func (m *Module) GetDriveDownloadLink(w http.ResponseWriter, r *http.Request, token string) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")
	if m.shareRateLimit(w, r) {
		return
	}
	var id, itemID int64
	var expires time.Time
	var used *time.Time
	err := m.d.DB.QueryRowContext(ctx, "SELECT id,item_id,expires_at,used_at FROM drive_download_links WHERE token_hash=?", linkHash(token)).Scan(&id, &itemID, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (used != nil || !expires.After(time.Now().UTC()))) {
		httpx.Fail(w, r, errLinkGone)
		return
	}
	if fail(w, r, err) {
		return
	}
	res, err := m.d.DB.ExecContext(ctx, "UPDATE drive_download_links SET used_at=? WHERE id=? AND used_at IS NULL", time.Now().UTC(), id)
	if fail(w, r, err) {
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.Fail(w, r, errLinkGone)
		return
	}
	item, err := m.row(ctx, itemID)
	if err != nil || item.IsDir != 0 || item.Hidden != 0 || item.TrashedAt != nil {
		httpx.Fail(w, r, errLinkGone)
		return
	}
	f, _, err := files.OpenSeeker(ctx, m.store, blobKey(item.Sha256))
	if errors.Is(err, files.ErrNotFound) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	defer f.Close()
	name := strings.NewReplacer("\"", "_", "\r", "_", "\n", "_").Replace(item.Name)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, item.Name, item.UpdatedAt, f)
	m.audit(ctx, "drive.download_link", item.ID, nil)
}

// pruneLinks removes links that ended long ago.
func (m *Module) pruneLinks(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	if _, err := m.d.DB.ExecContext(ctx, "DELETE FROM drive_upload_links WHERE expires_at < ?", cutoff); err != nil {
		return err
	}
	_, err := m.d.DB.ExecContext(ctx, "DELETE FROM drive_download_links WHERE expires_at < ?", cutoff)
	return err
}
