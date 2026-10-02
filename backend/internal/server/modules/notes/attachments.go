package notes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
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

// attachmentKey is the key of an attachment's content in the notes' store.
func attachmentKey(id int64) string { return "attachments/" + strconv.FormatInt(id, 10) }

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
	tmp, err := os.CreateTemp(m.d.Config.TmpDir(), "note-upload-")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer contracts.TrackTemporaryFile(tmp.Name())()
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
	var id int64
	now := m.now()
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer tx.Rollback()
	var exists int64
	if err = tx.QueryRowContext(r.Context(), `SELECT id FROM notes WHERE id = ? AND (hidden = 0 OR ? = 1)`, noteID, boolInt(auth.VaultUnlocked(r.Context()))).Scan(&exists); err == nil {
		var result sql.Result
		result, err = tx.ExecContext(r.Context(), `INSERT INTO note_attachments (note_id, name, mime, size, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?)`, noteID, name, kind, written, fmt.Sprintf("%x", hash.Sum(nil)), now)
		if err == nil {
			id, err = result.LastInsertId()
		}
	}
	if err == nil {
		_, err = tmp.Seek(0, io.SeekStart)
	}
	if err == nil {
		err = m.files.Put(r.Context(), attachmentKey(id), tmp, written)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		if id != 0 {
			_ = m.files.Delete(context.WithoutCancel(r.Context()), attachmentKey(id))
		}
		if errors.Is(err, sql.ErrNoRows) {
			err = httpx.ErrNotFound
		}
		httpx.Fail(w, r, err)
		return
	}
	out := attachmentDTO(attachmentRow{ID: id, NoteID: noteID, Name: name, Mime: kind, Size: written, CreatedAt: now})
	m.d.Audit.Record(r.Context(), "note.attachment.upload", strconv.FormatInt(noteID, 10), map[string]any{"id": id}, nil)
	m.noteChanged(r.Context(), noteID)
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) attachment(ctx context.Context, id int64) (attachmentRow, bool, error) {
	var row attachmentRow
	var hidden int64
	err := m.d.DB.QueryRowContext(ctx, `SELECT a.id, a.note_id, a.name, a.mime, a.size, a.created_at, n.hidden FROM note_attachments a JOIN notes n ON n.id = a.note_id WHERE a.id = ?`, id).Scan(&row.ID, &row.NoteID, &row.Name, &row.Mime, &row.Size, &row.CreatedAt, &hidden)
	if err != nil {
		return row, false, notFound(err)
	}
	if hidden != 0 && !auth.VaultUnlocked(ctx) {
		return row, false, httpx.ErrNotFound
	}
	return row, hidden != 0, nil
}

func (m *Module) DownloadNoteAttachment(w http.ResponseWriter, r *http.Request, attachmentID api.AttachmentId) {
	row, hidden, err := m.attachment(r.Context(), attachmentID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.getNote(r.Context(), row.NoteID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	file, _, err := files.OpenSeeker(r.Context(), m.files, attachmentKey(attachmentID))
	if err != nil {
		if errors.Is(err, files.ErrNotFound) {
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
	// Like the drive: never let the browser guess a type, and never run an
	// uploaded page with the panel's origin.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.Contains(row.Mime, "html") || strings.Contains(row.Mime, "svg") || strings.Contains(row.Mime, "xml") {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("Content-Disposition", disposition+"; filename*=UTF-8''"+url.PathEscape(row.Name))
	if hidden {
		w.Header().Set("Cache-Control", "private, no-store")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=31536000")
	}
	http.ServeContent(w, r, row.Name, row.CreatedAt, file)
}

func (m *Module) DeleteNoteAttachment(w http.ResponseWriter, r *http.Request, attachmentID api.AttachmentId) {
	row, _, err := m.attachment(r.Context(), attachmentID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.getNote(r.Context(), row.NoteID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// The row goes first: if that fails the file is still there and the note
	// still shows it. A file left after a failed delete is only wasted space.
	if _, err := m.d.DB.ExecContext(r.Context(), `DELETE FROM note_attachments WHERE id = ?`, attachmentID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.deleteAttachmentFiles(r.Context(), []int64{attachmentID})
	m.d.Audit.Record(r.Context(), "note.attachment.delete", strconv.FormatInt(attachmentID, 10), nil, nil)
	m.noteChanged(r.Context(), row.NoteID)
	httpx.NoContent(w)
}

// noteAttachmentIDs lists a note's attachments, read before the note row
// (and with it, by ON DELETE CASCADE, these rows) is deleted.
func (m *Module) noteAttachmentIDs(ctx context.Context, id int64) ([]int64, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT id FROM note_attachments WHERE note_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var fileID int64
		if err := rows.Scan(&fileID); err != nil {
			return nil, err
		}
		ids = append(ids, fileID)
	}
	return ids, rows.Err()
}

// deleteAttachmentFiles removes stored files whose rows are already gone.
func (m *Module) deleteAttachmentFiles(ctx context.Context, ids []int64) {
	for _, id := range ids {
		if err := m.files.Delete(ctx, attachmentKey(id)); err != nil {
			m.d.Log.Warn("note attachment file not deleted", "attachment", id, "err", err)
		}
	}
}

// thumbnails picks each note's first image attachment with one query for
// the whole list, instead of one per image link.
func (m *Module) thumbnails(ctx context.Context, notes []db.Note) map[int64]*string {
	type candidate struct {
		id  int64
		url string
	}
	byNote := map[int64][]candidate{}
	var ids []any
	for _, n := range notes {
		for _, match := range attachmentImage.FindAllStringSubmatch(n.Body, -1) {
			id, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				continue
			}
			byNote[n.ID] = append(byNote[n.ID], candidate{id, match[1]})
			ids = append(ids, id)
		}
	}
	out := map[int64]*string{}
	if len(ids) == 0 {
		return out
	}
	type owner struct {
		note  int64
		image bool
	}
	found := map[int64]owner{}
	for len(ids) > 0 {
		batch := ids[:min(len(ids), 500)]
		ids = ids[len(batch):]
		rows, err := m.d.DB.QueryContext(ctx, `SELECT id, note_id, mime FROM note_attachments WHERE id IN (?`+strings.Repeat(",?", len(batch)-1)+`)`, batch...)
		if err != nil {
			return out
		}
		for rows.Next() {
			var id, noteID int64
			var kind string
			if rows.Scan(&id, &noteID, &kind) == nil {
				found[id] = owner{noteID, inlineImage(kind)}
			}
		}
		rows.Close()
	}
	for noteID, candidates := range byNote {
		for _, c := range candidates {
			if o, ok := found[c.id]; ok && o.note == noteID && o.image {
				url := c.url
				out[noteID] = &url
				break
			}
		}
	}
	return out
}
