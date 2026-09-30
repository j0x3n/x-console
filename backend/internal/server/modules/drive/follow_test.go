package drive_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type driveFollowFrame struct {
	Type   string `json:"type"`
	Offset int64  `json:"offset"`
	Data   string `json:"data"`
}

func driveWSHeader(env *testutil.Env) http.Header {
	u, _ := url.Parse(env.Server.URL)
	parts := []string{}
	for _, cookie := range env.Client.Jar.Cookies(u) {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return http.Header{"Cookie": []string{strings.Join(parts, "; ")}}
}

func readDriveFollow(t *testing.T, ctx context.Context, ws *websocket.Conn) driveFollowFrame {
	t.Helper()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var frame driveFollowFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("frame %q: %v", raw, err)
	}
	return frame
}

func TestDriveFollowAppendResetAndDelete(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "log.txt", "first\n", false)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	path := "/drive/items/" + strconv.FormatInt(file.Id, 10) + "/follow?offset=6"
	ws, _, err := websocket.Dial(ctx, env.WSURL(path), &websocket.DialOptions{HTTPHeader: driveWSHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	resp, body := saveContent(t, env, file.Id, "first\nsecond\n", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("append save: %d %s", resp.StatusCode, body)
	}
	frame := readDriveFollow(t, ctx, ws)
	if frame.Type != "append" || frame.Offset != 6 || frame.Data != "second\n" {
		t.Fatalf("append frame: %+v", frame)
	}
	resp, body = saveContent(t, env, file.Id, "new\n", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset save: %d %s", resp.StatusCode, body)
	}
	frame = readDriveFollow(t, ctx, ws)
	if frame.Type != "reset" {
		t.Fatalf("reset frame: %+v", frame)
	}
	frame = readDriveFollow(t, ctx, ws)
	if frame.Type != "append" || frame.Offset != 0 || frame.Data != "new\n" {
		t.Fatalf("post-reset frame: %+v", frame)
	}
	env.MustDo(http.MethodDelete, "/drive/items/"+strconv.FormatInt(file.Id, 10), nil, nil)
	_, _, err = ws.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("delete close: %v", err)
	}
}

func TestDriveFollowUTF8BoundaryAndLockedItem(t *testing.T) {
	env := testutil.New(t, drive.New)
	data := strings.Repeat("x", 256<<10-1) + "中"
	file := upload(t, env, "large.log", data, false)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, env.WSURL("/drive/items/"+strconv.FormatInt(file.Id, 10)+"/follow?offset=0"), &websocket.DialOptions{HTTPHeader: driveWSHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	ws.SetReadLimit(1 << 20)
	first := readDriveFollow(t, ctx, ws)
	second := readDriveFollow(t, ctx, ws)
	ws.Close(websocket.StatusNormalClosure, "")
	if first.Type != "append" || first.Offset != 0 || len(first.Data) != 256<<10-1 || second.Type != "append" || second.Offset != int64(len(first.Data)) || second.Data != "中" {
		t.Fatalf("UTF-8 split: first=%d bytes second=%+v", len(first.Data), second)
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	hidden := upload(t, env, "hidden.log", "secret", true)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	_, resp, err := websocket.Dial(ctx, env.WSURL("/drive/items/"+strconv.FormatInt(hidden.Id, 10)+"/follow?offset=0"), &websocket.DialOptions{HTTPHeader: driveWSHeader(env)})
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("locked follow: %v %+v", err, resp)
	}
}
