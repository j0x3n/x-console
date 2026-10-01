package vault_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestHiddenModuleGate(t *testing.T) {
	env := testutil.New(t)
	serverID := env.Agent("server", nil, nil)
	pcID := env.Agent("desktop", nil, nil)

	env.MustDo(http.MethodGet, "/notes", nil, nil)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"notes", "pc", "reminders", "drive"}}, nil)

	env.MustDo(http.MethodGet, "/notes", nil, nil)
	if !hostListed(t, env, "/hosts?kind=desktop", pcID) {
		t.Fatal("unlocked list dropped the desktop")
	}
	if !aiHasNotes(t, env) {
		t.Fatal("unlocked tool list dropped notes")
	}

	env.Elevate()
	var created struct {
		Secret string `json:"secret"`
	}
	env.MustDo(http.MethodPost, "/api-tokens", map[string]any{"name": "隐藏测试", "access": "read"}, &created)
	names := mcpTools(t, env, created.Secret)
	for name := range names {
		if strings.HasPrefix(name, "notes_") {
			t.Fatalf("mcp lists %s while notes is hidden", name)
		}
	}
	hiddenCall := mcpCall(t, env, created.Secret, "notes_get")
	missingCall := mcpCall(t, env, created.Secret, "no_such_tool")
	if hiddenCall.code != missingCall.code || hiddenCall.message == "" || strings.Contains(hiddenCall.message, "隐藏") {
		t.Fatalf("mcp hidden %v %q missing %v %q", hiddenCall.code, hiddenCall.message, missingCall.code, missingCall.message)
	}
	if hiddenCall.message != "没有这个工具，或这个令牌不能用它: notes_get" || missingCall.message != "没有这个工具，或这个令牌不能用它: no_such_tool" {
		t.Fatalf("mcp messages %q %q", hiddenCall.message, missingCall.message)
	}

	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	checkCode(t, env, http.MethodGet, "/notes", nil, 404, "not_found")
	checkCode(t, env, http.MethodGet, "/reminders", nil, 404, "not_found")
	if n := hostCount(t, env, "/hosts?kind=pc"); n != 0 {
		t.Fatalf("kind=pc: %d", n)
	}
	if n := hostCount(t, env, "/hosts?kind=desktop"); n != 0 {
		t.Fatalf("kind=desktop: %d", n)
	}
	checkCode(t, env, http.MethodGet, "/hosts/"+pcID, nil, 404, "not_found")
	if !hostListed(t, env, "/hosts?kind=server", serverID) {
		t.Fatal("server missing")
	}
	env.MustDo(http.MethodGet, "/hosts/"+serverID, nil, nil)
	env.MustDo(http.MethodGet, "/notify/channels", nil, nil)
	checkCode(t, env, http.MethodGet, "/drive/usage", nil, 404, "not_found")
	status, raw := env.Do(http.MethodPost, "/files?scope=projects", map[string]any{"x": 1}, nil)
	if status == 404 || status != 400 || !bytes.Contains(raw, []byte("需要上传图片")) {
		t.Fatalf("files: %d %s", status, raw)
	}
	if aiHasNotes(t, env) {
		t.Fatal("locked tool list still has notes")
	}

	locked := sessionCtx(t, env)
	notesErr := actionErr(t, env, locked, "notes.get")
	missingErr := actionErr(t, env, locked, "no.such")
	if notesErr.Code != "unknown_action" || missingErr.Code != "unknown_action" || strings.Contains(notesErr.Message, "隐藏") {
		t.Fatalf("actions %s %q / %s %q", notesErr.Code, notesErr.Message, missingErr.Code, missingErr.Message)
	}
	if notesErr.Message != "没有这个动作: notes.get" || missingErr.Message != "没有这个动作: no.such" {
		t.Fatalf("action messages %q %q", notesErr.Message, missingErr.Message)
	}

	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodGet, "/notes", nil, nil)
	open := sessionCtx(t, env)
	if err := actionErr(t, env, open, "notes.get"); err != nil && err.Code == "unknown_action" {
		t.Fatalf("unlocked notes.get: %s %s", err.Code, err.Message)
	}
}

func hostCount(t *testing.T, env *testutil.Env, path string) int {
	t.Helper()
	var hosts []struct {
		ID string `json:"id"`
	}
	env.MustDo(http.MethodGet, path, nil, &hosts)
	return len(hosts)
}

func hostListed(t *testing.T, env *testutil.Env, path, id string) bool {
	t.Helper()
	var hosts []struct {
		ID string `json:"id"`
	}
	env.MustDo(http.MethodGet, path, nil, &hosts)
	for _, h := range hosts {
		if h.ID == id {
			return true
		}
	}
	return false
}

func aiHasNotes(t *testing.T, env *testutil.Env) bool {
	t.Helper()
	var tools []struct {
		Action string `json:"action"`
	}
	env.MustDo(http.MethodGet, "/ai/tools", nil, &tools)
	for _, tool := range tools {
		if strings.HasPrefix(tool.Action, "notes.") {
			return true
		}
	}
	return false
}

type rpcErr struct {
	code    float64
	message string
}

func mcpTools(t *testing.T, env *testutil.Env, secret string) map[string]bool {
	t.Helper()
	out := mcp(t, env, secret, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	result, _ := out["result"].(map[string]any)
	list, _ := result["tools"].([]any)
	names := map[string]bool{}
	for _, item := range list {
		tool, _ := item.(map[string]any)
		name, _ := tool["name"].(string)
		names[name] = true
	}
	if len(names) == 0 {
		t.Fatalf("mcp tools: %#v", out)
	}
	return names
}

func mcpCall(t *testing.T, env *testutil.Env, secret, name string) rpcErr {
	t.Helper()
	out := mcp(t, env, secret, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": name}})
	errObj, _ := out["error"].(map[string]any)
	code, _ := errObj["code"].(float64)
	message, _ := errObj["message"].(string)
	return rpcErr{code: code, message: message}
}

func mcp(t *testing.T, env *testutil.Env, secret string, msg any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(msg)
	req, err := http.NewRequest(http.MethodPost, env.URL("/mcp"), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("mcp %d %s", resp.StatusCode, body)
	}
	return out
}

func sessionCtx(t *testing.T, env *testutil.Env) context.Context {
	t.Helper()
	var id, username string
	var until *time.Time
	err := env.App.Deps.DB.QueryRow(`SELECT s.id, u.username, s.vault_until FROM sessions s JOIN users u ON u.id = s.user_id LIMIT 1`).Scan(&id, &username, &until)
	if err != nil {
		t.Fatal(err)
	}
	return auth.WithSession(context.Background(), &auth.Session{ID: id, Username: username, VaultUntil: until})
}

func actionErr(t *testing.T, env *testutil.Env, ctx context.Context, name string) *httpx.Error {
	t.Helper()
	_, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(`{"id":1}`))
	if err == nil {
		return nil
	}
	var apiErr *httpx.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("%s: %v", name, err)
	}
	return apiErr
}
