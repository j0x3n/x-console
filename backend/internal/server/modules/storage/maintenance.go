package storage

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) Stat(ctx context.Context, scope, key string) (files.Info, error) {
	m.mu.Lock()
	raw := m.raw
	m.mu.Unlock()
	return files.Scoped(raw, scope).Stat(ctx, key)
}

func (m *Module) Location() string { m.mu.Lock(); defer m.mu.Unlock(); return string(m.backend) }

func (m *Module) WithCleanup(ctx context.Context, fn func(context.Context) error) error {
	m.mu.Lock()
	if m.move != nil || m.cleaning {
		m.mu.Unlock()
		return httpx.ErrConflict
	}
	m.cleaning = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.cleaning = false; m.usage.clear(); m.mu.Unlock() }()
	return fn(ctx)
}

var _ contracts.MaintenanceStorage = (*Module)(nil)
