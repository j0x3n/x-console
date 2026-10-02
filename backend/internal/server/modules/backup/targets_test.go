package backup_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"github.com/j0x3n/x-console/backend/internal/server/files/fakegdrive"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// davServer is an in-memory WebDAV server below /dav/ that wants a password.
func davServer(t *testing.T) (*httptest.Server, webdav.FileSystem) {
	t.Helper()
	fs := webdav.NewMemFS()
	h := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "me" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, fs
}

func davNames(t *testing.T, fs webdav.FileSystem, dir string) []string {
	t.Helper()
	f, err := fs.OpenFile(context.Background(), dir, os.O_RDONLY, 0)
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
	slices.Sort(out)
	return out
}

// runThree runs three automatic backups a minute apart and checks each one.
func runThree(t *testing.T, env *testutil.Env, m *backup.Module) {
	t.Helper()
	env.Elevate()
	clock := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	for i := range 3 {
		at := clock.Add(time.Duration(i) * time.Minute)
		backup.SetNow(m, func() time.Time { return at })
		env.MustDo(http.MethodPost, "/backups/run", nil, nil)
		if job := waitJob(t, env); job.State != api.Done {
			t.Fatalf("run %d: %+v %v", i, job, deref(job.Error))
		}
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// checkRemote checks the list, a download and a restore of the remote copies.
func checkRemote(t *testing.T, env *testutil.Env, m *backup.Module, loc api.BackupLocation) {
	t.Helper()
	var list struct{ Items []api.Backup }
	env.MustDo(http.MethodGet, "/backups", nil, &list)
	if len(list.Items) != 2 {
		t.Fatalf("kept: %+v", list.Items)
	}
	for _, b := range list.Items {
		if b.Location != loc || b.Kind != api.BackupKindAuto {
			t.Fatalf("item: %+v", b)
		}
	}
	status, raw := rawGet(t, env, "/backups/"+list.Items[0].Id+"/download")
	if status != http.StatusOK {
		t.Fatalf("download: %d", status)
	}
	if p := unpack(t, raw); p.files["notes/attachments/1"] != "picture" {
		t.Fatalf("package: %v", p.files)
	}
	var stops atomic.Int32
	backup.SetExit(m, func() { stops.Add(1) })
	env.MustDo(http.MethodPost, "/backups/"+list.Items[1].Id+"/restore", map[string]string{"confirm": "恢复"}, nil)
	deadline := time.Now().Add(15 * time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stops.Load() != 1 || !backup.Pending(env.App.Deps.Config.RestoreDir()) {
		t.Fatalf("restore from %s was not staged", loc)
	}
}

// storageModule is the storage module, which owns the drive accounts.
func storageModule(t *testing.T, env *testutil.Env) *storage.Module {
	t.Helper()
	sm, ok := module.Lookup[*storage.Module](env.App.Deps.Registry, storage.ServiceKey)
	if !ok {
		t.Fatal("storage module not registered")
	}
	return sm
}

// addWebDAV adds a WebDAV account in the storage settings.
func addWebDAV(t *testing.T, env *testutil.Env, srv *httptest.Server) int64 {
	t.Helper()
	var out struct{ Id int64 }
	env.MustDo(http.MethodPost, "/storage/remotes", map[string]any{"kind": "webdav",
		"webdav": map[string]any{"url": srv.URL + "/dav/", "username": "me", "password": "pw"}}, &out)
	return out.Id
}

// addGDrive adds a Google Drive account and goes through the authorization.
func addGDrive(t *testing.T, env *testutil.Env, fake *fakegdrive.Server) int64 {
	t.Helper()
	var out struct{ Id int64 }
	env.MustDo(http.MethodPost, "/storage/remotes", map[string]any{"kind": "gdrive",
		"gdrive": map[string]any{"clientId": fake.ClientID, "clientSecret": fake.Secret}}, &out)
	var start struct{ Url string }
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/gdrive/auth", out.Id), nil, &start)
	state := fakegdrive.AuthURL(t, start.Url).Get("state")
	if got := callback(t, env, "/storage/remotes/gdrive/callback", url.Values{"state": {state}, "code": {fake.Code}}); got.Get("gdrive") != "ok" {
		t.Fatalf("callback: %v", got)
	}
	return out.Id
}

// noRedirect is a client that shows redirects instead of following them.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func callback(t *testing.T, env *testutil.Env, path string, query url.Values) url.Values {
	t.Helper()
	resp, err := noRedirect.Get(env.URL(path + "?" + query.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback: %d", resp.StatusCode)
	}
	u, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || u.Path != "/settings/storage" {
		t.Fatalf("callback went to %q", resp.Header.Get("Location"))
	}
	return u.Query()
}

func TestBackupToWebDAV(t *testing.T) {
	env, m := setup(t)
	srv, fs := davServer(t)
	putFile(t, env, "notes/attachments/1", "picture")
	env.Elevate()

	if code, _ := env.Do(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "remote"}, nil); code != http.StatusBadRequest {
		t.Fatalf("remote without an account: %d", code)
	}
	id := addWebDAV(t, env, srv)
	var test api.BackupTargetTest
	env.MustDo(http.MethodPost, "/backups/target/test", map[string]any{"target": "remote", "remoteId": id}, &test)
	if !test.Ok {
		t.Fatalf("test: %+v", test)
	}
	var s api.BackupSettings
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"mode": "full", "enabled": true, "target": "remote", "remoteId": id, "keep": 2}, &s)
	if s.Target != "remote" || s.RemoteId == nil || *s.RemoteId != id || s.Webdav == nil || s.Webdav.Folder != "x-console-backups" {
		t.Fatalf("settings: %+v", s)
	}
	// 自动备份在用的账号不能删
	if code, body := env.Do(http.MethodDelete, fmt.Sprintf("/storage/remotes/%d", id), nil, nil); code != http.StatusConflict || !strings.Contains(string(body), "设置 → 备份") {
		t.Fatalf("delete while used: %d %s", code, body)
	}

	runThree(t, env, m)
	// The packages sit right in the folder, two of them with their sidecars.
	names := davNames(t, fs, "/x-console-backups")
	if len(names) != 4 || !strings.HasSuffix(names[0], ".tar.gz") || !strings.HasSuffix(names[1], ".tar.gz.json") {
		t.Fatalf("folder: %v", names)
	}
	checkRemote(t, env, m, api.BackupLocationWebdav)
}

func TestBackupToGoogleDrive(t *testing.T) {
	env, m := setup(t)
	fake := fakegdrive.New(t)
	storageModule(t, env).UseGoogle(fake.Endpoints())
	putFile(t, env, "notes/attachments/1", "picture")
	env.Elevate()

	// 没授权时打不开
	var acc struct{ Id int64 }
	env.MustDo(http.MethodPost, "/storage/remotes", map[string]any{"kind": "gdrive",
		"gdrive": map[string]any{"clientId": fake.ClientID, "clientSecret": fake.Secret}}, &acc)
	if code, _ := env.Do(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "remote", "remoteId": acc.Id}, nil); code != http.StatusBadRequest {
		t.Fatalf("on without authorization: %d", code)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/storage/remotes/%d", acc.Id), nil, nil)

	id := addGDrive(t, env, fake)
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"mode": "full", "enabled": true, "target": "remote", "remoteId": id, "keep": 2}, nil)
	runThree(t, env, m)
	if names := fake.Names("X Console 备份"); len(names) != 4 {
		t.Fatalf("drive folder: %v", names)
	}
	checkRemote(t, env, m, api.BackupLocationGdrive)
}

func TestGoogleDriveExpiredToken(t *testing.T) {
	env, _ := setup(t)
	fake := fakegdrive.New(t)
	storageModule(t, env).UseGoogle(fake.Endpoints())
	env.Elevate()
	id := addGDrive(t, env, fake)
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"mode": "full", "enabled": true, "target": "remote", "remoteId": id}, nil)

	// The token expires: the run fails and a notification says so.
	fake.Expire()
	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	if job := waitJob(t, env); job.State != api.Failed || !strings.Contains(deref(job.Error), "授权过期") {
		t.Fatalf("expired run: %+v %v", job, deref(job.Error))
	}
	var notes struct {
		Items []struct{ Kind, Title string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) == 0 || notes.Items[0].Kind != "backup.failed" || notes.Items[0].Title != "Google Drive 授权过期，请重新授权" {
		t.Fatalf("notifications: %+v", notes.Items)
	}
}

// TestMoveB63Accounts: settings saved by B63 become storage accounts, the
// backups keep going, and the old Google callback address still works.
func TestMoveB63Accounts(t *testing.T) {
	env, m := setup(t)
	srv, _ := davServer(t)
	fake := fakegdrive.New(t)
	storageModule(t, env).UseGoogle(fake.Endpoints())
	putFile(t, env, "notes/attachments/1", "picture")
	ctx := context.Background()
	st := env.App.Deps.Settings
	old := map[string]any{
		"enabled": true, "frequency": "daily", "time": "03:00", "keep": 2, "target": "gdrive",
		"webdav": map[string]any{"url": srv.URL + "/dav/", "username": "me", "folder": "x-console-backups"},
		"gdrive": map[string]any{"clientId": fake.ClientID, "folderName": "X Console 备份", "account": fake.Email, "browse": true},
	}
	for key, v := range map[string]any{"backup.settings": old, "storage.migrated_b69": false} {
		if err := st.Set(ctx, key, v); err != nil {
			t.Fatal(err)
		}
	}
	for key, v := range map[string]string{"backup.webdav_password": "pw", "backup.gdrive_client_secret": fake.Secret, "backup.gdrive_refresh_token": fake.RefreshToken} {
		if err := st.SetSecret(ctx, key, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := backup.MigrateRemotes(m, ctx); err != nil {
		t.Fatal(err)
	}
	if err := backup.MigrateRemotes(m, ctx); err != nil { // 第二次什么都不做
		t.Fatal(err)
	}

	var list struct {
		Items []struct {
			Id           int64
			Kind, Name   string
			Ready        bool
			UsedByBackup bool
			Gdrive       *struct {
				Authorized  bool
				Account     *string
				RedirectUri string
			}
		}
	}
	env.MustDo(http.MethodGet, "/storage/remotes", nil, &list)
	if len(list.Items) != 2 || list.Items[0].Kind != "webdav" || list.Items[0].Name != "127.0.0.1" || !list.Items[0].Ready {
		t.Fatalf("accounts: %+v", list.Items)
	}
	g := list.Items[1]
	if g.Kind != "gdrive" || !g.Ready || !g.UsedByBackup || g.Gdrive == nil || !g.Gdrive.Authorized ||
		g.Gdrive.Account == nil || *g.Gdrive.Account != fake.Email || !strings.HasSuffix(g.Gdrive.RedirectUri, "/api/v1/backups/gdrive/callback") {
		t.Fatalf("google account: %+v %+v", g, g.Gdrive)
	}
	var s api.BackupSettings
	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if s.Target != "remote" || s.RemoteId == nil || *s.RemoteId != g.Id || !s.Enabled {
		t.Fatalf("backup settings: %+v", s)
	}
	runThree(t, env, m)
	if names := fake.Names("X Console 备份"); len(names) != 4 {
		t.Fatalf("drive folder: %v", names)
	}

	// 迁过来的账号重新授权，Google 跳回旧地址
	env.Elevate()
	var start struct{ Url, RedirectUri string }
	env.MustDo(http.MethodGet, fmt.Sprintf("/storage/remotes/%d/gdrive/auth", g.Id), nil, &start)
	if !strings.HasSuffix(start.RedirectUri, "/api/v1/backups/gdrive/callback") {
		t.Fatalf("redirect: %s", start.RedirectUri)
	}
	state := fakegdrive.AuthURL(t, start.Url).Get("state")
	if got := callback(t, env, "/backups/gdrive/callback", url.Values{"state": {state}, "code": {fake.Code}}); got.Get("gdrive") != "ok" {
		t.Fatalf("old callback: %v", got)
	}
}

func TestOldRemoteDriveAddresses(t *testing.T) {
	env, _ := setup(t)
	srv, fs := davServer(t)
	fake := fakegdrive.New(t)
	storageModule(t, env).UseGoogle(fake.Endpoints())
	env.Elevate()

	var drives struct{ Items []api.RemoteDrive }
	env.MustDo(http.MethodGet, "/remote-drives", nil, &drives)
	if len(drives.Items) != 0 {
		t.Fatalf("nothing bound: %+v", drives.Items)
	}
	if code, _ := env.Do(http.MethodGet, "/remote-drives/webdav/items", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unbound drive: %d", code)
	}
	if err := fs.Mkdir(context.Background(), "/照片", 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := fs.OpenFile(context.Background(), "/照片/a.jpg", os.O_CREATE|os.O_WRONLY, 0o644)
	f.Write([]byte("jpeg bytes"))
	f.Close()
	addWebDAV(t, env, srv)
	addWebDAV(t, env, srv) // 第二个不出现在旧接口里
	addGDrive(t, env, fake)

	env.MustDo(http.MethodGet, "/remote-drives", nil, &drives)
	if len(drives.Items) != 2 || drives.Items[0].Id != api.RemoteDriveIdWebdav || drives.Items[1].Name != "Google Drive" {
		t.Fatalf("drives: %+v", drives.Items)
	}
	var list api.RemoteDriveListing
	env.MustDo(http.MethodGet, "/remote-drives/webdav/items?ref="+url.QueryEscape("照片"), nil, &list)
	if len(list.Items) != 1 || list.Items[0].Ref != "照片/a.jpg" {
		t.Fatalf("webdav listing: %+v", list)
	}
	status, raw := rawGet(t, env, "/remote-drives/webdav/download?ref="+url.QueryEscape("照片/a.jpg"))
	if status != http.StatusOK || string(raw) != "jpeg bytes" {
		t.Fatalf("webdav download: %d %q", status, raw)
	}
	// 旧的授权地址转给备份在用的或第一个 Google 账号
	var start struct{ Url, RedirectUri string }
	env.MustDo(http.MethodGet, "/backups/gdrive/auth", nil, &start)
	if !strings.HasSuffix(start.RedirectUri, "/api/v1/storage/remotes/gdrive/callback") {
		t.Fatalf("old auth: %+v", start)
	}
}
