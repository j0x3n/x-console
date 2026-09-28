package projects

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.registerExtraActions()
	m.d.Actions.Register(actions.Action{
		Name:        "projects.list",
		Title:       "列出项目",
		Description: "List active projects with their key (e.g. XC), name and open issue count.",
		Input:       actions.Schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			return m.listProjects(ctx, false)
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "projects.list_issues",
		Title: "查找 Issue",
		Description: "List issues. Filter by project key, statuses (backlog, todo, in_progress, in_review, done, canceled), " +
			"due (today, week, overdue) or a title search q. Returns at most `limit` issues (default 50).",
		Input: actions.Schema(`{"type":"object","properties":{
			"projectKey":{"type":"string"},
			"status":{"type":"array","items":{"type":"string","enum":["backlog","todo","in_progress","in_review","done","canceled"]}},
			"due":{"type":"string","enum":["today","week","overdue"]},
			"q":{"type":"string"},
			"limit":{"type":"integer","minimum":1,"maximum":200}
		},"additionalProperties":false}`),
		Effect: actions.Read,
		Run:    m.actionListIssues,
	})
	m.d.Actions.Register(actions.Action{
		Name:        "projects.get_issue",
		Title:       "查看 Issue",
		Description: "Get one issue by key (e.g. XC-12) with its description, links and comments.",
		Input:       actions.Schema(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`),
		Effect:      actions.Read,
		Run:         m.actionGetIssue,
	})
	m.d.Actions.Register(actions.Action{
		Name:  "projects.create_issue",
		Title: "新建 Issue",
		Description: "Create an issue in a project given by projectKey (e.g. XC) or projectId. priority: 0 none, 1 urgent, " +
			"2 high, 3 medium, 4 low. dueDate is YYYY-MM-DD. Returns the issue with its key, e.g. XC-12.",
		Input: actions.Schema(`{"type":"object","properties":{
			"projectKey":{"type":"string"},
			"projectId":{"type":"integer"},
			"title":{"type":"string"},
			"description":{"type":"string"},
			"status":{"type":"string","enum":["backlog","todo","in_progress","in_review","done","canceled"]},
			"priority":{"type":"integer","minimum":0,"maximum":4},
			"dueDate":{"type":"string","format":"date"}
		},"required":["title"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionCreateIssue,
	})
	m.d.Actions.Register(actions.Action{
		Name:  "projects.update_issue",
		Title: "修改 Issue",
		Description: "Update fields of an issue given by key (e.g. XC-12). Only the given fields change. " +
			"Set dueDate to an empty string to clear it.",
		Input: actions.Schema(`{"type":"object","properties":{
			"key":{"type":"string"},
			"title":{"type":"string"},
			"description":{"type":"string"},
			"status":{"type":"string","enum":["backlog","todo","in_progress","in_review","done","canceled"]},
			"priority":{"type":"integer","minimum":0,"maximum":4},
			"dueDate":{"type":"string"}
		},"required":["key"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionUpdateIssue,
	})
}

func decodeInput(input json.RawMessage, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(input)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.Invalid("参数不正确: " + err.Error())
	}
	return nil
}

func (m *Module) actionListIssues(ctx context.Context, input json.RawMessage) (any, error) {
	var in struct {
		ProjectKey string   `json:"projectKey"`
		Status     []string `json:"status"`
		Due        string   `json:"due"`
		Q          string   `json:"q"`
		Limit      int      `json:"limit"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	f := issueFilter{Statuses: in.Status, Due: in.Due, Q: in.Q, Limit: in.Limit}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if in.ProjectKey != "" {
		id, err := m.projectIDByKey(ctx, in.ProjectKey)
		if err != nil {
			return nil, err
		}
		f.ProjectID = &id
	}
	items, _, err := m.listIssues(ctx, f)
	return items, err
}

func (m *Module) actionGetIssue(ctx context.Context, input json.RawMessage) (any, error) {
	var in struct {
		Key string `json:"key"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	issue, err := m.getIssue(ctx, in.Key)
	if err != nil {
		return nil, err
	}
	links, err := m.listLinks(ctx, in.Key)
	if err != nil {
		return nil, err
	}
	comments, err := m.listComments(ctx, in.Key)
	if err != nil {
		return nil, err
	}
	return map[string]any{"issue": issue, "links": links, "comments": comments}, nil
}

func (m *Module) actionCreateIssue(ctx context.Context, input json.RawMessage) (any, error) {
	var in struct {
		ProjectKey  string  `json:"projectKey"`
		ProjectID   int64   `json:"projectId"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Status      string  `json:"status"`
		Priority    int     `json:"priority"`
		DueDate     *string `json:"dueDate"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if in.ProjectID == 0 {
		if in.ProjectKey == "" {
			return nil, httpx.Invalid("需要 projectKey 或 projectId")
		}
		id, err := m.projectIDByKey(ctx, in.ProjectKey)
		if err != nil {
			return nil, err
		}
		in.ProjectID = id
	}
	if in.DueDate != nil && *in.DueDate == "" {
		in.DueDate = nil
	}
	return m.createIssue(ctx, in.ProjectID, issueInput{
		Title: in.Title, Description: in.Description, Status: in.Status, Priority: in.Priority, DueDate: in.DueDate,
	})
}

func (m *Module) actionUpdateIssue(ctx context.Context, input json.RawMessage) (any, error) {
	var in struct {
		Key         string  `json:"key"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Priority    *int    `json:"priority"`
		DueDate     *string `json:"dueDate"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	p := issuePatch{Title: in.Title, Description: in.Description, Status: in.Status, Priority: in.Priority}
	if in.DueDate != nil {
		if *in.DueDate == "" {
			p.ClearDueDate = true
		} else {
			p.DueDate = in.DueDate
		}
	}
	return m.updateIssue(ctx, in.Key, p)
}
