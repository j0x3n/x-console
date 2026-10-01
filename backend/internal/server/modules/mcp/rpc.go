package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/core"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// MCP over "Streamable HTTP", stateless: every POST carries one JSON-RPC
// message (or a batch) and gets the answer as application/json. There is no
// session id and no server-sent event stream. Only tools are offered.

// Protocol versions this server speaks, newest first.
var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const maxBody = 1 << 20

func (m *Module) serveMCP(w http.ResponseWriter, r *http.Request) {
	tok := auth.TokenFrom(r.Context())
	if tok == nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeInvalidRequest, "请求体太大"}})
		return
	}
	ctx := contracts.WithAIUsage(r.Context(), "mcp", tok.Name)
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []rpcRequest
		if err := json.Unmarshal(trimmed, &batch); err != nil || len(batch) == 0 {
			writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "JSON 格式不正确"}})
			return
		}
		var out []rpcResponse
		for _, req := range batch {
			if resp, ok := m.handle(ctx, tok, req); ok {
				out = append(out, resp)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPC(w, out)
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "JSON 格式不正确"}})
		return
	}
	resp, ok := m.handle(ctx, tok, req)
	if !ok {
		// A notification: nothing to answer.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, resp)
}

func writeRPC(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// handle answers one message. ok is false for notifications.
func (m *Module) handle(ctx context.Context, tok *auth.TokenInfo, req rpcRequest) (rpcResponse, bool) {
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		if isNotification {
			return resp, false
		}
		resp.Error = &rpcError{codeInvalidRequest, "不是 JSON-RPC 2.0 请求"}
		return resp, true
	}
	var result any
	var rerr *rpcError
	switch req.Method {
	case "initialize":
		result = m.initialize(req.Params)
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = m.listTools(ctx, tok)
	case "tools/call":
		result, rerr = m.callTool(ctx, tok, req.Params)
	default:
		if isNotification {
			// notifications/initialized, notifications/cancelled…
			return resp, false
		}
		rerr = &rpcError{codeMethodNotFound, "不支持的方法: " + req.Method}
	}
	if isNotification {
		return resp, false
	}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return resp, true
}

func (m *Module) initialize(params json.RawMessage) any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	version := protocolVersions[0]
	for _, v := range protocolVersions {
		if v == p.ProtocolVersion {
			version = v
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "x-console", "title": "X Console", "version": core.Version},
		"instructions": "X Console is a personal console: projects (boards, lists, cards), notes, reminders, habits, " +
			"calendar, servers. Use the tools to read and change the user's data. Times are RFC 3339 with a time zone. " +
			"Cards are identified by keys like XC-12.",
	}
}

type tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations map[string]any  `json:"annotations,omitempty"`
}

// allowed lists the actions this token may use, by MCP tool name.
func (m *Module) allowed(ctx context.Context, tok *auth.TokenInfo) map[string]actions.Action {
	out := map[string]actions.Action{}
	for _, a := range m.d.Actions.List(ctx) {
		if actions.AllowedFor(a, tok.Access, tok.Modules) {
			out[toolName(a.Name)] = a
		}
	}
	return out
}

func (m *Module) listTools(ctx context.Context, tok *auth.TokenInfo) any {
	tools := []tool{}
	for _, a := range m.d.Actions.List(ctx) {
		if !actions.AllowedFor(a, tok.Access, tok.Modules) {
			continue
		}
		desc := a.Description
		if desc == "" || desc == a.Title {
			desc = a.Title
		} else {
			desc = a.Title + "。" + desc
		}
		tools = append(tools, tool{
			Name: toolName(a.Name), Title: a.Title, Description: desc, InputSchema: a.Input,
			Annotations: map[string]any{
				"readOnlyHint":    a.Effect == actions.Read,
				"destructiveHint": actions.Deletes(a),
			},
		})
	}
	return map[string]any{"tools": tools}
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callTool runs an action. Tool errors come back as a result with isError,
// so the model can read them; protocol errors as JSON-RPC errors.
func (m *Module) callTool(ctx context.Context, tok *auth.TokenInfo, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, &rpcError{codeInvalidParams, "tools/call 需要 name"}
	}
	// Check again: the client may call a tool it was never shown.
	a, ok := m.allowed(ctx, tok)[p.Name]
	if !ok {
		m.d.Audit.Record(ctx, "mcp.call", p.Name, nil, errors.New("这个令牌不能用这个工具"))
		return nil, &rpcError{codeInvalidParams, "没有这个工具，或这个令牌不能用它: " + p.Name}
	}
	args := p.Arguments
	if len(bytes.TrimSpace(args)) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	out, err := a.Run(ctx, args)
	m.d.Audit.Record(ctx, "mcp.call", p.Name, nil, err)
	if err != nil {
		msg := err.Error()
		var apiErr *httpx.Error
		if errors.As(err, &apiErr) {
			msg = apiErr.Message
		}
		return map[string]any{"content": []textContent{{"text", msg}}, "isError": true}, nil
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, &rpcError{codeInternal, "结果无法转成 JSON"}
	}
	result := map[string]any{"content": []textContent{{"text", string(raw)}}}
	// structuredContent must be an object.
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '{' {
		result["structuredContent"] = json.RawMessage(raw)
	} else {
		result["structuredContent"] = map[string]any{"result": json.RawMessage(raw)}
	}
	return result, nil
}
