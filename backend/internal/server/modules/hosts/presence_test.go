package hosts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestLivePresenceStateChangesAndLegacy(t *testing.T) {
	env := testutil.New(t)
	provider, ok := module.Lookup[contracts.Presence](env.App.Deps.Registry, contracts.PresenceKey)
	if !ok {
		t.Fatal("presence provider missing")
	}
	clients := make(chan *conn.Client, 1)
	id := env.Agent("desktop", []string{protocol.CapPresence}, func(c *conn.Client) {
		c.Handle(protocol.MethodPresenceGet, func(context.Context, json.RawMessage) (any, error) {
			return protocol.PresenceSample{Known: true, IdleSeconds: 1}, nil
		})
		clients <- c
	})
	c := <-clients
	waitPresence := func(state string) contracts.HostPresence {
		t.Helper()
		end := time.Now().Add(3 * time.Second)
		for time.Now().Before(end) {
			p := provider.State(id)
			if p.State == state {
				return p
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("presence wanted %s got %+v", state, provider.State(id))
		return contracts.HostPresence{}
	}
	first := waitPresence("active")
	if err := c.Emit(context.Background(), protocol.EventPresenceUpdate, protocol.PresenceSample{Known: true, IdleSeconds: 3}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if p := provider.State(id); !p.Since.Equal(first.Since) {
		t.Fatalf("heartbeat changed since: %+v %+v", first, p)
	}
	ch, cancel := provider.Subscribe()
	defer cancel()
	if err := c.Emit(context.Background(), protocol.EventPresenceUpdate, protocol.PresenceSample{Known: true, IdleSeconds: 600}); err != nil {
		t.Fatal(err)
	}
	waitPresence("idle")
	select {
	case p := <-ch:
		if p.State != "idle" {
			t.Fatalf("subscription %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("missing subscription event")
	}
	if err := c.Emit(context.Background(), protocol.EventPresenceUpdate, protocol.PresenceSample{Known: true, Locked: new(true)}); err != nil {
		t.Fatal(err)
	}
	waitPresence("locked")
	var detail api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+id, nil, &detail)
	if detail.Presence == nil || string(detail.Presence.State) != "locked" {
		t.Fatalf("detail %+v", detail.Presence)
	}
	legacy := env.Agent("desktop", nil, nil)
	if p := provider.State(legacy); !p.Online || p.Known || p.State != "unknown" {
		t.Fatalf("legacy %+v", p)
	}
	env.MustDo(http.MethodDelete, "/agents/"+id, nil, nil)
	waitPresence("offline")
	if p := provider.State(id); p.Known || p.IdleSeconds != nil {
		t.Fatalf("revoked %+v", p)
	}
}
