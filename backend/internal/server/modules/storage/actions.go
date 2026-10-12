package storage

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
	m.addAIAction("storage.status", "查看存储状态", aiEmptySchema, actions.Read, func(m *Module, ctx context.Context, in aiActionInput) (any, error) {
		// 仅返回运行状态。容量明细通过维护模块的隐藏过滤读取。
		m.mu.Lock()
		backend, last, running := m.backend, m.last, m.move
		m.mu.Unlock()
		if running != nil {
			last = running.snapshot()
		}
		limit, err := m.cacheLimit(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"backend": backend, "cacheLimitBytes": limit, "migration": map[string]any{"state": last.State, "target": last.Target, "startedAt": last.StartedAt, "finishedAt": last.FinishedAt}}, nil
	})
	m.addAIAction("storage.remotes", "列出网盘账号状态", aiListSchema, actions.Read, func(m *Module, ctx context.Context, in aiActionInput) (any, error) {
		hidden, _ := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
		if contracts.BackendBlocked(ctx, hidden, "drive", "/drive") {
			return map[string]any{"items": []any{}}, nil
		}
		rows, err := m.q.ListRemotes(ctx)
		if err != nil {
			return nil, err
		}
		items := []any{}
		for _, row := range rows[:min(len(rows), in.Limit)] {
			info := m.info(ctx, row)
			items = append(items, map[string]any{"id": info.ID, "name": info.Name, "kind": info.Kind, "ready": info.Ready, "limited": info.Limited, "showInDrive": info.ShowInDrive})
		}
		return map[string]any{"items": items, "truncated": len(rows) > in.Limit}, nil
	})
}

type aiActionInput struct {
	Limit int `json:"limit"`
}

const aiListSchema = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`
const aiEmptySchema = `{"type":"object","properties":{},"additionalProperties":false}`

func (m *Module) addAIAction(name, title, schema string, effect actions.Effect, run func(*Module, context.Context, aiActionInput) (any, error)) {
	// 重建存储实例时保留注册。执行时使用当前实例。
	if _, ok := m.d.Actions.Get(context.Background(), name); ok {
		return
	}
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
		current, ok := module.Lookup[*Module](m.d.Registry, ServiceKey)
		if !ok {
			return nil, httpx.ErrNotFound
		}
		out, err := run(current, ctx, in)
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
