package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

type repoEvent struct {
	Kind, Object, State, Body, Tab string
	Default                        bool
	Commits                        []string
	Message                        string
}

type eventBuilder func(*sql.Tx) ([]repoEvent, error)

func (m *Module) processEvents(ctx context.Context, k repoKey, delivery string, build eventBuilder) error {
	settings, err := m.notifySettings(ctx)
	if err != nil {
		return err
	}
	policy, _ := effectiveNotify(settings, k)
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_event_seen WHERE expires_at<=?`, m.now()); err != nil {
		return err
	}
	if delivery != "" {
		fresh, err := m.markEvent(ctx, tx, k, "@delivery", delivery, "")
		if err != nil {
			return err
		}
		if !fresh {
			return tx.Commit()
		}
	}
	events, err := build(tx)
	if err != nil {
		return err
	}
	sent := []notify.Stored{}
	for _, e := range events {
		if e.Kind == "push" {
			fresh := []string{}
			for _, sha := range e.Commits {
				added, err := m.markEvent(ctx, tx, k, e.Kind, sha, e.State)
				if err != nil {
					return err
				}
				if added {
					fresh = append(fresh, sha)
				}
			}
			if len(fresh) == 0 {
				continue
			}
			e.Object = fresh[0]
			e.Body = fmt.Sprintf("%d 个新提交，最新：%s", len(fresh), firstLine(e.Message))
		} else {
			fresh, err := m.markEvent(ctx, tx, k, e.Kind, e.Object, e.State)
			if err != nil {
				return err
			}
			if !fresh {
				continue
			}
		}
		if !slices.Contains(policy.Events, e.Kind) {
			continue
		}
		if strings.HasPrefix(e.Kind, "ci_") && policy.CiBranches == "default" && !e.Default {
			continue
		}
		var at time.Time
		var count, suppressed int
		err := tx.QueryRowContext(ctx, `SELECT started_at,sent,suppressed FROM github_notify_windows WHERE connection_id=? AND repo=? AND event=?`, k.ConnectionID, k.Repo, e.Kind).Scan(&at, &count, &suppressed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if at.IsZero() || m.now().Sub(at) >= time.Minute {
			if suppressed > 0 {
				stored, err := m.d.Notify.SaveTx(ctx, tx, m.summary(k, e.Kind, suppressed))
				if err != nil {
					return err
				}
				sent = append(sent, stored)
			}
			at, count, suppressed = m.now(), 0, 0
		}
		if count < 3 {
			n := notify.Notification{Kind: "github." + e.Kind, Title: k.Repo + " · " + notifyEvents[e.Kind], Body: e.Body, Link: repoLink(k, e.Tab), Source: "github", Data: map[string]any{"connectionId": k.ConnectionID, "repo": k.Repo, "object": e.Object, "state": e.State}}
			if e.Kind == "ci_failed" {
				n.Priority = notify.PriorityHigh
			}
			stored, err := m.d.Notify.SaveTx(ctx, tx, n)
			if err != nil {
				return err
			}
			sent = append(sent, stored)
			count++
		} else {
			suppressed++
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO github_notify_windows(connection_id,repo,event,started_at,sent,suppressed) VALUES(?,?,?,?,?,?) ON CONFLICT(connection_id,repo,event) DO UPDATE SET started_at=excluded.started_at,sent=excluded.sent,suppressed=excluded.suppressed`, k.ConnectionID, k.Repo, e.Kind, at, count, suppressed); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, n := range sent {
		m.d.Notify.Dispatch(ctx, n)
	}
	return nil
}

func (m *Module) markEvent(ctx context.Context, tx *sql.Tx, k repoKey, kind, object, state string) (bool, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO github_event_seen(connection_id,repo,event,object_id,state,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(connection_id,repo,event,object_id,state) DO NOTHING`, k.ConnectionID, k.Repo, kind, object, state, m.now().Add(24*time.Hour))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (m *Module) summary(k repoKey, kind string, n int) notify.Notification {
	tab := "pulls"
	if strings.HasPrefix(kind, "ci_") {
		tab = "runs"
	} else if kind == "push" {
		tab = "commits"
	} else if strings.HasPrefix(kind, "issue_") || kind == "release" {
		tab = "issues"
	}
	return notify.Notification{Kind: "github." + kind, Title: k.Repo + " · " + notifyEvents[kind], Body: fmt.Sprintf("还有 %d 条", n), Link: repoLink(k, tab), Source: "github", Data: map[string]any{"connectionId": k.ConnectionID, "repo": k.Repo, "summary": true, "count": n}}
}

func (m *Module) flushNotify(ctx context.Context) error {
	settings, err := m.notifySettings(ctx)
	if err != nil {
		return err
	}
	keys, err := m.selected(ctx, nil, nil)
	if err != nil && !errors.Is(err, httpx.ErrIntegrationMissing) {
		return err
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT connection_id,repo,event,suppressed FROM github_notify_windows WHERE started_at<=? AND suppressed>0`, m.now().Add(-time.Minute))
	if err != nil {
		return err
	}
	type pending struct {
		k     repoKey
		event string
		n     int
	}
	pendingRows := []pending{}
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.k.ConnectionID, &p.k.Repo, &p.event, &p.n); err != nil {
			rows.Close()
			return err
		}
		pendingRows = append(pendingRows, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	sent := []notify.Stored{}
	for _, p := range pendingRows {
		policy, _ := effectiveNotify(settings, p.k)
		if slices.Contains(keys, p.k) && slices.Contains(policy.Events, p.event) {
			n, err := m.d.Notify.SaveTx(ctx, tx, m.summary(p.k, p.event, p.n))
			if err != nil {
				return err
			}
			sent = append(sent, n)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE github_notify_windows SET suppressed=0 WHERE connection_id=? AND repo=? AND event=?`, p.k.ConnectionID, p.k.Repo, p.event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, n := range sent {
		m.d.Notify.Dispatch(ctx, n)
	}
	return nil
}

func eventSnapshot(ctx context.Context, tx *sql.Tx, k repoKey, resource string) (map[string]json.RawMessage, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT object_id,data FROM github_event_states WHERE connection_id=? AND repo=? AND resource=?`, k.ConnectionID, k.Repo, resource)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	ready := false
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, false, err
		}
		if id == "@initialized" {
			ready = true
		} else {
			out[id] = json.RawMessage(raw)
		}
	}
	return out, ready, rows.Err()
}

func recordSnapshot(ctx context.Context, tx *sql.Tx, k repoKey, resource string, objects []cacheObject) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_event_states WHERE connection_id=? AND repo=? AND resource=?`, k.ConnectionID, k.Repo, resource); err != nil {
		return err
	}
	for _, o := range append(objects, object("@initialized", true)) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO github_event_states(connection_id,repo,resource,object_id,data) VALUES(?,?,?,?,?)`, k.ConnectionID, k.Repo, resource, o.ID, string(o.Data)); err != nil {
			return err
		}
	}
	return nil
}
