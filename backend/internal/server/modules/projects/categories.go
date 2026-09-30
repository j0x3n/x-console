package projects

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

type categoryRow struct {
	id, projectID int64
	parentID      sql.NullInt64
	name          string
	position      float64
	issueCount    int
}

func (m *Module) categoryProject(ctx context.Context, projectID int64, write bool) error {
	row, err := m.q.GetProject(ctx, projectID)
	if err != nil {
		return notFound(err)
	}
	if write && row.Project.ArchivedAt != nil {
		return httpx.NewError(409, "project_archived", "项目已归档")
	}
	return nil
}

func validCategoryName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if count := utf8.RuneCountInString(value); count < 1 || count > 40 {
		return "", httpx.Invalid("分类名需要 1 到 40 个字")
	}
	return value, nil
}

func categoryAPI(row categoryRow) api.ProjectCategory {
	out := api.ProjectCategory{Id: row.id, ProjectId: row.projectID, Name: row.name, Position: row.position, IssueCount: row.issueCount}
	if row.parentID.Valid {
		out.ParentId = &row.parentID.Int64
	}
	return out
}

func (m *Module) category(ctx context.Context, projectID, categoryID int64) (categoryRow, error) {
	var row categoryRow
	err := m.d.DB.QueryRowContext(ctx, "SELECT id,project_id,parent_id,name,position,(SELECT count(*) FROM issues WHERE category_id=project_categories.id) FROM project_categories WHERE project_id=? AND id=?", projectID, categoryID).Scan(&row.id, &row.projectID, &row.parentID, &row.name, &row.position, &row.issueCount)
	return row, notFound(err)
}

func (m *Module) listCategories(ctx context.Context, projectID int64) ([]api.ProjectCategory, error) {
	rows, err := m.d.DB.QueryContext(ctx, `SELECT c.id,c.project_id,c.parent_id,c.name,c.position,count(i.id)
FROM project_categories c LEFT JOIN issues i ON i.category_id=c.id
WHERE c.project_id=? GROUP BY c.id ORDER BY c.position,c.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roots := []api.ProjectCategory{}
	children := map[int64][]api.ProjectCategory{}
	for rows.Next() {
		var row categoryRow
		if err = rows.Scan(&row.id, &row.projectID, &row.parentID, &row.name, &row.position, &row.issueCount); err != nil {
			return nil, err
		}
		value := categoryAPI(row)
		if row.parentID.Valid {
			children[row.parentID.Int64] = append(children[row.parentID.Int64], value)
		} else {
			roots = append(roots, value)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]api.ProjectCategory, 0, len(roots))
	for _, root := range roots {
		out = append(out, root)
		out = append(out, children[root.Id]...)
	}
	return out, nil
}

func (m *Module) ListProjectCategories(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	if err := m.categoryProject(r.Context(), projectID, false); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.listCategories(r.Context(), projectID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) CreateProjectCategory(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	ctx := r.Context()
	var body api.CreateProjectCategoryJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, err := validCategoryName(body.Name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = m.categoryProject(ctx, projectID, true); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.ParentId != nil {
		parent, e := m.category(ctx, projectID, *body.ParentId)
		if e != nil || parent.parentID.Valid {
			httpx.Fail(w, r, httpx.NewError(400, "invalid_parent", "父分类必须是当前项目的一级分类"))
			return
		}
	}
	var parent any
	if body.ParentId != nil {
		parent = *body.ParentId
	}
	var position float64
	err = m.d.DB.QueryRowContext(ctx, "SELECT COALESCE(max(position),0)+? FROM project_categories WHERE project_id=? AND parent_id IS ?", sortGap, projectID, parent).Scan(&position)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var id int64
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO project_categories(project_id,parent_id,name,position) VALUES(?,?,?,?) RETURNING id", projectID, parent, name, position).Scan(&id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.category(ctx, projectID, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("project_category.changed", map[string]any{"projectId": projectID})
	m.d.Audit.Record(ctx, "project.category.create", strings.TrimSpace(name), map[string]any{"projectId": projectID, "categoryId": id}, nil)
	httpx.JSON(w, 201, categoryAPI(row))
}

func (m *Module) UpdateProjectCategory(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, categoryID int64) {
	ctx := r.Context()
	var body api.UpdateProjectCategoryJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.AfterId != nil && body.BeforeId != nil && *body.AfterId == *body.BeforeId {
		httpx.Fail(w, r, httpx.Invalid("排序位置无效"))
		return
	}
	if err := m.categoryProject(ctx, projectID, true); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.category(ctx, projectID, categoryID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Name != nil {
		row.name, err = validCategoryName(*body.Name)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if body.AfterId != nil || body.BeforeId != nil {
		var parent any
		if row.parentID.Valid {
			parent = row.parentID.Int64
		}
		rows, e := m.d.DB.QueryContext(ctx, "SELECT id,position FROM project_categories WHERE project_id=? AND parent_id IS ? AND id<>? ORDER BY position,id", projectID, parent, categoryID)
		if e != nil {
			httpx.Fail(w, r, e)
			return
		}
		type sibling struct {
			id       int64
			position float64
		}
		others := []sibling{}
		for rows.Next() {
			var x sibling
			if e = rows.Scan(&x.id, &x.position); e != nil {
				break
			}
			others = append(others, x)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			httpx.Fail(w, r, e)
			return
		}
		index := -1
		if body.AfterId != nil {
			for i, x := range others {
				if x.id == *body.AfterId {
					index = i + 1
					break
				}
			}
		}
		if body.BeforeId != nil {
			before := -1
			for i, x := range others {
				if x.id == *body.BeforeId {
					before = i
					break
				}
			}
			if before < 0 || index >= 0 && index != before {
				httpx.Fail(w, r, httpx.Invalid("分类不在同一级或排序位置无效"))
				return
			}
			index = before
		}
		if index < 0 {
			httpx.Fail(w, r, httpx.Invalid("分类不在同一级"))
			return
		}
		var prev, next *float64
		if index > 0 {
			prev = &others[index-1].position
		}
		if index < len(others) {
			next = &others[index].position
		}
		if value, ok := between(prev, next); ok {
			row.position = value
		} else {
			ids := make([]int64, 0, len(others)+1)
			for _, x := range others {
				ids = append(ids, x.id)
			}
			ids = slices.Insert(ids, index, categoryID)
			tx, e := m.d.DB.BeginTx(ctx, nil)
			if e != nil {
				httpx.Fail(w, r, e)
				return
			}
			defer tx.Rollback()
			for i, id := range ids {
				if _, e = tx.ExecContext(ctx, "UPDATE project_categories SET position=? WHERE id=?", float64(i+1)*sortGap, id); e != nil {
					httpx.Fail(w, r, e)
					return
				}
			}
			if _, e = tx.ExecContext(ctx, "UPDATE project_categories SET name=? WHERE id=?", row.name, categoryID); e != nil {
				httpx.Fail(w, r, e)
				return
			}
			if e = tx.Commit(); e != nil {
				httpx.Fail(w, r, e)
				return
			}
			row.position = float64(index+1) * sortGap
			m.d.Bus.Publish("project_category.changed", map[string]any{"projectId": projectID})
			httpx.JSON(w, 200, categoryAPI(row))
			return
		}
	}
	_, err = m.d.DB.ExecContext(ctx, "UPDATE project_categories SET name=?,position=? WHERE id=?", row.name, row.position, categoryID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("project_category.changed", map[string]any{"projectId": projectID})
	httpx.JSON(w, 200, categoryAPI(row))
}

func (m *Module) DeleteProjectCategory(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, categoryID int64) {
	ctx := r.Context()
	if err := m.categoryProject(ctx, projectID, true); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.category(ctx, projectID, categoryID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE issues SET category_id=NULL WHERE category_id IN (SELECT id FROM project_categories WHERE id=? OR parent_id=?)", categoryID, categoryID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM project_categories WHERE project_id=? AND id=?", projectID, categoryID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("project_category.changed", map[string]any{"projectId": projectID})
	m.d.Audit.Record(ctx, "project.category.delete", strings.TrimSpace(r.URL.Path), map[string]any{"projectId": projectID, "categoryId": categoryID}, nil)
	httpx.NoContent(w)
}
