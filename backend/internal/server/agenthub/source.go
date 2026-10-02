package agenthub

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

type sourceIPKey struct{}

func (h *Hub) SetTrustedProxies(prefixes []netip.Prefix) {
	h.mu.Lock()
	h.trustedProxies = append([]netip.Prefix(nil), prefixes...)
	h.mu.Unlock()
}

func (h *Hub) RequestIP(r *http.Request) string {
	h.mu.RLock()
	trusted := append([]netip.Prefix(nil), h.trustedProxies...)
	h.mu.RUnlock()
	return httpx.ClientIP(r, trusted)
}

func (h *Hub) SourceIP(id string) string { h.mu.RLock(); defer h.mu.RUnlock(); return h.sourceIPs[id] }

func (h *Hub) ServeWhoami(w http.ResponseWriter, r *http.Request) {
	value := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(value, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	agent, err := h.q.GetAgentByTokenHash(r.Context(), secrets.Hash(token))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	ip := h.RequestIP(r)
	if ip == "" {
		httpx.Fail(w, r, httpx.Invalid("连接来源地址无效"))
		return
	}
	h.mu.Lock()
	h.sourceIPs[agent.ID] = ip
	h.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]string{"ip": ip})
}
