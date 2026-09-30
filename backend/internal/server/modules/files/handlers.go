package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	filestore "github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/files/api"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxUpload = 20 << 20

func cleanName(value string) string {
	value = filepath.Base(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	chars := []rune(value)
	if len(chars) > 100 {
		value = string(chars[:100])
	}
	if value == "" || value == "." {
		return "image"
	}
	return value
}
func imageMime(sample []byte) string {
	mime := http.DetectContentType(sample)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return mime
	}
	return ""
}
func (m *Module) UploadFile(w http.ResponseWriter, r *http.Request, p api.UploadFileParams) {
	ctx := r.Context()
	scope := string(p.Scope)
	if !validScope(scope) {
		httpx.Fail(w, r, httpx.Invalid("上传范围无效"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	file, head, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, httpx.NewError(413, "too_large", "图片不能超过 20 MB"))
		} else {
			httpx.Fail(w, r, httpx.Invalid("需要上传图片"))
		}
		return
	}
	defer file.Close()
	defer r.MultipartForm.RemoveAll()
	data, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(data) > maxUpload {
		httpx.Fail(w, r, httpx.NewError(413, "too_large", "图片不能超过 20 MB"))
		return
	}
	mime := imageMime(data[:min(len(data), 512)])
	if mime == "" {
		httpx.Fail(w, r, httpx.NewError(415, "unsupported_type", "只支持 PNG、JPEG、GIF、WebP 图片"))
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 20_000_000 {
		httpx.Fail(w, r, httpx.Invalid("图片文件无效"))
		return
	}
	name := cleanName(head.Filename)
	sum := sha256.Sum256(data)
	var id int64
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO uploaded_files(scope,name,mime,size,sha256,created_at) VALUES(?,?,?,?,?,?) RETURNING id", scope, name, mime, len(data), hex.EncodeToString(sum[:]), m.now()).Scan(&id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.store(scope).Put(ctx, key(id), bytes.NewReader(data), int64(len(data))); err != nil {
		_, _ = m.d.DB.ExecContext(ctx, "DELETE FROM uploaded_files WHERE id=?", id)
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "files.upload", strconv.FormatInt(id, 10), map[string]any{"scope": scope, "size": len(data), "mime": mime}, nil)
	httpx.JSON(w, 201, api.UploadedFile{Id: id, Name: name, Mime: mime, Size: int64(len(data)), Url: "/api/v1/files/" + strconv.FormatInt(id, 10)})
}
func (m *Module) DownloadFile(w http.ResponseWriter, r *http.Request, id int64, p api.DownloadFileParams) {
	ctx := r.Context()
	x, err := m.file(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	store := m.store(x.scope)
	target := key(id)
	mime := x.mime
	if p.Thumb != nil && *p.Thumb && mime != "image/gif" {
		if err = m.ensureThumbnail(ctx, store, id); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if _, err = store.Stat(ctx, target+".thumb.jpg"); err == nil {
			target += ".thumb.jpg"
			mime = "image/jpeg"
		}
	}
	reader, _, err := store.Get(ctx, target)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(x.name))
	_, _ = io.Copy(w, reader)
}
func (m *Module) ensureThumbnail(ctx context.Context, store filestore.Store, id int64) error {
	thumb := key(id) + ".thumb.jpg"
	if _, err := store.Stat(ctx, thumb); err == nil {
		return nil
	} else if !errors.Is(err, filestore.ErrNotFound) {
		return err
	}
	reader, _, err := store.Get(ctx, key(id))
	if err != nil {
		return err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxUpload+1))
	if err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if cfg.Width <= 1600 && cfg.Height <= 1600 {
		return nil
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	width, height := cfg.Width, cfg.Height
	if width >= height {
		height = max(1, height*1600/width)
		width = 1600
	} else {
		width = max(1, width*1600/height)
		height = 1600
	}
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), draw.Over, nil)
	var output bytes.Buffer
	if err = jpeg.Encode(&output, target, &jpeg.Options{Quality: 85}); err != nil {
		return err
	}
	return store.Put(ctx, thumb, bytes.NewReader(output.Bytes()), int64(output.Len()))
}
func (m *Module) DeleteFile(w http.ResponseWriter, r *http.Request, id int64) {
	err := m.delete(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(r.Context(), "files.delete", strconv.FormatInt(id, 10), nil, nil)
	httpx.NoContent(w)
}
