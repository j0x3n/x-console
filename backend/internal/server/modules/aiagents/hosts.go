package aiagents

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// B60: the machines an agent may operate. The panel AI only works on a
// machine through an agent bound to it.

func hostIDsOf(raw string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// hostNames maps host id to name for every machine that exists now.
func (m *Module) hostNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	hosts, ok := module.Lookup[contracts.Hosts](m.d.Registry, contracts.HostsKey)
	if !ok {
		return out
	}
	list, err := hosts.Summaries(ctx)
	if err != nil {
		return out
	}
	for _, h := range list {
		out[h.ID] = h.Name
	}
	return out
}

// liveHosts drops machines that were deleted since they were bound.
func liveHosts(ids []string, names map[string]string) []string {
	out := []string{}
	for _, id := range ids {
		if _, ok := names[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// checkHosts validates f.hostIDs. Binding a new machine lets the panel AI
// run commands there, so it needs a fresh verification.
func (m *Module) checkHosts(ctx context.Context, f *fields, before *fields) error {
	slices.Sort(f.hostIDs)
	f.hostIDs = slices.Compact(f.hostIDs)
	if len(f.hostIDs) > 50 {
		return httpx.Invalid("最多绑定 50 台机器")
	}
	names := m.hostNames(ctx)
	var old []string
	if before != nil {
		old = before.hostIDs
	}
	kept := []string{}
	added := false
	for _, id := range f.hostIDs {
		switch {
		case names[id] != "":
			kept = append(kept, id)
			if !slices.Contains(old, id) {
				added = true
			}
		case slices.Contains(old, id):
			// The machine was deleted since: drop it quietly.
		default:
			return httpx.Invalid("机器 " + id + " 不存在")
		}
	}
	f.hostIDs = kept
	raw, _ := json.Marshal(f.hostIDs)
	f.HostIds = string(raw)
	if added {
		return auth.RequireElevated(ctx)
	}
	return nil
}

// ForHost implements contracts.AIAgents.
func (m *Module) ForHost(ctx context.Context, hostID string) ([]contracts.AIAgent, error) {
	rows, err := m.q.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := []contracts.AIAgent{}
	for _, a := range rows {
		if a.Enabled != 1 || !slices.Contains(hostIDsOf(a.HostIds), hostID) {
			continue
		}
		x, err := m.Get(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}
