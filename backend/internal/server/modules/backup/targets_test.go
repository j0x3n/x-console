package backup_test

import (
	"context"
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
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
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

func TestBackupToWebDAV(t *testing.T) {
	env, m := setup(t)
	srv, fs := davServer(t)
	putFile(t, env, "notes/attachments/1", "picture")
	env.Elevate()

	dav := map[string]any{"url": srv.URL + "/dav/", "username": "me", "password": "wrong"}
	var test api.BackupTargetTest
	env.MustDo(http.MethodPost, "/backups/target/test", map[string]any{"target": "webdav", "webdav": dav}, &test)
	if test.Ok || !strings.Contains(test.Message, "用户名或密码不对") {
		t.Fatalf("wrong password: %+v", test)
	}
	if code, _ := env.Do(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "webdav", "webdav": dav}, nil); code != http.StatusBadRequest {
		t.Fatalf("saving a wrong password: %d", code)
	}
	dav["password"] = "pw"
	env.MustDo(http.MethodPost, "/backups/target/test", map[string]any{"target": "webdav", "webdav": dav}, &test)
	if !test.Ok {
		t.Fatalf("test: %+v", test)
	}
	var s api.BackupSettings
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "webdav", "keep": 2, "webdav": dav}, &s)
	if s.Webdav == nil || !s.Webdav.PasswordSet || s.Webdav.Folder != "x-console-backups" || s.Webdav.Url != srv.URL+"/dav/" {
		t.Fatalf("settings: %+v", s.Webdav)
	}
	// Leaving the password out keeps the saved one.
	env.MustDo(http.MethodPost, "/backups/target/test", map[string]any{"webdav": map[string]any{"folder": "other"}}, &test)
	if !test.Ok {
		t.Fatalf("test with the saved password: %+v", test)
	}

	runThree(t, env, m)
	// The packages sit right in the folder, two of them with their sidecars.
	names := davNames(t, fs, "/x-console-backups")
	if len(names) != 4 || !strings.HasSuffix(names[0], ".tar.gz") || !strings.HasSuffix(names[1], ".tar.gz.json") {
		t.Fatalf("folder: %v", names)
	}
	checkRemote(t, env, m, api.BackupLocationWebdav)
}

// noRedirect is a client that shows redirects instead of following them.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func callback(t *testing.T, env *testutil.Env, query url.Values) url.Values {
	t.Helper()
	resp, err := noRedirect.Get(env.URL("/backups/gdrive/callback?" + query.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback: %d", resp.StatusCode)
	}
	u, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || u.Path != "/settings/backup" {
		t.Fatalf("callback went to %q", resp.Header.Get("Location"))
	}
	return u.Query()
}

func TestBackupToGoogleDrive(t *testing.T) {
	env, m := setup(t)
	fake := fakegdrive.New(t)
	backup.SetGoogle(m, fake.Endpoints())
	putFile(t, env, "notes/attachments/1", "picture")

	if code, _ := env.Do(http.MethodGet, "/backups/gdrive/auth", nil, nil); code != http.StatusForbidden {
		t.Fatalf("auth without elevation: %d", code)
	}
	env.Elevate()
	if code, _ := env.Do(http.MethodGet, "/backups/gdrive/auth", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("auth without a client: %d", code)
	}
	// Turning it on before the authorization is refused.
	gd := map[string]any{"clientId": fake.ClientID, "clientSecret": fake.Secret}
	if code, _ := env.Do(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "gdrive", "gdrive": gd}, nil); code != http.StatusBadRequest {
		t.Fatalf("on without authorization: %d", code)
	}
	var s api.BackupSettings
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"gdrive": gd}, &s)
	if s.Gdrive == nil || !s.Gdrive.SecretSet || s.Gdrive.Authorized || s.Gdrive.FolderName != "X Console 备份" {
		t.Fatalf("settings: %+v", s.Gdrive)
	}

	var start struct{ Url, RedirectUri string }
	env.MustDo(http.MethodGet, "/backups/gdrive/auth", nil, &start)
	q := fakegdrive.AuthURL(t, start.Url)
	if q.Get("client_id") != fake.ClientID || q.Get("redirect_uri") != start.RedirectUri ||
		!strings.HasSuffix(start.RedirectUri, "/api/v1/backups/gdrive/callback") || q.Get("state") == "" {
		t.Fatalf("auth url: %s %s", start.Url, start.RedirectUri)
	}
	// A wrong state is refused, and the right one works once.
	if got := callback(t, env, url.Values{"state": {"forged"}, "code": {fake.Code}}); got.Get("gdrive") != "error" {
		t.Fatalf("forged state: %v", got)
	}
	if got := callback(t, env, url.Values{"state": {q.Get("state")}, "code": {fake.Code}}); got.Get("gdrive") != "ok" {
		t.Fatalf("callback: %v", got)
	}
	if got := callback(t, env, url.Values{"state": {q.Get("state")}, "code": {fake.Code}}); got.Get("gdrive") != "error" {
		t.Fatalf("state used twice: %v", got)
	}
	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if !s.Gdrive.Authorized || s.Gdrive.Account == nil || *s.Gdrive.Account != fake.Email {
		t.Fatalf("after authorization: %+v", s.Gdrive)
	}
	if got := fake.Folders(); len(got) != 1 || got[0] != "X Console 备份" {
		t.Fatalf("folders: %v", got)
	}

	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "gdrive", "keep": 2}, nil)
	runThree(t, env, m)
	if names := fake.Names("X Console 备份"); len(names) != 4 {
		t.Fatalf("drive folder: %v", names)
	}
	checkRemote(t, env, m, api.BackupLocationGdrive)
}

// authorize goes through the Google authorization with the fake.
func authorize(t *testing.T, env *testutil.Env, fake *fakegdrive.Server) {
	t.Helper()
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"gdrive": map[string]any{"clientId": fake.ClientID, "clientSecret": fake.Secret}}, nil)
	var start struct{ Url string }
	env.MustDo(http.MethodGet, "/backups/gdrive/auth", nil, &start)
	if got := callback(t, env, url.Values{"state": {fakegdrive.AuthURL(t, start.Url).Get("state")}, "code": {fake.Code}}); got.Get("gdrive") != "ok" {
		t.Fatalf("callback: %v", got)
	}
}

func TestGoogleDriveExpiredTokenAndRevoke(t *testing.T) {
	env, m := setup(t)
	fake := fakegdrive.New(t)
	backup.SetGoogle(m, fake.Endpoints())
	env.Elevate()
	authorize(t, env, fake)
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "gdrive"}, nil)

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

	env.MustDo(http.MethodDelete, "/backups/gdrive/auth", nil, nil)
	if got := fake.Revoked(); len(got) != 1 || got[0] != fake.RefreshToken {
		t.Fatalf("revoked: %v", got)
	}
	var s api.BackupSettings
	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if s.Gdrive.Authorized || s.Gdrive.Account != nil {
		t.Fatalf("after revoke: %+v", s.Gdrive)
	}
}

func TestChangingTheGoogleClientDropsTheToken(t *testing.T) {
	env, m := setup(t)
	fake := fakegdrive.New(t)
	backup.SetGoogle(m, fake.Endpoints())
	env.Elevate()
	authorize(t, env, fake)
	var s api.BackupSettings
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"gdrive": map[string]any{"clientId": "another"}}, &s)
	if s.Gdrive.Authorized || s.Gdrive.ClientId != "another" {
		t.Fatalf("after changing the client: %+v", s.Gdrive)
	}
}

func TestBrowseRemoteDrives(t *testing.T) {
	env, m := setup(t)
	srv, fs := davServer(t)
	fake := fakegdrive.New(t)
	backup.SetGoogle(m, fake.Endpoints())
	env.Elevate()

	var drives struct{ Items []api.RemoteDrive }
	env.MustDo(http.MethodGet, "/remote-drives", nil, &drives)
	if len(drives.Items) != 0 {
		t.Fatalf("nothing bound: %+v", drives.Items)
	}
	if code, _ := env.Do(http.MethodGet, "/remote-drives/webdav/items", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unbound drive: %d", code)
	}

	// WebDAV: browse from the top of the address, not only the backup folder.
	if err := fs.Mkdir(context.Background(), "/照片", 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := fs.OpenFile(context.Background(), "/照片/a.jpg", os.O_CREATE|os.O_WRONLY, 0o644)
	f.Write([]byte("jpeg bytes"))
	f.Close()
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"webdav": map[string]any{"url": srv.URL + "/dav/", "username": "me", "password": "pw"}}, nil)
	authorize(t, env, fake)
	docs := fake.Add("root", "文档", fakegdrive.FolderMime, nil)
	fake.Add(docs, "说明.txt", "", []byte("hello"))

	env.MustDo(http.MethodGet, "/remote-drives", nil, &drives)
	if len(drives.Items) != 2 || drives.Items[0].Id != api.RemoteDriveIdWebdav || drives.Items[1].Name != "Google Drive" || drives.Items[1].Limited {
		t.Fatalf("drives: %+v", drives.Items)
	}
	var list api.RemoteDriveListing
	env.MustDo(http.MethodGet, "/remote-drives/webdav/items?ref="+url.QueryEscape("照片"), nil, &list)
	if len(list.Items) != 1 || list.Items[0].Ref != "照片/a.jpg" || len(list.Trail) != 1 || list.Trail[0].Name != "照片" {
		t.Fatalf("webdav listing: %+v", list)
	}
	status, raw := rawGet(t, env, "/remote-drives/webdav/download?ref="+url.QueryEscape("照片/a.jpg"))
	if status != http.StatusOK || string(raw) != "jpeg bytes" {
		t.Fatalf("webdav download: %d %q", status, raw)
	}

	env.MustDo(http.MethodGet, "/remote-drives/gdrive/items", nil, &list)
	var folder string
	for _, it := range list.Items {
		if it.Name == "文档" && it.IsDir {
			folder = it.Ref
		}
	}
	if folder == "" {
		t.Fatalf("gdrive root: %+v", list.Items)
	}
	env.MustDo(http.MethodGet, "/remote-drives/gdrive/items?ref="+folder, nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "说明.txt" || !list.Items[0].Downloadable || len(list.Trail) != 1 {
		t.Fatalf("gdrive folder: %+v", list)
	}
	status, raw = rawGet(t, env, "/remote-drives/gdrive/download?ref="+list.Items[0].Ref)
	if status != http.StatusOK || string(raw) != "hello" {
		t.Fatalf("gdrive download: %d %q", status, raw)
	}

	// Hidden and locked: the Google tab is gone, the backup still works.
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"drive-gdrive"}}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	env.MustDo(http.MethodGet, "/remote-drives", nil, &drives)
	if len(drives.Items) != 1 || drives.Items[0].Id != api.RemoteDriveIdWebdav {
		t.Fatalf("locked drives: %+v", drives.Items)
	}
	if code, _ := env.Do(http.MethodGet, "/remote-drives/gdrive/items", nil, nil); code != http.StatusNotFound {
		t.Fatalf("hidden drive: %d", code)
	}
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "gdrive"}, nil)
	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	if job := waitJob(t, env); job.State != api.Done {
		t.Fatalf("backup while hidden: %+v %v", job, deref(job.Error))
	}
}

func TestOldGoogleGrantIsLimited(t *testing.T) {
	env, m := setup(t)
	fake := fakegdrive.New(t)
	fake.Scope = "https://www.googleapis.com/auth/drive.file"
	backup.SetGoogle(m, fake.Endpoints())
	env.Elevate()
	authorize(t, env, fake)
	var drives struct{ Items []api.RemoteDrive }
	env.MustDo(http.MethodGet, "/remote-drives", nil, &drives)
	if len(drives.Items) != 1 || !drives.Items[0].Limited {
		t.Fatalf("drives: %+v", drives.Items)
	}
}
