package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

func (m *Module) Available(ctx context.Context) bool {
	_, err := m.resolveLLM(ctx, "fast")
	return err == nil
}

func (m *Module) CompleteText(ctx context.Context, purpose, system, user string) (string, error) {
	result, err := m.llm.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}})
	return strings.TrimSpace(result.Text), err
}

func (m *Module) CompleteJSON(ctx context.Context, purpose, system, user string, schema json.RawMessage, out any) error {
	result, err := m.llm.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}, JSONSchema: schema})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(result.Text), out)
}
