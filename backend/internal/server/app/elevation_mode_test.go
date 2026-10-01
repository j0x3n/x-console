package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// verifiedAgo pretends the last verification happened d ago.
func verifiedAgo(t *testing.T, env *testutil.Env, d time.Duration) {
	t.Helper()
	until := time.Now().UTC().Add(-d).Add(5 * time.Minute)
	if _, err := env.App.Deps.DB.Exec(`UPDATE sessions SET elevated_until = ?`, until); err != nil {
		t.Fatal(err)
	}
}

// pairing is an ordinary elevated operation.
func pairing(env *testutil.Env) int {
	status, _ := env.Do(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": "x", "kind": "server"}, nil)
	return status
}

func setMode(t *testing.T, env *testutil.Env, mode string) {
	t.Helper()
	env.Elevate()
	env.MustDo(http.MethodPut, "/auth/elevation-mode", map[string]string{"mode": mode}, nil)
}

// B48：四种二次验证方式。
func TestElevationModes(t *testing.T) {
	env := testutil.New(t)
	var got struct{ Mode string }
	env.MustDo(http.MethodGet, "/auth/elevation-mode", nil, &got)
	if got.Mode != "always" {
		t.Fatalf("default mode: %q", got.Mode)
	}
	// always: 6 minutes later it asks again.
	verifiedAgo(t, env, 6*time.Minute)
	if s := pairing(env); s != http.StatusForbidden {
		t.Fatalf("always after 6m: %d", s)
	}

	setMode(t, env, "30m")
	verifiedAgo(t, env, 29*time.Minute)
	if s := pairing(env); s >= 300 {
		t.Fatalf("30m after 29m: %d", s)
	}
	verifiedAgo(t, env, 31*time.Minute)
	if s := pairing(env); s != http.StatusForbidden {
		t.Fatalf("30m after 31m: %d", s)
	}

	setMode(t, env, "session")
	verifiedAgo(t, env, 20*24*time.Hour)
	if s := pairing(env); s >= 300 {
		t.Fatalf("session after days: %d", s)
	}
	// A new session has never verified.
	if _, err := env.App.Deps.DB.Exec(`UPDATE sessions SET elevated_until = NULL`); err != nil {
		t.Fatal(err)
	}
	if s := pairing(env); s != http.StatusForbidden {
		t.Fatalf("session never verified: %d", s)
	}

	setMode(t, env, "off")
	if _, err := env.App.Deps.DB.Exec(`UPDATE sessions SET elevated_until = NULL`); err != nil {
		t.Fatal(err)
	}
	if s := pairing(env); s >= 300 {
		t.Fatalf("off: %d", s)
	}
	// Strict operations still ask: changing the mode itself.
	if s, _ := env.Do(http.MethodPut, "/auth/elevation-mode", map[string]string{"mode": "always"}, nil); s != http.StatusForbidden {
		t.Fatalf("strict under off: %d", s)
	}
	if s, _ := env.Do(http.MethodPost, "/auth/totp/enroll", nil, nil); s != http.StatusForbidden {
		t.Fatalf("totp enroll under off: %d", s)
	}
	if s, _ := env.Do(http.MethodPut, "/auth/elevation-mode", map[string]string{"mode": "never"}, nil); s != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", s)
	}

	// The change is in the audit log with old and new value.
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'auth.elevation_mode'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("audit rows: %d %v", n, err)
	}
}
