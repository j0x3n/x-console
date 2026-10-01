package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

type repoKey struct {
	ConnectionID int64
	Repo         string
}
type cachedPull struct {
	api.GitHubPull
	HeadSHA string `json:"headSha"`
}
type cachedRun struct {
	api.GitHubRun
	WorkflowID int64 `json:"workflowId"`
	Attempt    int   `json:"attempt"`
	Synthetic  bool  `json:"synthetic"`
}
type cachedIssue struct {
	api.GitHubIssue
	State string `json:"state"`
}
type cacheObject struct {
	ID   string
	Data json.RawMessage
}

func object(id string, v any) cacheObject {
	raw, _ := json.Marshal(v)
	return cacheObject{ID: id, Data: raw}
}
func (m *Module) cached(ctx context.Context, k repoKey, kind string) ([]cacheObject, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT object_id,data FROM github_cache_v2 WHERE connection_id=? AND repo=? AND kind=? ORDER BY object_id`, k.ConnectionID, k.Repo, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cacheObject{}
	for rows.Next() {
		var o cacheObject
		var raw string
		if err := rows.Scan(&o.ID, &raw); err != nil {
			return nil, err
		}
		o.Data = json.RawMessage(raw)
		out = append(out, o)
	}
	return out, rows.Err()
}
func (m *Module) cachedOne(ctx context.Context, k repoKey, kind, id string, v any) error {
	var raw string
	err := m.d.DB.QueryRowContext(ctx, `SELECT data FROM github_cache_v2 WHERE connection_id=? AND repo=? AND kind=? AND object_id=?`, k.ConnectionID, k.Repo, kind, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}
func (m *Module) putObject(ctx context.Context, k repoKey, kind string, o cacheObject) error {
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO github_cache_v2(connection_id,repo,kind,object_id,data,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(connection_id,repo,kind,object_id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at`, k.ConnectionID, k.Repo, kind, o.ID, string(o.Data), m.now())
	return err
}
func (m *Module) replaceObjects(ctx context.Context, k repoKey, kind string, objects []cacheObject) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_cache_v2 WHERE connection_id=? AND repo=? AND kind=?`, k.ConnectionID, k.Repo, kind); err != nil {
		return err
	}
	for _, o := range objects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO github_cache_v2(connection_id,repo,kind,object_id,data,updated_at) VALUES(?,?,?,?,?,?)`, k.ConnectionID, k.Repo, kind, o.ID, string(o.Data), m.now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (m *Module) selected(ctx context.Context, connection *int64, repo *string) ([]repoKey, error) {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.configured() && !cfg.HasWatches {
		return nil, httpx.ErrIntegrationMissing
	}
	out := []repoKey{}
	for _, w := range cfg.Watches {
		if connection != nil && w.ConnectionId != *connection {
			continue
		}
		if repo != nil && *repo != "" && !strings.EqualFold(w.Repo, *repo) {
			continue
		}
		out = append(out, repoKey{w.ConnectionId, w.Repo})
	}
	return out, nil
}
func (m *Module) resolveRepo(ctx context.Context, connection *int64, repo string) (repoKey, error) {
	keys, err := m.selected(ctx, connection, &repo)
	if err != nil {
		return repoKey{}, err
	}
	if len(keys) == 0 {
		return repoKey{}, httpx.ErrNotFound
	}
	if len(keys) > 1 {
		return repoKey{}, httpx.Invalid("这个仓库属于多个账号，请指定 connectionId")
	}
	return keys[0], nil
}
func (m *Module) migrateB70(ctx context.Context) error {
	m.migrationMu.Lock()
	defer m.migrationMu.Unlock()
	var done bool
	if err := m.d.Settings.Get(ctx, keyMigratedB70, &done); err == nil && done {
		return nil
	} else if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	cfg, err := m.loadLegacyConfig(ctx)
	if err != nil {
		return err
	}
	has, err := m.d.Settings.Has(ctx, keyWatches)
	if err != nil {
		return err
	}
	if !has && cfg.Token == "" && cfg.ConnectionID == 0 && len(cfg.Repos) == 0 {
		return nil
	}
	if !has {
		watches := []api.RepoWatch{}
		for _, r := range cfg.Repos {
			watches = append(watches, api.RepoWatch{ConnectionId: cfg.ConnectionID, Repo: r})
		}
		if err := m.d.Settings.Set(ctx, keyWatches, watches); err != nil {
			return err
		}
	}
	pulls, err := m.q.ListPulls(ctx)
	if err != nil {
		return err
	}
	for _, p := range pulls {
		v := cachedPull{GitHubPull: pullToAPI(p, nil), HeadSHA: p.HeadSha}
		v.ConnectionId = ptr(cfg.ConnectionID)
		v.Forge = ptr(api.Github)
		if err := m.copyLegacy(ctx, repoKey{cfg.ConnectionID, p.Repo}, "pull", object(strconv.FormatInt(p.Number, 10), v)); err != nil {
			return err
		}
	}
	runs, err := m.q.ListRuns(ctx, 2147483647)
	if err != nil {
		return err
	}
	for _, r := range runs {
		v := cachedRun{GitHubRun: api.GitHubRun{Id: r.ID, Repo: r.Repo, ConnectionId: ptr(cfg.ConnectionID), Forge: ptr(api.Github), Name: r.Name, Branch: r.Branch, Event: r.Event, Status: r.Status, Conclusion: r.Conclusion, Url: r.Url, HeadSha: ptr(r.HeadSha), DefaultBranch: r.DefaultBranch, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, WorkflowID: r.WorkflowID, Attempt: 1}
		if err := m.copyLegacy(ctx, repoKey{cfg.ConnectionID, r.Repo}, "run", object(strconv.FormatInt(r.ID, 10), v)); err != nil {
			return err
		}
	}
	issues, err := m.q.ListIssues(ctx)
	if err != nil {
		return err
	}
	for _, r := range issues {
		v := cachedIssue{GitHubIssue: issueToAPI(r), State: "open"}
		v.ConnectionId = ptr(cfg.ConnectionID)
		v.Forge = ptr(api.Github)
		if err := m.copyLegacy(ctx, repoKey{cfg.ConnectionID, r.Repo}, "issue", object(strconv.FormatInt(r.Number, 10), v)); err != nil {
			return err
		}
	}
	links, err := m.q.ListLinks(ctx)
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := m.copyLegacy(ctx, repoKey{cfg.ConnectionID, l.Repo}, "link", object(fmt.Sprintf("%d:%s:%s", l.Number, l.Kind, l.Ref), l)); err != nil {
			return err
		}
	}
	rows, err := m.d.DB.QueryContext(ctx, `SELECT key,state,updated_at FROM github_ci_states`)
	if err != nil {
		return err
	}
	type oldState struct {
		key, state string
		at         time.Time
	}
	states := []oldState{}
	for rows.Next() {
		var s oldState
		if err := rows.Scan(&s.key, &s.state, &s.at); err != nil {
			rows.Close()
			return err
		}
		states = append(states, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, s := range states {
		prefix, rest, ok := strings.Cut(s.key, ":")
		if !ok {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s", prefix, cfg.ConnectionID, rest)
		if err := m.q.SetCIState(ctx, db.SetCIStateParams{Key: key, State: s.state, UpdatedAt: s.at}); err != nil {
			return err
		}
	}
	return m.d.Settings.Set(ctx, keyMigratedB70, true)
}
func (m *Module) copyLegacy(ctx context.Context, k repoKey, kind string, o cacheObject) error {
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO github_cache_v2(connection_id,repo,kind,object_id,data,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(connection_id,repo,kind,object_id) DO NOTHING`, k.ConnectionID, k.Repo, kind, o.ID, string(o.Data), m.now())
	return err
}
func issueToAPI(r db.GithubIssue) api.GitHubIssue {
	v := api.GitHubIssue{Repo: r.Repo, Number: int(r.Number), Title: r.Title, Url: r.Url, Author: r.Author, Assignees: []string{}, Labels: []string{}, Relation: api.GitHubIssueRelation(r.Relation), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	_ = json.Unmarshal([]byte(r.Assignees), &v.Assignees)
	_ = json.Unmarshal([]byte(r.Labels), &v.Labels)
	return v
}
func ptr[T any](v T) *T         { return &v }
func firstLine(s string) string { line, _, _ := strings.Cut(s, "\n"); return strings.TrimSpace(line) }
func utc(t time.Time) time.Time { return t.UTC() }
