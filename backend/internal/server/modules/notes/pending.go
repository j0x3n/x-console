package notes

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) GetNoteCounts(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
