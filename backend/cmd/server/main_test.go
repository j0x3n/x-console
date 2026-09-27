package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/store"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestPairingCodeCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XC_DATA_DIR", dir)
	t.Setenv("XC_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	var out bytes.Buffer
	if err := pairingCode([]string{"--name", "panel-host", "--kind", "server"}, &out); err != nil {
		t.Fatal(err)
	}
	code := strings.TrimSpace(out.String())
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("invalid pairing code: %q", code)
	}

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(dir, "x-console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := app.New(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()
	id, token, err := conn.Pair(context.Background(), srv.URL, code, protocol.Hello{
		OS: "linux", Arch: "amd64", Hostname: "panel-host", AgentVersion: "test",
	})
	if err != nil || id == "" || token == "" {
		t.Fatalf("pair: id=%q token=%q err=%v", id, token, err)
	}
	if _, _, err := conn.Pair(context.Background(), srv.URL, code, protocol.Hello{}); err == nil {
		t.Fatal("pairing code was accepted twice")
	}
	var actor string
	if err := db.QueryRow("SELECT actor FROM audit_log WHERE action = 'agent.pairing_code'").Scan(&actor); err != nil || actor != "system:deploy" {
		t.Fatalf("audit actor: %q, %v", actor, err)
	}
}
