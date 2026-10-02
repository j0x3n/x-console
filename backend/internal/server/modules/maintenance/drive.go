package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type DriveCleaner struct {
	Deps  *module.Deps
	Lock  func(string) func()
	Trash func(context.Context, int64, time.Time, []int64) (contracts.CleanupResult, error)
}

type driveCandidate struct {
	Type, Key, Hash, Token string
	ID                     int64
	Hidden                 bool
	Size                   int64
	Subtree                []int64
}

var driveBlob = regexp.MustCompile(`^blobs/([a-f0-9]{2})/([a-f0-9]{64})$`)

func driveKey(hash string) string { return "blobs/" + hash[:2] + "/" + hash }

func (c DriveCleaner) refs(ctx context.Context, q queryer, hash string) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM drive_items WHERE sha256=? AND is_dir=0)+(SELECT count(*) FROM drive_file_versions WHERE sha256=?)", hash, hash).Scan(&n)
	return n, err
}

func (c DriveCleaner) Scan(ctx context.Context) ([]contracts.CleanupItem, error) {
	out := []contracts.CleanupItem{}
	add := func(v driveCandidate, kind, name, reason string) {
		table := v.Type
		if table == "trash" {
			table = "drive_items"
		}
		if table == "share" {
			table = "drive_shares"
		}
		if table == "blob" {
			table = ""
		}
		out = append(out, contracts.CleanupItem{ID: candidateID(v), Kind: kind, Module: "drive", Hidden: v.Hidden, RecordTable: table, RecordID: v.ID, Name: name, Bytes: v.Size, Reason: reason})
	}
	for info, err := range c.Deps.Files.For("drive").List(ctx, "blobs") {
		if err != nil {
			return nil, err
		}
		match := driveBlob.FindStringSubmatch(info.Key)
		if match == nil || match[1] != match[2][:2] || !info.ModTime.Before(time.Now().Add(-24*time.Hour)) {
			continue
		}
		n, e := c.refs(ctx, c.Deps.DB, match[2])
		if e != nil {
			return nil, e
		}
		if n == 0 {
			add(driveCandidate{Type: "blob", Key: info.Key, Hash: match[2], Size: info.Size}, "drive_orphans", info.Key, "超过一天且条目和历史版本均未引用")
		}
	}
	var rows *sql.Rows
	var err error
	type record struct {
		id         int64
		name, hash string
		hidden     int64
		size       int64
	}
	records := []record{}
	for _, table := range []string{"drive_items", "drive_file_versions"} {
		query := "SELECT id,name,sha256,hidden,size FROM drive_items WHERE is_dir=0"
		if table == "drive_file_versions" {
			query = "SELECT v.id,i.name,v.sha256,i.hidden,v.size FROM drive_file_versions v JOIN drive_items i ON i.id=v.item_id"
		}
		rows, err = c.Deps.DB.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		records = records[:0]
		for rows.Next() {
			var v record
			if err = rows.Scan(&v.id, &v.name, &v.hash, &v.hidden, &v.size); err != nil {
				break
			}
			records = append(records, v)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
		for _, v := range records {
			if len(v.hash) != 64 {
				continue
			}
			_, e := StoredStat(ctx, c.Deps, "drive", driveKey(v.hash))
			if e != nil && !errors.Is(e, files.ErrNotFound) {
				return nil, e
			}
			if errors.Is(e, files.ErrNotFound) {
				add(driveCandidate{Type: table, ID: v.id, Hash: v.hash, Hidden: v.hidden != 0}, "missing_records", v.name, "存储中缺少原文件")
			}
		}
	}
	rows, err = c.Deps.DB.QueryContext(ctx, "SELECT id,name,hidden FROM drive_items WHERE trashed_at<? AND (parent_id IS NULL OR parent_id NOT IN (SELECT id FROM drive_items WHERE trashed_at IS NOT NULL))", time.Now().Add(-30*24*time.Hour))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, h int64
		var name string
		if err = rows.Scan(&id, &name, &h); err != nil {
			break
		}
		add(driveCandidate{Type: "trash", ID: id, Hidden: h != 0}, "drive_trash", name, "回收站条目超过三十天")
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Kind != "drive_trash" {
			continue
		}
		var v driveCandidate
		if err = decodeCandidate(out[i].ID, &v); err != nil {
			return nil, err
		}
		subrows, e := c.Deps.DB.QueryContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT id,hidden,is_dir,size FROM drive_items WHERE id IN(SELECT id FROM subtree) ORDER BY id`, v.ID)
		if e != nil {
			return nil, e
		}
		for subrows.Next() {
			var id, h, dir, size int64
			if e = subrows.Scan(&id, &h, &dir, &size); e != nil {
				break
			}
			v.Subtree = append(v.Subtree, id)
			v.Hidden = v.Hidden || h != 0
			if dir == 0 {
				v.Size += size
			}
		}
		if e == nil {
			e = subrows.Err()
		}
		subrows.Close()
		if e != nil {
			return nil, e
		}
		out[i].ID = candidateID(v)
		out[i].Hidden = v.Hidden
		out[i].Bytes = v.Size
	}
	rows, err = c.Deps.DB.QueryContext(ctx, "SELECT s.id,s.item_id,i.hidden,s.token FROM drive_shares s JOIN drive_items i ON i.id=s.item_id WHERE s.expires_at<? OR (s.max_downloads IS NOT NULL AND s.downloads>=s.max_downloads)", time.Now().UTC())
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, owner, h int64
		var token string
		if err = rows.Scan(&id, &owner, &h, &token); err != nil {
			break
		}
		add(driveCandidate{Type: "share", ID: id, Hidden: h != 0, Token: token}, "expired_shares", "云盘分享 #"+strconv.FormatInt(id, 10), "分享已过期或下载次数用完")
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	return out, err
}

func (c DriveCleaner) Clean(ctx context.Context, ids []string) (contracts.CleanupResult, error) {
	var out contracts.CleanupResult
	for _, id := range ids {
		var v driveCandidate
		if err := decodeCandidate(id, &v); err != nil {
			return out, err
		}
		if v.Hidden && !auth.VaultUnlocked(ctx) {
			out.Skipped++
			continue
		}
		if h, ok := module.Lookup[contracts.HiddenModules](c.Deps.Registry, contracts.HiddenModulesKey); ok && h.Hidden(ctx, "drive") {
			out.Skipped++
			continue
		}
		if v.Type == "trash" {
			result, err := c.Trash(ctx, v.ID, time.Now().Add(-30*24*time.Hour), v.Subtree)
			out.Deleted += result.Deleted
			out.Bytes += result.Bytes
			out.Skipped += result.Skipped
			out.Failed += result.Failed
			if err != nil {
				return out, err
			}
			continue
		}
		result, err := c.cleanOne(ctx, v)
		out.Deleted += result.Deleted
		out.Bytes += result.Bytes
		out.Skipped += result.Skipped
		out.Failed += result.Failed
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c DriveCleaner) cleanOne(ctx context.Context, v driveCandidate) (contracts.CleanupResult, error) {
	out := contracts.CleanupResult{Skipped: 1}
	if v.Type == "share" {
		res, err := c.Deps.DB.ExecContext(ctx, "DELETE FROM drive_shares WHERE id=? AND (expires_at<? OR (max_downloads IS NOT NULL AND downloads>=max_downloads)) AND item_id IN (SELECT id FROM drive_items WHERE hidden=0 OR ?=1) AND token=?", v.ID, time.Now().UTC(), auth.VaultUnlocked(ctx), v.Token)
		if err != nil {
			return out, err
		}
		n, err := res.RowsAffected()
		if n > 0 {
			out = contracts.CleanupResult{Deleted: n}
		}
		return out, err
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(v.Hash) {
		return out, errors.New("invalid blob hash")
	}
	release := c.Lock(v.Hash)
	defer release()
	tx, err := c.Deps.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	info, err := StoredStat(ctx, c.Deps, "drive", driveKey(v.Hash))
	if err != nil && !errors.Is(err, files.ErrNotFound) {
		return out, err
	}
	if v.Type == "blob" {
		if v.Key != driveKey(v.Hash) {
			return out, errors.New("invalid blob key")
		}
		if err != nil || !info.ModTime.Before(time.Now().Add(-24*time.Hour)) {
			return out, nil
		}
		n, e := c.refs(ctx, tx, v.Hash)
		if e != nil {
			return out, e
		}
		if n != 0 {
			return out, nil
		}
		freed, e := DeleteObject(ctx, c.Deps, "drive", v.Key)
		if e != nil {
			return out, e
		}
		thumb, e := DeleteObject(ctx, c.Deps, "drive", "thumbnails/"+v.Hash+".jpg")
		if e != nil {
			return contracts.CleanupResult{Deleted: 1, Bytes: freed, Failed: 1}, e
		}
		return contracts.CleanupResult{Deleted: 1, Bytes: freed + thumb}, tx.Commit()
	}
	if !errors.Is(err, files.ErrNotFound) {
		return out, nil
	}
	var res sql.Result
	switch v.Type {
	case "drive_items":
		var versions int64
		if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM drive_file_versions WHERE item_id=?", v.ID).Scan(&versions); e != nil {
			return out, e
		}
		if versions > 0 {
			return out, nil
		}
		var key *string
		err = tx.QueryRowContext(ctx, "SELECT s3_key FROM drive_items WHERE id=? AND is_dir=0 AND sha256=? AND (hidden=0 OR ?=1)", v.ID, v.Hash, auth.VaultUnlocked(ctx)).Scan(&key)
		if errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if key != nil {
			if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO drive_s3_deletions(key,created_at) VALUES(?,?)", *key, time.Now().UTC()); err != nil {
				return out, err
			}
		}
		res, err = tx.ExecContext(ctx, "DELETE FROM drive_items WHERE id=? AND is_dir=0 AND sha256=?", v.ID, v.Hash)
	case "drive_file_versions":
		res, err = tx.ExecContext(ctx, "DELETE FROM drive_file_versions WHERE id=? AND sha256=? AND item_id IN(SELECT id FROM drive_items WHERE hidden=0 OR ?=1)", v.ID, v.Hash, auth.VaultUnlocked(ctx))
	default:
		return out, errors.New("invalid drive candidate " + strings.TrimSpace(v.Type))
	}
	if err != nil {
		return out, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return out, err
	}
	err = tx.Commit()
	if n > 0 {
		out = contracts.CleanupResult{Deleted: n}
	}
	return out, err
}
