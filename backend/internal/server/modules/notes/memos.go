package notes

import (
	"context"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

func (m *Module) noteCounts(ctx context.Context) (api.NoteCounts, error) {
	var out api.NoteCounts
	err := m.d.DB.QueryRowContext(ctx, `SELECT
	COALESCE(sum(CASE WHEN kind='note' AND archived_at IS NULL THEN 1 ELSE 0 END),0),
	COALESCE(sum(CASE WHEN kind='memo' AND archived_at IS NULL THEN 1 ELSE 0 END),0),
	COALESCE(sum(CASE WHEN kind='note' AND pinned=1 AND archived_at IS NULL THEN 1 ELSE 0 END),0),
	COALESCE(sum(CASE WHEN archived_at IS NOT NULL THEN 1 ELSE 0 END),0)
	FROM notes WHERE hidden=0`).Scan(&out.Notes, &out.Memos, &out.Pinned, &out.Archived)
	return out, err
}

func (m *Module) GetNoteCounts(w http.ResponseWriter, r *http.Request) {
	out, err := m.noteCounts(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
