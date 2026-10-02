package maintenance

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type AttachmentCleaner struct {
	Deps  *module.Deps
	Notes bool
	Now   func() time.Time
}

type attachmentCandidate struct {
	ID                int64
	Scope, Name, Hash string
	Size              int64
	Hidden            bool
	Missing           bool
	Owned             bool
}

func candidateID(v any) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeCandidate(id string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
func (c AttachmentCleaner) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func (c AttachmentCleaner) table() string {
	if c.Notes {
		return "note_attachments"
	}
	return "uploaded_files"
}
func (c AttachmentCleaner) key(id int64) string {
	prefix := "uploads/"
	if c.Notes {
		prefix = "attachments/"
	}
	return prefix + strconv.FormatInt(id, 10)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (c AttachmentCleaner) referenced(ctx context.Context, q queryer, id int64) (bool, error) {
	queries := []string{"SELECT body FROM notes"}
	if !c.Notes {
		queries = []string{"SELECT description FROM projects", "SELECT description FROM issues", "SELECT body FROM issue_comments", "SELECT description FROM calendar_events", "SELECT body FROM reminders", "SELECT prompt FROM coding_tasks"}
		var n int
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM issues WHERE cover_file_id=?", id).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	for _, query := range queries {
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return false, err
		}
		found := false
		for rows.Next() {
			var body string
			if err = rows.Scan(&body); err != nil {
				break
			}
			if References(body, id, c.Notes) {
				found = true
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func (c AttachmentCleaner) Scan(ctx context.Context) ([]contracts.CleanupItem, error) {
	query := "SELECT id,scope,name,size,sha256,0,owner_kind IS NOT NULL FROM uploaded_files WHERE created_at<?"
	if c.Notes {
		query = "SELECT a.id,'notes',a.name,a.size,a.sha256,n.hidden,0 FROM note_attachments a JOIN notes n ON n.id=a.note_id WHERE a.created_at<?"
	}
	rows, err := c.Deps.DB.QueryContext(ctx, query, c.now().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	candidates := []attachmentCandidate{}
	for rows.Next() {
		var v attachmentCandidate
		var h, owned int64
		if err = rows.Scan(&v.ID, &v.Scope, &v.Name, &v.Size, &v.Hash, &h, &owned); err != nil {
			break
		}
		v.Hidden = h != 0
		v.Owned = owned != 0
		candidates = append(candidates, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []contracts.CleanupItem{}
	for _, v := range candidates {
		referenced, e := c.referenced(ctx, c.Deps.DB, v.ID)
		if e != nil {
			return nil, e
		}
		_, e = StoredStat(ctx, c.Deps, v.Scope, c.key(v.ID))
		if e != nil && !errors.Is(e, files.ErrNotFound) {
			return nil, e
		}
		kind := "unclaimed_uploads"
		reason := "超过一天且未被认领或引用"
		if c.Notes {
			kind = "note_attachments"
			reason = "超过一天且全部笔记正文均未引用"
		}
		if errors.Is(e, files.ErrNotFound) {
			v.Missing = true
			kind = "missing_records"
			reason = "存储中缺少原文件"
		} else if referenced || v.Owned {
			continue
		}
		out = append(out, contracts.CleanupItem{ID: candidateID(v), Kind: kind, Module: v.Scope, Hidden: v.Hidden, RecordTable: c.table(), RecordID: v.ID, Name: v.Name, Bytes: v.Size, Reason: reason})
	}
	return out, nil
}

func (c AttachmentCleaner) Clean(ctx context.Context, ids []string) (contracts.CleanupResult, error) {
	var out contracts.CleanupResult
	for _, id := range ids {
		var v attachmentCandidate
		if err := decodeCandidate(id, &v); err != nil {
			return out, err
		}
		tx, err := c.Deps.DB.BeginTx(ctx, nil)
		if err != nil {
			return out, err
		}
		deleted, bytes, e := c.cleanOne(ctx, tx, v)
		if e == nil {
			e = tx.Commit()
		}
		_ = tx.Rollback()
		out.Bytes += bytes
		if e != nil {
			out.Failed++
			return out, e
		}
		if deleted {
			out.Deleted++
		} else {
			out.Skipped++
		}
	}
	return out, nil
}

func (c AttachmentCleaner) cleanOne(ctx context.Context, tx *sql.Tx, v attachmentCandidate) (bool, int64, error) {
	query := "SELECT sha256 FROM uploaded_files WHERE id=? AND scope=? AND created_at<?"
	if !v.Missing {
		query += " AND owner_kind IS NULL"
	}
	args := []any{v.ID, v.Scope, c.now().Add(-24 * time.Hour)}
	if c.Notes {
		query = "SELECT a.sha256 FROM note_attachments a JOIN notes n ON n.id=a.note_id WHERE a.id=? AND a.created_at<? AND (n.hidden=0 OR ?=1)"
		args = []any{v.ID, c.now().Add(-24 * time.Hour), auth.VaultUnlocked(ctx)}
	}
	var hash string
	err := tx.QueryRowContext(ctx, query, args...).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if hash != v.Hash {
		return false, 0, nil
	}
	if h, ok := module.Lookup[contracts.HiddenModules](c.Deps.Registry, contracts.HiddenModulesKey); ok && contracts.BackendBlocked(ctx, h, v.Scope, "/"+v.Scope) {
		return false, 0, nil
	}
	info, err := StoredStat(ctx, c.Deps, v.Scope, c.key(v.ID))
	if err != nil && !errors.Is(err, files.ErrNotFound) {
		return false, 0, err
	}
	var freed int64
	if v.Missing {
		if !errors.Is(err, files.ErrNotFound) {
			return false, 0, nil
		}
	} else {
		ref, e := c.referenced(ctx, tx, v.ID)
		if e != nil {
			return false, 0, e
		}
		if ref {
			return false, 0, nil
		}
		if !c.Notes {
			n, e := DeleteObject(ctx, c.Deps, v.Scope, c.key(v.ID)+".thumb.jpg")
			if e != nil {
				return false, 0, e
			}
			freed += n
		}
		if e = c.Deps.Files.For(v.Scope).Delete(ctx, c.key(v.ID)); e != nil {
			return false, freed, e
		}
		if err == nil {
			freed += info.Size
		}
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM "+c.table()+" WHERE id=?", v.ID)
	return err == nil, freed, err
}
