package coding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Limits of one normalized event.
const (
	maxEventText  = 16 << 10
	maxToolResult = 4 << 10
	maxDataBytes  = 8 << 10
)

// parser turns one output line into zero or more events.
type parser interface {
	line(b []byte) []protocol.CodingEvent
}

func newParser(format string) parser {
	if format == protocol.ExecutorCodex {
		return codexParser{}
	}
	return claudeParser{}
}

// ParseLine normalizes one output line of the given format. Exported for
// tests and tools.
func ParseLine(format string, b []byte) []protocol.CodingEvent {
	return newParser(format).line(b)
}

func event(kind, text string, data any) protocol.CodingEvent {
	ev := protocol.CodingEvent{Kind: kind, Text: truncate(text, maxEventText), At: time.Now().UTC()}
	if data != nil {
		ev.Data = marshalData(data)
	}
	return ev
}

// plain is a line that is not JSON.
func plain(b []byte) []protocol.CodingEvent {
	s := strings.TrimRight(string(bytes.ToValidUTF8(b, []byte("?"))), "\r\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []protocol.CodingEvent{event(protocol.CodingEventText, s, nil)}
}

// marshalData encodes v; oversized values become a short preview.
func marshalData(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if len(raw) <= maxDataBytes {
		return raw
	}
	preview, _ := json.Marshal(map[string]any{"truncated": true, "preview": truncate(string(raw), 2000)})
	return preview
}

// truncate cuts s to at most n bytes on a rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// clip keeps the input of a tool call small: long strings are shortened.
func clip(v any) any {
	switch x := v.(type) {
	case string:
		return truncate(x, 2000)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = clip(val)
		}
		return out
	case []any:
		out := make([]any, 0, min(len(x), 50))
		for i, val := range x {
			if i >= 50 {
				break
			}
			out = append(out, clip(val))
		}
		return out
	}
	return v
}

// toolSummary is the one-line title of a tool call, for example
// "Edit: src/main.go" or "Bash: go test ./...".
func toolSummary(name string, input map[string]any) string {
	for _, k := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := input[k].(string); ok && strings.TrimSpace(v) != "" {
			line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(v), "\n", 2)[0])
			return name + ": " + truncate(line, 200)
		}
	}
	return name
}

// ---- Claude Code: --output-format stream-json ----

type claudeParser struct{}

type claudeLine struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype"`
	Model     string  `json:"model"`
	SessionID string  `json:"session_id"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	NumTurns  int     `json:"num_turns"`
	CostUSD   float64 `json:"total_cost_usd"`
	Duration  int64   `json:"duration_ms"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     map[string]any  `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (claudeParser) line(b []byte) []protocol.CodingEvent {
	var l claudeLine
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, &l); err != nil || l.Type == "" {
		return plain(b)
	}
	switch l.Type {
	case "system":
		if l.Subtype == "init" {
			return []protocol.CodingEvent{event(protocol.CodingEventStatus, "session started",
				map[string]any{"code": "session_started", "model": l.Model, "sessionId": l.SessionID})}
		}
		return nil // hooks, compaction and other housekeeping
	case "assistant", "user":
		return claudeBlocks(l.Message.Content)
	case "result":
		data := map[string]any{"code": "result", "subtype": l.Subtype, "isError": l.IsError,
			"turns": l.NumTurns, "costUsd": l.CostUSD, "durationMs": l.Duration}
		if l.IsError {
			text := l.Result
			if text == "" {
				text = l.Subtype
			}
			return []protocol.CodingEvent{event(protocol.CodingEventError, text, data)}
		}
		return []protocol.CodingEvent{event(protocol.CodingEventStatus, "finished", data)}
	case "stream_event":
		return nil // partial messages; the full message follows
	}
	return []protocol.CodingEvent{event(protocol.CodingEventStatus, l.Type, map[string]any{"code": l.Type})}
}

func claudeBlocks(raw json.RawMessage) []protocol.CodingEvent {
	var blocks []claudeBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		var s string
		if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
			return []protocol.CodingEvent{event(protocol.CodingEventText, s, nil)}
		}
		return nil
	}
	var out []protocol.CodingEvent
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			if strings.TrimSpace(bl.Text) != "" {
				out = append(out, event(protocol.CodingEventText, bl.Text, nil))
			}
		case "tool_use":
			out = append(out, event(protocol.CodingEventTool, toolSummary(bl.Name, bl.Input),
				map[string]any{"id": bl.ID, "name": bl.Name, "input": clip(bl.Input)}))
		case "tool_result":
			out = append(out, event(protocol.CodingEventTool, truncate(toolResultText(bl.Content), maxToolResult),
				map[string]any{"id": bl.ToolUseID, "result": true, "isError": bl.IsError}))
		}
	}
	return out
}

// toolResultText flattens a tool_result content (a string or text blocks).
func toolResultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			} else if b.Type != "" {
				parts = append(parts, "["+b.Type+"]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

// ---- Codex: codex exec --json ----

type codexParser struct{}

type codexItem struct {
	ID               string         `json:"id"`
	Type             string         `json:"type"` // older releases: item_type
	ItemType         string         `json:"item_type"`
	Text             string         `json:"text"`
	Command          string         `json:"command"`
	AggregatedOutput string         `json:"aggregated_output"`
	ExitCode         *int           `json:"exit_code"`
	Status           string         `json:"status"`
	Changes          []any          `json:"changes"`
	Server           string         `json:"server"`
	Tool             string         `json:"tool"`
	Query            string         `json:"query"`
	Message          string         `json:"message"`
	Items            []any          `json:"items"`
	Arguments        map[string]any `json:"arguments"`
}

type codexLine struct {
	Type     string         `json:"type"`
	ThreadID string         `json:"thread_id"`
	Item     *codexItem     `json:"item"`
	Usage    map[string]any `json:"usage"`
	Message  string         `json:"message"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
	// Releases before the item/thread events wrapped everything in "msg".
	Msg *struct {
		Type    string   `json:"type"`
		Message string   `json:"message"`
		Command []string `json:"command"`
		CallID  string   `json:"call_id"`
		Stdout  string   `json:"stdout"`
		Stderr  string   `json:"stderr"`
		Exit    *int     `json:"exit_code"`
	} `json:"msg"`
}

func (codexParser) line(b []byte) []protocol.CodingEvent {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	var l codexLine
	if err := json.Unmarshal(b, &l); err != nil {
		return plain(b)
	}
	if l.Msg != nil && l.Type == "" {
		return codexLegacy(l)
	}
	switch l.Type {
	case "thread.started":
		return []protocol.CodingEvent{event(protocol.CodingEventStatus, "session started",
			map[string]any{"code": "session_started", "sessionId": l.ThreadID})}
	case "turn.started":
		return nil
	case "turn.completed":
		return []protocol.CodingEvent{event(protocol.CodingEventStatus, "finished", map[string]any{"code": "result", "usage": l.Usage})}
	case "turn.failed":
		msg := "turn failed"
		if l.Error != nil && l.Error.Message != "" {
			msg = l.Error.Message
		}
		return []protocol.CodingEvent{event(protocol.CodingEventError, msg, nil)}
	case "error":
		return []protocol.CodingEvent{event(protocol.CodingEventError, l.Message, nil)}
	case "item.started", "item.updated", "item.completed":
		if l.Item == nil {
			return nil
		}
		return codexItemEvents(l.Type, *l.Item)
	case "":
		return plain(b)
	}
	return nil
}

func codexItemEvents(phase string, it codexItem) []protocol.CodingEvent {
	kind := it.Type
	if kind == "" {
		kind = it.ItemType
	}
	done := phase == "item.completed"
	switch kind {
	case "agent_message", "assistant_message":
		if done && strings.TrimSpace(it.Text) != "" {
			return []protocol.CodingEvent{event(protocol.CodingEventText, it.Text, nil)}
		}
	case "command_execution":
		if phase == "item.started" {
			return []protocol.CodingEvent{event(protocol.CodingEventTool, "Shell: "+truncate(it.Command, 200),
				map[string]any{"id": it.ID, "name": "Shell", "input": map[string]any{"command": clip(it.Command)}})}
		}
		if done {
			isErr := it.Status == "failed" || (it.ExitCode != nil && *it.ExitCode != 0)
			return []protocol.CodingEvent{event(protocol.CodingEventTool, truncate(it.AggregatedOutput, maxToolResult),
				map[string]any{"id": it.ID, "result": true, "isError": isErr, "exitCode": it.ExitCode})}
		}
	case "file_change":
		if done {
			return []protocol.CodingEvent{event(protocol.CodingEventTool, fmt.Sprintf("Edit: %d files", len(it.Changes)),
				map[string]any{"id": it.ID, "name": "Edit", "input": map[string]any{"changes": clip(it.Changes)}})}
		}
	case "mcp_tool_call":
		if phase == "item.started" {
			name := it.Server + "." + it.Tool
			return []protocol.CodingEvent{event(protocol.CodingEventTool, name,
				map[string]any{"id": it.ID, "name": name, "input": clip(it.Arguments)})}
		}
		if done {
			return []protocol.CodingEvent{event(protocol.CodingEventTool, it.Status,
				map[string]any{"id": it.ID, "result": true, "isError": it.Status == "failed"})}
		}
	case "web_search":
		if done {
			return []protocol.CodingEvent{event(protocol.CodingEventTool, "WebSearch: "+truncate(it.Query, 200),
				map[string]any{"id": it.ID, "name": "WebSearch", "input": map[string]any{"query": it.Query}})}
		}
	case "todo_list":
		return []protocol.CodingEvent{event(protocol.CodingEventStatus, "plan", map[string]any{"code": "plan", "items": clip(it.Items)})}
	case "error":
		if done {
			return []protocol.CodingEvent{event(protocol.CodingEventError, it.Message, nil)}
		}
	}
	return nil // reasoning and in-progress updates
}

func codexLegacy(l codexLine) []protocol.CodingEvent {
	m := l.Msg
	switch m.Type {
	case "agent_message":
		return []protocol.CodingEvent{event(protocol.CodingEventText, m.Message, nil)}
	case "exec_command_begin":
		cmd := strings.Join(m.Command, " ")
		return []protocol.CodingEvent{event(protocol.CodingEventTool, "Shell: "+truncate(cmd, 200),
			map[string]any{"id": m.CallID, "name": "Shell", "input": map[string]any{"command": cmd}})}
	case "exec_command_end":
		isErr := m.Exit != nil && *m.Exit != 0
		return []protocol.CodingEvent{event(protocol.CodingEventTool, truncate(m.Stdout+m.Stderr, maxToolResult),
			map[string]any{"id": m.CallID, "result": true, "isError": isErr})}
	case "error", "stream_error":
		return []protocol.CodingEvent{event(protocol.CodingEventError, m.Message, nil)}
	case "task_complete":
		return []protocol.CodingEvent{event(protocol.CodingEventStatus, "finished", map[string]any{"code": "result"})}
	}
	return nil
}
