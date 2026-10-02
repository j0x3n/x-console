package notes

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

var sharedAttachment = regexp.MustCompile(`/api/v1/notes/attachments/([0-9]+)(?:\?[^\s\)\]>"']*)?`)

func rewriteNoteAttachments(body, token, access string) string {
	return sharedAttachment.ReplaceAllStringFunc(body, func(match string) string {
		parts := sharedAttachment.FindStringSubmatch(match)
		query := url.Values{}
		if u, err := url.Parse(match); err == nil && u.Query().Get("thumb") != "" {
			query.Set("thumb", u.Query().Get("thumb"))
		}
		if access != "" {
			query.Set("t", access)
		}
		path := "/api/v1/public/notes/" + token + "/files/" + parts[1]
		if len(query) != 0 {
			path += "?" + query.Encode()
		}
		return path
	})
}

func noteReferencesAttachment(body string, id int64) bool {
	for _, match := range sharedAttachment.FindAllStringSubmatch(body, -1) {
		found, err := strconv.ParseInt(match[1], 10, 64)
		if err == nil && found == id {
			return true
		}
	}
	return false
}

func (m *Module) GetPublicNoteFile(w http.ResponseWriter, r *http.Request, token api.NoteShareToken, id api.AttachmentId, params api.GetPublicNoteFileParams) {
	_, n, ok := m.publicNote(w, r, token, params.T)
	if !ok {
		return
	}
	if !noteReferencesAttachment(n.Body, id) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	row, _, err := m.attachment(r.Context(), id)
	if err != nil || row.NoteID != n.ID {
		if err == nil {
			err = httpx.ErrNotFound
		}
		httpx.Fail(w, r, err)
		return
	}
	file, _, err := files.OpenSeeker(r.Context(), m.files, attachmentKey(id))
	if err != nil {
		if errors.Is(err, files.ErrNotFound) {
			err = httpx.ErrNotFound
		}
		httpx.Fail(w, r, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(row.Name))
	w.Header().Set("Content-Type", row.Mime)
	if deref(params.Thumb) {
		if !inlineImage(row.Mime) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		thumb, err := noteShareThumbnail(file)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(w, r, row.Name, row.CreatedAt, bytes.NewReader(thumb))
		return
	}
	http.ServeContent(w, r, row.Name, row.CreatedAt, file)
}

func noteShareThumbnail(file io.ReadSeeker) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(io.LimitReader(file, maxAttachmentBytes))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 50_000_000 {
		return nil, httpx.Invalid("图片无法生成缩略图")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(io.LimitReader(file, maxAttachmentBytes))
	if err != nil {
		return nil, httpx.Invalid("图片无法生成缩略图")
	}
	width, height := cfg.Width, cfg.Height
	if width >= height && width > 480 {
		height = max(1, height*480/width)
		width = 480
	} else if height > 480 {
		width = max(1, width*480/height)
		height = 480
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(out, out.Bounds(), src, src.Bounds(), draw.Over, nil)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, out, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}
