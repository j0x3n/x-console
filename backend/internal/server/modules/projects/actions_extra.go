package projects

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

func (m *Module) registerExtraActions() {
	// issues.list/get/create/update do the same as projects.list_issues,
	// get_issue, create_issue and update_issue, which have fuller schemas.
	// They stay for saved automation rules but are hidden from the lists.
	aliases := map[string]string{"issues.list": "projects.list_issues", "issues.get": "projects.get_issue",
		"issues.create": "projects.create_issue", "issues.update": "projects.update_issue"}
	add := func(name, title, schema string, effect actions.Effect, run func(context.Context, json.RawMessage) (any, error)) {
		m.d.Actions.Register(actions.Action{Name: name, Title: title, Description: title, Input: actions.Schema(schema), Effect: effect, Run: run, AliasOf: aliases[name]})
	}
	add("projects.get", "查看项目", `{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`, actions.Read, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Key string `json:"key"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		id, err := m.projectIDByKey(ctx, in.Key)
		if err != nil {
			return nil, err
		}
		return m.getProject(ctx, id)
	})
	add("projects.create", "新建项目", `{"type":"object","properties":{"key":{"type":"string"},"name":{"type":"string"},"description":{"type":"string"}},"required":["key","name"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in api.CreateProject
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		return m.createProject(ctx, in)
	})
	add("projects.update", "修改项目", `{"type":"object","properties":{"key":{"type":"string"},"name":{"type":"string"},"description":{"type":"string"}},"required":["key"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Key         string  `json:"key"`
			Name        *string `json:"name"`
			Description *string `json:"description"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		id, err := m.projectIDByKey(ctx, in.Key)
		if err != nil {
			return nil, err
		}
		return m.updateProject(ctx, id, api.UpdateProject{Name: in.Name, Description: in.Description})
	})
	add("projects.archive", "归档项目", `{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Key string `json:"key"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		id, err := m.projectIDByKey(ctx, in.Key)
		if err != nil {
			return nil, err
		}
		yes := true
		return m.updateProject(ctx, id, api.UpdateProject{Archived: &yes})
	})
	add("issues.list", "查询 Issue", `{"type":"object","properties":{"projectKey":{"type":"string"},"status":{"type":"array","items":{"type":"string"}},"q":{"type":"string"}}}`, actions.Read, m.actionListIssues)
	add("issues.get", "查看 Issue", `{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`, actions.Read, m.actionGetIssue)
	add("issues.create", "新建 Issue", `{"type":"object","properties":{"projectKey":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"dueDate":{"type":"string"}},"required":["projectKey","title"]}`, actions.Write, m.actionCreateIssue)
	add("issues.update", "修改 Issue", `{"type":"object","properties":{"key":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"status":{"type":"string"},"dueDate":{"type":"string"}},"required":["key"]}`, actions.Write, m.actionUpdateIssue)
	add("issues.move", "移动 Issue", `{"type":"object","properties":{"key":{"type":"string"},"status":{"type":"string"},"afterKey":{"type":"string"},"beforeKey":{"type":"string"}},"required":["key","status"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			Key, Status         string
			AfterKey, BeforeKey *string
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		return m.moveIssue(ctx, in.Key, in.Status, in.AfterKey, in.BeforeKey)
	})
	add("issues.comment", "评论 Issue", `{"type":"object","properties":{"key":{"type":"string"},"body":{"type":"string"}},"required":["key","body"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{ Key, Body string }
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		return m.createComment(ctx, in.Key, in.Body)
	})
	add("issues.delete", "删除 Issue", `{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{ Key string }
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		if in.Key == "" {
			return nil, httpx.Invalid("缺少 key")
		}
		return map[string]any{"deleted": true}, m.deleteIssue(ctx, in.Key)
	})
	add("milestones.create", "新建里程碑", `{"type":"object","properties":{"projectKey":{"type":"string"},"name":{"type":"string"},"dueDate":{"type":"string","format":"date"}},"required":["projectKey","name"]}`, actions.Write, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ProjectKey string  `json:"projectKey"`
			Name       string  `json:"name"`
			DueDate    *string `json:"dueDate"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		id, err := m.projectIDByKey(ctx, in.ProjectKey)
		if err != nil {
			return nil, err
		}
		body := api.CreateMilestone{Name: in.Name}
		if in.DueDate != nil {
			var d api.CreateMilestone
			if err = json.Unmarshal(raw, &d); err != nil {
				return nil, httpx.Invalid("日期不正确")
			}
			body.DueDate = d.DueDate
		}
		return m.createMilestone(ctx, id, body)
	})
}
