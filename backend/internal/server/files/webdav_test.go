package files_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

// davServer is an in-memory WebDAV server below /dav/ that wants a password
// and remembers which methods it got.
type davServer struct {
	*httptest.Server
	fs webdav.FileSystem

	mu      sync.Mutex
	methods []string
}

func newDAV(t *testing.T) *davServer {
	t.Helper()
	d := &davServer{fs: webdav.NewMemFS()}
	h := &webdav.Handler{Prefix: "/dav", FileSystem: d.fs, LockSystem: webdav.NewMemLS()}
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "me" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		d.mu.Lock()
		d.methods = append(d.methods, r.Method+" "+r.URL.Path)
		d.mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(d.Close)
	return d
}

func (d *davServer) names(t *testing.T, dir string) []string {
	t.Helper()
	f, err := d.fs.OpenFile(context.Background(), dir, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	list, err := f.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, fi := range list {
		out = append(out, fi.Name())
	}
	return out
}

func newWebDAV(t *testing.T, d *davServer, folder string) *files.WebDAV {
	t.Helper()
	s, err := files.NewWebDAV(files.WebDAVConfig{URL: d.URL + "/dav/", Username: "me", Password: "pw", Folder: folder})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWebDAV(t *testing.T) {
	d := newDAV(t)
	runStoreTests(t, newWebDAV(t, d, "x-console-backups"))
	// Everything lives in the folder and no .part file is left.
	if got := d.names(t, "/"); len(got) != 1 || got[0] != "x-console-backups" {
		t.Fatalf("root: %v", got)
	}
	// Failed uploads are deleted again a moment later, in case the server
	// wrote the half file after the first delete.
	var left []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		left = nil
		for _, name := range d.names(t, "/x-console-backups/drive") {
			if strings.HasSuffix(name, ".part") {
				left = append(left, name)
			}
		}
		if len(left) == 0 {
			break
		}
	}
	if len(left) > 0 {
		t.Errorf("left %v", left)
	}
}

func TestWebDAVWithoutFolder(t *testing.T) {
	runStoreTests(t, newWebDAV(t, newDAV(t), ""))
}

func TestWebDAVUploadsThroughPart(t *testing.T) {
	d := newDAV(t)
	s := newWebDAV(t, d, "备份 目录")
	put(t, s, "a.tar.gz", "data")
	var put, move bool
	for _, m := range d.methods {
		put = put || m == "PUT /dav/备份 目录/a.tar.gz.part"
		move = move || m == "MOVE /dav/备份 目录/a.tar.gz.part"
	}
	if !put || !move {
		t.Fatalf("methods: %v", d.methods)
	}
	// A .part file left by a crash is not listed.
	f, err := d.fs.OpenFile(context.Background(), "/备份 目录/b.tar.gz.part", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(f, "half")
	f.Close()
	if got := keys(t, s, ""); len(got) != 1 || got[0] != "a.tar.gz" {
		t.Fatalf("list: %v", got)
	}
}

func TestWebDAVCheck(t *testing.T) {
	d := newDAV(t)
	if err := newWebDAV(t, d, "a/b").Check(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
	if got := d.names(t, "/a/b"); len(got) != 0 {
		t.Fatalf("check left files: %v", got)
	}
	bad, err := files.NewWebDAV(files.WebDAVConfig{URL: d.URL + "/dav/", Username: "me", Password: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bad.Check(context.Background()); err == nil || err.Error() != "用户名或密码不对" {
		t.Fatalf("wrong password: %v", err)
	}
	for _, c := range []files.WebDAVConfig{{URL: "ftp://x"}, {URL: "dav.example.com"}, {URL: "https://x/?a=1"}, {URL: "https://x/", Folder: "../x"}} {
		if _, err := files.NewWebDAV(c); err == nil {
			t.Errorf("config %+v should be rejected", c)
		}
	}
}

func TestWebDAVReadDir(t *testing.T) {
	d := newDAV(t)
	s := newWebDAV(t, d, "")
	put(t, s, "照片/2026/a.jpg", "jpg")
	put(t, s, "照片/readme.txt", "hi")
	put(t, s, "top.txt", "x")
	top, err := s.ReadDir(context.Background(), "")
	if err != nil || len(top) != 2 || top[0].Name != "照片" || !top[0].Dir || top[1].Ref != "top.txt" {
		t.Fatalf("top: %+v %v", top, err)
	}
	list, err := s.ReadDir(context.Background(), "照片")
	if err != nil || len(list) != 2 || list[0].Ref != "照片/2026" || list[1].Name != "readme.txt" || list[1].Size != 2 {
		t.Fatalf("photos: %+v %v", list, err)
	}
	if _, err := s.ReadDir(context.Background(), "nope"); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
