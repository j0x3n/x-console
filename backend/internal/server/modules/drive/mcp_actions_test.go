package drive_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func setupMCP(t *testing.T) (*testutil.Env, *drive.Module) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*drive.Module](env.App.Deps.Registry, drive.ServiceKey)
	if !ok {
		t.Fatal("drive module not registered")
	}
	return env, m
}

func act(env *testutil.Env, name string, in any) (map[string]any, error) {
	raw, _ := json.Marshal(in)
	res, err := env.App.Deps.Actions.Run(context.Background(), name, raw)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(res)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out, nil
}

func mustAct(t *testing.T, env *testutil.Env, name string, in any) map[string]any {
	t.Helper()
	out, err := act(env, name, in)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func statusOf(err error) int {
	var e *httpx.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func id(m map[string]any, key string) int64 {
	f, _ := m[key].(float64)
	return int64(f)
}

// plain does a request with no login.
func plain(t *testing.T, env *testutil.Env, method, path string, body io.Reader, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, strings.TrimSuffix(env.URL(""), "/")+strings.TrimPrefix(path, "/api/v1"), body)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// ---- paths, info, copy ----

func TestPathsInfoAndCopy(t *testing.T) {
	env, _ := setupMCP(t)
	out := mustAct(t, env, "drive.upload_base64", map[string]any{"name": "a.txt", "dataBase64": b64("hello"), "parentPath": "/音乐/周杰伦"})
	if out["path"] != "/音乐/周杰伦/a.txt" || id(out, "size") != 5 {
		t.Fatalf("upload: %+v", out)
	}
	file := id(out, "id")

	r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/音乐//周杰伦/a.txt"})
	if r["found"] != true || id(r, "id") != file || r["isDir"] != false {
		t.Fatalf("resolve: %+v", r)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/音乐/没有"}); r["found"] != false {
		t.Fatalf("missing: %+v", r)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/"}); r["found"] != true || r["isDir"] != true {
		t.Fatalf("root: %+v", r)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/音乐/周杰伦/a.txt/下面"}); r["found"] != false {
		t.Fatalf("below a file: %+v", r)
	}
	if _, err := act(env, "drive.resolve_path", map[string]any{"path": "/a/../b"}); statusOf(err) != http.StatusBadRequest {
		t.Fatalf("dot dot: %v", err)
	}

	info := mustAct(t, env, "drive.info", map[string]any{"id": file})
	if info["path"] != "/音乐/周杰伦/a.txt" || info["shares"] != float64(0) {
		t.Fatalf("info: %+v", info)
	}
	folder := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/音乐"})
	if got := mustAct(t, env, "drive.info", map[string]any{"id": id(folder, "id")}); got["children"] != float64(1) {
		t.Fatalf("folder info: %+v", got)
	}

	// Copy a file next to itself: never overwrites.
	cp := mustAct(t, env, "drive.copy", map[string]any{"id": file, "parentPath": "/音乐/周杰伦"})
	if cp["name"] != "a (1).txt" || id(cp, "size") != 5 {
		t.Fatalf("copy: %+v", cp)
	}
	// Copy a folder tree.
	tree := mustAct(t, env, "drive.copy", map[string]any{"id": id(folder, "id"), "parentPath": "/备份"})
	if tree["path"] != "/备份/音乐" {
		t.Fatalf("tree: %+v", tree)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/备份/音乐/周杰伦/a (1).txt"}); r["found"] != true {
		t.Fatalf("copied tree misses a file: %+v", r)
	}
	// Into itself.
	sub := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/音乐/周杰伦"})
	if _, err := act(env, "drive.copy", map[string]any{"id": id(folder, "id"), "parentId": id(sub, "id")}); statusOf(err) != http.StatusBadRequest {
		t.Fatalf("copy into itself: %v", err)
	}
	// Deleting the original keeps the copy readable (content is shared by reference counting).
	env.Elevate()
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/drive/items/%d?permanent=true", file), nil, nil); status >= 300 && status != http.StatusNotFound {
		t.Fatalf("permanent delete: %d", status)
	}
	if got := mustAct(t, env, "drive.read_text", map[string]any{"id": id(cp, "id")}); got["text"] != "hello" {
		t.Fatalf("copy after deleting the original: %+v", got)
	}
	if _, err := act(env, "drive.info", map[string]any{"id": 99999}); statusOf(err) != http.StatusNotFound {
		t.Fatalf("unknown id: %v", err)
	}
}

// ---- base64 ----

func TestUploadBase64Rules(t *testing.T) {
	env, _ := setupMCP(t)
	in := func(extra map[string]any) map[string]any {
		m := map[string]any{"name": "c.jpg", "dataBase64": b64("x")}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	mustAct(t, env, "drive.upload_base64", in(nil))
	if _, err := act(env, "drive.upload_base64", in(nil)); statusOf(err) != http.StatusConflict {
		t.Fatalf("same name: %v", err)
	}
	if got := mustAct(t, env, "drive.upload_base64", in(map[string]any{"onConflict": "rename"})); got["name"] != "c (1).jpg" {
		t.Fatalf("rename: %+v", got)
	}
	for name, bad := range map[string]map[string]any{
		"too big":        in(map[string]any{"name": "big", "dataBase64": base64.StdEncoding.EncodeToString(make([]byte, 600<<10+1))}),
		"empty":          in(map[string]any{"name": "e", "dataBase64": ""}),
		"not base64":     in(map[string]any{"name": "n", "dataBase64": "***"}),
		"bad name":       in(map[string]any{"name": "a/b"}),
		"dots":           in(map[string]any{"name": ".."}),
		"both parents":   in(map[string]any{"name": "p", "parentId": 1, "parentPath": "/x"}),
		"bad onConflict": in(map[string]any{"name": "q", "onConflict": "overwrite"}),
	} {
		if _, err := act(env, "drive.upload_base64", bad); statusOf(err) != http.StatusBadRequest {
			t.Errorf("%s: %v", name, err)
		}
	}
	no := false
	if _, err := act(env, "drive.upload_base64", in(map[string]any{"name": "z", "parentPath": "/没有的", "createParents": no})); statusOf(err) != http.StatusNotFound {
		t.Fatalf("createParents false: %v", err)
	}
	// A file in the way of a path.
	if _, err := act(env, "drive.upload_base64", in(map[string]any{"name": "z", "parentPath": "/c.jpg/下面"})); statusOf(err) != http.StatusBadRequest {
		t.Fatalf("path through a file: %v", err)
	}
}

// ---- one-time links ----

func TestUploadLinkWorksOnce(t *testing.T) {
	env, _ := setupMCP(t)
	link := mustAct(t, env, "drive.create_upload_link", map[string]any{"name": "歌.mp3", "parentPath": "/音乐/新歌"})
	up, _ := link["uploadUrl"].(string)
	if !strings.HasPrefix(up, "/api/v1/drive/upload-links/") || link["method"] != "PUT" || id(link, "maxSize") != 1<<30 {
		t.Fatalf("link: %+v", link)
	}
	// Only the hash is stored.
	token := strings.TrimPrefix(up, "/api/v1/drive/upload-links/")
	var n int
	env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_upload_links WHERE token_hash = ? OR token_hash LIKE ?", token, "%"+token+"%").Scan(&n)
	if n != 0 {
		t.Fatal("the token itself is stored")
	}

	data := bytes.Repeat([]byte("song "), 1000)
	resp, body := plain(t, env, http.MethodPut, up, bytes.NewReader(data), map[string]string{"Content-Type": "application/octet-stream"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %s", resp.StatusCode, body)
	}
	var item struct {
		Id   int64
		Name string
		Size int64
	}
	json.Unmarshal(body, &item)
	if item.Name != "歌.mp3" || item.Size != int64(len(data)) {
		t.Fatalf("item: %+v", item)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/音乐/新歌/歌.mp3"}); r["found"] != true {
		t.Fatalf("file not in the folder: %+v", r)
	}
	// Used up, wrong token, and a token that never existed look the same.
	for name, path := range map[string]string{"second use": up, "wrong": "/api/v1/drive/upload-links/nope", "empty": "/api/v1/drive/upload-links/x"} {
		if resp, _ := plain(t, env, http.MethodPut, path, strings.NewReader("x"), nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", name, resp.StatusCode)
		}
	}
}

func TestUploadLinkEndsAfterTenMinutesAndOnFailure(t *testing.T) {
	env, _ := setupMCP(t)
	pathOf := func(l map[string]any) string { return l["uploadUrl"].(string) }

	expired := mustAct(t, env, "drive.create_upload_link", map[string]any{"name": "a.bin"})
	if _, err := env.App.Deps.DB.Exec("UPDATE drive_upload_links SET expires_at = ?", time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if resp, _ := plain(t, env, http.MethodPut, pathOf(expired), strings.NewReader("x"), nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expired: %d", resp.StatusCode)
	}

	// Too big: refused with 413, and the link is spent all the same.
	if err := env.App.Deps.Settings.Set(context.Background(), "drive.mcp_max_upload", 10); err != nil {
		t.Fatal(err)
	}
	small := mustAct(t, env, "drive.create_upload_link", map[string]any{"name": "b.bin"})
	if id(small, "maxSize") != 10 {
		t.Fatalf("max size: %+v", small)
	}
	if resp, _ := plain(t, env, http.MethodPut, pathOf(small), strings.NewReader(strings.Repeat("x", 100)), nil); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %d", resp.StatusCode)
	}
	if resp, _ := plain(t, env, http.MethodPut, pathOf(small), strings.NewReader("ok"), nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("link of a failed upload: %d", resp.StatusCode)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/b.bin"}); r["found"] != false {
		t.Fatal("a refused upload left a file")
	}
	// A size known in advance that is over the limit is refused when the link is made.
	if _, err := act(env, "drive.create_upload_link", map[string]any{"name": "c.bin", "size": 11}); statusOf(err) != http.StatusRequestEntityTooLarge {
		t.Fatalf("size over the limit: %v", err)
	}
	// Empty body.
	empty := mustAct(t, env, "drive.create_upload_link", map[string]any{"name": "d.bin"})
	if resp, _ := plain(t, env, http.MethodPut, pathOf(empty), nil, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body: %d", resp.StatusCode)
	}
}

func TestUploadLinkNameConflicts(t *testing.T) {
	env, _ := setupMCP(t)
	mustAct(t, env, "drive.upload_base64", map[string]any{"name": "x.mp3", "dataBase64": b64("old")})
	if _, err := act(env, "drive.create_upload_link", map[string]any{"name": "x.mp3"}); statusOf(err) != http.StatusConflict {
		t.Fatalf("name taken: %v", err)
	}
	link := mustAct(t, env, "drive.create_upload_link", map[string]any{"name": "x.mp3", "onConflict": "rename"})
	resp, body := plain(t, env, http.MethodPut, link["uploadUrl"].(string), strings.NewReader("new"), nil)
	if resp.StatusCode != http.StatusCreated || !strings.Contains(string(body), `"x (1).mp3"`) {
		t.Fatalf("rename on upload: %d %s", resp.StatusCode, body)
	}
	if got := mustAct(t, env, "drive.read_text", map[string]any{"id": id(mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/x.mp3"}), "id")}); got["text"] != "old" {
		t.Fatalf("the original was changed: %+v", got)
	}
	// The name is taken between creating the link and uploading: a plain link then fails with 409.
	link = mustAct(t, env, "drive.create_upload_link", map[string]any{"name": "late.mp3"})
	mustAct(t, env, "drive.upload_base64", map[string]any{"name": "late.mp3", "dataBase64": b64("first")})
	if resp, _ := plain(t, env, http.MethodPut, link["uploadUrl"].(string), strings.NewReader("second"), nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("name taken meanwhile: %d", resp.StatusCode)
	}
}

func TestDownloadLinkWorksOnce(t *testing.T) {
	env, _ := setupMCP(t)
	file := id(mustAct(t, env, "drive.upload_base64", map[string]any{"name": "a.txt", "dataBase64": b64("content")}), "id")
	link := mustAct(t, env, "drive.create_download_link", map[string]any{"id": file})
	dl := link["downloadUrl"].(string)
	resp, body := plain(t, env, http.MethodGet, dl, nil, nil)
	if resp.StatusCode != 200 || string(body) != "content" || !strings.Contains(resp.Header.Get("Content-Disposition"), "a.txt") {
		t.Fatalf("download: %d %q %q", resp.StatusCode, body, resp.Header.Get("Content-Disposition"))
	}
	if resp, _ := plain(t, env, http.MethodGet, dl, nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second use: %d", resp.StatusCode)
	}
	// Not for folders; and a file that went to the trash after the link was made cannot be fetched.
	folder := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/"})
	_ = folder
	mustAct(t, env, "drive.create_folder", map[string]any{"name": "F"})
	f := id(mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/F"}), "id")
	if _, err := act(env, "drive.create_download_link", map[string]any{"id": f}); statusOf(err) != http.StatusBadRequest {
		t.Fatalf("folder: %v", err)
	}
	link = mustAct(t, env, "drive.create_download_link", map[string]any{"id": file})
	mustAct(t, env, "drive.delete", map[string]any{"id": file})
	if resp, _ := plain(t, env, http.MethodGet, link["downloadUrl"].(string), nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("trashed: %d", resp.StatusCode)
	}
	if _, err := act(env, "drive.create_download_link", map[string]any{"id": 99999}); statusOf(err) != http.StatusNotFound {
		t.Fatalf("unknown: %v", err)
	}
}

// ---- upload from a URL ----

func fileServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string) {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return srv, u.Port()
}

func TestUploadFromURL(t *testing.T) {
	env, m := setupMCP(t)
	srv, port := fileServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/song.mp3":
			w.Header().Set("Content-Disposition", `attachment; filename="真名.mp3"`)
			w.Write(bytes.Repeat([]byte("a"), 5000))
		case "/plain/x.txt":
			w.Write([]byte("body"))
		case "/redirect":
			http.Redirect(w, r, "/plain/x.txt", http.StatusFound)
		case "/missing":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	m.AllowLocalFetchForTest(port)

	// The name comes from Content-Disposition, then from the address; an explicit name wins.
	got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/song.mp3", "parentPath": "/下载"})
	if got["state"] != "done" || got["path"] != "/下载/真名.mp3" || id(got, "size") != 5000 {
		t.Fatalf("download: %+v", got)
	}
	if got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/plain/x.txt"}); got["name"] != "x.txt" {
		t.Fatalf("name from the address: %+v", got)
	}
	if got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/redirect", "name": "mine.txt"}); got["name"] != "mine.txt" || got["state"] != "done" {
		t.Fatalf("redirect and explicit name: %+v", got)
	}
	// Same name again: 409 style failure unless rename.
	failed := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/plain/x.txt"})
	if failed["state"] != "failed" || failed["errorCode"] != "name_conflict" {
		t.Fatalf("conflict: %+v", failed)
	}
	if got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/plain/x.txt", "onConflict": "rename"}); got["name"] != "x (1).txt" {
		t.Fatalf("rename: %+v", got)
	}
	// An error answer from the other side.
	if got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/missing"}); got["state"] != "failed" || got["errorCode"] != "fetch_failed" {
		t.Fatalf("404: %+v", got)
	}
	// The task can be looked up afterwards.
	st := mustAct(t, env, "drive.task_status", map[string]any{"taskId": failed["taskId"]})
	if st["state"] != "failed" {
		t.Fatalf("task status: %+v", st)
	}
	if _, err := act(env, "drive.task_status", map[string]any{"taskId": "nope"}); statusOf(err) != http.StatusNotFound {
		t.Fatalf("unknown task: %v", err)
	}
}

func TestUploadFromURLLimitsAndSlowServers(t *testing.T) {
	env, m := setupMCP(t)
	if err := env.App.Deps.Settings.Set(context.Background(), "drive.mcp_max_upload", 1000); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	srv, port := fileServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/declared": // Content-Length over the limit
			w.Header().Set("Content-Length", "5000")
			w.Write(make([]byte, 5000))
		case "/streamed": // no Content-Length, too much data
			w.(http.Flusher).Flush()
			for i := 0; i < 20; i++ {
				w.Write(make([]byte, 500))
				w.(http.Flusher).Flush()
			}
		case "/stall": // starts and then goes quiet
			w.Header().Set("Content-Length", "500")
			w.Write([]byte("start"))
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
		case "/slow": // fine, but later than the action waits
			select {
			case <-time.After(400 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			w.Write([]byte("late but fine"))
		}
	})
	t.Cleanup(func() { close(release) })
	m.AllowLocalFetchForTest(port)
	m.SetDownloadTimingForTest(300*time.Millisecond, 100*time.Millisecond)

	for _, path := range []string{"/declared", "/streamed"} {
		got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + path, "name": "big.bin"})
		for i := 0; got["state"] == "running" && i < 50; i++ {
			time.Sleep(100 * time.Millisecond)
			got = mustAct(t, env, "drive.task_status", map[string]any{"taskId": got["taskId"]})
		}
		if got["state"] != "failed" || got["errorCode"] != "too_large" {
			t.Fatalf("%s: %+v", path, got)
		}
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/big.bin"}); r["found"] != false {
		t.Fatal("an over-size download left a file")
	}
	got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/stall", "name": "stall.bin"})
	deadline := time.Now().Add(5 * time.Second)
	for got["state"] == "running" && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		got = mustAct(t, env, "drive.task_status", map[string]any{"taskId": got["taskId"]})
	}
	if got["state"] != "failed" || got["errorCode"] != "fetch_stalled" {
		t.Fatalf("stalled: %+v", got)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/stall.bin"}); r["found"] != false {
		t.Fatal("a stalled download left a file")
	}

	// A slow but healthy download: the action returns a task id and the task finishes.
	m.SetDownloadTimingForTest(5*time.Second, 50*time.Millisecond)
	got = mustAct(t, env, "drive.upload_from_url", map[string]any{"url": srv.URL + "/slow", "name": "slow.txt"})
	if got["state"] != "running" || got["taskId"] == nil {
		t.Fatalf("slow: %+v", got)
	}
	for got["state"] == "running" && time.Now().Before(deadline.Add(5*time.Second)) {
		time.Sleep(100 * time.Millisecond)
		got = mustAct(t, env, "drive.task_status", map[string]any{"taskId": got["taskId"]})
	}
	if got["state"] != "done" || got["name"] != "slow.txt" || id(got, "size") != 13 {
		t.Fatalf("slow result: %+v", got)
	}
}

func TestUploadFromURLRefusesInternalAddresses(t *testing.T) {
	env, m := setupMCP(t)
	_ = m
	srv, _ := fileServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("secret")) })
	for name, target := range map[string]string{
		"loopback address": srv.URL + "/x",
		"private address":  "http://10.1.2.3/x",
		"metadata address": "http://169.254.169.254/latest/meta-data",
		"cgnat address":    "http://100.64.1.1/x",
		"ipv6 loopback":    "http://[::1]/x",
		"ipv4 in ipv6":     "http://[::ffff:127.0.0.1]/x",
		"unspecified":      "http://0.0.0.0/x",
		"file scheme":      "file:///etc/passwd",
		"ftp scheme":       "ftp://example.com/x",
		"with credentials": "http://user:pass@example.com/x",
		"no host":          "http:///x",
		"not an address":   "this is not a url",
	} {
		if _, err := act(env, "drive.upload_from_url", map[string]any{"url": target}); statusOf(err) != http.StatusBadRequest {
			t.Errorf("%s (%s): %v", name, target, err)
		}
	}
	// A name that resolves to the loopback address is refused when connecting (the
	// port is not 80 or 443 either).
	u, _ := url.Parse(srv.URL)
	got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": "http://localhost:" + u.Port() + "/x"})
	if got["state"] != "failed" || got["errorCode"] != "fetch_failed" {
		t.Fatalf("localhost: %+v", got)
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/x"}); r["found"] != false {
		t.Fatal("an internal address was fetched")
	}
}

func TestUploadFromURLChecksEveryRedirect(t *testing.T) {
	env, m := setupMCP(t)
	inner, innerPort := fileServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("internal secret")) })
	outer, outerPort := fileServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-internal":
			http.Redirect(w, r, inner.URL+"/secret", http.StatusFound)
		case "/to-file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		}
	})
	_ = innerPort
	m.AllowLocalFetchForTest(outerPort) // the second server is on a port that is not allowed
	for _, path := range []string{"/to-internal", "/to-file", "/loop"} {
		got := mustAct(t, env, "drive.upload_from_url", map[string]any{"url": outer.URL + path, "name": "r.txt"})
		if got["state"] != "failed" {
			t.Errorf("%s: %+v", path, got)
		}
	}
	if r := mustAct(t, env, "drive.resolve_path", map[string]any{"path": "/r.txt"}); r["found"] != false {
		t.Fatal("a redirect to a forbidden target was followed")
	}
}

func TestPublicAddressRules(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true, "93.184.216.34": true,
		"127.0.0.1": false, "10.0.0.1": false, "172.16.5.5": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "100.127.255.255": false, "100.128.0.1": true, "0.0.0.0": false, "224.0.0.1": false,
		"198.18.0.1": false, "240.0.0.1": false, "::1": false, "fc00::1": false, "fe80::1": false, "::": false,
		"::ffff:127.0.0.1": false, "::ffff:10.0.0.1": false, "::ffff:8.8.8.8": true, "255.255.255.255": false,
	} {
		if got := drive.PublicAddrForTest(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s: %v, want %v", addr, got, want)
		}
	}
}

// ---- sharing ----

func TestShareActions(t *testing.T) {
	env, _ := setupMCP(t)
	file := id(mustAct(t, env, "drive.upload_base64", map[string]any{"name": "a.txt", "dataBase64": b64("x")}), "id")
	// Creating a public link needs the user's own password check, so there is no such action.
	if _, err := act(env, "drive.share_create", map[string]any{"id": file}); err == nil {
		t.Fatal("drive.share_create should not exist")
	}
	env.Elevate()
	var share map[string]any
	env.MustDo(http.MethodPost, "/drive/shares", map[string]any{"itemId": file, "code": "abcd", "expiresIn": "1d"}, &share)
	token, _ := share["token"].(string)
	list := mustAct(t, env, "drive.share_list", map[string]any{"itemId": file})
	items, _ := list["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["path"] != "/s/"+token {
		t.Fatalf("list: %+v", list)
	}
	first := items[0].(map[string]any)
	if _, ok := first["code"]; ok {
		t.Fatalf("access code leaked into the tool result: %+v", first)
	}
	if _, ok := first["url"]; ok {
		t.Fatalf("url should not be a made-up address: %v", first["url"])
	}
	if info := mustAct(t, env, "drive.info", map[string]any{"id": file}); info["shares"] != float64(1) {
		t.Fatalf("info: %+v", info)
	}
	mustAct(t, env, "drive.share_revoke", map[string]any{"shareId": id(first, "id")})
	if list := mustAct(t, env, "drive.share_list", map[string]any{"itemId": file}); len(list["items"].([]any)) != 0 {
		t.Fatalf("after revoke: %+v", list)
	}
	if _, err := act(env, "drive.share_revoke", map[string]any{"shareId": 9999}); statusOf(err) != http.StatusNotFound {
		t.Fatalf("revoke unknown: %v", err)
	}
}

// ---- the MCP side: who sees what ----

func TestMCPToolsAreFilteredByAccess(t *testing.T) {
	env, _ := setupMCP(t)
	for name, mcpOnly := range map[string]bool{"drive.create_upload_link": true, "drive.create_download_link": true, "drive.upload_from_url": false} {
		seen := false
		for _, a := range env.App.Deps.Actions.List(context.Background()) {
			if a.Name == name {
				seen = true
			}
		}
		if seen == mcpOnly {
			t.Errorf("%s: panel assistant sees it = %v, MCP-only = %v", name, seen, mcpOnly)
		}
		inMCP := false
		for _, a := range env.App.Deps.Actions.ListForMCP(context.Background()) {
			if a.Name == name {
				inMCP = true
			}
		}
		if !inMCP {
			t.Errorf("%s is not offered to MCP", name)
		}
	}

	env.Elevate()
	tokens := map[string]string{}
	for _, access := range []string{"read", "write", "write_delete"} {
		var out struct{ Secret string }
		env.MustDo(http.MethodPost, "/api-tokens", map[string]any{"name": "t-" + access, "access": access}, &out)
		tokens[access] = out.Secret
	}
	names := func(secret string) map[string]bool {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
		req, _ := http.NewRequest(http.MethodPost, env.URL("/mcp"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Result struct{ Tools []struct{ Name string } }
		}
		json.NewDecoder(resp.Body).Decode(&out)
		m := map[string]bool{}
		for _, tool := range out.Result.Tools {
			m[tool.Name] = true
		}
		return m
	}
	read, write, del := names(tokens["read"]), names(tokens["write"]), names(tokens["write_delete"])
	for _, n := range []string{"drive_resolve_path", "drive_info", "drive_share_list", "drive_task_status", "drive_create_download_link"} {
		if !read[n] || !write[n] || !del[n] {
			t.Errorf("%s: read %v write %v delete %v", n, read[n], write[n], del[n])
		}
	}
	for _, n := range []string{"drive_upload_from_url", "drive_upload_base64", "drive_create_upload_link", "drive_copy"} {
		if read[n] || !write[n] || !del[n] {
			t.Errorf("%s: read %v write %v delete %v", n, read[n], write[n], del[n])
		}
	}
	if read["drive_share_create"] || write["drive_share_create"] || del["drive_share_create"] {
		t.Error("drive_share_create must not be offered to any token")
	}
	if read["drive_share_revoke"] || write["drive_share_revoke"] || !del["drive_share_revoke"] {
		t.Errorf("share_revoke: read %v write %v delete %v", read["drive_share_revoke"], write["drive_share_revoke"], del["drive_share_revoke"])
	}

	// End to end: a write token asks for an upload link and uses it.
	call := func(secret, tool string, args any) map[string]any {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
		req, _ := http.NewRequest(http.MethodPost, env.URL("/mcp"), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	res := call(tokens["write"], "drive_create_upload_link", map[string]any{"name": "mcp.bin", "parentPath": "/mcp"})
	result, _ := res["result"].(map[string]any)
	structured, _ := result["structuredContent"].(map[string]any)
	up, _ := structured["uploadUrl"].(string)
	if up == "" {
		t.Fatalf("tools/call: %+v", res)
	}
	if resp, _ := plain(t, env, http.MethodPut, up, strings.NewReader("from mcp"), nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload by the link: %d", resp.StatusCode)
	}
	// A read token is refused the write tool.
	denied := call(tokens["read"], "drive_create_upload_link", map[string]any{"name": "x"})
	if denied["error"] == nil {
		if r, _ := denied["result"].(map[string]any); r == nil || r["isError"] != true {
			t.Fatalf("read token was allowed to create an upload link: %+v", denied)
		}
	}
}
