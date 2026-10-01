package brief

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// Endpoints that are in the contract but not built yet. See docs/specs/B58.md.

// GetQWeatherConfig is GET /weather/qweather (B58).
func (m *Module) GetQWeatherConfig(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

// PutQWeatherConfig is PUT /weather/qweather (B58).
func (m *Module) PutQWeatherConfig(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

// GetWeatherExtra is GET /weather/extra (B58).
func (m *Module) GetWeatherExtra(w http.ResponseWriter, r *http.Request, params api.GetWeatherExtraParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

// GetWeatherNotify is GET /weather/notify (B58).
func (m *Module) GetWeatherNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

// PutWeatherNotify is PUT /weather/notify (B58).
func (m *Module) PutWeatherNotify(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
