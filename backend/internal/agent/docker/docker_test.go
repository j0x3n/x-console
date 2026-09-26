package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// fakeDocker serves a small part of the Engine API on a unix socket.
type fakeDocker struct {
	mu    sync.Mutex
	calls []string
	// followLines is written on a follow log request, then the request
	// blocks until the client goes away.
	followLines []string
	released    chan struct{}
}

func frame(stream byte, s string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, s...)
}

func (f *fakeDocker) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	f.mu.Unlock()
	path := r.URL.Path
	switch {
	case path == "/_ping":
		_, _ = w.Write([]byte("OK"))
	case path == "/containers/json":
		items := []map[string]any{
			{"Id": "aaa111", "Names": []string{"/web"}, "Image": "nginx:1", "State": "running", "Status": "Up 2 hours",
				"Created": 1700000000, "Ports": []map[string]any{{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}}},
		}
		if r.URL.Query().Get("all") == "1" {
			items = append(items, map[string]any{"Id": "bbb222", "Names": []string{"/old"}, "Image": "redis", "State": "exited",
				"Status": "Exited (0) 3 days ago", "Created": 1690000000, "Ports": []any{}})
		}
		_ = json.NewEncoder(w).Encode(items)
	case path == "/containers/missing/start" || path == "/containers/missing/json":
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container: missing"}`))
	case path == "/containers/aaa111/start":
		w.WriteHeader(http.StatusNotModified) // already started
	case strings.HasPrefix(path, "/containers/") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/containers/") && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case path == "/containers/tty1/json":
		_, _ = w.Write([]byte(`{"Config":{"Tty":true}}`))
	case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
		_, _ = w.Write([]byte(`{"Config":{"Tty":false}}`))
	case path == "/containers/tty1/logs":
		_, _ = w.Write([]byte("raw tty output\n"))
	case strings.HasSuffix(path, "/logs"):
		if r.URL.Query().Get("follow") == "1" {
			for _, l := range f.followLines {
				_, _ = w.Write(frame(1, l))
				w.(http.Flusher).Flush()
			}
			<-r.Context().Done()
			close(f.released)
			return
		}
		_, _ = w.Write(frame(1, "line one\n"))
		_, _ = w.Write(frame(2, "an error\n"))
		_, _ = w.Write(frame(1, "line three\n"))
	case strings.HasSuffix(path, "/stats"):
		id := strings.Split(path, "/")[2]
		_, _ = fmt.Fprintf(w, `{"id":"%s","name":"/n-%s","pids_stats":{"current":5},
			"networks":{"eth0":{"rx_bytes":100,"tx_bytes":50},"eth1":{"rx_bytes":1,"tx_bytes":2}},
			"memory_stats":{"usage":1000,"limit":4000,"stats":{"inactive_file":200}},
			"blkio_stats":{"io_service_bytes_recursive":[{"op":"Read","value":7},{"op":"write","value":9}]},
			"cpu_stats":{"cpu_usage":{"total_usage":300},"system_cpu_usage":2000,"online_cpus":2},
			"precpu_stats":{"cpu_usage":{"total_usage":100},"system_cpu_usage":1000}}`, id, id)
	case path == "/images/json":
		_, _ = w.Write([]byte(`[{"Id":"sha256:1","RepoTags":["nginx:1","nginx:latest"],"Size":1234,"Created":1700000000,"Containers":1},
			{"Id":"sha256:2","RepoTags":["<none>:<none>"],"Size":10,"Created":1600000000,"Containers":-1}]`))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeDocker) seen(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// startFake listens on a socket in a short temp dir (unix socket paths are
// limited to about 100 bytes).
func startFake(t *testing.T) (*fakeDocker, *Client) {
	t.Helper()
	dir, err := os.MkdirTemp("", "xcdk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "docker.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets not available: %v", err)
	}
	f := &fakeDocker{released: make(chan struct{})}
	srv := &http.Server{Handler: http.HandlerFunc(f.handler)}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return f, New(sock)
}

func TestPingAndMissingSocket(t *testing.T) {
	_, c := startFake(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	missing := New(filepath.Join(t.TempDir(), "none.sock"))
	var pe *protocol.Error
	if err := missing.Ping(context.Background()); !errors.As(err, &pe) || pe.Code != protocol.CodeFailed {
		t.Fatalf("missing socket: %v", err)
	}
}

func TestPS(t *testing.T) {
	_, c := startFake(t)
	running, err := c.PS(context.Background(), protocol.DockerPSParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(running.Items) != 1 {
		t.Fatalf("running: %+v", running)
	}
	web := running.Items[0]
	if web.Name != "web" || web.State != "running" || web.Created.Unix() != 1700000000 ||
		len(web.Ports) != 1 || web.Ports[0].PublicPort != 8080 || web.Ports[0].Type != "tcp" {
		t.Fatalf("container: %+v", web)
	}
	all, err := c.PS(context.Background(), protocol.DockerPSParams{All: true})
	if err != nil || len(all.Items) != 2 || all.Items[1].State != "exited" || all.Items[1].Ports == nil {
		t.Fatalf("all: %+v %v", all, err)
	}
}

func TestAction(t *testing.T) {
	f, c := startFake(t)
	ctx := context.Background()
	for _, a := range []string{protocol.DockerStart, protocol.DockerStop, protocol.DockerRestart, protocol.DockerRemove} {
		if err := c.Action(ctx, protocol.DockerActionParams{ID: "bbb222", Action: a}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	for _, want := range []string{"POST /containers/bbb222/start", "POST /containers/bbb222/stop", "POST /containers/bbb222/restart", "DELETE /containers/bbb222?force=1"} {
		if !f.seen(want) {
			t.Fatalf("missing call %q in %v", want, f.calls)
		}
	}
	// 304 Not Modified means it is already in that state.
	if err := c.Action(ctx, protocol.DockerActionParams{ID: "aaa111", Action: "start"}); err != nil {
		t.Fatal(err)
	}
	var pe *protocol.Error
	if err := c.Action(ctx, protocol.DockerActionParams{ID: "missing", Action: "start"}); !errors.As(err, &pe) || pe.Code != protocol.CodeNotFound || !strings.Contains(pe.Message, "No such container") {
		t.Fatalf("missing: %v", err)
	}
	if err := c.Action(ctx, protocol.DockerActionParams{ID: "aaa111", Action: "pause"}); !errors.As(err, &pe) || pe.Code != protocol.CodeBadParams {
		t.Fatalf("bad action: %v", err)
	}
	if err := c.Action(ctx, protocol.DockerActionParams{ID: "../images", Action: "start"}); !errors.As(err, &pe) || pe.Code != protocol.CodeBadParams {
		t.Fatalf("bad id: %v", err)
	}
}

func TestLogs(t *testing.T) {
	f, c := startFake(t)
	var buf bytes.Buffer
	if err := c.Logs(context.Background(), protocol.DockerLogsParams{ID: "aaa111", Tail: 50}, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "line one\nan error\nline three\n" {
		t.Fatalf("demuxed: %q", buf.String())
	}
	if !f.seen("GET /containers/aaa111/logs?stderr=1&stdout=1&tail=50") {
		t.Fatalf("calls: %v", f.calls)
	}
	buf.Reset()
	if err := c.Logs(context.Background(), protocol.DockerLogsParams{ID: "tty1"}, &buf); err != nil || buf.String() != "raw tty output\n" {
		t.Fatalf("tty: %q %v", buf.String(), err)
	}
	var pe *protocol.Error
	if err := c.Logs(context.Background(), protocol.DockerLogsParams{ID: "missing"}, &buf); !errors.As(err, &pe) || pe.Code != protocol.CodeNotFound {
		t.Fatalf("missing: %v", err)
	}
}

// lineWriter reports each write on a channel.
type lineWriter struct{ ch chan string }

func (w lineWriter) Write(b []byte) (int, error) {
	w.ch <- string(b)
	return len(b), nil
}

func TestLogsFollowStopsWhenCanceled(t *testing.T) {
	f, c := startFake(t)
	f.followLines = []string{"first\n", "second\n"}
	ctx, cancel := context.WithCancel(context.Background())
	w := lineWriter{ch: make(chan string, 10)}
	done := make(chan error, 1)
	go func() { done <- c.Logs(ctx, protocol.DockerLogsParams{ID: "aaa111", Follow: true}, w) }()
	var got string
	for !strings.Contains(got, "second") {
		select {
		case s := <-w.ch:
			got += s
		case <-time.After(5 * time.Second):
			t.Fatalf("no follow output, got %q", got)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("follow ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not stop")
	}
	select {
	case <-f.released:
	case <-time.After(5 * time.Second):
		t.Fatal("docker request still open")
	}
	if !f.seen("GET /containers/aaa111/logs?follow=1") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestDemuxPartialFrame(t *testing.T) {
	in := append(frame(1, "ok\n"), frame(1, "cut")[:9]...)
	var out bytes.Buffer
	if err := Demux(bytes.NewReader(in), &out); err != nil || out.String() != "ok\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
}

func TestStats(t *testing.T) {
	_, c := startFake(t)
	list, err := c.Stats(context.Background(), protocol.DockerStatsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("stats: %+v", list)
	}
	s := list.Items[0]
	// cpu: 200/1000 * 2 cores * 100 = 40%; memory: 1000-200 of 4000.
	if s.ID != "aaa111" || s.Name != "n-aaa111" || s.CPUPercent != 40 || s.MemUsage != 800 || s.MemPercent != 20 ||
		s.NetRx != 101 || s.NetTx != 52 || s.BlockRead != 7 || s.BlockWrite != 9 || s.PIDs != 5 {
		t.Fatalf("stats: %+v", s)
	}
	one, err := c.Stats(context.Background(), protocol.DockerStatsParams{ID: "bbb222"})
	if err != nil || len(one.Items) != 1 || one.Items[0].ID != "bbb222" {
		t.Fatalf("one: %+v %v", one, err)
	}
}

func TestComputeStatsCgroupV1(t *testing.T) {
	var r rawStats
	_ = json.Unmarshal([]byte(`{"memory_stats":{"usage":500,"limit":0,"stats":{"total_inactive_file":100,"inactive_file":1}},
		"cpu_stats":{"cpu_usage":{"total_usage":10,"percpu_usage":[1,2,3,4]},"system_cpu_usage":100},
		"precpu_stats":{"cpu_usage":{"total_usage":0},"system_cpu_usage":0}}`), &r)
	s := computeStats(r)
	if s.MemUsage != 400 || s.MemPercent != 0 || s.CPUPercent != 40 {
		t.Fatalf("%+v", s)
	}
}

func TestImages(t *testing.T) {
	_, c := startFake(t)
	list, err := c.Images(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || len(list.Items[0].Tags) != 2 || len(list.Items[1].Tags) != 0 || list.Items[0].Size != 1234 {
		t.Fatalf("images: %+v", list)
	}
}

// registrar records handlers so they can be called without a connection.
type registrar struct {
	h map[string]rpc.Handler
	s map[string]rpc.StreamHandler
}

func (r *registrar) Handle(m string, h rpc.Handler)             { r.h[m] = h }
func (r *registrar) HandleStream(m string, h rpc.StreamHandler) { r.s[m] = h }

func TestRegisterClient(t *testing.T) {
	_, c := startFake(t)
	reg := &registrar{h: map[string]rpc.Handler{}, s: map[string]rpc.StreamHandler{}}
	RegisterClient(reg, c)
	for _, m := range []string{protocol.MethodDockerPS, protocol.MethodDockerAction, protocol.MethodDockerStats, protocol.MethodDockerImages} {
		if reg.h[m] == nil {
			t.Fatalf("%s not registered", m)
		}
	}
	if reg.s[protocol.MethodDockerLogs] == nil {
		t.Fatal("logs stream not registered")
	}
	out, err := reg.h[protocol.MethodDockerPS](context.Background(), json.RawMessage(`{"all":true}`))
	if err != nil || len(out.(protocol.DockerContainerList).Items) != 2 {
		t.Fatalf("ps: %+v %v", out, err)
	}
	var pe *protocol.Error
	if _, err := reg.h[protocol.MethodDockerAction](context.Background(), json.RawMessage(`{"id":1}`)); !errors.As(err, &pe) || pe.Code != protocol.CodeBadParams {
		t.Fatalf("bad params: %v", err)
	}
}
