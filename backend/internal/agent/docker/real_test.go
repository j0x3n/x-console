package docker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// TestRealDocker runs against a real Docker Engine when XC_TEST_DOCKER_SOCKET
// points at its socket. Set XC_TEST_DOCKER_CONTAINER to a running container
// that writes logs to also check logs and stats.
func TestRealDocker(t *testing.T) {
	sock := os.Getenv("XC_TEST_DOCKER_SOCKET")
	if sock == "" {
		t.Skip("XC_TEST_DOCKER_SOCKET not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New(sock)
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := c.PS(ctx, protocol.DockerPSParams{All: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("containers: %+v", list.Items)
	images, err := c.Images(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("images: %+v", images.Items)
	id := os.Getenv("XC_TEST_DOCKER_CONTAINER")
	if id == "" {
		return
	}
	stats, err := c.Stats(ctx, protocol.DockerStatsParams{})
	if err != nil || len(stats.Items) == 0 {
		t.Fatalf("stats: %+v %v", stats, err)
	}
	t.Logf("stats: %+v", stats.Items)
	var b strings.Builder
	if err := c.Logs(ctx, protocol.DockerLogsParams{ID: id, Tail: 5}, &b); err != nil || b.Len() == 0 {
		t.Fatalf("logs: %q %v", b.String(), err)
	}
	t.Logf("logs: %q", b.String())
	fctx, fcancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer fcancel()
	w := lineWriter{ch: make(chan string, 100)}
	if err := c.Logs(fctx, protocol.DockerLogsParams{ID: id, Tail: 1, Follow: true}, w); err != nil {
		t.Fatalf("follow: %v", err)
	}
	if len(w.ch) < 2 {
		t.Fatalf("follow got %d chunks", len(w.ch))
	}
	for _, a := range []string{protocol.DockerRestart, protocol.DockerStop, protocol.DockerStart} {
		if err := c.Action(ctx, protocol.DockerActionParams{ID: id, Action: a}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
}
