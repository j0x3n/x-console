package conn_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestNetFetchIsBuiltInAndAdvertised(t *testing.T) {
	for _, os := range []string{"linux", "windows"} {
		caps := []string{protocol.CapSystemInfo}
		c := conn.New("https://fake.invalid", "test", protocol.Hello{OS: os, Capabilities: caps})
		if !slices.Contains(c.Hello.Capabilities, protocol.CapNetFetch) || len(caps) != 1 {
			t.Fatalf("hello: %+v", c.Hello)
		}
	}
	env := testutil.New(t)
	id := env.Agent("desktop", nil, nil)
	hello, ok := env.App.Deps.Agents.Hello(id)
	if !ok || !slices.Contains(hello.Capabilities, protocol.CapNetFetch) {
		t.Fatal("hello omitted net.fetch")
	}
	var out protocol.NetFetchResult
	err := env.App.Deps.Agents.Call(context.Background(), id, protocol.MethodNetFetch, protocol.NetFetchParams{URL: "http://music.163.com"}, &out)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("built-in validation handler: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("handler not registered: %v", err)
	}
}
