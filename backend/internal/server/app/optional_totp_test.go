package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func TestSkipTOTPFromFreshSetup(t *testing.T) {
	conn, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	instance, err := app.New(config.Config{MasterKey: bytes.Repeat([]byte{7}, 32), Dev: true, Location: time.UTC, DataDir: t.TempDir()}, conn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := instance.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer instance.Stop()
	server := httptest.NewServer(instance.Handler)
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	request := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var input *bytes.Reader
		if body == nil {
			input = bytes.NewReader(nil)
		} else {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			input = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, server.URL+app.APIPrefix+path, input)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "x-console")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var output bytes.Buffer
		if _, err := output.ReadFrom(resp.Body); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, output.Bytes()
	}
	assertStatus := func(want int, method, path string, body any) []byte {
		t.Helper()
		code, raw := request(method, path, body)
		if code != want {
			t.Fatalf("%s %s: %d %s", method, path, code, raw)
		}
		return raw
	}
	assertStatus(http.StatusConflict, "POST", "/auth/setup/skip-totp", map[string]string{"password": "first-password-2026"})
	assertStatus(http.StatusOK, "POST", "/auth/setup", map[string]string{"username": "first", "password": "first-password-2026"})
	assertStatus(http.StatusUnauthorized, "POST", "/auth/setup/skip-totp", map[string]string{"password": "wrong-password"})
	assertStatus(http.StatusNoContent, "POST", "/auth/setup/skip-totp", map[string]string{"password": "first-password-2026"})
	var status struct {
		SetupRequired bool
		Authenticated bool
		TotpEnabled   *bool
	}
	if err := json.Unmarshal(assertStatus(http.StatusOK, "GET", "/auth/status", nil), &status); err != nil {
		t.Fatal(err)
	}
	if status.SetupRequired || !status.Authenticated || status.TotpEnabled == nil || *status.TotpEnabled {
		t.Fatalf("after skip: %+v", status)
	}
	assertStatus(http.StatusConflict, "POST", "/auth/setup/skip-totp", map[string]string{"password": "first-password-2026"})
	assertStatus(http.StatusNoContent, "POST", "/auth/logout", nil)
	assertStatus(http.StatusNoContent, "POST", "/auth/login", map[string]string{"username": "first", "password": "first-password-2026"})
	assertStatus(http.StatusForbidden, "POST", "/agents/pairing-codes", map[string]string{"name": "a", "kind": "server"})
	assertStatus(http.StatusUnauthorized, "POST", "/auth/elevate", map[string]string{"password": "wrong-password"})
	assertStatus(http.StatusOK, "POST", "/auth/elevate", map[string]string{"password": "first-password-2026"})
	assertStatus(http.StatusOK, "POST", "/agents/pairing-codes", map[string]string{"name": "a", "kind": "server"})
}

func TestLoginFailureLimitWithoutTOTP(t *testing.T) {
	conn, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	instance, err := app.New(config.Config{MasterKey: bytes.Repeat([]byte{7}, 32), Dev: true, Location: time.UTC, DataDir: t.TempDir()}, conn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := instance.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer instance.Stop()
	server := httptest.NewServer(instance.Handler)
	defer server.Close()
	client := &http.Client{}
	for _, step := range []struct {
		path string
		body any
	}{{"/auth/setup", map[string]string{"username": "first", "password": "first-password-2026"}}, {"/auth/setup/skip-totp", map[string]string{"password": "first-password-2026"}}} {
		raw, _ := json.Marshal(step.body)
		req, _ := http.NewRequest(http.MethodPost, server.URL+app.APIPrefix+step.path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Fatalf("%s: %d", step.path, resp.StatusCode)
		}
	}
	for i := 0; i < 6; i++ {
		raw, _ := json.Marshal(map[string]string{"username": "first", "password": "wrong-password"})
		req, _ := http.NewRequest(http.MethodPost, server.URL+app.APIPrefix+"/auth/login", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := http.StatusUnauthorized
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		if resp.StatusCode != want {
			t.Fatalf("attempt %d: %d", i, resp.StatusCode)
		}
	}
}
