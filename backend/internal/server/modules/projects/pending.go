package projects

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// These endpoints are in the contract but not built yet. See docs/specs/B84.md.

func (m *Module) BindBoardRepo(w http.ResponseWriter, r *http.Request, boardID int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UnbindBoardRepo(w http.ResponseWriter, r *http.Request, boardID int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) SyncBoardRepo(w http.ResponseWriter, r *http.Request, boardID int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
