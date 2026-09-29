package monitoring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestDockerImages(t *testing.T) {
	env, _ := setup(t)
	fake := &fakeDockerAgent{followEnd: make(chan struct{})}
	id := startAgent(t, env, "box", []string{protocol.CapExec, protocol.CapDocker, protocol.CapDockerLines}, fake.register)
	base := "/hosts/" + id + "/docker/images"

	relogin(t, env)
	expectStatus(t, env, http.MethodDelete, base+"/sha256:2", nil, 403, "elevation_required")
	expectStatus(t, env, http.MethodPost, base+"/prune", nil, 403, "elevation_required")
	env.Elevate()

	env.MustDo(http.MethodDelete, base+"/sha256:2", nil, nil)
	if len(fake.removed) != 1 || fake.removed[0] != "sha256:2" {
		t.Fatalf("removed: %v", fake.removed)
	}
	// An image in use: 409 and the names of the containers.
	code, body := env.Do(http.MethodDelete, base+"/sha256:1", nil, nil)
	if code != 409 || !strings.Contains(string(body), "web") {
		t.Fatalf("busy image: %d %s", code, body)
	}
	code, body = env.Do(http.MethodDelete, base+"/sha256:ghost", nil, nil)
	if code != 404 || !strings.Contains(string(body), "镜像不存在") {
		t.Fatalf("missing image: %d %s", code, body)
	}
	expectStatus(t, env, http.MethodDelete, base+"/a%3Bb", nil, 400, "validation_failed")

	events, cancel := env.App.Deps.Bus.Subscribe("docker.", 8)
	defer cancel()
	var res struct {
		Deleted        int   `json:"deleted"`
		SpaceReclaimed int64 `json:"spaceReclaimed"`
	}
	env.MustDo(http.MethodPost, base+"/prune", nil, &res)
	if res.Deleted != 3 || res.SpaceReclaimed != 1<<30 {
		t.Fatalf("prune: %+v", res)
	}
	select {
	case ev := <-events:
		if ev.Topic != "docker.images_pruned" {
			t.Fatalf("event: %s", ev.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no docker.images_pruned event")
	}
	for action, n := range map[string]int{"docker.image_remove": 3, "docker.image_prune": 1} {
		if got := auditCount(t, env, action); got != n {
			t.Errorf("audit %s: %d, want %d", action, got, n)
		}
	}
}

// readFrames reads log frames until the text has all the wanted words.
func readFrames(t *testing.T, ctx context.Context, ws *websocket.Conn, want ...string) []string {
	t.Helper()
	var frames []string
	joined := ""
	for {
		ok := true
		for _, w := range want {
			ok = ok && strings.Contains(joined, w)
		}
		if ok {
			return frames
		}
		_, b, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("read (have %q): %v", joined, err)
		}
		frames = append(frames, string(b))
		joined += string(b)
	}
}

func TestFollowLogsAsJSON(t *testing.T) {
	env, _ := setup(t)
	newFake := func() *fakeDockerAgent { return &fakeDockerAgent{followEnd: make(chan struct{})} }
	oldFake, newAgent := newFake(), newFake()
	oldID := startAgent(t, env, "old", []string{protocol.CapExec, protocol.CapDocker}, oldFake.register)
	newID := startAgent(t, env, "new", []string{protocol.CapExec, protocol.CapDocker, protocol.CapDockerLines}, newAgent.register)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A new agent: its lines are passed on with the stream and time.
	ws, _, err := websocket.Dial(ctx, env.WSURL("/hosts/"+newID+"/docker/containers/c1/logs/follow?format=json"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	frames := readFrames(t, ctx, ws, "stderr")
	var lines []protocol.DockerLogLine
	if err := json.Unmarshal([]byte(frames[0]), &lines); err != nil || len(lines) != 2 || lines[1].Stream != "stderr" || lines[0].Time == "" {
		t.Fatalf("new agent frame %q: %v", frames[0], err)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "")
	<-newAgent.followEnd

	// An old agent sends text; the server makes lines of it.
	ws, _, err = websocket.Dial(ctx, env.WSURL("/hosts/"+oldID+"/docker/containers/c1/logs/follow?format=json"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	frames = readFrames(t, ctx, ws, "new line")
	lines = nil
	for _, f := range frames {
		var part []protocol.DockerLogLine
		if err := json.Unmarshal([]byte(f), &part); err != nil {
			t.Fatalf("frame %q: %v", f, err)
		}
		lines = append(lines, part...)
	}
	texts := []string{}
	for _, l := range lines {
		if l.Stream != "stdout" {
			t.Fatalf("stream: %+v", l)
		}
		texts = append(texts, l.Text)
	}
	if strings.Join(texts, "|") != "line 1|line 2|new line" {
		t.Fatalf("text lines: %v", texts)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "")
	<-oldFake.followEnd

	// Without format=json nothing changes: plain text, and the agent is not asked for lines.
	ws, _, err = websocket.Dial(ctx, env.WSURL("/hosts/"+newID+"/docker/containers/c1/logs/follow"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	readFrames(t, ctx, ws, "new line")
	_ = ws.Close(websocket.StatusNormalClosure, "")
	newAgent.mu.Lock()
	last := newAgent.logParams[len(newAgent.logParams)-1]
	newAgent.mu.Unlock()
	if last.Lines {
		t.Fatalf("text mode asked for lines: %+v", last)
	}
}
