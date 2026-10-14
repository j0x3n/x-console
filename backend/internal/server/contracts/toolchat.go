package contracts

import (
	"context"
	"encoding/json"
	"time"
)

// ToolChatKey is where the ai module provides ToolChat (B206).
const ToolChatKey = "ai.toolchat"

// ChatImage is a picture sent with a user message.
type ChatImage struct {
	MIME string
	Data []byte
}

// ChatToolCall is a tool call the model asked for.
type ChatToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ChatMessage is one message of a tool conversation. Role is "user",
// "assistant" or "tool". A tool message answers ToolCallID.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ChatToolCall
	Images     []ChatImage
}

// ChatTool describes a tool with a JSON schema for its arguments.
type ChatTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ChatReply is the model's answer.
type ChatReply struct {
	Text         string
	ToolCalls    []ChatToolCall
	InputTokens  int64
	OutputTokens int64
}

// ToolChat lets a module run its own tools with the configured model, for
// example a browser agent (B206). Calls are recorded in the AI usage like
// any other; mark them with WithAIUsage.
type ToolChat interface {
	// Available reports whether the model for purpose ("agent" or "fast")
	// is configured.
	Available(ctx context.Context, purpose string) bool
	// Chat sends one turn. The caller runs the tool calls and sends the
	// results back in the next turn.
	Chat(ctx context.Context, purpose, system string, messages []ChatMessage, tools []ChatTool) (ChatReply, error)
	// Spent returns the AI cost in USD recorded since since for source, and
	// for one ref when ref is not empty. Calls of models without a price
	// count as 0.
	Spent(ctx context.Context, source, ref string, since time.Time) (float64, error)
}
