package automations

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations/api"
)

func (m *Module) registerActions() {
	m.addAIAction("automations.list", "列出自动化规则", aiListSchema, actions.Read, func(ctx context.Context, in aiActionInput) (any, error) {
		rows, err := m.listRules(ctx)
		if err != nil {
			return nil, err
		}
		items := []any{}
		truncated := false
		for _, row := range rows {
			if m.ruleHidden(ctx, row) {
				continue
			}
			if len(items) == in.Limit {
				truncated = true
				break
			}
			items = append(items, map[string]any{"id": row.Id, "name": row.Name, "enabled": row.Enabled, "triggerType": row.Trigger.Type, "actionCount": len(row.Actions), "lastRun": aiAutomationRunSummary(row.LastRun)})
		}
		return map[string]any{"items": items, "truncated": truncated}, nil
	})
	m.addAIAction("automations.get", "查看自动化规则", aiIDSchema, actions.Read, func(ctx context.Context, in aiActionInput) (any, error) {
		row, err := m.visibleRule(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		names := []string{}
		for _, step := range row.Actions {
			names = append(names, step.Action)
		}
		// 参数和触发数据可能含隐藏正文或密钥，详情只显示动作名称。
		trigger := row.Trigger
		trigger.WebhookPath = nil
		return map[string]any{"id": row.Id, "name": row.Name, "enabled": row.Enabled, "trigger": trigger, "actions": names, "cooldownSeconds": row.CooldownSeconds, "authorized": row.Authorized, "lastRun": aiAutomationRunSummary(row.LastRun)}, nil
	})
	m.addAIAction("automations.runs", "查看自动化运行记录", `{"type":"object","properties":{"id":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":50}},"required":["id"],"additionalProperties":false}`, actions.Read, func(ctx context.Context, in aiActionInput) (any, error) {
		if _, err := m.visibleRule(ctx, in.ID); err != nil {
			return nil, err
		}
		rows, err := m.d.DB.QueryContext(ctx, "SELECT id,status,started_at,finished_at FROM automation_runs WHERE automation_id=? ORDER BY id DESC LIMIT ?", in.ID, in.Limit+1)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := []any{}
		for rows.Next() {
			var id int64
			var status string
			var started time.Time
			var finished *time.Time
			if err := rows.Scan(&id, &status, &started, &finished); err != nil {
				return nil, err
			}
			items = append(items, map[string]any{"id": id, "status": status, "startedAt": started, "finishedAt": finished})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return map[string]any{"items": items[:min(len(items), in.Limit)], "truncated": len(items) > in.Limit}, nil
	})
}

func aiAutomationRunSummary(run *api.Run) any {
	if run == nil {
		return nil
	}
	return map[string]any{"id": run.Id, "status": run.Status, "startedAt": run.StartedAt, "finishedAt": run.FinishedAt}
}

type aiActionInput struct {
	ID    int64 `json:"id"`
	Limit int   `json:"limit"`
}

const aiListSchema = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`
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
