package settings_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func TestPlainAndSecret(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	box, _ := secrets.NewBox(bytes.Repeat([]byte{1}, 32))
	s := settings.New(conn, box)

	var missing string
	if err := s.Get(ctx, "ha.url", &missing); !errors.Is(err, settings.ErrNotSet) {
		t.Fatalf("want ErrNotSet, got %v", err)
	}
	if err := s.Set(ctx, "ha.url", "http://ha.local:8123"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSecret(ctx, "ha.token", "super-secret"); err != nil {
		t.Fatal(err)
	}
	var url, token string
	_ = s.Get(ctx, "ha.url", &url)
	_ = s.Get(ctx, "ha.token", &token)
	if url != "http://ha.local:8123" || token != "super-secret" {
		t.Fatalf("got %q %q", url, token)
	}
	var raw string
	_ = conn.QueryRow("SELECT value FROM settings WHERE key = 'ha.token'").Scan(&raw)
	if strings.Contains(raw, "super-secret") {
		t.Fatal("secret stored in plain text")
	}
}
