package ai

import (
	"context"
	"time"
)

func (m *Module) Polish(ctx context.Context, markdown string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return m.CompleteText(ctx, "fast", "用两三句中文总结早报，只返回总结。", markdown)
}
