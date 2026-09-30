package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

// B46：给 AI 助手和远程 AI（B43）用的看板动作。

func (m *Module) registerBoardActions() {
	reg := func(a actions.Action) { m.d.Actions.Register(a) }
	reg(actions.Action{
		Name:        "projects.list_boards",
		Title:       "列出看板",
		Description: "List the boards of a project (projectKey, e.g. XC) with their lists. Each list may stand for a status; moving a card into it sets that status.",
		Input:       actions.Schema(`{"type":"object","properties":{"projectKey":{"type":"string"}},"required":["projectKey"],"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ProjectKey string `json:"projectKey"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			id, err := m.projectIDByKey(ctx, in.ProjectKey)
			if err != nil {
				return nil, err
			}
			return m.listBoards(ctx, id, false)
		},
	})
	reg(actions.Action{
		Name:  "projects.get_board",
		Title: "查看看板",
		Description: "Get one board with its lists and the cards in each list (key, title, status, dueAt, members). " +
			"Give boardId, or projectKey and boardName.",
		Input: actions.Schema(`{"type":"object","properties":{
			"boardId":{"type":"integer"},"projectKey":{"type":"string"},"boardName":{"type":"string"}
		},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				BoardID    int64  `json:"boardId"`
				ProjectKey string `json:"projectKey"`
				BoardName  string `json:"boardName"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			b, err := m.findBoard(ctx, in.BoardID, in.ProjectKey, in.BoardName)
			if err != nil {
				return nil, err
			}
			cards, _, err := m.listIssues(ctx, issueFilter{BoardID: &b.Id, Sort: "manual", Limit: 1000})
			if err != nil {
				return nil, err
			}
			type card struct {
				Key     string             `json:"key"`
				Title   string             `json:"title"`
				Status  api.IssueStatus    `json:"status"`
				DueAt   *time.Time         `json:"dueAt,omitempty"`
				Members *[]api.IssueMember `json:"members,omitempty"`
				Labels  []string           `json:"labels,omitempty"`
			}
			type list struct {
				api.BoardList
				Cards []card `json:"cards"`
			}
			byList := map[int64][]card{}
			for _, c := range cards {
				if c.ListId == nil {
					continue
				}
				var labels []string
				for _, l := range c.Labels {
					labels = append(labels, l.Name)
				}
				byList[*c.ListId] = append(byList[*c.ListId], card{c.Key, c.Title, c.Status, c.DueAt, c.Members, labels})
			}
			out := struct {
				ID    int64  `json:"id"`
				Name  string `json:"name"`
				Lists []list `json:"lists"`
			}{ID: b.Id, Name: b.Name}
			for _, l := range b.Lists {
				cs := byList[l.Id]
				if cs == nil {
					cs = []card{}
				}
				out.Lists = append(out.Lists, list{l, cs})
			}
			return out, nil
		},
	})
	reg(actions.Action{
		Name:  "projects.create_card",
		Title: "新建卡片",
		Description: "Create a card (issue) on a board. projectKey is required (e.g. XC). boardName and listName pick the " +
			"place by name (case-insensitive); without them the first board and the list for the status are used. " +
			"dueAt is RFC 3339 with a time zone, e.g. 2026-10-01T18:00:00+08:00. priority: 0 none, 1 urgent, 2 high, 3 medium, 4 low.",
		Input: actions.Schema(`{"type":"object","properties":{
			"projectKey":{"type":"string"},"boardName":{"type":"string"},"listName":{"type":"string"},
			"title":{"type":"string"},"description":{"type":"string"},
			"status":{"type":"string","enum":["backlog","todo","in_progress","in_review","done","canceled"]},
			"priority":{"type":"integer","minimum":0,"maximum":4},"dueAt":{"type":"string","format":"date-time"}
		},"required":["projectKey","title"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ProjectKey  string     `json:"projectKey"`
				BoardName   string     `json:"boardName"`
				ListName    string     `json:"listName"`
				Title       string     `json:"title"`
				Description string     `json:"description"`
				Status      string     `json:"status"`
				Priority    int        `json:"priority"`
				DueAt       *time.Time `json:"dueAt"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			projectID, err := m.projectIDByKey(ctx, in.ProjectKey)
			if err != nil {
				return nil, err
			}
			var boardID, listID *int64
			if in.BoardName != "" || in.ListName != "" {
				b, err := m.findBoard(ctx, 0, in.ProjectKey, in.BoardName)
				if err != nil {
					return nil, err
				}
				boardID = &b.Id
				if in.ListName != "" {
					l, err := listByName(b, in.ListName)
					if err != nil {
						return nil, err
					}
					listID = &l.Id
				}
			}
			return m.createIssue(ctx, projectID, issueInput{Title: in.Title, Description: in.Description, Status: in.Status,
				Priority: in.Priority, DueAt: in.DueAt, BoardID: boardID, ListID: listID})
		},
	})
	reg(actions.Action{
		Name:  "projects.move_card",
		Title: "移动卡片",
		Description: "Move a card (key, e.g. XC-12) to the end of a list. Give listId, or listName (and boardName to switch " +
			"board, on the card's project). Moving into a list that stands for a status changes the card's status.",
		Input: actions.Schema(`{"type":"object","properties":{
			"key":{"type":"string"},"listId":{"type":"integer"},"boardName":{"type":"string"},"listName":{"type":"string"}
		},"required":["key"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Key       string `json:"key"`
				ListID    int64  `json:"listId"`
				BoardName string `json:"boardName"`
				ListName  string `json:"listName"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			if in.ListID == 0 {
				if in.ListName == "" {
					return nil, httpx.Invalid("需要 listId 或 listName")
				}
				issue, err := m.getIssue(ctx, in.Key)
				if err != nil {
					return nil, err
				}
				var boardID int64
				if in.BoardName == "" && issue.BoardId != nil {
					boardID = *issue.BoardId
				}
				b, err := m.findBoard(ctx, boardID, issue.ProjectKey, in.BoardName)
				if err != nil {
					return nil, err
				}
				l, err := listByName(b, in.ListName)
				if err != nil {
					return nil, err
				}
				in.ListID = l.Id
			}
			return m.moveToList(ctx, in.Key, in.ListID, nil, nil)
		},
	})
	reg(actions.Action{
		Name:        "projects.comment",
		Title:       "评论卡片",
		Description: "Add a Markdown comment to a card (key, e.g. XC-12).",
		Input:       actions.Schema(`{"type":"object","properties":{"key":{"type":"string"},"body":{"type":"string"}},"required":["key","body"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Key  string `json:"key"`
				Body string `json:"body"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			return m.createComment(ctx, in.Key, in.Body)
		},
	})
	reg(actions.Action{
		Name:  "projects.add_checklist_item",
		Title: "加清单条目",
		Description: "Add an item to a checklist of a card (key). checklist is the checklist title; it is created when " +
			"missing. Without it the first checklist is used, or a new one named 清单.",
		Input:  actions.Schema(`{"type":"object","properties":{"key":{"type":"string"},"text":{"type":"string"},"checklist":{"type":"string"}},"required":["key","text"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Key       string `json:"key"`
				Text      string `json:"text"`
				Checklist string `json:"checklist"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			var lists []api.Checklist
			if err := callHandler(ctx, http.MethodGet, nil, &lists, func(w http.ResponseWriter, r *http.Request) {
				m.ListChecklists(w, r, in.Key)
			}); err != nil {
				return nil, err
			}
			var listID int64
			for _, l := range lists {
				if in.Checklist == "" || strings.EqualFold(l.Title, in.Checklist) {
					listID = l.Id
					break
				}
			}
			if listID == 0 {
				title := in.Checklist
				if title == "" {
					title = "清单"
				}
				var created api.Checklist
				if err := callHandler(ctx, http.MethodPost, map[string]string{"title": title}, &created, func(w http.ResponseWriter, r *http.Request) {
					m.CreateChecklist(w, r, in.Key)
				}); err != nil {
					return nil, err
				}
				listID = created.Id
			}
			var item api.ChecklistItem
			err := callHandler(ctx, http.MethodPost, map[string]string{"text": in.Text}, &item, func(w http.ResponseWriter, r *http.Request) {
				m.CreateChecklistItem(w, r, in.Key, listID)
			})
			return item, err
		},
	})
	reg(actions.Action{
		Name:        "projects.check_item",
		Title:       "勾选清单条目",
		Description: "Mark a checklist item of a card as done or not done. itemId comes from projects.get_issue.",
		Input:       actions.Schema(`{"type":"object","properties":{"key":{"type":"string"},"itemId":{"type":"integer"},"done":{"type":"boolean"}},"required":["key","itemId","done"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Key    string `json:"key"`
				ItemID int64  `json:"itemId"`
				Done   bool   `json:"done"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			var item api.ChecklistItem
			err := callHandler(ctx, http.MethodPatch, map[string]bool{"done": in.Done}, &item, func(w http.ResponseWriter, r *http.Request) {
				m.UpdateChecklistItem(w, r, in.Key, in.ItemID)
			})
			return item, err
		},
	})
	reg(actions.Action{
		Name:        "projects.archive_card",
		Title:       "归档卡片",
		Description: "Archive a card (key). It leaves the board but can be restored.",
		Input:       actions.Schema(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Key string `json:"key"`
			}
			if err := decodeInput(raw, &in); err != nil {
				return nil, err
			}
			return m.setArchived(ctx, in.Key, true)
		},
	})
}

// findBoard finds a board by id, or by project key and name (the first
// board when name is empty).
func (m *Module) findBoard(ctx context.Context, id int64, projectKey, name string) (api.Board, error) {
	if id != 0 {
		b, err := m.q.GetBoard(ctx, id)
		if err != nil {
			return api.Board{}, notFound(err)
		}
		return m.boardWithLists(ctx, m.q, b, false)
	}
	if projectKey == "" {
		return api.Board{}, httpx.Invalid("需要 boardId 或 projectKey")
	}
	projectID, err := m.projectIDByKey(ctx, projectKey)
	if err != nil {
		return api.Board{}, err
	}
	all, err := m.listBoards(ctx, projectID, false)
	if err != nil {
		return api.Board{}, err
	}
	for _, b := range all {
		if name == "" || strings.EqualFold(b.Name, name) {
			return b, nil
		}
	}
	names := make([]string, len(all))
	for i, b := range all {
		names[i] = b.Name
	}
	return api.Board{}, httpx.Invalid(fmt.Sprintf("没有叫“%s”的看板，有：%s", name, strings.Join(names, "、")))
}

func listByName(b api.Board, name string) (api.BoardList, error) {
	for _, l := range b.Lists {
		if strings.EqualFold(l.Name, name) {
			return l, nil
		}
	}
	names := make([]string, len(b.Lists))
	for i, l := range b.Lists {
		names[i] = l.Name
	}
	return api.BoardList{}, httpx.Invalid(fmt.Sprintf("看板“%s”里没有叫“%s”的列表，有：%s", b.Name, name, strings.Join(names, "、")))
}

// callHandler runs an HTTP handler of this module in-process, so actions
// reuse its checks. The context keeps the caller's session and actor.
func callHandler(ctx context.Context, method string, body, out any, h http.HandlerFunc) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req := httptest.NewRequestWithContext(ctx, method, "/", &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code >= 400 {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		return httpx.NewError(rec.Code, e.Code, e.Message)
	}
	if out != nil && rec.Body.Len() > 0 {
		return json.Unmarshal(rec.Body.Bytes(), out)
	}
	return nil
}
