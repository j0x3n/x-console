package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func TestDeployPairingCodeWorksThroughHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x-console.db")
	code, err := pairingCode(context.Background(), path, "deploy-host", "server")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("invalid code: %q", code)
	}

	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var actor string
	if err := db.QueryRow(`SELECT actor FROM audit_log WHERE action = 'agent.pairing_code' ORDER BY id DESC LIMIT 1`).Scan(&actor); err != nil || actor != "system:deploy" {
		t.Fatalf("audit actor: %q, %v", actor, err)
	}
	a, err := app.New(config.Config{DataDir: filepath.Dir(path), MasterKey: make([]byte, 32), Dev: true}, db)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"code": code, "hostname": "deploy-host", "os": "linux", "arch": "amd64", "version": "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/pair", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"agentId"`) {
		t.Fatalf("pair response: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agent/pair", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	a.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("reused pairing code: %d %s", w.Code, w.Body.String())
	}
}
