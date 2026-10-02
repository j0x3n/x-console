package backup

import (
	"context"
	"errors"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func (m *Module) withStableStorage(ctx context.Context, fn func(context.Context) error) error {
	if m.d.Registry == nil {
		return fn(ctx)
	}
	if service, ok := module.Lookup[contracts.MaintenanceStorage](m.d.Registry, contracts.MaintenanceStorageKey); ok {
		return service.WithCleanup(ctx, fn)
	}
	return fn(ctx)
}

func targetUnconfigured(err error) bool {
	var h *httpx.Error
	return errors.As(err, &h) && h.Status == http.StatusPreconditionFailed && h.Code == "integration_not_configured"
}
