package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type recordCleaner struct{ db *sql.DB }

type retention struct{ kind, table, at, extra, source string }

type recordCandidate struct {
	Table string
	ID    int64
	Token string
}

func (r retention) idColumn() string {
	if r.table == "note_shares" {
		return "note_id"
	}
	return "id"
}

var retentions = []retention{
	{"script_runs", "script_runs", "finished_at", " AND finished_at IS NOT NULL", "monitoring"},
	{"ai_usage", "ai_usage", "created_at", "", ""},
	{"audit_logs", "audit_log", "at", "", ""},
	{"expired_shares", "note_shares", "expires_at", "", "notes"},
}

func (c recordCleaner) Scan(ctx context.Context) ([]contracts.CleanupItem, error) {
	out := []contracts.CleanupItem{}
	for _, r := range retentions {
		cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
		if r.kind == "expired_shares" {
			cutoff = time.Now().UTC()
		}
		query := "SELECT " + r.idColumn() + ",0,'' FROM " + r.table + " WHERE " + r.at + "<?" + r.extra
		if r.table == "note_shares" {
			query = "SELECT note_id,hidden,token FROM note_shares JOIN notes ON notes.id=note_shares.note_id WHERE expires_at<?"
		}
		rows, err := c.db.QueryContext(ctx, query, cutoff)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, hidden int64
			var token string
			if err = rows.Scan(&id, &hidden, &token); err != nil {
				break
			}
			recordTable := ""
			if r.table == "note_shares" {
				recordTable = r.table
			}
			out = append(out, contracts.CleanupItem{ID: candidateID(recordCandidate{Table: r.table, ID: id, Token: token}), Kind: r.kind, Module: r.source, Hidden: hidden != 0, RecordTable: recordTable, RecordID: id, Name: r.table + " #" + strconv.FormatInt(id, 10), Reason: "记录已超过保留期限"})
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c recordCleaner) Clean(ctx context.Context, ids []string) (contracts.CleanupResult, error) {
	var out contracts.CleanupResult
	for _, key := range ids {
		var v recordCandidate
		if err := decodeCandidate(key, &v); err != nil {
			return out, err
		}
		table, id := v.Table, v.ID
		var rule *retention
		for i := range retentions {
			if retentions[i].table == table {
				rule = &retentions[i]
				break
			}
		}
		if rule == nil {
			return out, errors.New("invalid retention table")
		}
		cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
		if rule.kind == "expired_shares" {
			cutoff = time.Now().UTC()
		}
		query := "DELETE FROM " + table + " WHERE " + rule.idColumn() + "=? AND " + rule.at + "<?" + rule.extra
		args := []any{id, cutoff}
		if table == "note_shares" {
			query += " AND note_id IN(SELECT id FROM notes WHERE hidden=0 OR ?=1) AND token=?"
			args = append(args, auth.VaultUnlocked(ctx), v.Token)
		}
		res, err := c.db.ExecContext(ctx, query, args...)
		if err != nil {
			out.Failed++
			return out, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return out, err
		}
		out.Deleted += n
		if n == 0 {
			out.Skipped++
		}
	}
	return out, nil
}

type temporaryCleaner struct {
	root     string
	registry *module.Registry
}

type temporaryCandidate struct {
	Path     string
	Size     int64
	Modified time.Time
}

func (c temporaryCleaner) Scan(ctx context.Context) ([]contracts.CleanupItem, error) {
	out := []contracts.CleanupItem{}
	err := filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(time.Now().Add(-24*time.Hour)) {
			return nil
		}
		if active, ok := module.Lookup[contracts.ActiveTemporaryFiles](c.registry, contracts.ActiveTemporaryFilesKey); ok && active.TemporaryFileActive(path) {
			return nil
		}
		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}
		_, err = contracts.WithInactiveTemporaryFile(path, func() error {
			out = append(out, contracts.CleanupItem{ID: candidateID(temporaryCandidate{Path: filepath.ToSlash(rel), Size: info.Size(), Modified: info.ModTime()}), Kind: "temporary_files", Name: filepath.Base(path), Bytes: info.Size(), Reason: "临时文件超过一天"})
			return nil
		})
		return err
	})
	return out, err
}

func (c temporaryCleaner) Clean(ctx context.Context, ids []string) (contracts.CleanupResult, error) {
	var out contracts.CleanupResult
	root, err := os.OpenRoot(c.root)
	if errors.Is(err, fs.ErrNotExist) {
		out.Skipped = int64(len(ids))
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer root.Close()
	for _, raw := range ids {
		var v temporaryCandidate
		if err = decodeCandidate(raw, &v); err != nil {
			return out, err
		}
		id := v.Path
		if !filepath.IsLocal(id) || strings.Contains(id, "\\") {
			return out, fmt.Errorf("invalid temporary key")
		}
		if err = ctx.Err(); err != nil {
			return out, err
		}
		path := filepath.Join(c.root, filepath.FromSlash(id))
		deleted := false
		_, err = contracts.WithInactiveTemporaryFile(path, func() error {
			if active, ok := module.Lookup[contracts.ActiveTemporaryFiles](c.registry, contracts.ActiveTemporaryFilesKey); ok && active.TemporaryFileActive(path) {
				return nil
			}
			info, e := root.Lstat(id)
			if errors.Is(e, fs.ErrNotExist) {
				return nil
			}
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || !info.ModTime().Before(time.Now().Add(-24*time.Hour)) || info.Size() != v.Size || !info.ModTime().Equal(v.Modified) {
				return nil
			}
			if e = root.Remove(id); e != nil {
				return e
			}
			deleted = true
			out.Deleted++
			out.Bytes += info.Size()
			return nil
		})
		if err != nil {
			out.Failed++
			return out, err
		}
		if !deleted {
			out.Skipped++
		}
	}
	return out, nil
}
