package agenthub_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type failPairing struct {
	contracts.HostPairingInfo
	save, apply atomic.Bool
}

func (p *failPairing) SavePairing(ctx context.Context, tx *sql.Tx, hash string, in contracts.HostInfoInput) error {
	if p.save.Load() {
		return errors.New("save failure")
	}
	return p.HostPairingInfo.SavePairing(ctx, tx, hash, in)
}
func (p *failPairing) ApplyPairing(ctx context.Context, tx *sql.Tx, hash, id, kind string) error {
	if p.apply.Load() {
		return errors.New("apply failure")
	}
	return p.HostPairingInfo.ApplyPairing(ctx, tx, hash, id, kind)
}

func TestPairingInfoRollbackRetryAndConcurrentUse(t *testing.T) {
	env := testutil.New(t)
	original, _ := module.Lookup[contracts.HostPairingInfo](env.App.Deps.Registry, contracts.HostPairingInfoKey)
	provider := &failPairing{HostPairingInfo: original}
	hub := env.App.Deps.Agents
	hub.SetPairingInfo(provider)
	note := "配对备注"
	input := &contracts.HostInfoInput{Note: &note}
	provider.save.Store(true)
	if _, _, err := hub.CreatePairingCodeWithInfo(context.Background(), "test", "server", input); err == nil {
		t.Fatal("save should fail")
	}
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM pairing_codes").Scan(&count); err != nil || count != 0 {
		t.Fatalf("creation rollback %d %v", count, err)
	}
	provider.save.Store(false)
	code, _, err := hub.CreatePairingCodeWithInfo(context.Background(), "test", "server", input)
	if err != nil {
		t.Fatal(err)
	}
	provider.apply.Store(true)
	if _, _, err = hub.Pair(context.Background(), code, protocol.Hello{}); err == nil {
		t.Fatal("apply should fail")
	}
	if valid, err := hub.PairingCodeValid(context.Background(), code); err != nil || !valid {
		t.Fatalf("code consumed after failure %v %v", valid, err)
	}
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM agents").Scan(&count); err != nil || count != 0 {
		t.Fatalf("agent rollback %d %v", count, err)
	}
	provider.apply.Store(false)
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := hub.Pair(context.Background(), code, protocol.Hello{}); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("concurrent successful pairs %d", successes.Load())
	}
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM host_info WHERE note=?", note).Scan(&count); err != nil || count != 1 {
		t.Fatalf("metadata %d %v", count, err)
	}
}

func TestWhoamiAgentAuthenticationAndTrustedPeer(t *testing.T) {
	env := testutil.New(t)
	hub := env.App.Deps.Agents
	code, _, err := hub.CreatePairingCode(context.Background(), "whoami", "server")
	if err != nil {
		t.Fatal(err)
	}
	id, token, err := hub.Pair(context.Background(), code, protocol.Hello{})
	if err != nil {
		t.Fatal(err)
	}
	trusted, _ := httpx.ParseTrustedProxies("127.0.0.1,::1")
	hub.SetTrustedProxies(trusted)
	for _, tc := range []struct{ peer, header, want string }{{"192.0.2.1:42", "8.8.8.8", "192.0.2.1"}, {"127.0.0.1:42", "8.8.8.8", "8.8.8.8"}, {"[::1]:42", "2001:4860::8888", "2001:4860::8888"}} {
		req := httptest.NewRequest(http.MethodGet, "/agent/whoami", nil)
		req.RemoteAddr = tc.peer
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Forwarded-For", tc.header)
		w := httptest.NewRecorder()
		hub.ServeWhoami(w, req)
		if w.Code != 200 || hub.SourceIP(id) != tc.want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("whoami %d %s", w.Code, w.Body.String())
		}
	}
	if err := hub.Revoke(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	hub.ServeWhoami(w, req)
	if w.Code != 401 {
		t.Fatalf("revoked token %d", w.Code)
	}
	response, err := http.Get(env.URL("/agent/whoami"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("public whoami without token %d", response.StatusCode)
	}
}
