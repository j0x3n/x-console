package projects

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

// The issue list has too many optional filters and sort orders for sqlc, so
// it is built here with database/sql. Pages use an offset cursor because the
// sort order is not always by id.

// issueFilter is the input of listIssues.
type issueFilter struct {
	ProjectID   *int64
	Statuses    []string
	Priority    *int
	LabelID     *int64
	MilestoneID *int64
	CategoryID  *int64
	BoardID     *int64
	// Archived includes archived cards (B46). They are left out by default.
	Archived bool
	Due      string // today, week, overdue
	Q        string
	Sort     string // updated, priority, due, manual
	Limit    int
	Offset   int
}

const issueColumns = `i.id, i.project_id, i.number, i.title, i.description, i.status, i.priority, i.due_date,
	i.milestone_id, i.sort_order, i.external_source, i.external_id, i.created_at, i.updated_at, i.completed_at,
	i.category_id, i.due_at, i.due_remind, i.due_notified_at, i.board_id, i.list_id, i.archived_at, i.cover_file_id, p.key`

// listIssues returns one page and the offset of the next page (0 when done).
func (m *Module) listIssues(ctx context.Context, f issueFilter) ([]api.Issue, int, error) {
	var where []string
	var args []any
	add := func(cond string, a ...any) {
		where = append(where, cond)
		args = append(args, a...)
	}
	if f.ProjectID != nil {
		add("i.project_id = ?", *f.ProjectID)
	} else {
		add("p.archived_at IS NULL")
	}
	if f.BoardID != nil {
		add("i.board_id = ?", *f.BoardID)
	}
	if !f.Archived {
		add("i.archived_at IS NULL")
	}
	if len(f.Statuses) > 0 {
		marks := make([]string, len(f.Statuses))
		for i, s := range f.Statuses {
			if !validStatus(s) {
				return nil, 0, httpx.Invalid("状态不正确: " + s)
			}
			marks[i] = "?"
			args = append(args, s)
		}
		where = append(where, "i.status IN ("+strings.Join(marks, ", ")+")")
	}
	if f.Priority != nil {
		add("i.priority = ?", *f.Priority)
	}
	if f.LabelID != nil {
		add("i.id IN (SELECT issue_id FROM issue_labels WHERE label_id = ?)", *f.LabelID)
	}
	if f.MilestoneID != nil {
		add("i.milestone_id = ?", *f.MilestoneID)
	}
	if f.CategoryID != nil {
		if *f.CategoryID == 0 {
			add("i.category_id IS NULL")
		} else {
			add("i.category_id IN (SELECT id FROM project_categories WHERE id=? OR parent_id=?)", *f.CategoryID, *f.CategoryID)
		}
	}
	today := m.today()
	start := today.UTC().Format(time.RFC3339)
	tomorrow := today.AddDate(0, 0, 1).UTC().Format(time.RFC3339)
	switch f.Due {
	case "":
	case "today":
		add("i.due_at >= ? AND i.due_at < ?", start, tomorrow)
	case "week":
		add("i.due_at >= ? AND i.due_at < ?", start, today.AddDate(0, 0, 7).UTC().Format(time.RFC3339))
	case "overdue":
		add("i.due_at < ? AND i.status NOT IN ('done', 'canceled')", m.now().UTC().Format(time.RFC3339))
	default:
		return nil, 0, httpx.Invalid("due 只能是 today、week 或 overdue")
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		cond := `i.title LIKE ? ESCAPE '\'`
		a := []any{"%" + escapeLike(q) + "%"}
		if pk, n, err := parseIssueKey(q); err == nil {
			cond += " OR (p.key = ? AND i.number = ?)"
			a = append(a, pk, n)
		} else if n, err := strconv.ParseInt(strings.TrimPrefix(q, "#"), 10, 64); err == nil {
			cond += " OR i.number = ?"
			a = append(a, n)
		}
		add("("+cond+")", a...)
	}
	order := "i.updated_at DESC, i.id DESC"
	switch f.Sort {
	case "", "updated":
	case "priority":
		order = "CASE i.priority WHEN 0 THEN 5 ELSE i.priority END, i.updated_at DESC, i.id DESC"
	case "due":
		order = "i.due_at IS NULL, i.due_at, CASE i.priority WHEN 0 THEN 5 ELSE i.priority END, i.id"
	case "manual":
		order = "i.sort_order, i.id"
	default:
		return nil, 0, httpx.Invalid("sort 不正确")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	query := "SELECT " + issueColumns + " FROM issues i JOIN projects p ON p.id = i.project_id WHERE " +
		strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, limit+1, f.Offset)
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []issueRow
	for rows.Next() {
		var r issueRow
		i := &r.Issue
		if err := rows.Scan(&i.ID, &i.ProjectID, &i.Number, &i.Title, &i.Description, &i.Status, &i.Priority,
			&i.DueDate, &i.MilestoneID, &i.SortOrder, &i.ExternalSource, &i.ExternalID, &i.CreatedAt, &i.UpdatedAt,
			&i.CompletedAt, &i.CategoryID, &i.DueAt, &i.DueRemind, &i.DueNotifiedAt,
			&i.BoardID, &i.ListID, &i.ArchivedAt, &i.CoverFileID, &r.ProjectKey); err != nil {
			return nil, 0, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	next := 0
	if len(list) > limit {
		list = list[:limit]
		next = f.Offset + limit
	}
	out, err := toIssues(ctx, m.q, list)
	return out, next, err
}

// escapeLike escapes LIKE wildcards; use with ESCAPE '\'.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
