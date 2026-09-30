package projects

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

func checklistText(value string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > limit {
		return "", httpx.Invalid("文字长度不正确")
	}
	return value, nil
}
func (m *Module) checklistIssue(ctx context.Context, key string, write bool) (issueRow, error) {
	issue, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return issueRow{}, err
	}
	if write {
		if err = m.categoryProject(ctx, issue.Issue.ProjectID, true); err != nil {
			return issueRow{}, err
		}
	}
	return issue, nil
}
func (m *Module) checklistChanged(ctx context.Context, issue issueRow) error {
	_, err := m.d.DB.ExecContext(ctx, "UPDATE issues SET updated_at=? WHERE id=?", m.now(), issue.Issue.ID)
	if err == nil {
		m.d.Bus.Publish("issue.checklist_changed", map[string]any{"key": issueKey(issue.ProjectKey, issue.Issue.Number)})
	}
	return err
}
func (m *Module) checklist(ctx context.Context, issueID, listID int64) (api.Checklist, error) {
	var c api.Checklist
	err := m.d.DB.QueryRowContext(ctx, "SELECT id,issue_id,title,position FROM issue_checklists WHERE issue_id=? AND id=?", issueID, listID).Scan(&c.Id, &c.IssueId, &c.Title, &c.Position)
	return c, notFound(err)
}
func (m *Module) checklistItem(ctx context.Context, issueID, itemID int64) (api.ChecklistItem, error) {
	var it api.ChecklistItem
	var doneAt sql.NullString
	err := m.d.DB.QueryRowContext(ctx, `SELECT it.id,it.checklist_id,it.text,it.done,it.position,it.done_at FROM issue_checklist_items it JOIN issue_checklists c ON c.id=it.checklist_id WHERE c.issue_id=? AND it.id=?`, issueID, itemID).Scan(&it.Id, &it.ChecklistId, &it.Text, &it.Done, &it.Position, &doneAt)
	if doneAt.Valid {
		parsed, e := time.Parse(time.RFC3339Nano, doneAt.String)
		if e == nil {
			it.DoneAt = &parsed
		}
	}
	return it, notFound(err)
}
func (m *Module) ListChecklists(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	ctx := r.Context()
	issue, err := m.checklistIssue(ctx, key, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,issue_id,title,position FROM issue_checklists WHERE issue_id=? ORDER BY position,id", issue.Issue.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.Checklist{}
	indexes := map[int64]int{}
	for rows.Next() {
		var c api.Checklist
		if err = rows.Scan(&c.Id, &c.IssueId, &c.Title, &c.Position); err != nil {
			break
		}
		c.Items = []api.ChecklistItem{}
		indexes[c.Id] = len(out)
		out = append(out, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := m.d.DB.QueryContext(ctx, `SELECT it.id,it.checklist_id,it.text,it.done,it.position,it.done_at FROM issue_checklist_items it JOIN issue_checklists c ON c.id=it.checklist_id WHERE c.issue_id=? ORDER BY c.position,c.id,it.position,it.id`, issue.Issue.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for items.Next() {
		var it api.ChecklistItem
		var doneAt sql.NullString
		if err = items.Scan(&it.Id, &it.ChecklistId, &it.Text, &it.Done, &it.Position, &doneAt); err != nil {
			break
		}
		if doneAt.Valid {
			parsed, e := time.Parse(time.RFC3339Nano, doneAt.String)
			if e == nil {
				it.DoneAt = &parsed
			}
		}
		out[indexes[it.ChecklistId]].Items = append(out[indexes[it.ChecklistId]].Items, it)
	}
	if err == nil {
		err = items.Err()
	}
	items.Close()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) CreateChecklist(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	ctx := r.Context()
	var body api.CreateChecklistJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	title, err := checklistText(body.Title, 200)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var count int
	var position float64
	err = m.d.DB.QueryRowContext(ctx, "SELECT count(*),COALESCE(max(position),0)+? FROM issue_checklists WHERE issue_id=?", sortGap, issue.Issue.ID).Scan(&count, &position)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if count >= 20 {
		httpx.Fail(w, r, httpx.NewError(400, "too_many", "一个 Issue 最多 20 个清单"))
		return
	}
	var id int64
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO issue_checklists(issue_id,title,position) VALUES(?,?,?) RETURNING id", issue.Issue.ID, title, position).Scan(&id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.checklistChanged(ctx, issue); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "issue.checklist.create", key, map[string]any{"checklistId": id}, nil)
	httpx.JSON(w, 201, api.Checklist{Id: id, IssueId: issue.Issue.ID, Title: title, Position: position, Items: []api.ChecklistItem{}})
}
func (m *Module) UpdateChecklist(w http.ResponseWriter, r *http.Request, key api.IssueKey, listID api.ChecklistId) {
	ctx := r.Context()
	var body api.UpdateChecklistJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	title, err := checklistText(body.Title, 200)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.checklist(ctx, issue.Issue.ID, listID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = m.d.DB.ExecContext(ctx, "UPDATE issue_checklists SET title=? WHERE id=?", title, listID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c.Title = title
	c.Items = []api.ChecklistItem{}
	if err = m.checklistChanged(ctx, issue); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "issue.checklist.update", key, map[string]any{"checklistId": listID}, nil)
	httpx.JSON(w, 200, c)
}
func (m *Module) DeleteChecklist(w http.ResponseWriter, r *http.Request, key api.IssueKey, listID api.ChecklistId) {
	ctx := r.Context()
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = m.checklist(ctx, issue.Issue.ID, listID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = m.d.DB.ExecContext(ctx, "DELETE FROM issue_checklists WHERE id=?", listID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.checklistChanged(ctx, issue); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "issue.checklist.delete", key, map[string]any{"checklistId": listID}, nil)
	httpx.NoContent(w)
}

type orderedItem struct {
	id       int64
	position float64
}

func (m *Module) itemPosition(ctx context.Context, listID, self int64, after, before *int64) (float64, error) {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT id,position FROM issue_checklist_items WHERE checklist_id=? AND id<>? ORDER BY position,id", listID, self)
	if err != nil {
		return 0, err
	}
	items := []orderedItem{}
	for rows.Next() {
		var x orderedItem
		if err = rows.Scan(&x.id, &x.position); err != nil {
			break
		}
		items = append(items, x)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return 0, err
	}
	index := len(items)
	if after != nil {
		index = -1
		for i, x := range items {
			if x.id == *after {
				index = i + 1
				break
			}
		}
	}
	if before != nil {
		b := -1
		for i, x := range items {
			if x.id == *before {
				b = i
				break
			}
		}
		if b < 0 || after != nil && index != b {
			return 0, httpx.Invalid("排序位置无效")
		}
		index = b
	}
	if index < 0 {
		return 0, httpx.Invalid("条目不在这个清单")
	}
	var prev, next *float64
	if index > 0 {
		prev = &items[index-1].position
	}
	if index < len(items) {
		next = &items[index].position
	}
	if value, ok := between(prev, next); ok {
		return value, nil
	}
	ids := make([]int64, 0, len(items)+1)
	for _, x := range items {
		ids = append(ids, x.id)
	}
	ids = slices.Insert(ids, index, self)
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if id == self {
			continue
		}
		if _, err = tx.ExecContext(ctx, "UPDATE issue_checklist_items SET position=? WHERE id=?", float64(i+1)*sortGap, id); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return float64(index+1) * sortGap, nil
}
func (m *Module) CreateChecklistItem(w http.ResponseWriter, r *http.Request, key api.IssueKey, listID api.ChecklistId) {
	ctx := r.Context()
	var body api.CreateChecklistItemJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	value, err := checklistText(body.Text, 500)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = m.checklist(ctx, issue.Issue.ID, listID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var count int
	if err = m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM issue_checklist_items WHERE checklist_id=?", listID).Scan(&count); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if count >= 200 {
		httpx.Fail(w, r, httpx.NewError(400, "too_many", "一个清单最多 200 条"))
		return
	}
	position, err := m.itemPosition(ctx, listID, 0, body.AfterId, nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var id int64
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO issue_checklist_items(checklist_id,text,position) VALUES(?,?,?) RETURNING id", listID, value, position).Scan(&id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.checklistChanged(ctx, issue); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "issue.checklist_item.create", key, map[string]any{"itemId": id}, nil)
	httpx.JSON(w, 201, api.ChecklistItem{Id: id, ChecklistId: listID, Text: value, Position: position})
}
func (m *Module) UpdateChecklistItem(w http.ResponseWriter, r *http.Request, key api.IssueKey, itemID api.ChecklistItemId) {
	ctx := r.Context()
	var body api.UpdateChecklistItemJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	it, err := m.checklistItem(ctx, issue.Issue.ID, itemID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Text != nil {
		it.Text, err = checklistText(*body.Text, 500)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if body.Done != nil && *body.Done != it.Done {
		it.Done = *body.Done
		if it.Done {
			now := m.now().UTC()
			it.DoneAt = &now
		} else {
			it.DoneAt = nil
		}
	}
	if body.ChecklistId != nil {
		if _, err = m.checklist(ctx, issue.Issue.ID, *body.ChecklistId); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		it.ChecklistId = *body.ChecklistId
	}
	if body.AfterId != nil || body.BeforeId != nil || body.ChecklistId != nil {
		it.Position, err = m.itemPosition(ctx, it.ChecklistId, itemID, body.AfterId, body.BeforeId)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if _, err = m.d.DB.ExecContext(ctx, "UPDATE issue_checklist_items SET checklist_id=?,text=?,done=?,done_at=?,position=? WHERE id=?", it.ChecklistId, it.Text, it.Done, it.DoneAt, it.Position, itemID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.checklistChanged(ctx, issue); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "issue.checklist_item.update", key, map[string]any{"itemId": itemID}, nil)
	httpx.JSON(w, 200, it)
}
func (m *Module) DeleteChecklistItem(w http.ResponseWriter, r *http.Request, key api.IssueKey, itemID api.ChecklistItemId) {
	ctx := r.Context()
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = m.checklistItem(ctx, issue.Issue.ID, itemID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = m.d.DB.ExecContext(ctx, "DELETE FROM issue_checklist_items WHERE id=?", itemID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.checklistChanged(ctx, issue); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Audit.Record(ctx, "issue.checklist_item.delete", key, map[string]any{"itemId": itemID}, nil)
	httpx.NoContent(w)
}
func (m *Module) ConvertChecklistItem(w http.ResponseWriter, r *http.Request, key api.IssueKey, itemID api.ChecklistItemId) {
	ctx := r.Context()
	issue, err := m.checklistIssue(ctx, key, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	it, err := m.checklistItem(ctx, issue.Issue.ID, itemID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer tx.Rollback()
	var next int64
	if err = tx.QueryRowContext(ctx, "UPDATE projects SET next_number=next_number+1 WHERE id=? RETURNING next_number", issue.Issue.ProjectID).Scan(&next); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var top sql.NullFloat64
	if err = tx.QueryRowContext(ctx, "SELECT min(sort_order) FROM issues WHERE project_id=? AND status='todo'", issue.Issue.ProjectID).Scan(&top); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	position := float64(0)
	if top.Valid {
		position = top.Float64 - sortGap
	}
	now := m.now().UTC()
	var newID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO issues(project_id,number,title,status,priority,sort_order,category_id,created_at,updated_at) VALUES(?,?,?,'todo',0,?,?,?,?) RETURNING id`, issue.Issue.ProjectID, next-1, it.Text, position, issue.Issue.CategoryID, now, now).Scan(&newID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM issue_checklist_items WHERE id=? AND checklist_id=?", itemID, it.ChecklistId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		httpx.Fail(w, r, httpx.ErrConflict)
		return
	}
	if _, err = tx.ExecContext(ctx, "UPDATE issues SET updated_at=? WHERE id=?", now, issue.Issue.ID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := m.issueByID(ctx, newID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("issue.created", created)
	m.d.Bus.Publish("issue.checklist_changed", map[string]any{"key": key})
	m.d.Audit.Record(ctx, "issue.create", created.Key, map[string]any{"projectId": issue.Issue.ProjectID, "title": it.Text}, nil)
	m.d.Audit.Record(ctx, "issue.checklist_item.convert", key, map[string]any{"itemId": itemID, "newIssue": created.Key}, nil)
	httpx.JSON(w, 201, created)
}
