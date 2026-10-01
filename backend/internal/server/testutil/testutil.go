// Package testutil starts a full server on an in-memory database for tests.
// Module tests use it to call their HTTP API as a logged-in user and to
// attach a real agent client:
//
//	env := testutil.New(t, projects.New)
//	var p api.Project
//	env.MustDo(http.MethodPost, "/projects", api.CreateProject{Name: "X"}, &p)
package testutil

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/store"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Username and Password of the test user.
const (
	Username = "jo"
	Password = "correct-horse-battery"
)

// Env is a running test server with a logged-in client.
type Env struct {
	T      testing.TB
	App    *app.App
	Server *httptest.Server
	Client *http.Client
	Secret string // TOTP secret of the test user
}

// migrated is an empty, fully migrated database, made once per test binary.
var migrated struct {
	once  sync.Once
	image []byte
	err   error
}

// openDB returns a fresh in-memory database with every migration applied.
// It copies a snapshot instead of migrating again: under -race the
// migrations took about 3 seconds for every test (2026-10-01).
func openDB(ctx context.Context) (*sql.DB, error) {
	migrated.once.Do(func() {
		db, err := store.Open(ctx, ":memory:")
		if err != nil {
			migrated.err = err
			return
		}
		defer db.Close()
		migrated.image, migrated.err = store.Snapshot(ctx, db)
	})
	if migrated.err != nil {
		return nil, migrated.err
	}
	return store.OpenSnapshot(ctx, migrated.image)
}

// New starts a server with the given extra modules and logs in.
func New(t testing.TB, modules ...func(*module.Deps) (module.Module, error)) *Env {
	t.Helper()
	return NewWithConfig(t, nil, modules...)
}

// NewWithConfig is New with a chance to change the server configuration.
func NewWithConfig(t testing.TB, tune func(*config.Config), modules ...func(*module.Deps) (module.Module, error)) *Env {
	t.Helper()
	ctx := context.Background()
	conn, err := openDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cfg := config.Config{MasterKey: bytes.Repeat([]byte{7}, 32), Dev: true, Location: loc, DataDir: t.TempDir()}
	if tune != nil {
		tune(&cfg)
	}
	a, err := app.New(cfg, conn, modules...)
	if err != nil {
		t.Fatal(err)
	}
	startCtx, cancel := context.WithCancel(ctx)
	if err := a.Start(startCtx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	jar, _ := cookiejar.New(nil)
	env := &Env{T: t, App: a, Server: srv, Client: &http.Client{Jar: jar}}
	t.Cleanup(func() {
		cancel()
		srv.CloseClientConnections()
		srv.Close()
		a.Stop()
		conn.Close()
	})

	var enroll struct{ Secret string }
	env.MustDo(http.MethodPost, "/auth/setup", map[string]string{"username": Username, "password": Password}, &enroll)
	env.Secret = enroll.Secret
	env.MustDo(http.MethodPost, "/auth/setup/confirm", map[string]string{"code": env.Code()}, nil)
	return env
}

// Code returns a valid TOTP code for the test user.
func (e *Env) Code() string {
	code, err := totp.GenerateCode(e.Secret, time.Now())
	if err != nil {
		e.T.Fatal(err)
	}
	return code
}

// Elevate opens the 5 minute window for dangerous operations.
func (e *Env) Elevate() {
	e.MustDo(http.MethodPost, "/auth/elevate", map[string]string{"code": e.Code()}, nil)
}

// URL returns the absolute URL of an API path such as "/projects".
func (e *Env) URL(path string) string { return e.Server.URL + app.APIPrefix + path }

// Do sends a JSON request to an API path and decodes a 2xx JSON body into out.
// It returns the status code and the raw body.
func (e *Env) Do(method, path string, body, out any) (int, []byte) {
	e.T.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.T.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.URL(path), rd)
	if err != nil {
		e.T.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := e.Client.Do(req)
	if err != nil {
		e.T.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.T.Fatalf("%s %s: decode: %v (%s)", method, path, err, raw)
		}
	}
	return resp.StatusCode, raw
}

// MustDo is Do that fails the test on a non-2xx status.
func (e *Env) MustDo(method, path string, body, out any) {
	e.T.Helper()
	status, raw := e.Do(method, path, body, out)
	if status >= 300 {
		e.T.Fatalf("%s %s: status %d: %s", method, path, status, raw)
	}
}

// Agent pairs and runs a real agent client of the given kind ("server" or
// "desktop"). register adds handlers before it connects. It returns the agent
// id once the hub sees it online.
func (e *Env) Agent(kind string, caps []string, register func(c *conn.Client)) string {
	e.T.Helper()
	e.Elevate()
	var pc struct{ Code string }
	e.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": "test-" + kind, "kind": kind}, &pc)
	hello := protocol.Hello{AgentVersion: "test", OS: "linux", Arch: "amd64", Hostname: "test-host", Capabilities: caps}
	if kind == "desktop" {
		hello.OS = "windows"
	}
	id, token, err := conn.Pair(context.Background(), e.Server.URL, pc.Code, hello)
	if err != nil {
		e.T.Fatal(err)
	}
	client := conn.New(e.Server.URL, token, hello)
	if register != nil {
		register(client)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.T.Cleanup(cancel)
	go client.Run(ctx)
	deadline := time.Now().Add(5 * time.Second)
	for !e.App.Deps.Agents.Online(id) {
		if time.Now().After(deadline) {
			e.T.Fatal("agent did not connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return id
}

// WSURL turns an API path into a ws:// URL, for streaming endpoints.
func (e *Env) WSURL(path string) string {
	return strings.Replace(e.URL(path), "http://", "ws://", 1)
}
