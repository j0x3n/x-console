package hosts

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func canonicalHostKind(kind string) string {
	if kind == "pc" {
		return "desktop"
	}
	return kind
}

func (m *Module) kindHidden(ctx context.Context, kind string) bool {
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	if !ok {
		return false
	}
	switch canonicalHostKind(kind) {
	case "desktop":
		return h.Hidden(ctx, "pc")
	case "server":
		return h.Hidden(ctx, "servers")
	default:
		return false
	}
}

func (m *Module) alertVisible(ctx context.Context, hostID string) bool {
	ref, err := m.lookupHost(ctx, hostID)
	if err != nil {
		return true
	}
	return !m.kindHidden(ctx, ref.Kind)
}
