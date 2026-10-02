package projects

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

// decodePatch decodes a JSON body into v and also reports which fields were
// sent as an explicit null, so PATCH can tell "clear" from "unchanged".
func decodePatch(r *http.Request, v any) (nulls map[string]bool, err error) {
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		return nil, httpx.Invalid("请求体太大")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return nil, httpx.Invalid("请求体格式不正确: " + err.Error())
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	nulls = map[string]bool{}
	for k, val := range fields {
		if string(bytes.TrimSpace(val)) == "null" {
			nulls[k] = true
		}
	}
	return nulls, nil
}

func (m *Module) ListProjects(w http.ResponseWriter, r *http.Request, params api.ListProjectsParams) {
	out, err := m.listProjects(r.Context(), deref(params.Archived))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateProject(w http.ResponseWriter, r *http.Request) {
	var body api.CreateProject
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createProject(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) GetProject(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	out, err := m.getProject(r.Context(), projectID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UpdateProject(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	var body api.UpdateProject
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.updateProject(r.Context(), projectID, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ArchiveProject(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	archived := true
	if _, err := m.updateProject(r.Context(), projectID, api.UpdateProject{Archived: &archived}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ListIssues(w http.ResponseWriter, r *http.Request, params api.ListIssuesParams) {
	f := issueFilter{
		ProjectID: params.ProjectId, Priority: params.Priority, LabelID: params.LabelId,
		MilestoneID: params.MilestoneId, CategoryID: params.CategoryId, Q: deref(params.Q), Limit: deref(params.Limit),
		BoardID: params.BoardId,
	}
	if params.Status != nil {
		for _, s := range *params.Status {
			f.Statuses = append(f.Statuses, string(s))
		}
	}
	if params.Due != nil {
		f.Due = string(*params.Due)
	}
	if params.Sort != nil {
		f.Sort = string(*params.Sort)
	}
	if params.Cursor != nil && *params.Cursor != "" {
		off, err := httpx.DecodeIDCursor(params.Cursor)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		f.Offset = int(off)
	}
	items, next, err := m.listIssues(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if items == nil {
		items = []api.Issue{}
	}
	out := map[string]any{"items": items}
	if next > 0 {
		out["nextCursor"] = httpx.EncodeIDCursor(int64(next))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateIssue(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	var body api.CreateIssue
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := issueInput{
		Title: body.Title, Description: deref(body.Description), Priority: deref(body.Priority),
		DueDate: fromDate(body.DueDate), DueAt: body.DueAt, CategoryID: body.CategoryId,
		MilestoneID: body.MilestoneId, LabelIDs: deref(body.LabelIds),
		BoardID: body.BoardId, ListID: body.ListId,
	}
	if body.DueRemind != nil {
		in.DueRemind = string(*body.DueRemind)
	}
	if body.Status != nil {
		in.Status = string(*body.Status)
	}
	out, err := m.createIssue(r.Context(), projectID, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) GetIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.getIssue(r.Context(), key)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UpdateIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	var body api.UpdateIssue
	nulls, err := decodePatch(r, &body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := issuePatch{
		Title: body.Title, Description: body.Description, Priority: body.Priority,
		DueDate: fromDate(body.DueDate), ClearDueDate: nulls["dueDate"], DueAt: body.DueAt, ClearDueAt: nulls["dueAt"],
		CategoryID: body.CategoryId, ClearCategory: nulls["categoryId"],
		MilestoneID: body.MilestoneId, ClearMilestone: nulls["milestoneId"], LabelIDs: body.LabelIds,
	}
	if body.DueRemind != nil {
		value := string(*body.DueRemind)
		p.DueRemind = &value
	}
	if body.Color != nil {
		value := string(*body.Color)
		p.Color = &value
	}
	if body.Status != nil {
		s := string(*body.Status)
		p.Status = &s
	}
	out, err := m.updateIssue(r.Context(), key, p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	if err := m.deleteIssue(r.Context(), key); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) MoveIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	var body api.MoveIssue
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var out api.Issue
	var err error
	switch {
	case body.ListId != nil:
		out, err = m.moveToList(r.Context(), key, *body.ListId, body.AfterKey, body.BeforeKey)
	case body.Status != nil:
		out, err = m.moveIssue(r.Context(), key, string(*body.Status), body.AfterKey, body.BeforeKey)
	default:
		err = httpx.Invalid("要传 listId 或 status")
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ListComments(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.listComments(r.Context(), key)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateComment(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	var body api.CreateComment
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createComment(r.Context(), key, body.Body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) DeleteComment(w http.ResponseWriter, r *http.Request, key api.IssueKey, commentID int64) {
	if err := m.deleteComment(r.Context(), key, commentID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ListLinks(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.listLinks(r.Context(), key)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateLink(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	var body api.CreateIssueLink
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createLink(r.Context(), key, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) DeleteLink(w http.ResponseWriter, r *http.Request, key api.IssueKey, linkID int64) {
	if err := m.deleteLink(r.Context(), key, linkID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ListLabels(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	out, err := m.listLabels(r.Context(), projectID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateLabel(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	var body api.CreateLabel
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createLabel(r.Context(), projectID, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) UpdateLabel(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, labelID int64) {
	var body api.UpdateLabel
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.updateLabel(r.Context(), projectID, labelID, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteLabel(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, labelID int64) {
	if err := m.deleteLabel(r.Context(), projectID, labelID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ListMilestones(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	out, err := m.listMilestones(r.Context(), projectID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateMilestone(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	var body api.CreateMilestone
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createMilestone(r.Context(), projectID, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) UpdateMilestone(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, milestoneID int64) {
	var body api.UpdateMilestone
	nulls, err := decodePatch(r, &body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.updateMilestone(r.Context(), projectID, milestoneID, body, nulls["dueDate"])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteMilestone(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, milestoneID int64) {
	if err := m.deleteMilestone(r.Context(), projectID, milestoneID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
