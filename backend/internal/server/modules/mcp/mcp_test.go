package mcp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type created struct {
	Token struct {
		ID     int64 `json:"id"`
		Prefix string
	}
	Secret string
}

func newToken(t *testing.T, env *testutil.Env, name, access string, modules []string) created {
	t.Helper()
	env.Elevate()
	var out created
	body := map[string]any{"name": name, "access": access}
	if modules != nil {
		body["modules"] = modules
	}
	env.MustDo(http.MethodPost, "/api-tokens", body, &out)
	if !strings.HasPrefix(out.Secret, "xc_") || out.Token.Prefix != out.Secret[:8] {
		t.Fatalf("token: %+v", out)
	}
	return out
}

// rpc sends one JSON-RPC message with a bearer token.
func rpc(t *testing.T, env *testutil.Env, secret string, msg any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPost, env.URL("/mcp"), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	return resp.StatusCode, out
}

func toolNames(t *testing.T, env *testutil.Env, secret string) map[string]bool {
	t.Helper()
	_, out := rpc(t, env, secret, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	names := map[string]bool{}
	for _, x := range out["result"].(map[string]any)["tools"].([]any) {
		names[x.(map[string]any)["name"].(string)] = true
	}
	return names
}

func TestTokensAndMCPFlow(t *testing.T) {
	env := testutil.New(t)
	// Creating a token needs a fresh verification, even with elevation off.
	env.Elevate()
	env.MustDo(http.MethodPut, "/auth/elevation-mode", map[string]string{"mode": "off"}, nil)
	if _, err := env.App.Deps.DB.Exec(`UPDATE sessions SET elevated_until = NULL`); err != nil {
		t.Fatal(err)
	}
	if s, _ := env.Do(http.MethodPost, "/api-tokens", map[string]any{"name": "x", "access": "read"}, nil); s != http.StatusForbidden {
		t.Fatalf("create without verification: %d", s)
	}
	w := newToken(t, env, "电脑上的 Claude", "write", nil)

	// The secret is not stored.
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM api_tokens WHERE token_hash = ? OR prefix = ?`, w.Secret, w.Secret).Scan(&n); err != nil || n != 0 {
		t.Fatalf("plain secret stored: %d %v", n, err)
	}

	// initialize and the initialized notification.
	status, out := rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}}})
	if status != 200 || out["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize: %d %+v", status, out)
	}
	if s, _ := rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); s != http.StatusAccepted {
		t.Fatalf("notification: %d", s)
	}

	// write: reads and writes, no deletes, never dangerous.
	names := toolNames(t, env, w.Secret)
	if !names["reminders_create"] || !names["notes_search"] || names["notes_delete"] || names["hosts_exec"] {
		t.Fatalf("write tools: %v", names)
	}
	// B61: remote AI never sees the AI memory.
	for name := range names {
		if strings.HasPrefix(name, "memory_") {
			t.Fatalf("mcp lists %s", name)
		}
	}

	// Call a tool: the reminder shows up, audited as the token.
	status, out = rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "notes_create", "arguments": map[string]any{"body": "# 来自 MCP\n\n远程写的"}}})
	res := out["result"].(map[string]any)
	if status != 200 || res["isError"] == true || res["structuredContent"] == nil {
		t.Fatalf("call: %d %+v", status, out)
	}
	var actor string
	if err := env.App.Deps.DB.QueryRow(`SELECT actor FROM audit_log WHERE action = 'mcp.call' ORDER BY id DESC`).Scan(&actor); err != nil || actor != "token:电脑上的 Claude" {
		t.Fatalf("audit actor: %q %v", actor, err)
	}
	var calls []struct{ Tool, Token, Result string }
	env.MustDo(http.MethodGet, "/api-tokens/calls", nil, &calls)
	if len(calls) != 1 || calls[0].Tool != "notes_create" || calls[0].Result != "ok" {
		t.Fatalf("calls: %+v", calls)
	}
	_, out = rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "id": 30, "method": "tools/call",
		"params": map[string]any{"name": "memory_save", "arguments": map[string]any{"text": "x"}}})
	if out["error"] == nil {
		t.Fatalf("memory_save over mcp: %+v", out)
	}

	// A tool the token was not shown is refused and not run.
	_, out = rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{"name": "notes_delete", "arguments": map[string]any{"id": 1}}})
	if out["error"] == nil {
		t.Fatalf("delete with write token: %+v", out)
	}
	// Tool errors come back as isError results.
	_, out = rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "id": 5, "method": "tools/call",
		"params": map[string]any{"name": "projects_get_issue", "arguments": map[string]any{"key": "NO-1"}}})
	if out["result"].(map[string]any)["isError"] != true {
		t.Fatalf("tool error: %+v", out)
	}
	// Batches.
	raw, _ := json.Marshal([]any{
		map[string]any{"jsonrpc": "2.0", "id": 6, "method": "ping"},
		map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled"},
		map[string]any{"jsonrpc": "2.0", "id": 7, "method": "nope"},
	})
	req, _ := http.NewRequest(http.MethodPost, env.URL("/mcp"), bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+w.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var batch []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&batch)
	resp.Body.Close()
	if len(batch) != 2 || batch[1]["error"] == nil {
		t.Fatalf("batch: %+v", batch)
	}

	// The token opens nothing but /mcp.
	req, _ = http.NewRequest(http.MethodGet, env.URL("/notes"), nil)
	req.Header.Set("Authorization", "Bearer "+w.Secret)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token on /notes: %d", resp.StatusCode)
	}
	// No token on /mcp.
	if s, _ := rpc(t, env, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); s != http.StatusUnauthorized {
		t.Fatalf("no token: %d", s)
	}

	// Revoked tokens stop at once.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/api-tokens/%d", w.Token.ID), nil, nil)
	if s, _ := rpc(t, env, w.Secret, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); s != http.StatusUnauthorized {
		t.Fatalf("revoked: %d", s)
	}
}

func TestTokenAccessModulesAndExpiry(t *testing.T) {
	env := testutil.New(t)
	r := newToken(t, env, "只读", "read", nil)
	names := toolNames(t, env, r.Secret)
	if !names["notes_search"] || names["notes_create"] || names["reminders_create"] {
		t.Fatalf("read tools: %v", names)
	}
	wd := newToken(t, env, "只管笔记", "write_delete", []string{"notes"})
	names = toolNames(t, env, wd.Secret)
	if !names["notes_delete"] || names["reminders_create"] || names["hosts_exec"] {
		t.Fatalf("notes-only tools: %v", names)
	}
	if s, _ := env.Do(http.MethodPost, "/api-tokens", map[string]any{"name": "x", "access": "read", "modules": []string{"nope"}}, nil); s != 400 {
		t.Fatalf("unknown module: %d", s)
	}
	// Expired.
	if _, err := env.App.Deps.DB.Exec(`UPDATE api_tokens SET expires_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Minute), r.Token.ID); err != nil {
		t.Fatal(err)
	}
	if s, _ := rpc(t, env, r.Secret, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); s != http.StatusUnauthorized {
		t.Fatalf("expired: %d", s)
	}
	// Rate limit: 60 calls a minute per token.
	status := 0
	for i := 0; i < 61; i++ {
		status, _ = rpc(t, env, wd.Secret, map[string]any{"jsonrpc": "2.0", "id": i, "method": "ping"})
	}
	if status != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", status)
	}
}
