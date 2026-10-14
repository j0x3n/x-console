package ai

import (
	"context"
	"errors"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// B206: tool conversations for other modules, such as the web watcher's
// browser agent. The module keeps the loop; this only maps the types.

var _ contracts.ToolChat = toolChat{}

type toolChat struct{ m *Module }

func (c toolChat) Available(ctx context.Context, purpose string) bool {
	_, err := c.m.resolveLLM(ctx, purpose)
	return err == nil
}

func (c toolChat) Chat(ctx context.Context, purpose, system string, messages []contracts.ChatMessage, tools []contracts.ChatTool) (contracts.ChatReply, error) {
	req := llm.Request{Purpose: purpose, CacheKey: cacheKey(ctx, purpose), System: system}
	for _, msg := range messages {
		lm := llm.Message{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID}
		for _, call := range msg.ToolCalls {
			lm.ToolCalls = append(lm.ToolCalls, llm.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		for _, img := range msg.Images {
			lm.Images = append(lm.Images, llm.Image{MIME: img.MIME, Data: img.Data})
		}
		req.Messages = append(req.Messages, lm)
	}
	for _, tool := range tools {
		req.Tools = append(req.Tools, llm.Tool{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters})
	}
	res, err := c.m.llm.Complete(ctx, req)
	if err != nil {
		return contracts.ChatReply{}, err
	}
	if res.Truncated && len(res.ToolCalls) == 0 && res.Text == "" {
		return contracts.ChatReply{}, errors.New("模型回复达到长度上限，被截断了")
	}
	out := contracts.ChatReply{Text: res.Text, InputTokens: res.InputTokens, OutputTokens: res.OutputTokens}
	for _, call := range res.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, contracts.ChatToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
	}
	return out, nil
}

func (c toolChat) Spent(ctx context.Context, source, ref string, since time.Time) (float64, error) {
	q := `SELECT COALESCE(SUM(cost),0) FROM ai_usage WHERE source=? AND created_at>=?`
	args := []any{source, since.UTC()}
	if ref != "" {
		q += ` AND ref=?`
		args = append(args, ref)
	}
	var v float64
	err := c.m.d.DB.QueryRowContext(ctx, q, args...).Scan(&v)
	return v, err
}
