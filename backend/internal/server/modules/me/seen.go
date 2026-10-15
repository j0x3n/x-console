package me

// B164: the phone's panel calls POST /me/seen every minute while it is open.
// Two minutes without a call and the phone no longer counts as in use.

import (
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
)

const phoneActiveFor = 2 * time.Minute

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// MeSeen implements api.ServerInterface.
func (m *Module) MeSeen(w http.ResponseWriter, r *http.Request) {
	var body api.MeSeenJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !body.Device.Valid() {
		httpx.Fail(w, r, httpx.Invalid("device 只能是 phone"))
		return
	}
	m.mu.Lock()
	m.phoneSeen = m.nowLocked()
	err := m.recomputeLocked(r.Context())
	m.mu.Unlock()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
