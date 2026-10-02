package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type ArtifactCleaner struct{ Deps *module.Deps }

var artifactKey = regexp.MustCompile(`^artifacts/([0-9]+)/[^/]+$`)

func (c ArtifactCleaner) references(ctx context.Context, q queryer) (map[string]bool, map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, "SELECT id,artifacts,build_status FROM coding_tasks")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	refs := map[string]bool{}
	running := map[int64]bool{}
	for rows.Next() {
		var id int64
		var raw, status string
		if err = rows.Scan(&id, &raw, &status); err != nil {
			return nil, nil, err
		}
		if status == "running" {
			running[id] = true
		}
		var artifacts []struct {
			Key string `json:"key"`
		}
		if err = json.Unmarshal([]byte(raw), &artifacts); err != nil {
			return nil, nil, err
		}
		for _, a := range artifacts {
			refs[a.Key] = true
		}
	}
	return refs, running, rows.Err()
}

func (c ArtifactCleaner) Scan(ctx context.Context) ([]contracts.CleanupItem, error) {
	refs, running, err := c.references(ctx, c.Deps.DB)
	if err != nil {
		return nil, err
	}
	out := []contracts.CleanupItem{}
	for info, err := range c.Deps.Files.For("coding").List(ctx, "artifacts") {
		if err != nil {
			return nil, err
		}
		match := artifactKey.FindStringSubmatch(info.Key)
		if match == nil || refs[info.Key] || !info.ModTime.Before(time.Now().Add(-24*time.Hour)) {
			continue
		}
		id, e := strconv.ParseInt(match[1], 10, 64)
		if e != nil || running[id] {
			continue
		}
		out = append(out, contracts.CleanupItem{ID: info.Key, Kind: "agent_artifacts", Module: "coding", Name: info.Key, Bytes: info.Size, Reason: "超过一天且全部任务产物列表均未引用"})
	}
	return out, nil
}

func (c ArtifactCleaner) Clean(ctx context.Context, ids []string) (contracts.CleanupResult, error) {
	var out contracts.CleanupResult
	for _, key := range ids {
		match := artifactKey.FindStringSubmatch(key)
		if match == nil {
			return out, errors.New("invalid artifact key")
		}
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return out, err
		}
		tx, err := c.Deps.DB.BeginTx(ctx, nil)
		if err != nil {
			return out, err
		}
		result, e := c.cleanOne(ctx, tx, key, id)
		if e == nil {
			e = tx.Commit()
		}
		_ = tx.Rollback()
		out.Deleted += result.Deleted
		out.Bytes += result.Bytes
		out.Skipped += result.Skipped
		if e != nil {
			out.Failed++
			return out, e
		}
	}
	return out, nil
}

func (c ArtifactCleaner) cleanOne(ctx context.Context, tx *sql.Tx, key string, id int64) (contracts.CleanupResult, error) {
	out := contracts.CleanupResult{Skipped: 1}
	if h, ok := module.Lookup[contracts.HiddenModules](c.Deps.Registry, contracts.HiddenModulesKey); ok && h.Hidden(ctx, "coding") {
		return out, nil
	}
	refs, running, err := c.references(ctx, tx)
	if err != nil {
		return out, err
	}
	if refs[key] || running[id] {
		return out, nil
	}
	info, err := StoredStat(ctx, c.Deps, "coding", key)
	if errors.Is(err, files.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if !info.ModTime.Before(time.Now().Add(-24 * time.Hour)) {
		return out, nil
	}
	bytes, err := DeleteObject(ctx, c.Deps, "coding", key)
	if err != nil {
		return out, err
	}
	return contracts.CleanupResult{Deleted: 1, Bytes: bytes}, nil
}
