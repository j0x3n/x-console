package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp/totp"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/store"
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

func TestOptionalTotpFlow(t *testing.T) {
	env := testutil.New(t)
	status, raw := env.Do(http.MethodPost, "/auth/setup/skip-totp", map[string]string{"password": testutil.Password}, nil)
	if status != http.StatusConflict {
		t.Fatalf("skip after setup: %d %s", status, raw)
	}
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	var state struct {
		Authenticated bool
		TotpEnabled   *bool
	}
	env.MustDo(http.MethodGet, "/auth/status", nil, &state)
	if state.Authenticated || state.TotpEnabled != nil {
		t.Fatalf("logged out status: %+v", state)
	}
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)
	env.MustDo(http.MethodGet, "/auth/status", nil, &state)
	if !state.Authenticated || state.TotpEnabled == nil || !*state.TotpEnabled {
		t.Fatalf("enabled status: %+v", state)
	}
	status, _ = env.Do(http.MethodPost, "/auth/totp/enroll", nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("enroll without elevation: %d", status)
	}
	env.Elevate()
	status, _ = env.Do(http.MethodPost, "/auth/totp/enroll", nil, nil)
	if status != http.StatusConflict {
		t.Fatalf("enroll while enabled: %d", status)
	}
	for _, body := range []map[string]string{
		{"password": "wrong", "code": env.Code()},
		{"password": testutil.Password, "code": "000000"},
	} {
		status, _ = env.Do(http.MethodPost, "/auth/totp/disable", body, nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("disable invalid credentials: %d", status)
		}
	}
	env.MustDo(http.MethodPost, "/auth/totp/disable", map[string]string{"password": testutil.Password, "code": env.Code()}, nil)
	env.MustDo(http.MethodGet, "/auth/status", nil, &state)
	if state.TotpEnabled == nil || *state.TotpEnabled {
		t.Fatalf("disabled status: %+v", state)
	}
	status, _ = env.Do(http.MethodPost, "/auth/elevate", map[string]string{"code": env.Code()}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("elevate with code while disabled: %d", status)
	}
	env.MustDo(http.MethodPost, "/auth/elevate", map[string]string{"password": testutil.Password}, nil)
	var enrollment struct{ Secret string }
	env.MustDo(http.MethodPost, "/auth/totp/enroll", nil, &enrollment)
	if enrollment.Secret == "" {
		t.Fatal("enrollment secret missing")
	}
	status, _ = env.Do(http.MethodPost, "/auth/totp/confirm", map[string]string{"code": "000000"}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("confirm invalid code: %d", status)
	}
	code, err := totp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, "/auth/totp/confirm", map[string]string{"code": code}, nil)
	env.MustDo(http.MethodGet, "/auth/status", nil, &state)
	if state.TotpEnabled == nil || !*state.TotpEnabled {
		t.Fatalf("reenabled status: %+v", state)
	}
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	env := testutil.New(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	other := &http.Client{Jar: jar}
	req, err := http.NewRequest(http.MethodPost, env.URL("/auth/login"), strings.NewReader(`{"username":"`+testutil.Username+`","password":"`+testutil.Password+`","code":"`+env.Code()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := other.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("other login: %d", resp.StatusCode)
	}
	status, _ := env.Do(http.MethodPost, "/auth/password", map[string]string{"oldPassword": "wrong", "newPassword": "new-password-2026"}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("invalid old password: %d", status)
	}
	env.MustDo(http.MethodPost, "/auth/password", map[string]string{"oldPassword": testutil.Password, "newPassword": "new-password-2026"}, nil)
	req, err = http.NewRequest(http.MethodGet, env.URL("/agents"), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = other.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("other session retained: %d", resp.StatusCode)
	}
	env.MustDo(http.MethodGet, "/agents", nil, nil)
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	status, _ = env.Do(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("old password accepted: %d", status)
	}
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": "new-password-2026", "code": env.Code()}, nil)
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

func TestStartMovesOldFileDirectories(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "drive", "blobs", "ab")
	if err := os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "abcdef"), []byte("blob"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "notes", "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes", "attachments", "7"), []byte("att"), 0600); err != nil {
		t.Fatal(err)
	}
	conn, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	a, err := app.New(config.Config{MasterKey: bytes.Repeat([]byte{7}, 32), Dev: true, Location: loc, DataDir: dir}, conn)
	if err != nil {
		t.Fatal(err)
	}
	rc, _, err := a.Deps.Files.For("drive").Get(context.Background(), "blobs/ab/abcdef")
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	if _, err := a.Deps.Files.For("notes").Stat(context.Background(), "attachments/7"); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"drive", "notes"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("old %s directory still exists: %v", gone, err)
		}
	}
}
