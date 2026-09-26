package monitoring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func setup(t *testing.T) (*testutil.Env, *monitoring.Module) {
	t.Helper()
	env := testutil.New(t, monitoring.New)
	m, ok := module.Lookup[*monitoring.Module](env.App.Deps.Registry, monitoring.SelfKey)
	if !ok {
		t.Fatal("monitoring module not registered")
	}
	return env, m
}

// startAgent pairs and runs an agent with its own name.
func startAgent(t *testing.T, env *testutil.Env, name string, caps []string, register func(c *conn.Client)) string {
	id, _ := startStoppableAgent(t, env, name, caps, register)
	return id
}

// startStoppableAgent is startAgent that also returns a function taking the
// agent offline.
func startStoppableAgent(t *testing.T, env *testutil.Env, name string, caps []string, register func(c *conn.Client)) (string, func()) {
	t.Helper()
	env.Elevate()
	var pc struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": name, "kind": "server"}, &pc)
	hello := protocol.Hello{AgentVersion: "test", OS: "linux", Arch: "amd64", Hostname: name + "-host", Capabilities: caps}
	id, token, err := conn.Pair(context.Background(), env.Server.URL, pc.Code, hello)
	if err != nil {
		t.Fatal(err)
	}
	client := conn.New(env.Server.URL, token, hello)
	if register != nil {
		register(client)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = client.Run(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		<-done
		waitFor(t, "agent offline", func() bool { return !env.App.Deps.Agents.Online(id) })
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitFor(t, "agent online", func() bool { return env.App.Deps.Agents.Online(id) })
	return id, stop
}

// relogin starts a fresh session that is not elevated.
func relogin(t *testing.T, env *testutil.Env) {
	t.Helper()
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func expectStatus(t *testing.T, env *testutil.Env, method, path string, body any, status int, code string) {
	t.Helper()
	got, raw := env.Do(method, path, body, nil)
	if got != status || (code != "" && errCode(raw) != code) {
		t.Fatalf("%s %s: got %d %s, want %d %s", method, path, got, raw, status, code)
	}
}

// notifications returns the titles of stored notifications of one kind.
func notifications(t *testing.T, env *testutil.Env, kind string) []string {
	t.Helper()
	rows, err := env.App.Deps.DB.Query(`SELECT title FROM notifications WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// auditCount counts audit entries of one action.
func auditCount(t *testing.T, env *testutil.Env, action string) int {
	t.Helper()
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wsHeader(env *testutil.Env) http.Header {
	u, _ := url.Parse(env.Server.URL)
	var parts []string
	for _, c := range env.Client.Jar.Cookies(u) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return http.Header{"Cookie": []string{strings.Join(parts, "; ")}}
}

func runAction(t *testing.T, env *testutil.Env, name string, input any, out any) {
	t.Helper()
	raw, _ := json.Marshal(input)
	res, err := env.App.Deps.Actions.Run(context.Background(), name, raw)
	if err != nil {
		t.Fatalf("action %s: %v", name, err)
	}
	b, _ := json.Marshal(res)
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("action %s: decode %s: %v", name, b, err)
	}
}
