package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func hiddenGate(d *module.Deps, backend string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h, ok := module.Lookup[contracts.HiddenModules](d.Registry, contracts.HiddenModulesKey)
			if ok && contracts.BackendBlocked(r.Context(), h, backend, r.URL.Path) {
				httpx.Fail(w, r, httpx.ErrNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (a *App) allowEvent(ctx context.Context, topic string, data any) bool {
	if !contracts.EventRelevant(topic) {
		return true
	}
	h, ok := module.Lookup[contracts.HiddenModules](a.Deps.Registry, contracts.HiddenModulesKey)
	if !ok {
		return true
	}
	ctx = a.Deps.Auth.FreshVault(ctx)
	if strings.HasPrefix(topic, "host.") {
		return !a.hostEventHidden(ctx, h, data)
	}
	return !contracts.EventHidden(ctx, h, topic)
}

func (a *App) hostEventHidden(ctx context.Context, h contracts.HiddenModules, data any) bool {
	id := eventHostID(data)
	if id == "" {
		return false
	}
	kind, ok := a.hostKind(ctx, id)
	if !ok {
		return false
	}
	switch kind {
	case "desktop":
		return h.Hidden(ctx, "pc")
	case "server":
		return h.Hidden(ctx, "servers")
	default:
		return false
	}
}

func eventHostID(data any) string {
	raw, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	var v struct {
		HostID string `json:"hostId"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return v.HostID
}

func (a *App) hostKind(ctx context.Context, id string) (string, bool) {
	a.kindMu.Lock()
	if a.kinds != nil && time.Since(a.kindAt) < time.Second {
		kind, ok := a.kinds[id]
		a.kindMu.Unlock()
		return kind, ok
	}
	a.kindMu.Unlock()
	hosts, ok := module.Lookup[contracts.Hosts](a.Deps.Registry, contracts.HostsKey)
	if !ok {
		return "", false
	}
	list, err := hosts.Summaries(ctx)
	if err != nil {
		return "", false
	}
	kinds := make(map[string]string, len(list))
	for _, h := range list {
		kinds[h.ID] = h.Kind
	}
	a.kindMu.Lock()
	a.kinds = kinds
	a.kindAt = time.Now()
	kind, found := kinds[id]
	a.kindMu.Unlock()
	return kind, found
}
