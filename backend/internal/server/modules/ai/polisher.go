package ai

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

func (m *Module) Polish(ctx context.Context, markdown string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ctx = contracts.WithAIUsage(ctx, "brief", "")
	return m.CompleteText(ctx, "fast", "用两三句中文总结早报，只返回总结。", markdown)
}
