package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

func runEvents(r cachedRun, old *cachedRun) []repoEvent {
	if old != nil && old.Status == r.Status && old.Conclusion == r.Conclusion && old.Attempt == r.Attempt {
		return nil
	}
	e := repoEvent{Object: fmt.Sprintf("%d:%d", r.Id, r.Attempt), State: r.Status + ":" + r.Conclusion, Body: r.Name + " · " + r.Branch, Default: r.DefaultBranch, Tab: "runs"}
	if r.HeadSha != nil {
		e.Body += " · " + *r.HeadSha
	}
	switch {
	case r.Status == "in_progress":
		e.Kind = "ci_started"
	case r.Status == "completed" && r.Conclusion == "success":
		e.Kind = "ci_succeeded"
	case r.Status == "completed" && runState(r.Conclusion) == "failure":
		e.Kind = "ci_failed"
	case r.Status == "completed" && r.Conclusion == "cancelled":
		e.Kind = "ci_cancelled"
	default:
		return nil
	}
	return []repoEvent{e}
}

type settledRun struct {
	State   string
	ID      int64
	Attempt int
	At      time.Time
}

func (m *Module) settleRun(ctx context.Context, tx *sql.Tx, k repoKey, r cachedRun, ready bool) ([]repoEvent, error) {
	state := runState(r.Conclusion)
	if !r.DefaultBranch || r.Status != "completed" || state == "" {
		return nil, nil
	}
	id := strconv.FormatInt(r.WorkflowID, 10)
	var raw string
	previous := settledRun{}
	err := tx.QueryRowContext(ctx, `SELECT data FROM github_event_states WHERE connection_id=? AND repo=? AND resource='settled' AND object_id=?`, k.ConnectionID, k.Repo, id).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &previous); err != nil {
			return nil, err
		}
	}
	if r.CreatedAt.Before(previous.At) || r.CreatedAt.Equal(previous.At) && (r.Id < previous.ID || r.Id == previous.ID && r.Attempt < previous.Attempt) {
		return nil, nil
	}
	events := []repoEvent{}
	if ready && previous.State == "failure" && state == "success" {
		events = append(events, repoEvent{Kind: "ci_recovered", Object: fmt.Sprintf("%d:%d", r.Id, r.Attempt), State: "success", Body: r.Name + " · " + r.Branch, Default: true, Tab: "runs"})
	}
	o := object(id, settledRun{state, r.Id, r.Attempt, r.CreatedAt})
	_, err = tx.ExecContext(ctx, `INSERT INTO github_event_states(connection_id,repo,resource,object_id,data) VALUES(?,?,'settled',?,?) ON CONFLICT(connection_id,repo,resource,object_id) DO UPDATE SET data=excluded.data`, k.ConnectionID, k.Repo, id, string(o.Data))
	return events, err
}

func loginIncluded(logins []string, login string) bool {
	for _, v := range logins {
		if strings.EqualFold(v, login) {
			return true
		}
	}
	return false
}

func (m *Module) compareObjects(ctx context.Context, k repoKey, resource string, objects []cacheObject) error {
	login := ""
	if resource == "issue" {
		cfg, err := m.accountConfig(ctx, k.ConnectionID)
		if err != nil {
			return err
		}
		login = cfg.Login
	}
	return m.processEvents(ctx, k, "", func(tx *sql.Tx) ([]repoEvent, error) {
		old, ready, err := eventSnapshot(ctx, tx, k, resource)
		if err != nil {
			return nil, err
		}
		events := []repoEvent{}
		latest := map[int64]cachedRun{}
		freshCommits := []string{}
		message := ""
		for _, o := range objects {
			prev, exists := old[o.ID]
			switch resource {
			case "pull":
				var p, previous cachedPull
				if err := json.Unmarshal(o.Data, &p); err != nil {
					return nil, err
				}
				if exists {
					if err := json.Unmarshal(prev, &previous); err != nil {
						return nil, err
					}
				}
				if !ready {
					continue
				}
				e := repoEvent{Object: o.ID, Body: p.Title, Tab: "pulls"}
				if !exists && p.State == "open" {
					e.Kind, e.State = "pr_opened", "open"
					events = append(events, e)
				}
				if p.State != previous.State && (p.State == "merged" || p.State == "closed") {
					e.Kind, e.State = "pr_closed", string(p.State)
					if p.State == "merged" {
						e.Kind = "pr_merged"
					}
					events = append(events, e)
				}
				if p.ReviewState != previous.ReviewState && (p.ReviewState == "approved" || p.ReviewState == "changes_requested") {
					e.Kind, e.State = "pr_review", string(p.ReviewState)
					events = append(events, e)
				}
			case "issue":
				var i, previous cachedIssue
				if err := json.Unmarshal(o.Data, &i); err != nil {
					return nil, err
				}
				if exists {
					if err := json.Unmarshal(prev, &previous); err != nil {
						return nil, err
					}
				}
				if !ready {
					continue
				}
				e := repoEvent{Object: o.ID, Body: i.Title, Tab: "issues"}
				if !exists && i.State == "open" {
					e.Kind, e.State = "issue_opened", "open"
					events = append(events, e)
				}
				if login != "" && loginIncluded(i.Assignees, login) && !loginIncluded(previous.Assignees, login) {
					e.Kind, e.State = "issue_assigned", strings.ToLower(login)
					events = append(events, e)
				}
			case "run":
				var r cachedRun
				if err := json.Unmarshal(o.Data, &r); err != nil {
					return nil, err
				}
				if ready {
					var previous *cachedRun
					if exists {
						previous = &cachedRun{}
						if err := json.Unmarshal(prev, previous); err != nil {
							return nil, err
						}
					}
					events = append(events, runEvents(r, previous)...)
				}
				if r.DefaultBranch && r.Status == "completed" && runState(r.Conclusion) != "" {
					p, ok := latest[r.WorkflowID]
					if !ok || r.CreatedAt.After(p.CreatedAt) || r.CreatedAt.Equal(p.CreatedAt) && (r.Id > p.Id || r.Id == p.Id && r.Attempt > p.Attempt) {
						latest[r.WorkflowID] = r
					}
				}
			case "release":
				var release remoteRelease
				if err := json.Unmarshal(o.Data, &release); err != nil {
					return nil, err
				}
				if ready && !exists {
					events = append(events, repoEvent{Kind: "release", Object: release.Tag, State: "published", Body: release.Tag + " · " + release.Name, Tab: "issues"})
				}
			case "commit":
				if ready && !exists {
					var c api.GitHubCommit
					if err := json.Unmarshal(o.Data, &c); err != nil {
						return nil, err
					}
					if len(freshCommits) == 0 {
						message = c.Message
					}
					freshCommits = append(freshCommits, o.ID)
				}
			}
		}
		if len(freshCommits) > 0 {
			events = append(events, repoEvent{Kind: "push", State: "default", Commits: freshCommits, Message: message, Default: true, Tab: "commits"})
		}
		for _, r := range latest {
			es, err := m.settleRun(ctx, tx, k, r, ready)
			if err != nil {
				return nil, err
			}
			events = append(events, es...)
		}
		if err := recordSnapshot(ctx, tx, k, resource, objects); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM github_cache_v2 WHERE connection_id=? AND repo=? AND kind=?`, k.ConnectionID, k.Repo, resource); err != nil {
			return nil, err
		}
		for _, o := range objects {
			if _, err := tx.ExecContext(ctx, `INSERT INTO github_cache_v2(connection_id,repo,kind,object_id,data,updated_at) VALUES(?,?,?,?,?,?)`, k.ConnectionID, k.Repo, resource, o.ID, string(o.Data), m.now()); err != nil {
				return nil, err
			}
		}
		return events, nil
	})
}
