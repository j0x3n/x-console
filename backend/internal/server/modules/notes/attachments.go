package notes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

const maxAttachmentBytes = 50 << 20

func inlineImage(kind string) bool {
	switch kind {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif":
		return true
	}
	return false
}

var attachmentImage = regexp.MustCompile(`!\[[^\]]*\]\((/api/v1/notes/attachments/([0-9]+))\)`)

type attachmentRow struct {
	ID        int64
	NoteID    int64
	Name      string
	Mime      string
	Size      int64
	CreatedAt time.Time
}

func (m *Module) attachmentPath(id int64) string {
	return filepath.Join(m.d.Config.DataDir, "notes", "attachments", strconv.FormatInt(id, 10))
}

func attachmentURL(id int64) string {
	return "/api/v1/notes/attachments/" + strconv.FormatInt(id, 10)
}

func attachmentDTO(row attachmentRow) api.Attachment {
	return api.Attachment{Id: row.ID, NoteId: row.NoteID, Name: row.Name, Mime: row.Mime, Size: row.Size,
		CreatedAt: row.CreatedAt, Url: attachmentURL(row.ID)}
}

func (m *Module) ListNoteAttachments(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	if _, err := m.getNote(r.Context(), noteID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := m.d.DB.QueryContext(r.Context(), `SELECT id, note_id, name, mime, size, created_at FROM note_attachments WHERE note_id = ? ORDER BY created_at DESC, id DESC`, noteID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	out := []api.Attachment{}
	for rows.Next() {
		var row attachmentRow
		if err := rows.Scan(&row.ID, &row.NoteID, &row.Name, &row.Mime, &row.Size, &row.CreatedAt); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out = append(out, attachmentDTO(row))
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UploadNoteAttachment(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	if _, err := m.getNote(r.Context(), noteID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+(1<<20))
	file, head, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文件不能超过 50 MB"))
		} else {
			httpx.Fail(w, r, httpx.Invalid("需要上传文件"))
		}
		return
	}
	defer file.Close()
	defer r.MultipartForm.RemoveAll()
	name := filepath.Base(strings.ReplaceAll(head.Filename, "\\", "/"))
	if name == "." || name == "" {
		httpx.Fail(w, r, httpx.Invalid("文件名不能为空"))
		return
	}
	dir := filepath.Join(m.d.Config.DataDir, "notes", "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tmp, err := os.CreateTemp(dir, ".upload-")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	var sample [512]byte
	n, readErr := io.ReadFull(file, sample[:])
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		httpx.Fail(w, r, readErr)
		return
	}
	kind := http.DetectContentType(sample[:n])
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.MultiReader(bytes.NewReader(sample[:n]), io.LimitReader(file, maxAttachmentBytes+1)))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if written > maxAttachmentBytes {
		httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文件不能超过 50 MB"))
		return
	}
	if err := tmp.Close(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var id int64
	now := m.now()
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer tx.Rollback()
	var exists int64
	if err = tx.QueryRowContext(r.Context(), `SELECT id FROM notes WHERE id = ?`, noteID).Scan(&exists); err == nil {
		var result sql.Result
		result, err = tx.ExecContext(r.Context(), `INSERT INTO note_attachments (note_id, name, mime, size, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?)`, noteID, name, kind, written, fmt.Sprintf("%x", hash.Sum(nil)), now)
		if err == nil {
			id, err = result.LastInsertId()
		}
	}
	if err == nil {
		err = os.Rename(tmp.Name(), m.attachmentPath(id))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		if id != 0 {
			os.Remove(m.attachmentPath(id))
		}
		httpx.Fail(w, r, err)
		return
	}
	out := attachmentDTO(attachmentRow{ID: id, NoteID: noteID, Name: name, Mime: kind, Size: written, CreatedAt: now})
	m.d.Audit.Record(r.Context(), "note.attachment.upload", strconv.FormatInt(noteID, 10), map[string]any{"id": id}, nil)
	m.d.Bus.Publish("note.updated", map[string]int64{"id": noteID})
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) attachment(ctx context.Context, id int64) (attachmentRow, error) {
	var row attachmentRow
	err := m.d.DB.QueryRowContext(ctx, `SELECT id, note_id, name, mime, size, created_at FROM note_attachments WHERE id = ?`, id).Scan(&row.ID, &row.NoteID, &row.Name, &row.Mime, &row.Size, &row.CreatedAt)
	return row, notFound(err)
}

func (m *Module) DownloadNoteAttachment(w http.ResponseWriter, r *http.Request, attachmentID api.AttachmentId) {
	row, err := m.attachment(r.Context(), attachmentID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.getNote(r.Context(), row.NoteID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	file, err := os.Open(m.attachmentPath(attachmentID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = httpx.ErrNotFound
		}
		httpx.Fail(w, r, err)
		return
	}
	defer file.Close()
	disposition := "attachment"
	if inlineImage(row.Mime) {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", row.Mime)
	w.Header().Set("Content-Disposition", disposition+"; filename*=UTF-8''"+url.PathEscape(row.Name))
	w.Header().Set("Cache-Control", "private, max-age=31536000")
	http.ServeContent(w, r, row.Name, row.CreatedAt, file)
}

func (m *Module) DeleteNoteAttachment(w http.ResponseWriter, r *http.Request, attachmentID api.AttachmentId) {
	row, err := m.attachment(r.Context(), attachmentID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.getNote(r.Context(), row.NoteID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := os.Remove(m.attachmentPath(attachmentID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.d.DB.ExecContext(r.Context(), `DELETE FROM note_attachments WHERE id = ?`, attachmentID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(r.Context(), "note.attachment.delete", strconv.FormatInt(attachmentID, 10), nil, nil)
	m.d.Bus.Publish("note.updated", map[string]int64{"id": row.NoteID})
	httpx.NoContent(w)
}

func (m *Module) removeNoteFiles(ctx context.Context, id int64) error {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT id FROM note_attachments WHERE note_id = ?`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var fileID int64
		if err := rows.Scan(&fileID); err != nil {
			return err
		}
		if err := os.Remove(m.attachmentPath(fileID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return rows.Err()
}

func (m *Module) thumbnail(ctx context.Context, noteID int64, body string) *string {
	for _, match := range attachmentImage.FindAllStringSubmatch(body, -1) {
		id, err := strconv.ParseInt(match[2], 10, 64)
		if err != nil {
			continue
		}
		var kind string
		if err := m.d.DB.QueryRowContext(ctx, `SELECT mime FROM note_attachments WHERE id = ? AND note_id = ?`, id, noteID).Scan(&kind); err == nil && inlineImage(kind) {
			return &match[1]
		}
	}
	return nil
}
