package files

import "context"

func (m *Module) CleanupForTest(ctx context.Context) error { return m.cleanup(ctx) }
