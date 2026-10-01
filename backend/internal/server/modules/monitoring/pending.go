package monitoring

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
)

// Endpoints that are in the contract but not built yet. See docs/specs/B50.md.

// GetMonitorIcon is GET /monitors/{monitorId}/icon (B50).
func (m *Module) GetMonitorIcon(w http.ResponseWriter, r *http.Request, monitorID api.MonitorId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
