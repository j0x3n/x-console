package readlater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/db"
)

// Archiving a page (B143): the HTML is cleaned and every picture is saved on
// this server, so the page still reads the same when the original is gone.

const (
	filesModule    = "readlater"
	maxImages      = 50
	maxImageBytes  = 10 << 20
	maxImageTotal  = 50 << 20
	maxArchiveHTML = 2 << 20
	archiveTimeout = 90 * time.Second
	assetPath      = "/api/v1/readlater/%d/assets/%s"
)

// archiver downloads the pictures of one page while its HTML is cleaned.
type archiver struct {
	m       *Module
	ctx     context.Context
	id      int64
	referer string
	total   int64
	mapped  map[string]string // original address -> address on this server ("" when dropped)
	kept    map[string]bool   // hashes in use
}

// archive cleans the HTML of a page and saves its pictures. It returns the
// cleaned HTML and the hashes of the pictures it uses. The HTML is empty when
// the page has none worth keeping.
func (m *Module) archive(ctx context.Context, id int64, pageURL *url.URL, raw string) (string, map[string]bool) {
	a := &archiver{m: m, ctx: ctx, id: id, referer: pageURL.String(), mapped: map[string]string{}, kept: map[string]bool{}}
	s := &sanitizer{base: pageURL, image: a.image}
	out := strings.TrimSpace(s.clean(raw))
	if len(out) > maxArchiveHTML {
		return "", a.kept
	}
	return out, a.kept
}

// image is called for each picture of the page. It returns the address to use
// in the archive: ours when the picture was saved, the original when it could
// not be (too big, too many, not reachable), and "" when it must not be shown
// at all (an internal address).
func (a *archiver) image(src string) string {
	if v, ok := a.mapped[src]; ok {
		return v
	}
	v := src
	if len(a.mapped) < maxImages && a.total < maxImageTotal {
		switch hash, err := a.save(src); {
		case err == nil:
			v = fmt.Sprintf(assetPath, a.id, hash)
		case errors.Is(err, errBlocked):
			v = ""
		default:
			a.m.d.Log.Debug("readlater image", "id", a.id, "err", err)
		}
	}
	a.mapped[src] = v
	return v
}

// save downloads one picture and keeps it. It returns the hash of its content.
func (a *archiver) save(src string) (string, error) {
	body, mime, err := a.m.downloadImage(a.ctx, src, a.referer)
	if err != nil {
		return "", err
	}
	if a.total+int64(len(body)) > maxImageTotal {
		return "", errors.New("图片总量超过限制")
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	key := strconv.FormatInt(a.id, 10) + "/" + hash
	if !a.kept[hash] {
		if err := a.m.fileStore().Put(a.ctx, key, bytes.NewReader(body), int64(len(body))); err != nil {
			return "", err
		}
		err := a.m.q.InsertReadAsset(a.ctx, db.InsertReadAssetParams{ItemID: a.id, Hash: hash, Mime: mime, Size: int64(len(body)), SrcUrl: clipRunes(src, maxURL), CreatedAt: a.m.now().UTC()})
		if err != nil {
			return "", err
		}
		a.kept[hash] = true
		a.total += int64(len(body))
	}
	return hash, nil
}

func (m *Module) fileStore() files.Store { return m.d.Files.For(filesModule) }

// downloadImage reads one picture with the same guarded client as the pages.
func (m *Module) downloadImage(ctx context.Context, src, referer string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", errors.New("图片地址不对")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif,image/*;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("图片状态码 %d", resp.StatusCode)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, "", errors.New("图片太大")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxImageBytes {
		return nil, "", errors.New("图片太大")
	}
	mime, ok := imageType(body)
	if !ok {
		return nil, "", errors.New("不是可存的图片")
	}
	return body, mime, nil
}

// imageType decides the type from the content, not from what the server says.
// SVG is refused because it can carry scripts.
func imageType(b []byte) (string, bool) {
	t := http.DetectContentType(b)
	switch t {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return t, true
	}
	if len(b) > 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "avif" || string(b[8:12]) == "avis") {
		return "image/avif", true
	}
	return "", false
}

// dropAssets removes the saved pictures of an item that are not in keep.
func (m *Module) dropAssets(ctx context.Context, id int64, keep map[string]bool) {
	rows, err := m.q.ListReadAssets(ctx, id)
	if err != nil {
		return
	}
	for _, r := range rows {
		if keep[r.Hash] {
			continue
		}
		if err := m.fileStore().Delete(ctx, strconv.FormatInt(id, 10)+"/"+r.Hash); err != nil && !errors.Is(err, files.ErrNotFound) {
			m.d.Log.Warn("readlater delete image", "id", id, "err", err)
		}
		_ = m.q.DeleteReadAsset(ctx, db.DeleteReadAssetParams{ItemID: id, Hash: r.Hash})
	}
}

// GetReadAsset implements api.ServerInterface.
func (m *Module) GetReadAsset(w http.ResponseWriter, r *http.Request, id api.ItemId, hash string) {
	ctx := r.Context()
	row, err := m.q.GetReadAsset(ctx, db.GetReadAssetParams{ItemID: id, Hash: hash})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	rc, _, err := m.fileStore().Get(ctx, strconv.FormatInt(id, 10)+"/"+hash)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", row.Mime)
	w.Header().Set("Content-Length", strconv.FormatInt(row.Size, 10))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, rc)
}

const exportPage = `<!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>body{max-width:760px;margin:0 auto;padding:24px 16px;font:16px/1.7 system-ui,sans-serif;color:#222;overflow-wrap:anywhere}
img{max-width:100%%;height:auto}pre{overflow:auto;background:#f4f4f4;padding:12px}blockquote{margin:12px 0;padding-left:12px;border-left:3px solid #ccc;color:#555}
table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:4px 8px}
.xc-tweet-head{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.xc-avatar{width:40px;height:40px;border-radius:50%%}
.xc-tweet-handle,.xc-tweet-time,.meta{color:#666;font-size:14px}.xc-tweet-media{margin:8px 0}</style></head><body>
<h1>%s</h1><p class="meta">来源：<a href="%s">%s</a>，存档于 %s</p>%s</body></html>`

// ExportReadItem implements api.ServerInterface.
func (m *Module) ExportReadItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	ctx := r.Context()
	row, err := m.get(ctx, id)
	if err == nil && row.ContentHtml == "" {
		err = httpx.NewError(http.StatusConflict, "no_archive", "这一条还没有存档的图文，先重新抓取")
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	body := row.ContentHtml
	assets, _ := m.q.ListReadAssets(ctx, id)
	for _, a := range assets {
		prefix := fmt.Sprintf(assetPath, id, a.Hash)
		if !strings.Contains(body, prefix) {
			continue
		}
		rc, _, err := m.fileStore().Get(ctx, strconv.FormatInt(id, 10)+"/"+a.Hash)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		body = strings.ReplaceAll(body, prefix, "data:"+a.Mime+";base64,"+base64.StdEncoding.EncodeToString(data))
	}
	page := fmt.Sprintf(exportPage, html.EscapeString(row.Title), html.EscapeString(row.Title), html.EscapeString(row.Url), html.EscapeString(row.Url),
		m.now().In(m.location()).Format("2006-01-02 15:04"), body)
	m.d.Audit.Record(ctx, "readlater.export", strconv.FormatInt(id, 10), nil, nil)
	name := url.PathEscape(clipRunes(strings.Join(strings.Fields(row.Title), " "), 60)) + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"readlater.html\"; filename*=UTF-8''"+name)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data: https: http:; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.WriteString(w, page)
}
