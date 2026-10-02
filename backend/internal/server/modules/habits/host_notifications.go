package habits

import (
	"context"
	"slices"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func (m *Module) visibleReminderHosts(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	service, ok := module.Lookup[contracts.Hosts](m.d.Registry, contracts.HostsKey)
	if !ok {
		return out, nil
	}
	hosts, err := service.Summaries(ctx)
	if err != nil {
		return nil, err
	}
	hidden, hasHidden := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	for _, host := range hosts {
		side := "servers"
		if host.Kind == "desktop" {
			side = "pc"
		}
		if hasHidden && hidden.Hidden(ctx, side) {
			continue
		}
		out[host.ID] = true
	}
	return out, nil
}

func (m *Module) sendHostReminder(ctx context.Context, ids []string, title, body string) {
	visible, err := m.visibleReminderHosts(ctx)
	if err != nil {
		m.d.Log.Warn("habit system notification hosts", "err", err)
		return
	}
	sent := []string{}
	for _, id := range ids {
		if !visible[id] || slices.Contains(sent, id) {
			continue
		}
		agent, err := m.d.Agents.Get(ctx, id)
		if err != nil || !agent.Online || !agent.Has("notify.show") {
			continue
		}
		sent = append(sent, id)
		callCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err = m.d.Agents.Call(callCtx, id, "notify.show", struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}{title, body}, nil)
		cancel()
		if err != nil {
			m.d.Log.Warn("habit system notification", "host", id, "err", err)
		}
	}
}
