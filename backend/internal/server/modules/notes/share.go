package notes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
)

type noteShareRow struct {
	NoteID       int64
	Token        string
	PasswordHash *string
	ExpiresAt    *time.Time
	Visits       int
	LastVisitAt  *time.Time
	CreatedAt    time.Time
}

var errNoteShare = httpx.NewError(404, "share_not_found", "分享链接已失效")

func scanNoteShare(row interface{ Scan(...any) error }) (noteShareRow, error) {
	var s noteShareRow
	err := row.Scan(&s.NoteID, &s.Token, &s.PasswordHash, &s.ExpiresAt, &s.Visits, &s.LastVisitAt, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNoteShare
	}
	return s, err
}

const shareColumns = "note_id, token, password_hash, expires_at, visits, last_visit_at, created_at"

func (m *Module) shareDTO(r *http.Request, s noteShareRow) api.NoteShare {
	base := strings.TrimRight(m.d.Config.PublicURL, "/")
	if base == "" {
		proto := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			proto = "https"
		}
		base = proto + "://" + r.Host
	}
	return api.NoteShare{Token: s.Token, Url: base + "/n/" + s.Token, ExpiresAt: s.ExpiresAt, HasPassword: s.PasswordHash != nil, Visits: s.Visits, LastVisitAt: s.LastVisitAt, CreatedAt: s.CreatedAt}
}

func (m *Module) GetNoteShare(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	if _, err := m.noteRow(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := scanNoteShare(m.d.DB.QueryRowContext(r.Context(), "SELECT "+shareColumns+" FROM note_shares WHERE note_id=?", id))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, m.shareDTO(r, s))
}

func (m *Module) putNoteShare(ctx context.Context, id int64, in api.NoteShareInput) (out noteShareRow, err error) {
	defer func() { m.d.Audit.Record(ctx, "note.share.put", strconv.FormatInt(id, 10), nil, err) }()
	if err = auth.RequireElevated(ctx); err != nil {
		return out, err
	}
	var expires *time.Time
	now := m.now()
	switch in.ExpiresIn {
	case api.N1d:
		expires = new(now.Add(24 * time.Hour))
	case api.N7d:
		expires = new(now.Add(7 * 24 * time.Hour))
	case api.N30d:
		expires = new(now.Add(30 * 24 * time.Hour))
	case api.Never:
	default:
		return out, httpx.Invalid("有效期不正确")
	}
	var hash *string
	if in.Password != nil {
		length := utf8.RuneCountInString(*in.Password)
		if length < 4 || length > 32 || !utf8.ValidString(*in.Password) || deref(in.ClearPassword) {
			return out, httpx.Invalid("密码需要 4 到 32 个字，不能同时清除密码")
		}
		digest := sha256.Sum256([]byte(*in.Password))
		var value []byte
		value, err = bcrypt.GenerateFromPassword(digest[:], bcrypt.DefaultCost)
		if err != nil {
			return out, err
		}
		hash = new(string(value))
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var hidden int64
	if err = tx.QueryRowContext(ctx, "SELECT hidden FROM notes WHERE id=?", id).Scan(&hidden); err != nil {
		return out, notFound(err)
	}
	if hidden != 0 {
		return out, httpx.Invalid("隐藏笔记不能分享")
	}
	out, err = scanNoteShare(tx.QueryRowContext(ctx, "SELECT "+shareColumns+" FROM note_shares WHERE note_id=?", id))
	if errors.Is(err, errNoteShare) {
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return out, err
		}
		out = noteShareRow{NoteID: id, Token: base64.RawURLEncoding.EncodeToString(random[:]), CreatedAt: now}
	} else if err != nil {
		return out, err
	}
	if in.Password != nil {
		out.PasswordHash = hash
	} else if deref(in.ClearPassword) {
		out.PasswordHash = nil
	}
	out.ExpiresAt = expires
	_, err = tx.ExecContext(ctx, `INSERT INTO note_shares (note_id,token,password_hash,expires_at,created_at) VALUES (?,?,?,?,?) ON CONFLICT(note_id) DO UPDATE SET password_hash=excluded.password_hash,expires_at=excluded.expires_at`, id, out.Token, out.PasswordHash, expires, out.CreatedAt)
	if err != nil {
		return out, err
	}
	err = tx.Commit()
	return out, err
}

func (m *Module) PutNoteShare(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.NoteShareInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.putNoteShare(r.Context(), id, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, m.shareDTO(r, out))
}

func (m *Module) DeleteNoteShare(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	err := func() error {
		if _, err := m.noteRow(r.Context(), id); err != nil {
			return err
		}
		_, err := m.q.DeleteNoteShare(r.Context(), id)
		return err
	}()
	m.d.Audit.Record(r.Context(), "note.share.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) sharesForNotes(ctx context.Context, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids), len(ids)+1)
	for i, id := range ids {
		args[i] = id
	}
	args = append(args, m.now())
	rows, err := m.d.DB.QueryContext(ctx, "SELECT s.note_id FROM note_shares s JOIN notes n ON n.id=s.note_id WHERE n.hidden=0 AND s.note_id IN (?"+strings.Repeat(",?", len(ids)-1)+") AND (s.expires_at IS NULL OR s.expires_at>?)", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (m *Module) resolvePublicNote(ctx context.Context, token string) (noteShareRow, db.Note, error) {
	s, err := scanNoteShare(m.d.DB.QueryRowContext(ctx, "SELECT "+shareColumns+" FROM note_shares WHERE token=?", token))
	if err != nil {
		return s, db.Note{}, err
	}
	if s.ExpiresAt != nil && !s.ExpiresAt.After(m.now()) {
		return s, db.Note{}, errNoteShare
	}
	n, err := m.q.GetNote(ctx, s.NoteID)
	if errors.Is(err, sql.ErrNoRows) || n.Hidden != 0 {
		return s, n, errNoteShare
	}
	return s, n, err
}
