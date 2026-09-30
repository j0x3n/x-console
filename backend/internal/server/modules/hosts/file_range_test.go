package hosts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	agentfiles "github.com/j0x3n/x-console/backend/internal/agent/files"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestRemoteFileRangeAndFollow(t *testing.T) {
	env, _ := setup(t)
	path := filepath.Join(t.TempDir(), "access.log")
	data := bytes.Repeat([]byte("hello line\n"), 280000)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	register := func(c *conn.Client) { agentfiles.Register(c) }
	id, _, _ := startAgent(t, env, "range", "server", []string{protocol.CapFiles, protocol.CapFilesRange}, register)
	oldID, _, _ := startAgent(t, env, "old", "server", []string{protocol.CapFiles}, register)
	base := "/hosts/" + id + "/files"
	q := url.QueryEscape(path)

	for _, tc := range []struct{ offset, length int }{{len(data) - 1<<20, 1 << 20}, {15, 120}} {
		resp, body := rawDo(t, env, http.MethodGet, base+"/range?path="+q+"&offset="+strconv.Itoa(tc.offset)+"&length="+strconv.Itoa(tc.length), nil, 0)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("X-File-Size") != strconv.Itoa(len(data)) || !bytes.Equal(body, data[tc.offset:tc.offset+tc.length]) {
			t.Fatalf("range %d: %d %s, %d bytes", tc.offset, resp.StatusCode, resp.Header.Get("X-File-Size"), len(body))
		}
	}
	expectStatus(t, env, http.MethodGet, base+"/range?path="+q+"&offset=0&length=1048577", nil, http.StatusBadRequest, "validation_failed")
	expectStatus(t, env, http.MethodGet, "/hosts/"+oldID+"/files/range?path="+q+"&offset=0&length=10", nil, http.StatusNotImplemented, "feature_unavailable")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, env.WSURL(base+"/follow?path="+q+"&offset="+strconv.Itoa(len(data))), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("new line\n")
	_ = f.Close()
	frame := readFollowFrame(t, ctx, ws)
	if frame.Type != "append" || frame.Offset != int64(len(data)) || frame.Data != "new line\n" {
		t.Fatalf("append: %+v", frame)
	}
	if err := os.WriteFile(path, []byte("fresh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frame = readFollowFrame(t, ctx, ws)
	if frame.Type != "reset" {
		t.Fatalf("reset: %+v", frame)
	}
	frame = readFollowFrame(t, ctx, ws)
	if frame.Type != "append" || frame.Offset != 0 || frame.Data != "fresh\n" {
		t.Fatalf("after reset: %+v", frame)
	}
	_, resp, err := websocket.Dial(ctx, env.WSURL("/hosts/"+oldID+"/files/follow?path="+q+"&offset=0"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("old agent follow: %v %+v", err, resp)
	}
}

func readFollowFrame(t *testing.T, ctx context.Context, ws *websocket.Conn) struct {
	Type   string
	Offset int64
	Data   string
} {
	t.Helper()
	_, b, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Type   string
		Offset int64
		Data   string
	}
	if json.Unmarshal(b, &frame) != nil {
		t.Fatalf("frame: %q", b)
	}
	return frame
}
