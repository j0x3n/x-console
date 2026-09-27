package vault_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/modules/vault/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func checkStatus(t *testing.T, env *testutil.Env, configured, unlocked bool) api.VaultStatus {
	t.Helper()
	var status api.VaultStatus
	env.MustDo(http.MethodGet, "/vault/status", nil, &status)
	if status.Configured != configured || status.Unlocked != unlocked || (status.UnlockedUntil != nil) != unlocked {
		t.Fatalf("vault status: %+v", status)
	}
	return status
}

func checkCode(t *testing.T, env *testutil.Env, method, path string, body any, want int, code string) {
	t.Helper()
	status, raw := env.Do(method, path, body, nil)
	var response struct{ Code string }
	if err := json.Unmarshal(raw, &response); status != want || err != nil || response.Code != code {
		t.Fatalf("%s %s: status %d, body %s", method, path, status, raw)
	}
}

func TestVaultSessionLifecycle(t *testing.T) {
	env := testutil.New(t)
	checkStatus(t, env, false, false)
	checkCode(t, env, http.MethodPost, "/vault/setup", map[string]string{"password": "short"}, 400, "validation_failed")
	var status api.VaultStatus
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, &status)
	if !status.Configured || !status.Unlocked || status.UnlockedUntil == nil {
		t.Fatalf("setup: %+v", status)
	}
	checkCode(t, env, http.MethodPost, "/vault/setup", map[string]string{"password": "secret-two"}, 409, "already_setup")
	var hash string
	var encrypted int
	if err := env.App.Deps.DB.QueryRow("SELECT value, encrypted FROM settings WHERE key = 'vault.password_hash'").Scan(&hash, &encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "secret-one") || !strings.Contains(hash, "$argon2id$") || encrypted != 0 {
		t.Fatalf("stored vault password is not an argon2id hash: %q", hash)
	}
	var sessionID string
	if err := env.App.Deps.DB.QueryRow("SELECT id FROM sessions LIMIT 1").Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	near := time.Now().UTC().Add(time.Minute)
	if _, err := env.App.Deps.DB.Exec("UPDATE sessions SET vault_until = ? WHERE id = ?", near, sessionID); err != nil {
		t.Fatal(err)
	}
	status = checkStatus(t, env, true, true)
	if status.UnlockedUntil.Before(time.Now().Add(14 * time.Minute)) {
		t.Fatalf("vault window did not slide: %v", status.UnlockedUntil)
	}
	past := time.Now().UTC().Add(-time.Second)
	if _, err := env.App.Deps.DB.Exec("UPDATE sessions SET vault_until = ? WHERE id = ?", past, sessionID); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, env, true, false)
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "secret-one"}, &status)
	if !status.Unlocked {
		t.Fatalf("unlock: %+v", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	checkStatus(t, env, true, false)
	checkCode(t, env, http.MethodPost, "/vault/password", map[string]string{"oldPassword": "wrong-password", "newPassword": "secret-two"}, 401, "invalid_credentials")
	env.MustDo(http.MethodPost, "/vault/password", map[string]string{"oldPassword": "secret-one", "newPassword": "secret-two"}, nil)
	checkCode(t, env, http.MethodPost, "/vault/unlock", map[string]string{"password": "secret-one"}, 401, "invalid_credentials")
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "secret-two"}, &status)
	checkStatus(t, env, true, true)
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	statusCode, _ := env.Do(http.MethodGet, "/vault/status", nil, nil)
	if statusCode != 401 {
		t.Fatalf("logged out status: %d", statusCode)
	}
}

func TestVaultRateLimitAndAudit(t *testing.T) {
	env := testutil.New(t)
	secret := "private-vault-password"
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": secret}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	for range 5 {
		checkCode(t, env, http.MethodPost, "/vault/unlock", map[string]string{"password": "wrong-password"}, 401, "invalid_credentials")
	}
	checkCode(t, env, http.MethodPost, "/vault/unlock", map[string]string{"password": secret}, 429, "rate_limited")
	rows, err := env.App.Deps.DB.Query("SELECT target, detail, result FROM audit_log WHERE action LIKE 'vault.%'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var target, detail, result string
		if err := rows.Scan(&target, &detail, &result); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(target+detail+result, secret) || strings.Contains(target+detail+result, "wrong-password") || strings.Contains(target+detail+result, "$argon2id$") {
			t.Fatalf("audit contains password: %s %s %s", target, detail, result)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestResetVaultPasswordRetainsHiddenContent(t *testing.T) {
	env := testutil.New(t)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "original-secret"}, nil)
	var noteID int64
	if err := env.App.Deps.DB.QueryRow("INSERT INTO notes (title, body, hidden, created_at, updated_at) VALUES (?, ?, 1, ?, ?) RETURNING id", "private-note", "content", time.Now().UTC(), time.Now().UTC()).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: jar}
	req, err := http.NewRequest(http.MethodPost, env.URL("/auth/login"), strings.NewReader(`{"username":"`+testutil.Username+`","password":"`+testutil.Password+`","code":"`+env.Code()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(auth.CSRFHeader, "x-console")
	resp, err := other.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("other login: %d", resp.StatusCode)
	}
	if err := auth.ResetVaultPassword(context.Background(), env.App.Deps.DB, "reset-secret"); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, env, true, false)
	var active int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM sessions WHERE vault_until IS NOT NULL").Scan(&active); err != nil || active != 0 {
		t.Fatalf("unlocked sessions: %d, %v", active, err)
	}
	var hidden int
	if err := env.App.Deps.DB.QueryRow("SELECT hidden FROM notes WHERE id = ?", noteID).Scan(&hidden); err != nil || hidden != 1 {
		t.Fatalf("hidden note: %d, %v", hidden, err)
	}
	checkCode(t, env, http.MethodPost, "/vault/unlock", map[string]string{"password": "original-secret"}, 401, "invalid_credentials")
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "reset-secret"}, nil)
	checkStatus(t, env, true, true)
	if err := auth.ResetVaultPassword(context.Background(), env.App.Deps.DB, "tiny"); err == nil {
		t.Fatal("accepted a short password")
	}
	var until sql.NullTime
	if err := env.App.Deps.DB.QueryRow("SELECT vault_until FROM sessions WHERE vault_until IS NOT NULL LIMIT 1").Scan(&until); err != nil {
		t.Fatal(err)
	}
}
