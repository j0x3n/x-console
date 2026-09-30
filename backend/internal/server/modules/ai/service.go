package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

func (m *Module) Available(ctx context.Context) bool {
	_, err := m.resolveLLM(ctx, "fast")
	return err == nil
}

func (m *Module) CompleteText(ctx context.Context, purpose, system, user string) (string, error) {
	result, err := m.llm.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}})
	text := strings.TrimSpace(result.Text)
	if err == nil && result.Truncated {
		if text == "" {
			return "", errTruncated
		}
		text += "\n（回复达到长度上限，后面被截断了）"
	}
	return text, err
}

var errTruncated = errors.New("模型回复达到长度上限，被截断了，没有拿到结果")

func (m *Module) CompleteJSON(ctx context.Context, purpose, system, user string, schema json.RawMessage, out any) error {
	result, err := m.llm.Complete(ctx, llm.Request{Purpose: purpose, System: system, Messages: []llm.Message{{Role: "user", Content: user}}, JSONSchema: schema})
	if err != nil {
		return err
	}
	if result.Truncated {
		return errTruncated
	}
	return json.Unmarshal([]byte(result.Text), out)
}
