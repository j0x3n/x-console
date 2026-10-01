package projects

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

// B46 看板、列表和卡片的接口。

func writeOr(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}

func (m *Module) ListBoards(w http.ResponseWriter, r *http.Request, projectID api.ProjectId, params api.ListBoardsParams) {
	out, err := m.listBoards(r.Context(), projectID, deref(params.Archived))
	writeOr(w, r, http.StatusOK, out, err)
}

func (m *Module) CreateBoard(w http.ResponseWriter, r *http.Request, projectID api.ProjectId) {
	var body api.CreateBoard
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createBoard(r.Context(), projectID, body)
	writeOr(w, r, http.StatusCreated, out, err)
}

func (m *Module) ListStarredBoards(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.StarredBoards(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type item struct {
		ID          int64  `json:"id"`
		ProjectID   int64  `json:"projectId"`
		ProjectKey  string `json:"projectKey"`
		ProjectName string `json:"projectName"`
		Name        string `json:"name"`
		Icon        string `json:"icon"`
	}
	out := make([]item, 0, len(rows))
	for _, b := range rows {
		out = append(out, item{b.ID, b.ProjectID, b.ProjectKey, b.ProjectName, b.Name, b.Icon})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UpdateBoard(w http.ResponseWriter, r *http.Request, boardID int64) {
	var body api.UpdateBoard
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.updateBoard(r.Context(), boardID, body)
	writeOr(w, r, http.StatusOK, out, err)
}

func (m *Module) DeleteBoard(w http.ResponseWriter, r *http.Request, boardID int64) {
	if err := m.deleteBoard(r.Context(), boardID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) CopyBoard(w http.ResponseWriter, r *http.Request, boardID int64) {
	var body struct {
		Name *string `json:"name"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	out, err := m.copyBoard(r.Context(), boardID, deref(body.Name))
	writeOr(w, r, http.StatusCreated, out, err)
}

func (m *Module) CreateBoardList(w http.ResponseWriter, r *http.Request, boardID int64) {
	var body api.CreateBoardList
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createList(r.Context(), boardID, body)
	writeOr(w, r, http.StatusCreated, out, err)
}

func (m *Module) ListBoardArchive(w http.ResponseWriter, r *http.Request, boardID int64) {
	issues, lists, err := m.boardArchive(r.Context(), boardID)
	writeOr(w, r, http.StatusOK, map[string]any{"issues": issues, "lists": lists}, err)
}

func (m *Module) UpdateBoardList(w http.ResponseWriter, r *http.Request, listID int64) {
	var body api.UpdateBoardList
	nulls, err := decodePatch(r, &body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.updateList(r.Context(), listID, body, nulls)
	writeOr(w, r, http.StatusOK, out, err)
}

func (m *Module) DeleteBoardList(w http.ResponseWriter, r *http.Request, listID int64) {
	if err := m.deleteList(r.Context(), listID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) ArchiveListCards(w http.ResponseWriter, r *http.Request, listID int64) {
	n, err := m.archiveListCards(r.Context(), listID)
	writeOr(w, r, http.StatusOK, map[string]int64{"archived": n}, err)
}

func (m *Module) MoveListCards(w http.ResponseWriter, r *http.Request, listID int64) {
	var body struct {
		ToListID int64 `json:"toListId"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	n, err := m.moveListCards(r.Context(), listID, body.ToListID)
	writeOr(w, r, http.StatusOK, map[string]int{"moved": n}, err)
}

func (m *Module) ArchiveIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.setArchived(r.Context(), key, true)
	writeOr(w, r, http.StatusOK, out, err)
}

func (m *Module) RestoreIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.setArchived(r.Context(), key, false)
	writeOr(w, r, http.StatusOK, out, err)
}

func (m *Module) CopyIssue(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.copyIssue(r.Context(), key)
	writeOr(w, r, http.StatusCreated, out, err)
}

func (m *Module) SetIssueMembers(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	var body struct {
		Members []api.IssueMember `json:"members"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.setMembers(r.Context(), key, body.Members)
	writeOr(w, r, http.StatusOK, out, err)
}

func (m *Module) ListIssueActivity(w http.ResponseWriter, r *http.Request, key api.IssueKey) {
	out, err := m.listActivity(r.Context(), key)
	writeOr(w, r, http.StatusOK, out, err)
}
