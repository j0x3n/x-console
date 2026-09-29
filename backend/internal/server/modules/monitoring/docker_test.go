package monitoring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// fakeDockerAgent answers docker.* like the agent package would.
type fakeDockerAgent struct {
	mu        sync.Mutex
	actions   []protocol.DockerActionParams
	removed   []string
	logParams []protocol.DockerLogsParams
	followEnd chan struct{} // closed when a follow stream ends
}

func (f *fakeDockerAgent) register(c *conn.Client) {
	c.Handle(protocol.MethodDockerPS, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerPSParams
		_ = json.Unmarshal(raw, &p)
		items := []protocol.DockerContainer{{ID: "c1", Name: "web", Image: "nginx:1", State: "running", Status: "Up 1 hour",
			Created: time.Unix(1700000000, 0).UTC(), Ports: []protocol.DockerPort{{PrivatePort: 80, PublicPort: 8080, Type: "tcp", IP: "0.0.0.0"}}}}
		if p.All {
			items = append(items, protocol.DockerContainer{ID: "c2", Name: "old", Image: "redis", State: "exited", Ports: []protocol.DockerPort{}})
		}
		return protocol.DockerContainerList{Items: items}, nil
	})
	c.Handle(protocol.MethodDockerAction, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerActionParams
		_ = json.Unmarshal(raw, &p)
		if p.ID == "ghost" {
			return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: "No such container: ghost"}
		}
		f.mu.Lock()
		f.actions = append(f.actions, p)
		f.mu.Unlock()
		return nil, nil
	})
	c.Handle(protocol.MethodDockerStats, func(ctx context.Context, raw json.RawMessage) (any, error) {
		return protocol.DockerStatsList{Items: []protocol.DockerStats{{ID: "c1", Name: "web", CPUPercent: 12.5, MemUsage: 100, MemLimit: 1000, MemPercent: 10}}}, nil
	})
	c.Handle(protocol.MethodDockerImages, func(ctx context.Context, raw json.RawMessage) (any, error) {
		return protocol.DockerImageList{Items: []protocol.DockerImage{{ID: "sha256:1", Tags: []string{"nginx:1"}, Size: 42}}}, nil
	})
	c.Handle(protocol.MethodDockerImageRemove, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerImageRemoveParams
		_ = json.Unmarshal(raw, &p)
		switch p.ID {
		case "sha256:1":
			return nil, &protocol.Error{Code: protocol.CodeExists, Message: "conflict: image is being used by running container c1"}
		case "sha256:ghost":
			return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: "No such image: sha256:ghost"}
		}
		f.mu.Lock()
		f.removed = append(f.removed, p.ID)
		f.mu.Unlock()
		return nil, nil
	})
	c.Handle(protocol.MethodDockerImagePrune, func(ctx context.Context, raw json.RawMessage) (any, error) {
		return protocol.DockerImagePruneResult{Deleted: 3, SpaceReclaimed: 1 << 30}, nil
	})
	c.HandleStream(protocol.MethodDockerLogs, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		var p protocol.DockerLogsParams
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.logParams = append(f.logParams, p)
		f.mu.Unlock()
		if p.ID == "ghost" {
			return &protocol.Error{Code: protocol.CodeNotFound, Message: "No such container: ghost"}
		}
		if p.Lines {
			// A new agent: one frame of lines with their stream.
			if err := s.Send(ctx, []byte(`[{"stream":"stdout","text":"out","time":"2026-09-29T10:00:00Z"},{"stream":"stderr","text":"err"}]`)); err != nil {
				return err
			}
			if !p.Follow {
				return nil
			}
			<-s.Context().Done()
			close(f.followEnd)
			return nil
		}
		if err := s.Send(ctx, []byte("line 1\nline 2\r\n")); err != nil {
			return err
		}
		if !p.Follow {
			return nil
		}
		_ = s.Send(ctx, []byte("new line\n"))
		<-s.Context().Done()
		close(f.followEnd)
		return nil
	})
}

func (f *fakeDockerAgent) actionList() []protocol.DockerActionParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]protocol.DockerActionParams(nil), f.actions...)
}

func TestDocker(t *testing.T) {
	env, _ := setup(t)
	fake := &fakeDockerAgent{followEnd: make(chan struct{})}
	id := startAgent(t, env, "box", []string{protocol.CapExec, protocol.CapDocker}, fake.register)
	plain := startAgent(t, env, "plain", []string{protocol.CapExec}, nil)
	base := "/hosts/" + id + "/docker"

	var list api.DockerContainerList
	env.MustDo(http.MethodGet, base+"/containers", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "web" || *list.Items[0].Ports[0].PublicPort != 8080 {
		t.Fatalf("containers: %+v", list)
	}
	env.MustDo(http.MethodGet, base+"/containers?all=true", nil, &list)
	if len(list.Items) != 2 || list.Items[1].Ports == nil {
		t.Fatalf("all containers: %+v", list)
	}
	expectStatus(t, env, http.MethodGet, "/hosts/"+plain+"/docker/containers", nil, 501, "unsupported")
	expectStatus(t, env, http.MethodGet, "/hosts/ssh:1/docker/containers", nil, 501, "unsupported")
	expectStatus(t, env, http.MethodGet, "/hosts/nope/docker/containers", nil, 404, "not_found")

	var stats api.DockerStatsList
	env.MustDo(http.MethodGet, base+"/stats", nil, &stats)
	if len(stats.Items) != 1 || stats.Items[0].CpuPercent != 12.5 || stats.Items[0].MemLimit != 1000 {
		t.Fatalf("stats: %+v", stats)
	}
	var images api.DockerImageList
	env.MustDo(http.MethodGet, base+"/images", nil, &images)
	if len(images.Items) != 1 || images.Items[0].Tags[0] != "nginx:1" {
		t.Fatalf("images: %+v", images)
	}

	var logs api.DockerLogs
	env.MustDo(http.MethodGet, base+"/containers/c1/logs?tail=50", nil, &logs)
	if strings.Join(logs.Lines, "|") != "line 1|line 2" {
		t.Fatalf("logs: %+v", logs)
	}
	expectStatus(t, env, http.MethodGet, base+"/containers/ghost/logs", nil, 404, "not_found")

	// Start and restart need no elevation; stop and remove do. All are audited.
	relogin(t, env)
	env.MustDo(http.MethodPost, base+"/containers/c1/restart", nil, nil)
	expectStatus(t, env, http.MethodPost, base+"/containers/c1/stop", nil, 403, "elevation_required")
	expectStatus(t, env, http.MethodPost, base+"/containers/c2/remove", nil, 403, "elevation_required")
	env.Elevate()
	env.MustDo(http.MethodPost, base+"/containers/c1/stop", nil, nil)
	env.MustDo(http.MethodPost, base+"/containers/c2/remove", nil, nil)
	expectStatus(t, env, http.MethodPost, base+"/containers/ghost/start", nil, 404, "not_found")
	expectStatus(t, env, http.MethodPost, base+"/containers/c1/pause", nil, 400, "validation_failed")
	acts := fake.actionList()
	if len(acts) != 3 || acts[0].Action != "restart" || acts[1].Action != "stop" || acts[2] != (protocol.DockerActionParams{ID: "c2", Action: "remove"}) {
		t.Fatalf("actions: %+v", acts)
	}
	if auditCount(t, env, "docker.container.remove") != 1 || auditCount(t, env, "docker.container.start") != 1 {
		t.Fatal("docker actions not audited")
	}

	// Following logs over a WebSocket.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, env.WSURL(base+"/containers/c1/logs/follow?tail=10"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for !strings.Contains(got, "new line") {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("follow read (have %q): %v", got, err)
		}
		if typ != websocket.MessageText {
			t.Fatalf("frame type %v", typ)
		}
		got += string(b)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "")
	select {
	case <-fake.followEnd:
	case <-ctx.Done():
		t.Fatal("follow stream not closed on the agent")
	}
	fake.mu.Lock()
	last := fake.logParams[len(fake.logParams)-1]
	fake.mu.Unlock()
	if !last.Follow || last.Tail != 10 || last.ID != "c1" {
		t.Fatalf("follow params: %+v", last)
	}
	_, resp, err := websocket.Dial(ctx, env.WSURL("/hosts/"+plain+"/docker/containers/c1/logs/follow"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("follow without docker: %v %+v", err, resp)
	}
}
