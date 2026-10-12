package aiagents

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func (m *Module) registerActions() {
	m.addAIAction("aiagents.list", "列出 AI Agent 状态", aiListSchema, actions.Read, func(ctx context.Context, in aiActionInput) (any, error) {
		rows, err := m.q.ListAgents(ctx)
		if err != nil {
			return nil, err
		}
		counts, err := m.taskCounts(ctx)
		if err != nil {
			return nil, err
		}
		items := []any{}
		for _, row := range rows[:min(len(rows), in.Limit)] {
			cost, err := m.monthCost(ctx, row.ID)
			if err != nil {
				return nil, err
			}
			items = append(items, map[string]any{"id": row.ID, "name": row.Name, "kind": row.Kind, "model": row.Model, "enabled": row.Enabled != 0, "runningTasks": counts[row.ID].running + m.busy(row.ID), "queuedTasks": counts[row.ID].queued, "monthCostUsd": cost})
		}
		return map[string]any{"items": items, "truncated": len(rows) > in.Limit}, nil
	})
	m.addAIAction("aiagents.get", "查看 AI Agent", aiIDSchema, actions.Read, func(ctx context.Context, in aiActionInput) (any, error) {
		out, err := m.agent(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		result := map[string]any{"id": out.Id, "name": out.Name, "kind": out.Kind, "model": out.Model, "enabled": out.Enabled, "access": out.Access, "maxParallel": out.MaxParallel, "monthlyBudgetUsd": out.MonthlyBudgetUsd, "monthCostUsd": out.MonthCostUsd, "runningTasks": out.RunningTasks, "queuedTasks": out.QueuedTasks}
		// 指令和仓库配置可能引用隐藏内容。只返回状态与权限摘要。
		return result, nil
	})
	m.addAIAction("aiagents.runs", "列出 AI Agent 运行记录", `{"type":"object","properties":{"agentId":{"type":"integer","minimum":1},"issueKey":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`, actions.Read, func(ctx context.Context, in aiActionInput) (any, error) {
		hidden, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
		if contracts.BackendBlocked(ctx, hidden, "projects", "/projects") {
			return map[string]any{"items": []any{}}, nil
		}
		where, args := "1=1", []any{}
		if in.AgentID != nil {
			where += " AND r.agent_id=?"
			args = append(args, *in.AgentID)
		}
		if in.IssueKey != "" {
			where += " AND r.issue_key=?"
			args = append(args, strings.ToUpper(strings.TrimSpace(in.IssueKey)))
		}
		rows, err := m.runList(ctx, where, args, in.Limit+1)
		if err != nil {
			return nil, err
		}
		items := []any{}
		for _, row := range rows[:min(len(rows), in.Limit)] {
			items = append(items, map[string]any{"id": row.Id, "agentId": row.AgentId, "agentName": row.AgentName, "issueKey": row.IssueKey, "issueTitle": row.IssueTitle, "status": row.Status, "kind": row.Kind, "taskId": row.TaskId, "createdAt": row.CreatedAt, "startedAt": row.StartedAt, "finishedAt": row.FinishedAt})
		}
		return map[string]any{"items": items, "truncated": len(rows) > in.Limit}, nil
	})
}

type aiActionInput struct {
	ID       int64  `json:"id"`
	Limit    int    `json:"limit"`
	AgentID  *int64 `json:"agentId"`
	IssueKey string `json:"issueKey"`
}

const aiListSchema = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`
const aiEmptySchema = `{"type":"object","properties":{},"additionalProperties":false}`
const aiIDSchema = `{"type":"object","properties":{"id":{"type":"integer","minimum":1}},"required":["id"],"additionalProperties":false}`

func (m *Module) addAIAction(name, title, schema string, effect actions.Effect, run func(context.Context, aiActionInput) (any, error)) {
	var spec struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal([]byte(schema), &spec); err != nil {
		panic(err)
	}
	m.d.Actions.Register(actions.Action{Name: name, Title: title, Description: title + "。列表默认 20 项，最多 50 项。长文本最多返回 2000 个字符。截断字段有对应的 Truncated 标记。", Input: actions.Schema(schema), Effect: effect, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return nil, httpx.Invalid("参数必须是对象")
		}
		for key, value := range fields {
			prop, ok := spec.Properties[key]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, httpx.Invalid("参数不正确: " + key)
			}
			var rule struct {
				Enum []string `json:"enum"`
			}
			_ = json.Unmarshal(prop, &rule)
			if len(rule.Enum) > 0 {
				var text string
				if json.Unmarshal(value, &text) != nil {
					return nil, httpx.Invalid("参数不正确: " + key)
				}
				found := false
				for _, choice := range rule.Enum {
					found = found || text == choice
				}
				if !found {
					return nil, httpx.Invalid("参数不正确: " + key)
				}
			}
		}
		for _, key := range spec.Required {
			if fields[key] == nil {
				return nil, httpx.Invalid("缺少参数: " + key)
			}
		}
		var in aiActionInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, httpx.Invalid("参数格式不正确")
		}
		if fields["limit"] != nil && (in.Limit < 1 || in.Limit > 50) {
			return nil, httpx.Invalid("limit 要在 1 到 50 之间")
		}
		for _, key := range []string{"id", "accountId", "agentId"} {
			if value := fields[key]; value != nil {
				var id int64
				if json.Unmarshal(value, &id) != nil || id <= 0 {
					return nil, httpx.Invalid("编号必须大于 0")
				}
			}
		}
		if in.Limit == 0 {
			in.Limit = 20
		}
		ctx = auth.WithoutVault(ctx)
		hidden, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
		if contracts.ActionHidden(ctx, hidden, name) || contracts.BackendBlocked(ctx, hidden, actions.Module(name), "/"+actions.Module(name)) {
			return nil, httpx.ErrNotFound
		}
		out, err := run(ctx, in)
		if err != nil {
			return nil, err
		}
		return boundedAIAction(out, in.Limit)
	}})
}

func boundedAIAction(value any, limit int) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return trimAIAction(out, limit), nil
}

func trimAIAction(value any, limit int) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			switch x := item.(type) {
			case []any:
				if len(x) > limit {
					v[key+"Truncated"] = true
					item = x[:limit]
				}
			case string:
				if len([]rune(x)) > 2000 {
					v[key+"Truncated"] = true
				}
			}
			v[key] = trimAIAction(item, limit)
		}
	case []any:
		v = v[:min(len(v), limit)]
		for i := range v {
			v[i] = trimAIAction(v[i], limit)
		}
		return v
	case string:
		text := []rune(v)
		if len(text) > 2000 {
			return strings.TrimSpace(string(text[:1999])) + "…"
		}
	}
	return value
}
