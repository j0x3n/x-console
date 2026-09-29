package drive

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
)

func (m *Module) UploadDriveFiles(w http.ResponseWriter, r *http.Request, p api.UploadDriveFilesParams) {
	ctx := r.Context()
	hidden := boolValue(p.Hidden)
	if hidden && !auth.VaultUnlocked(ctx) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	parent, err := m.parent(ctx, p.Parent, hidden)
	if fail(w, r, err) {
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("需要 multipart/form-data"))
		return
	}
	items := []api.DriveItem{}
	for {
		part, e := reader.NextPart()
		if errors.Is(e, io.EOF) {
			break
		}
		if fail(w, r, e) {
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		name := filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
		if !validName(name) {
			part.Close()
			httpx.Fail(w, r, httpx.Invalid("文件名不正确"))
			return
		}
		tmp, e := os.CreateTemp(m.root, "upload-*")
		if fail(w, r, e) {
			part.Close()
			return
		}
		hash := sha256.New()
		buffered := bufio.NewReader(part)
		sniff, _ := buffered.Peek(512)
		mime := http.DetectContentType(sniff)
		size, e := io.CopyBuffer(io.MultiWriter(tmp, hash), buffered, make([]byte, 32*1024))
		part.Close()
		closeErr := tmp.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			os.Remove(tmp.Name())
			httpx.Fail(w, r, e)
			return
		}
		digest := hex.EncodeToString(hash.Sum(nil))
		dest := m.blobPath(digest)
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			os.Remove(tmp.Name())
			httpx.Fail(w, r, e)
			return
		}
		if _, e = os.Stat(dest); errors.Is(e, os.ErrNotExist) {
			e = os.Rename(tmp.Name(), dest)
		} else {
			os.Remove(tmp.Name())
		}
		if fail(w, r, e) {
			return
		}
		item, e := m.insert(ctx, parent, name, false, size, mime, digest, hidden, true)
		if fail(w, r, e) {
			return
		}
		items = append(items, m.dto(ctx, item))
	}
	if len(items) == 0 {
		httpx.Fail(w, r, httpx.Invalid("请选择文件"))
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"items": items})
}

func (m *Module) GetDriveItemContent(w http.ResponseWriter, r *http.Request, id api.ItemId, p api.GetDriveItemContentParams) {
	item, err := m.visibleRow(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.TrashedAt != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	f, err := os.Open(m.blobPath(item.Sha256))
	if fail(w, r, err) {
		return
	}
	defer f.Close()
	name := strings.NewReplacer("\"", "_", "\r", "_", "\n", "_").Replace(item.Name)
	disposition := "attachment"
	if boolValue(p.Inline) {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, disposition, name))
	w.Header().Set("Content-Type", item.Mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", etag(item.Sha256))
	if strings.Contains(item.Mime, "html") || strings.Contains(item.Mime, "svg") || strings.Contains(item.Mime, "xml") {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	http.ServeContent(w, r, item.Name, item.UpdatedAt, f)
}

func (m *Module) GetDriveItemThumbnail(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	item, err := m.visibleRow(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.TrashedAt != nil || !strings.HasPrefix(item.Mime, "image/") {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	dest := filepath.Join(m.root, "thumbnails", item.Sha256+".jpg")
	if _, err = os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		if err = m.makeThumbnail(item.Sha256, dest); fail(w, r, err) {
			return
		}
	} else if fail(w, r, err) {
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, dest)
}

func (m *Module) makeThumbnail(hash, dest string) error {
	src, err := os.Open(m.blobPath(hash))
	if err != nil {
		return err
	}
	defer src.Close()
	config, _, err := image.DecodeConfig(io.LimitReader(src, 64<<20))
	if err != nil {
		return httpx.Invalid("图片无法预览")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 50_000_000 {
		return httpx.Invalid("图片过大，无法生成缩略图")
	}
	if _, err = src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	imageData, _, err := image.Decode(src)
	if err != nil {
		return httpx.Invalid("图片无法预览")
	}
	width, height := config.Width, config.Height
	if width > height && width > 320 {
		height = height * 320 / width
		width = 320
	} else if height > 320 {
		width = width * 320 / height
		height = 320
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	thumb := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), imageData, imageData.Bounds(), draw.Over, nil)
	tmp, err := os.CreateTemp(filepath.Dir(dest), "thumbnail-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = jpeg.Encode(tmp, thumb, &jpeg.Options{Quality: 80}); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}
