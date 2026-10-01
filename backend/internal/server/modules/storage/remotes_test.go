package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/webdav"

	"github.com/j0x3n/x-console/backend/internal/server/files/fakegdrive"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const davPassword = "dav-secret-pw"

// davServer is an in-memory WebDAV server below /dav/ with one file.
func davServer(t *testing.T, file string) *httptest.Server {
	t.Helper()
	fs := webdav.NewMemFS()
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/docs", 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := fs.OpenFile(ctx, "/docs/"+file, os.O_CREATE|os.O_WRONLY, 0o644)
	_, _ = f.Write([]byte("content of " + file))
	f.Close()
	h := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "me" || p != davPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func davInput(srv *httptest.Server, name, password string) map[string]any {
	return map[string]any{"kind": "webdav", "name": name, "webdav": map[string]any{"url": srv.URL + "/dav/", "username": "me", "password": password}}
}

func remotes(t *testing.T, env *testutil.Env, query string) []api.StorageRemote {
	t.Helper()
	var out struct{ Items []api.StorageRemote }
	env.MustDo(http.MethodGet, "/storage/remotes"+query, nil, &out)
	return out.Items
}

func get(t *testing.T, env *testutil.Env, path string) (int, string) {
	t.Helper()
	resp, err := env.Client.Get(env.URL(path))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestWebDAVAccounts(t *testing.T) {
	env := testutil.New(t, storage.New)
	one, two := davServer(t, "a.txt"), davServer(t, "b.txt")
	if code, _ := env.Do(http.MethodPost, "/storage/remotes", davInput(one, "", davPassword), nil); code != http.StatusForbidden {
		t.Fatalf("without elevation: %d", code)
	}
	env.Elevate()
	if code, body := env.Do(http.MethodPost, "/storage/remotes", davInput(one, "", "wrong"), nil); code != http.StatusBadRequest || !strings.Contains(string(body), "用户名或密码不对") {
		t.Fatalf("wrong password: %d %s", code, body)
	}
	var a, b api.StorageRemote
	env.MustDo(http.MethodPost, "/storage/remotes", davInput(one, "", davPassword), &a)
	if a.Name != "127.0.0.1" || a.Webdav == nil || !a.Webdav.PasswordSet || !a.Ready || !a.ShowInDrive || a.UsedByBackup {
		t.Fatalf("first: %+v", a)
	}
	env.MustDo(http.MethodPost, "/storage/remotes", davInput(two, "Nextcloud", davPassword), &b)
	if _, raw := env.Do(http.MethodGet, "/storage/remotes", nil, nil); strings.Contains(string(raw), davPassword) {
		t.Fatalf("password shown: %s", raw)
	}
	if got := remotes(t, env, "?drive=true"); len(got) != 2 {
		t.Fatalf("drive tabs: %+v", got)
	}

	// 两个账号各看各的
	var list api.StorageRemoteListing
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/items?ref=docs", a.Id), nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "a.txt" || len(list.Trail) != 1 {
		t.Fatalf("first listing: %+v", list)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/items?ref=docs", b.Id), nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "b.txt" {
		t.Fatalf("second listing: %+v", list)
	}
	if code, body := get(t, env, fmt.Sprintf("/storage/remotes/%d/download?ref=%s", b.Id, url.QueryEscape("docs/b.txt"))); code != 200 || body != "content of b.txt" {
		t.Fatalf("download: %d %q", code, body)
	}

	// 改：关掉云盘页显示；密码错了不保存
	off := false
	env.MustDo(http.MethodPatch, fmt.Sprintf("/storage/remotes/%d", a.Id), api.StorageRemoteInput{ShowInDrive: &off}, &a)
	if got := remotes(t, env, "?drive=true"); len(got) != 1 || got[0].Id != b.Id {
		t.Fatalf("after hiding one tab: %+v", got)
	}
	if code, _ := env.Do(http.MethodPatch, fmt.Sprintf("/storage/remotes/%d", a.Id), map[string]any{"webdav": map[string]any{"password": "nope"}}, nil); code != http.StatusBadRequest {
		t.Fatalf("wrong new password: %d", code)
	}
	var test api.TestResult
	env.MustDo(http.MethodPost, fmt.Sprintf("/storage/remotes/test?id=%d", a.Id), map[string]any{}, &test)
	if !test.Ok {
		t.Fatalf("saved password kept: %+v", test)
	}
	env.MustDo(http.MethodPost, "/storage/remotes/test", davInput(one, "", "nope"), &test)
	if test.Ok {
		t.Fatalf("test with a wrong password: %+v", test)
	}

	// 锁定并隐藏 WebDAV：标签和浏览都没了
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"drive-webdav"}}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if got := remotes(t, env, "?drive=true"); len(got) != 0 {
		t.Fatalf("locked: %+v", got)
	}
	if code, _ := get(t, env, fmt.Sprintf("/storage/remotes/%d/items", b.Id)); code != http.StatusNotFound {
		t.Fatalf("hidden browse: %d", code)
	}

	env.MustDo(http.MethodDelete, fmt.Sprintf("/storage/remotes/%d", b.Id), nil, nil)
	if got := remotes(t, env, ""); len(got) != 1 {
		t.Fatalf("after delete: %+v", got)
	}
}

// noRedirect shows redirects instead of following them.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func callback(t *testing.T, env *testutil.Env, path string, query url.Values) url.Values {
	t.Helper()
	resp, err := noRedirect.Get(env.URL(path + "?" + query.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	u, err := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || err != nil || u.Path != "/settings/storage" {
		t.Fatalf("callback: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	return u.Query()
}

func TestGoogleDriveAccount(t *testing.T) {
	env := testutil.New(t, storage.New)
	m, _ := module.Lookup[*storage.Module](env.App.Deps.Registry, storage.ServiceKey)
	fake := fakegdrive.New(t)
	m.UseGoogle(fake.Endpoints())
	env.Elevate()

	if code, _ := env.Do(http.MethodPost, "/storage/remotes", map[string]any{"kind": "gdrive", "gdrive": map[string]any{"clientId": fake.ClientID}}, nil); code != http.StatusBadRequest {
		t.Fatalf("without a secret: %d", code)
	}
	var g api.StorageRemote
	env.MustDo(http.MethodPost, "/storage/remotes", map[string]any{"kind": "gdrive", "gdrive": map[string]any{"clientId": fake.ClientID, "clientSecret": fake.Secret}}, &g)
	if g.Name != "Google Drive" || g.Ready || g.Gdrive == nil || g.Gdrive.Authorized || !g.Gdrive.SecretSet ||
		!strings.HasSuffix(g.Gdrive.RedirectUri, "/api/v1/storage/remotes/gdrive/callback") {
		t.Fatalf("created: %+v %+v", g, g.Gdrive)
	}
	if got := remotes(t, env, "?drive=true"); len(got) != 0 {
		t.Fatalf("not authorized yet, still a tab: %+v", got)
	}

	var start struct{ Url, RedirectUri string }
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/gdrive/auth", g.Id), nil, &start)
	q := fakegdrive.AuthURL(t, start.Url)
	if q.Get("redirect_uri") != start.RedirectUri || start.RedirectUri != g.Gdrive.RedirectUri {
		t.Fatalf("auth: %+v", start)
	}
	path := "/storage/remotes/gdrive/callback"
	if got := callback(t, env, path, url.Values{"state": {"forged"}, "code": {fake.Code}}); got.Get("gdrive") != "error" {
		t.Fatalf("forged: %v", got)
	}
	if got := callback(t, env, path, url.Values{"state": {q.Get("state")}, "code": {fake.Code}}); got.Get("gdrive") != "ok" {
		t.Fatalf("callback: %v", got)
	}
	list := remotes(t, env, "?drive=true")
	if len(list) != 1 || !list[0].Ready || list[0].Gdrive.Account == nil || *list[0].Gdrive.Account != fake.Email || list[0].Gdrive.Limited {
		t.Fatalf("authorized: %+v", list)
	}

	docs := fake.Add("root", "文档", fakegdrive.FolderMime, nil)
	fake.Add(docs, "说明.txt", "", []byte("hello"))
	var listing api.StorageRemoteListing
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/items?ref=%s", g.Id, docs), nil, &listing)
	if len(listing.Items) != 1 || listing.Items[0].Name != "说明.txt" || len(listing.Trail) != 1 {
		t.Fatalf("listing: %+v", listing)
	}
	if code, body := get(t, env, fmt.Sprintf("/storage/remotes/%d/download?ref=%s", g.Id, listing.Items[0].Ref)); code != 200 || body != "hello" {
		t.Fatalf("download: %d %q", code, body)
	}
	var test api.TestResult
	env.MustDo(http.MethodPost, fmt.Sprintf("/storage/remotes/test?id=%d", g.Id), map[string]any{}, &test)
	if !test.Ok || !strings.Contains(test.Message, fake.Email) {
		t.Fatalf("test: %+v", test)
	}

	env.MustDo(http.MethodDelete, fmt.Sprintf("/storage/remotes/%d/gdrive/auth", g.Id), nil, nil)
	if got := fake.Revoked(); len(got) != 1 {
		t.Fatalf("revoked: %v", got)
	}
	list = remotes(t, env, "")
	if list[0].Ready || list[0].Gdrive.Authorized {
		t.Fatalf("after revoke: %+v", list[0].Gdrive)
	}

	// 换了客户端 ID，原来的授权作废
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/gdrive/auth", g.Id), nil, &start)
	callback(t, env, path, url.Values{"state": {fakegdrive.AuthURL(t, start.Url).Get("state")}, "code": {fake.Code}})
	env.MustDo(http.MethodPatch, fmt.Sprintf("/storage/remotes/%d", g.Id), map[string]any{"gdrive": map[string]any{"clientId": "another"}}, &g)
	if g.Gdrive.Authorized || g.Gdrive.ClientId != "another" || g.Gdrive.Account != nil {
		t.Fatalf("after changing the client: %+v", g.Gdrive)
	}
}
