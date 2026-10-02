package maintenance

import (
	"context"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

const driveSubtree = `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) `

func driveHashes(ctx context.Context, q queryer, id int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, driveSubtree+`SELECT sha256 FROM drive_items WHERE id IN(SELECT id FROM subtree) AND is_dir=0 UNION SELECT sha256 FROM drive_file_versions WHERE item_id IN(SELECT id FROM subtree)`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			return nil, err
		}
		if len(hash) == 64 {
			out = append(out, hash)
		}
	}
	sort.Strings(out)
	return slices.Compact(out), rows.Err()
}

func PurgeDriveTrash(ctx context.Context, d *module.Deps, id int64, cutoff time.Time, snapshot []int64, lock func(string) func()) (contracts.CleanupResult, error) {
	out := contracts.CleanupResult{Skipped: 1}
	hashes, err := driveHashes(ctx, d.DB, id)
	if err != nil {
		return out, err
	}
	releases := []func(){}
	for _, hash := range hashes {
		releases = append(releases, lock(hash))
	}
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, driveSubtree+`SELECT id,trashed_at,hidden,s3_key FROM drive_items WHERE id IN(SELECT id FROM subtree) ORDER BY id`, id)
	if err != nil {
		return out, err
	}
	ids := []int64{}
	keys := []string{}
	safe := true
	for rows.Next() {
		var current, hidden int64
		var at *time.Time
		var key *string
		if err = rows.Scan(&current, &at, &hidden, &key); err != nil {
			break
		}
		ids = append(ids, current)
		if at == nil || !at.Before(cutoff) || (hidden != 0 && !auth.VaultUnlocked(ctx)) {
			safe = false
		}
		if key != nil {
			keys = append(keys, *key)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if !safe || len(ids) == 0 {
		return out, nil
	}
	if snapshot != nil && !slices.Equal(ids, snapshot) {
		return out, nil
	}
	currentHashes, err := driveHashes(ctx, tx, id)
	if err != nil {
		return out, err
	}
	if !slices.Equal(currentHashes, hashes) {
		return out, nil
	}
	for _, key := range keys {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO drive_s3_deletions(key,created_at) VALUES(?,?)", key, time.Now().UTC()); err != nil {
			return out, err
		}
	}
	if _, err = tx.ExecContext(ctx, driveSubtree+"DELETE FROM drive_items WHERE id IN(SELECT id FROM subtree)", id); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	out = contracts.CleanupResult{Deleted: int64(len(ids))}
	var failures error
	for _, hash := range hashes {
		var refs int64
		err = d.DB.QueryRowContext(ctx, "SELECT(SELECT count(*) FROM drive_items WHERE sha256=? AND is_dir=0)+(SELECT count(*) FROM drive_file_versions WHERE sha256=?)", hash, hash).Scan(&refs)
		if err != nil {
			failures = errors.Join(failures, err)
			out.Failed++
			continue
		}
		if refs > 0 {
			continue
		}
		for _, key := range []string{driveKey(hash), "thumbnails/" + hash + ".jpg"} {
			bytes, e := DeleteObject(ctx, d, "drive", key)
			out.Bytes += bytes
			if e != nil {
				out.Failed++
				failures = errors.Join(failures, e)
			}
		}
	}
	return out, failures
}
