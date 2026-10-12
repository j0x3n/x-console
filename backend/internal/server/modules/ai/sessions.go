package ai

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
)

func (m *Module) ListAiSessions(w http.ResponseWriter, r *http.Request, params api.ListAiSessionsParams) {
	ctx := r.Context()
	hidden, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	limit := 100
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 300)
	}
	// 等确认的排在前面，其余取最近的 limit 条。正在生成的对话刚更新过，也在里面。
	rows, err := m.d.DB.QueryContext(ctx, `SELECT c.id,c.title,c.host_id,c.updated_at,
 (SELECT count(*) FROM ai_pending_actions p WHERE p.conversation_id=c.id AND p.status='pending') AS pending
 FROM ai_conversations c ORDER BY pending > 0 DESC, c.updated_at DESC LIMIT ?`, limit)
	if m.fail(w, r, err) {
		return
	}
	out := []api.AiSession{}
	for rows.Next() {
		var s api.AiSession
		if err = rows.Scan(&s.RefId, &s.Title, &s.HostId, &s.UpdatedAt, &s.PendingCount); err != nil {
			break
		}
		s.Kind, s.Status = "chat", "idle"
		if s.PendingCount > 0 {
			s.Status = "waiting"
		}
		m.mu.Lock()
		_, running := m.running[s.RefId]
		m.mu.Unlock()
		if running {
			s.Status = "working"
		}
		if s.HostId != nil {
			s.Kind, s.SourceName = "host", *s.HostId
		}
		s.Id = string(s.Kind) + ":" + strconv.FormatInt(s.RefId, 10)
		out = append(out, s)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if m.fail(w, r, err) {
		return
	}
	visible := out[:0]
	for _, s := range out {
		if s.HostId != nil {
			side := "servers"
			if host, e := m.d.Agents.Get(ctx, *s.HostId); e == nil {
				s.SourceName = host.Name
				if host.Kind == "desktop" {
					side = "pc"
				}
			}
			if hidden != nil && hidden.Hidden(ctx, side) {
				continue
			}
		}
		visible = append(visible, s)
	}
	out = visible
	if svc, ok := module.Lookup[contracts.AISessionRuns](m.d.Registry, contracts.AISessionRunsKey); ok {
		if hidden != nil && hidden.Hidden(ctx, "coding") {
			svc = nil
		}
		if svc != nil {
			runs, err := svc.SessionRuns(ctx)
			if m.fail(w, r, err) {
				return
			}
			for _, run := range runs {
				if run.IssueKey != "" && hidden != nil && hidden.Hidden(ctx, "projects") {
					continue
				}
				status := run.Status
				switch status {
				case "queued", "running":
					status = "working"
				case "pr_opened":
					status = "done"
				}
				title := truncateRunes(strings.TrimSpace(run.Summary), 60)
				if run.IssueKey != "" {
					title = strings.TrimSpace(run.IssueKey + " " + run.IssueTitle)
				}
				out = append(out, api.AiSession{Id: "run:" + strconv.FormatInt(run.ID, 10), Kind: "agent", RefId: run.ID,
					Title: title, SourceName: run.AgentName, AgentId: &run.AgentID, AgentKind: &run.AgentKind,
					AgentAvatar: &run.AgentAvatar, AgentColor: &run.AgentColor, IssueKey: &run.IssueKey, TaskId: run.TaskID,
					Status: api.AiSessionStatus(status), PendingCount: run.PendingCount, UpdatedAt: run.UpdatedAt})
			}
		}
	}
	active := func(s api.AiSession) bool { return s.Status == "working" || s.Status == "waiting" }
	sort.Slice(out, func(i, j int) bool {
		if active(out[i]) != active(out[j]) {
			return active(out[i])
		}
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Id < out[j].Id
	})
	httpx.JSON(w, 200, out[:min(limit, len(out))])
}
