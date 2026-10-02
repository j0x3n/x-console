package maintenance

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type Group struct {
	Kind     string                  `json:"kind"`
	Count    int64                   `json:"count"`
	Bytes    int64                   `json:"bytes"`
	Selected bool                    `json:"selected"`
	Items    []contracts.CleanupItem `json:"items"`
}

type Job struct {
	ID         string                  `json:"id"`
	ScanID     string                  `json:"scanId,omitempty"`
	State      string                  `json:"state"`
	StartedAt  *time.Time              `json:"startedAt,omitempty"`
	FinishedAt *time.Time              `json:"finishedAt,omitempty"`
	Groups     []Group                 `json:"groups"`
	Total      int64                   `json:"total"`
	Done       int64                   `json:"done"`
	Result     contracts.CleanupResult `json:"result"`
	Error      string                  `json:"error,omitempty"`
}

func jobID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }

func (m *Module) visible(ctx context.Context, item contracts.CleanupItem) bool {
	if m.hidden(ctx, item.Module) {
		return false
	}
	if auth.VaultUnlocked(ctx) {
		return true
	}
	if item.Hidden {
		return false
	}
	if item.RecordTable == "" || m.d.DB == nil {
		return true
	}
	var hidden int64
	var query string
	switch item.RecordTable {
	case "note_attachments":
		query = "SELECT n.hidden FROM note_attachments a JOIN notes n ON n.id=a.note_id WHERE a.id=?"
	case "note_shares":
		query = "SELECT hidden FROM notes WHERE id=?"
	case "drive_items":
		query = `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT COALESCE(max(hidden),0) FROM drive_items WHERE id IN(SELECT id FROM subtree)`
	case "drive_file_versions":
		query = "SELECT i.hidden FROM drive_file_versions v JOIN drive_items i ON i.id=v.item_id WHERE v.id=?"
	case "drive_shares":
		query = "SELECT i.hidden FROM drive_shares s JOIN drive_items i ON i.id=s.item_id WHERE s.id=?"
	default:
		return true
	}
	err := m.d.DB.QueryRowContext(ctx, query, item.RecordID).Scan(&hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	return err == nil && hidden == 0
}

func (m *Module) job(ctx context.Context, cleanup bool) Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cleanup {
		out := m.cleanup
		for _, items := range m.snapshot {
			for _, item := range items {
				if !m.visible(ctx, item) {
					out.Total = 0
					out.Done = 0
					out.Result = contracts.CleanupResult{}
					return out
				}
			}
		}
		return out
	}
	out := m.scan
	out.Groups = []Group{}
	out.Total = 0
	byKind := map[string]*Group{}
	for _, items := range m.snapshot {
		for _, item := range items {
			if !m.visible(ctx, item) {
				continue
			}
			g := byKind[item.Kind]
			if g == nil {
				g = &Group{Kind: item.Kind, Selected: item.Kind != "missing_records" && item.Kind != "audit_logs", Items: []contracts.CleanupItem{}}
				byKind[item.Kind] = g
			}
			g.Count++
			g.Bytes += item.Bytes
			out.Total++
			if len(g.Items) < 50 {
				g.Items = append(g.Items, item)
			}
		}
	}
	for _, g := range byKind {
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Kind < out.Groups[j].Kind })
	return out
}

func (m *Module) publish(cleanup bool) {
	m.mu.Lock()
	j := m.scan
	kind := "scan"
	if cleanup {
		j = m.cleanup
		kind = "cleanup"
	}
	m.mu.Unlock()
	if m.d.Bus != nil {
		m.d.Bus.Publish("maintenance.job", map[string]string{"id": j.ID, "kind": kind, "state": j.State})
	}
}

func (m *Module) scanHandler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	m.busy = true
	at := time.Now().UTC()
	m.scan = Job{ID: jobID(), State: "running", StartedAt: &at, Groups: []Group{}}
	m.snapshot = map[string][]contracts.CleanupItem{}
	m.consumed = map[string]bool{}
	ctx := m.ctx
	m.mu.Unlock()
	go m.runScan(ctx)
	httpx.JSON(w, http.StatusAccepted, m.job(r.Context(), false))
}

func (m *Module) runScan(ctx context.Context) {
	all := module.All[contracts.Cleaner](m.d.Registry)
	keys := make([]string, 0, len(all))
	for key := range all {
		if strings.HasPrefix(key, contracts.MaintenanceCleanerPrefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var failed error
	for _, key := range keys {
		items, err := all[key].Scan(ctx)
		if err != nil {
			failed = errors.Join(failed, err)
			continue
		}
		m.mu.Lock()
		m.snapshot[key] = append([]contracts.CleanupItem(nil), items...)
		m.mu.Unlock()
		m.publish(false)
	}
	m.mu.Lock()
	at := time.Now().UTC()
	m.scan.FinishedAt = &at
	m.scan.State = "done"
	m.busy = false
	if failed != nil {
		m.scan.State = "failed"
		m.scan.Error = "部分扫描失败，请重试"
	}
	m.mu.Unlock()
	if failed != nil && m.d.Log != nil {
		m.d.Log.WarnContext(ctx, "maintenance scan failed", "error", failed)
	}
	m.publish(false)
}

func (m *Module) cleanupHandler(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body struct {
		Kinds  []string `json:"kinds"`
		ScanID string   `json:"scanId"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(body.Kinds) == 0 {
		httpx.Fail(w, r, httpx.Invalid("请选择清理类型"))
		return
	}
	selected := map[string]bool{}
	for _, k := range body.Kinds {
		selected[k] = true
	}
	m.mu.Lock()
	if m.busy || m.scan.State != "done" || (body.ScanID != "" && body.ScanID != m.scan.ID) {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	ids := map[string][]string{}
	found := map[string]bool{}
	var total int64
	for key, items := range m.snapshot {
		for _, item := range items {
			if selected[item.Kind] && m.visible(r.Context(), item) {
				found[item.Kind] = true
			}
			if selected[item.Kind] && m.visible(r.Context(), item) && !m.consumed[key+"\x00"+item.ID] {
				ids[key] = append(ids[key], item.ID)
				total++
			}
		}
	}
	for k := range selected {
		if !found[k] {
			m.mu.Unlock()
			httpx.Fail(w, r, httpx.Invalid("清理类型不在本次扫描结果中"))
			return
		}
	}
	if total == 0 {
		m.mu.Unlock()
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	m.busy = true
	at := time.Now().UTC()
	m.cleanup = Job{ID: jobID(), ScanID: m.scan.ID, State: "running", StartedAt: &at, Groups: []Group{}, Total: total}
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	stop := context.AfterFunc(m.ctx, cancel)
	m.mu.Unlock()
	go func() { defer cancel(); defer stop(); m.runCleanup(ctx, ids) }()
	httpx.JSON(w, http.StatusAccepted, m.job(r.Context(), true))
}

func (m *Module) runCleanup(ctx context.Context, ids map[string][]string) {
	fn := func(ctx context.Context) error {
		keys := make([]string, 0, len(ids))
		for key := range ids {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var failures error
		for _, key := range keys {
			byID := map[string]contracts.CleanupItem{}
			m.mu.Lock()
			for _, item := range m.snapshot[key] {
				byID[item.ID] = item
			}
			m.mu.Unlock()
			cleaner, ok := module.Lookup[contracts.Cleaner](m.d.Registry, key)
			if !ok {
				failures = errors.Join(failures, errors.New("cleaner unavailable"))
				continue
			}
			for _, id := range ids[key] {
				if m.d.Auth != nil {
					ctx = m.d.Auth.FreshVault(ctx)
				}
				if err := ctx.Err(); err != nil {
					return errors.Join(failures, err)
				}
				item, found := byID[id]
				allowed := !found || m.visible(ctx, item)
				result := contracts.CleanupResult{Skipped: 1}
				var err error
				if allowed {
					result, err = cleaner.Clean(ctx, []string{id})
				}
				failures = errors.Join(failures, err)
				m.mu.Lock()
				m.cleanup.Done++
				m.cleanup.Result.Deleted += result.Deleted
				m.cleanup.Result.Bytes += result.Bytes
				m.cleanup.Result.Skipped += result.Skipped
				m.cleanup.Result.Failed += result.Failed
				if err != nil && result.Failed == 0 {
					m.cleanup.Result.Failed++
				}
				if err == nil {
					m.consumed[key+"\x00"+id] = true
				}
				m.mu.Unlock()
				m.publish(true)
			}
		}
		return failures
	}
	var err error
	if storage, ok := module.Lookup[contracts.MaintenanceStorage](m.d.Registry, contracts.MaintenanceStorageKey); ok {
		err = storage.WithCleanup(ctx, fn)
	} else {
		err = fn(ctx)
	}
	m.mu.Lock()
	at := time.Now().UTC()
	m.cleanup.FinishedAt = &at
	m.cleanup.State = "done"
	if err != nil {
		m.cleanup.State = "failed"
		m.cleanup.Error = "部分清理失败，请重新扫描后重试"
	}
	result := m.cleanup.Result
	id := m.cleanup.ID
	m.usageAt = time.Time{}
	m.mu.Unlock()
	if m.d.Audit != nil {
		m.d.Audit.Record(context.WithoutCancel(ctx), "maintenance.cleanup", id, map[string]any{"deleted": result.Deleted, "bytes": result.Bytes, "skipped": result.Skipped, "failed": result.Failed}, err)
	}
	m.mu.Lock()
	m.busy = false
	m.mu.Unlock()
	m.publish(true)
}
