package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestAuthFlow(t *testing.T) {
	env := testutil.New(t)
	var st struct {
		SetupRequired bool
		Authenticated bool
		Username      string
	}
	env.MustDo(http.MethodGet, "/auth/status", nil, &st)
	if st.SetupRequired || !st.Authenticated || st.Username != testutil.Username {
		t.Fatalf("status after setup: %+v", st)
	}
	// Setup cannot run twice.
	if status, _ := env.Do(http.MethodPost, "/auth/setup", map[string]string{"username": "x", "password": "0123456789"}, nil); status != http.StatusConflict {
		t.Fatalf("second setup: %d", status)
	}
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/agents", nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": "wrong-password", "code": env.Code()}, nil); status != http.StatusUnauthorized {
		t.Fatalf("bad password: %d", status)
	}
	// Right password without a code: ask for the code, and do not count it as a
	// failure (six in a row would otherwise lock the IP).
	for i := 0; i < 6; i++ {
		status, raw := env.Do(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password}, nil)
		var e struct{ Code string }
		_ = json.Unmarshal(raw, &e)
		if status != http.StatusUnauthorized || e.Code != "totp_required" {
			t.Fatalf("login without code: %d %s", status, raw)
		}
	}
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)
	env.MustDo(http.MethodGet, "/agents", nil, nil)
}

// B12 前端先做，这些接口先回 501 not_ready。
func TestOptionalTotpNotReady(t *testing.T) {
	env := testutil.New(t)
	for _, path := range []string{"/auth/setup/skip-totp", "/auth/totp/enroll", "/auth/totp/confirm", "/auth/totp/disable", "/auth/password"} {
		status, raw := env.Do(http.MethodPost, path, map[string]string{}, nil)
		var e struct{ Code string }
		_ = json.Unmarshal(raw, &e)
		if status != http.StatusNotImplemented || e.Code != "not_ready" {
			t.Fatalf("%s: %d %s", path, status, raw)
		}
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	env := testutil.New(t)
	req, _ := http.NewRequest(http.MethodPost, env.URL("/notifications/read-all"), nil)
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403, got %d", resp.StatusCode)
	}
}

func TestElevationRequired(t *testing.T) {
	env := testutil.New(t)
	status, raw := env.Do(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": "a", "kind": "server"}, nil)
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	if status != http.StatusForbidden || e.Code != "elevation_required" {
		t.Fatalf("got %d %s", status, raw)
	}
}

func TestAgentPairCallRevoke(t *testing.T) {
	env := testutil.New(t)
	id := env.Agent("server", []string{protocol.CapSystemInfo}, func(c *conn.Client) {
		c.Handle(protocol.MethodPing, func(ctx context.Context, _ json.RawMessage) (any, error) {
			return protocol.Pong{Time: "now"}, nil
		})
	})
	var pong protocol.Pong
	if err := env.App.Deps.Agents.Call(context.Background(), id, protocol.MethodPing, nil, &pong); err != nil || pong.Time != "now" {
		t.Fatalf("ping: %+v %v", pong, err)
	}
	var agents []struct {
		ID           string
		Online       bool
		Capabilities []string
	}
	env.MustDo(http.MethodGet, "/agents", nil, &agents)
	if len(agents) != 1 || !agents[0].Online || agents[0].Capabilities[0] != protocol.CapSystemInfo {
		t.Fatalf("agents: %+v", agents)
	}
	env.MustDo(http.MethodDelete, "/agents/"+id, nil, nil)
	deadline := time.Now().Add(3 * time.Second)
	for env.App.Deps.Agents.Online(id) {
		if time.Now().After(deadline) {
			t.Fatal("agent still online after revoke")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNotifications(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := env.App.Deps.Notify.Send(ctx, notify.Notification{Kind: "test", Title: "hi", Source: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	var list struct {
		Items       []struct{ ID int64 }
		NextCursor  string
		UnreadCount int
	}
	env.MustDo(http.MethodGet, "/notifications?limit=2", nil, &list)
	if len(list.Items) != 2 || list.NextCursor == "" || list.UnreadCount != 3 {
		t.Fatalf("page 1: %+v", list)
	}
	env.MustDo(http.MethodGet, "/notifications?limit=2&cursor="+list.NextCursor, nil, &list)
	if len(list.Items) != 1 {
		t.Fatalf("page 2: %+v", list)
	}
	env.MustDo(http.MethodPost, "/notifications/read-all", nil, nil)
	env.MustDo(http.MethodGet, "/notifications?unread=true", nil, &list)
	if len(list.Items) != 0 || list.UnreadCount != 0 {
		t.Fatalf("after read-all: %+v", list)
	}
	var audit struct{ Items []struct{ Action string } }
	env.MustDo(http.MethodGet, "/audit", nil, &audit)
	if len(audit.Items) == 0 {
		t.Fatal("audit log empty")
	}
}

func TestDuplicateConstructorsBuiltOnce(t *testing.T) {
	built := 0
	ctor := func(d *module.Deps) (module.Module, error) {
		built++
		return fakeModule{}, nil
	}
	testutil.New(t, ctor, ctor)
	if built != 1 {
		t.Fatalf("constructor ran %d times", built)
	}
}

type fakeModule struct{}

func (fakeModule) Name() string       { return "fake" }
func (fakeModule) Mount(r chi.Router) {}
