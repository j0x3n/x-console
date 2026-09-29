package storage_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakes3"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const bucket = "site"

func s3Body(srv *fakes3.Server) map[string]any {
	return map[string]any{"endpoint": srv.URL, "region": "us-east-1", "bucket": bucket,
		"accessKeyId": "AK", "secretAccessKey": "SK", "pathStyle": true}
}

func status(t *testing.T, env *testutil.Env) api.StorageStatus {
	t.Helper()
	var out api.StorageStatus
	env.MustDo(http.MethodGet, "/storage", nil, &out)
	return out
}

func put(t *testing.T, env *testutil.Env, key, content string) {
	t.Helper()
	if err := env.App.Deps.Files.Store().Put(context.Background(), key, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, s files.Store, key string) string {
	t.Helper()
	rc, _, err := s.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	defer rc.Close()
	raw, _ := io.ReadAll(rc)
	return string(raw)
}

// waitMove waits until the move is no longer running.
func waitMove(t *testing.T, env *testutil.Env) api.StorageStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := status(t, env); st.Migration.State != api.Running {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("move did not finish")
	return api.StorageStatus{}
}

func TestStatusStartsLocalWithUsage(t *testing.T) {
	env := testutil.New(t, storage.New)
	put(t, env, "drive/blobs/ab/abc", "12345")
	put(t, env, "drive/blobs/cd/cde", "678")
	put(t, env, "notes/attachments/1", "x")
	st := status(t, env)
	if st.Backend != api.Local || st.S3 != nil || st.Migration.State != api.Idle {
		t.Fatalf("status: %+v", st)
	}
	if st.LocalPath != env.App.Deps.Config.FilesDir() {
		t.Fatalf("localPath: %q", st.LocalPath)
	}
	if st.CacheLimitBytes != 1<<30 {
		t.Fatalf("cache limit: %d", st.CacheLimitBytes)
	}
	want := map[string][2]int64{"drive": {2, 8}, "notes": {1, 1}}
	if len(st.Usage) != len(want) {
		t.Fatalf("usage: %+v", st.Usage)
	}
	for _, u := range st.Usage {
		if w := want[u.Module]; u.Files != w[0] || u.Bytes != w[1] {
			t.Fatalf("usage of %s: %+v", u.Module, u)
		}
	}
}

func TestS3SettingsNeedElevationAndHideSecret(t *testing.T) {
	env := testutil.New(t, storage.New)
	srv := fakes3.New(t, bucket)
	if code, _ := env.Do(http.MethodPut, "/storage/s3", s3Body(srv), nil); code != http.StatusForbidden {
		t.Fatalf("without elevation: %d", code)
	}
	env.Elevate()
	var st api.StorageStatus
	env.MustDo(http.MethodPut, "/storage/s3", s3Body(srv), &st)
	if st.S3 == nil || st.S3.Bucket != bucket || !st.S3.HasSecret || st.Backend != api.Local {
		t.Fatalf("saved: %+v", st.S3)
	}
	// A later edit that leaves the secret out keeps the old one.
	env.MustDo(http.MethodPut, "/storage/s3", map[string]any{"prefix": "site1"}, &st)
	if st.S3.Prefix != "site1" || !st.S3.HasSecret || st.S3.AccessKeyId != "AK" {
		t.Fatalf("partial edit: %+v", st.S3)
	}
	if code, _ := env.Do(http.MethodPut, "/storage/s3", map[string]any{"endpoint": "ftp://x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad endpoint: %d", code)
	}
}

func TestS3ConnectionTest(t *testing.T) {
	env := testutil.New(t, storage.New)
	srv := fakes3.New(t, bucket)
	var res api.TestResult
	env.MustDo(http.MethodPost, "/storage/s3/test", nil, &res)
	if res.Ok || res.Message == "" {
		t.Fatalf("nothing saved: %+v", res)
	}
	env.MustDo(http.MethodPost, "/storage/s3/test", s3Body(srv), &res)
	if !res.Ok {
		t.Fatalf("good setup: %+v", res)
	}
	bad := s3Body(srv)
	bad["bucket"] = "other"
	env.MustDo(http.MethodPost, "/storage/s3/test", bad, &res)
	if res.Ok || res.Message == "" {
		t.Fatalf("wrong bucket: %+v", res)
	}
	// Testing must not save anything.
	if st := status(t, env); st.S3 != nil {
		t.Fatalf("test saved settings: %+v", st.S3)
	}
}

func TestSwitchChecks(t *testing.T) {
	env := testutil.New(t, storage.New)
	if code, _ := env.Do(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil); code != http.StatusForbidden {
		t.Fatalf("without elevation: %d", code)
	}
	env.Elevate()
	if code, _ := env.Do(http.MethodPost, "/storage/switch", map[string]any{"target": "local"}, nil); code != http.StatusBadRequest {
		t.Fatalf("same place: %d", code)
	}
	if code, _ := env.Do(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil); code != http.StatusPreconditionFailed {
		t.Fatalf("s3 not set up: %d", code)
	}
	if code, _ := env.Do(http.MethodPost, "/storage/switch/cancel", nil, nil); code != http.StatusConflict {
		t.Fatalf("cancel with nothing running: %d", code)
	}
}

func TestMoveToS3FailsThenRetriesAndSwitches(t *testing.T) {
	env := testutil.New(t, storage.New)
	srv := fakes3.New(t, bucket)
	keys := []string{"drive/blobs/aa/1", "drive/blobs/bb/2", "drive/blobs/cc/3", "notes/attachments/4"}
	for _, k := range keys {
		put(t, env, k, "data of "+k)
	}
	env.Elevate()
	env.MustDo(http.MethodPut, "/storage/s3", s3Body(srv), nil)

	var mu sync.Mutex
	broken := true
	srv.Fail = func(method, key string) int {
		mu.Lock()
		defer mu.Unlock()
		if broken && method == http.MethodPut && key == "drive/blobs/cc/3" {
			return http.StatusForbidden
		}
		return 0
	}
	env.MustDo(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil)
	st := waitMove(t, env)
	if st.Migration.State != api.Failed || st.Migration.Error == nil || !strings.Contains(*st.Migration.Error, "drive/blobs/cc/3") {
		t.Fatalf("after failure: %+v", st.Migration)
	}
	if st.Backend != api.Local {
		t.Fatal("switched although a file failed")
	}
	if got := read(t, env.App.Deps.Files.Store(), keys[0]); got != "data of "+keys[0] {
		t.Fatalf("source damaged: %q", got)
	}

	mu.Lock()
	broken = false
	mu.Unlock()
	putsBefore := srv.Puts
	env.MustDo(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil)
	st = waitMove(t, env)
	if st.Migration.State != api.Done || st.Backend != api.S3 {
		t.Fatalf("after retry: %+v backend=%s", st.Migration, st.Backend)
	}
	if st.Migration.DoneFiles != int64(len(keys)) || st.Migration.TotalFiles != int64(len(keys)) {
		t.Fatalf("counts: %+v", st.Migration)
	}
	// Files that already made it over are not sent again.
	// Two files still to send, plus the small file the connection check writes.
	if sent := srv.Puts - putsBefore; sent != 3 {
		t.Fatalf("retry sent %d files", sent)
	}
	objects := srv.Objects()
	for _, k := range keys {
		if string(objects[k]) != "data of "+k {
			t.Fatalf("bucket %s: %q", k, objects[k])
		}
	}
	// The old files stay by default.
	if _, err := os.Stat(filepath.Join(env.App.Deps.Config.FilesDir(), "drive", "blobs", "aa", "1")); err != nil {
		t.Fatalf("source removed: %v", err)
	}
	// Everything now reads through the bucket.
	if got := read(t, env.App.Deps.Files.For("notes"), "attachments/4"); got != "data of notes/attachments/4" {
		t.Fatalf("read after switch: %q", got)
	}
	// The choice survives a restart.
	var saved string
	if err := env.App.Deps.Settings.Get(context.Background(), "storage.backend", &saved); err != nil || saved != "s3" {
		t.Fatalf("saved backend: %q %v", saved, err)
	}
	if _, err := storage.New(env.App.Deps); err != nil {
		t.Fatal(err)
	}
	if got := read(t, env.App.Deps.Files.Store(), keys[3]); got != "data of "+keys[3] {
		t.Fatalf("read after reload: %q", got)
	}
}

func TestFilesWrittenDuringMoveReachBothPlaces(t *testing.T) {
	env := testutil.New(t, storage.New)
	srv := fakes3.New(t, bucket)
	put(t, env, "drive/blobs/aa/1", "first")
	put(t, env, "drive/blobs/bb/2", "second")
	env.Elevate()
	env.MustDo(http.MethodPut, "/storage/s3", s3Body(srv), nil)

	// Hold the copy of the second file until the test has written a new one.
	release := make(chan struct{})
	reached := make(chan struct{})
	var once sync.Once
	srv.Fail = func(method, key string) int {
		if method == http.MethodPut && key == "drive/blobs/bb/2" {
			once.Do(func() { close(reached) })
			<-release
		}
		return 0
	}
	env.MustDo(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil)
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("move did not reach the second file")
	}
	put(t, env, "notes/attachments/new", "written during the move")
	if err := env.App.Deps.Files.Store().Delete(context.Background(), "drive/blobs/aa/1"); err != nil {
		t.Fatal(err)
	}
	close(release)

	st := waitMove(t, env)
	if st.Migration.State != api.Done {
		t.Fatalf("move: %+v", st.Migration)
	}
	objects := srv.Objects()
	if string(objects["notes/attachments/new"]) != "written during the move" {
		t.Fatalf("new file missing in bucket: %q", objects["notes/attachments/new"])
	}
	if _, ok := objects["drive/blobs/aa/1"]; ok {
		t.Fatal("file deleted during the move was left in the bucket")
	}
	if got := read(t, env.App.Deps.Files.Store(), "notes/attachments/new"); got != "written during the move" {
		t.Fatalf("read: %q", got)
	}
}

func TestCancelStopsMoveAndKeepsLocal(t *testing.T) {
	env := testutil.New(t, storage.New)
	srv := fakes3.New(t, bucket)
	put(t, env, "drive/blobs/aa/1", "first")
	put(t, env, "drive/blobs/bb/2", "second")
	env.Elevate()
	env.MustDo(http.MethodPut, "/storage/s3", s3Body(srv), nil)

	release := make(chan struct{})
	reached := make(chan struct{})
	var once sync.Once
	srv.Fail = func(method, key string) int {
		if method == http.MethodPut && key == "drive/blobs/bb/2" {
			once.Do(func() { close(reached) })
			select {
			case <-release:
			case <-time.After(5 * time.Second):
			}
		}
		return 0
	}
	defer close(release)
	env.MustDo(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil)
	<-reached
	if code, _ := env.Do(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil); code != http.StatusConflict {
		t.Fatalf("second move while running: %d", code)
	}
	env.MustDo(http.MethodPost, "/storage/switch/cancel", nil, nil)
	st := status(t, env)
	if st.Migration.State != api.Canceled || st.Backend != api.Local {
		t.Fatalf("after cancel: %+v backend=%s", st.Migration, st.Backend)
	}
	// Writes go to one place again.
	before := srv.Puts
	put(t, env, "notes/attachments/after", "x")
	if srv.Puts != before {
		t.Fatal("the bucket still gets writes after cancel")
	}
}

func TestMoveBackToLocalAndDeleteSource(t *testing.T) {
	env := testutil.New(t, storage.New)
	srv := fakes3.New(t, bucket)
	srv.Set("drive/blobs/aa/1", []byte("in the bucket"))
	srv.Set("notes/attachments/2", []byte("also there"))
	srv.Set("backups/x-console-1.tar.gz", []byte("a backup"))
	env.Elevate()
	env.MustDo(http.MethodPut, "/storage/s3", s3Body(srv), nil)
	env.MustDo(http.MethodPost, "/storage/switch", map[string]any{"target": "s3"}, nil)
	if st := waitMove(t, env); st.Migration.State != api.Done || st.Backend != api.S3 {
		t.Fatalf("to s3: %+v", st.Migration)
	}
	env.MustDo(http.MethodPost, "/storage/switch", map[string]any{"target": "local", "deleteSource": true}, nil)
	st := waitMove(t, env)
	if st.Migration.State != api.Done || st.Backend != api.Local {
		t.Fatalf("to local: %+v", st.Migration)
	}
	if got := read(t, env.App.Deps.Files.Store(), "drive/blobs/aa/1"); got != "in the bucket" {
		t.Fatalf("local copy: %q", got)
	}
	// Backups share the bucket but are not site files: they stay put.
	if left := srv.Objects(); len(left) != 1 || string(left["backups/x-console-1.tar.gz"]) != "a backup" {
		t.Fatalf("bucket after delete: %v", left)
	}
	if _, err := os.Stat(filepath.Join(env.App.Deps.Config.FilesDir(), "backups")); err == nil {
		t.Fatal("a backup was moved to the local directory")
	}
	for _, u := range st.Usage {
		if u.Module == "backups" {
			t.Fatal("backups counted as site files")
		}
	}
}

func TestCacheLimit(t *testing.T) {
	env := testutil.New(t, storage.New)
	var st api.StorageStatus
	env.MustDo(http.MethodPut, "/storage/cache", map[string]any{"limitBytes": 1024}, &st)
	if st.CacheLimitBytes != 1024 {
		t.Fatalf("limit: %d", st.CacheLimitBytes)
	}
	if code, _ := env.Do(http.MethodPut, "/storage/cache", map[string]any{"limitBytes": -1}, nil); code != http.StatusBadRequest {
		t.Fatalf("negative limit: %d", code)
	}
}

func TestDriveS3SettingsAreCopiedOnce(t *testing.T) {
	env := testutil.New(t, storage.New)
	d := env.App.Deps
	ctx := context.Background()
	old := map[string]any{"endpoint": "https://s3.example.com", "region": "auto", "bucket": "old",
		"prefix": "drive", "accessKeyId": "OLDKEY", "pathStyle": true}
	if err := d.Settings.Set(ctx, "drive.s3.config", old); err != nil {
		t.Fatal(err)
	}
	if err := d.Settings.SetSecret(ctx, "drive.s3.secret", "OLDSECRET"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.New(d); err != nil {
		t.Fatal(err)
	}
	st := status(t, env)
	if st.S3 == nil || st.S3.Bucket != "old" || st.S3.AccessKeyId != "OLDKEY" || !st.S3.HasSecret || !st.S3.PathStyle {
		t.Fatalf("copied: %+v", st.S3)
	}
	if st.Backend != api.Local {
		t.Fatalf("copy must not switch: %s", st.Backend)
	}
	var secret string
	if err := d.Settings.Get(ctx, "storage.s3_secret", &secret); err != nil || secret != "OLDSECRET" {
		t.Fatalf("secret: %q %v", secret, err)
	}
	// A second start does not overwrite what the user changed since.
	env.Elevate()
	env.MustDo(http.MethodPut, "/storage/s3", map[string]any{"bucket": "new"}, nil)
	if _, err := storage.New(d); err != nil {
		t.Fatal(err)
	}
	if st := status(t, env); st.S3.Bucket != "new" {
		t.Fatalf("overwritten: %+v", st.S3)
	}
}

func TestRestartDuringMoveMarksItFailed(t *testing.T) {
	env := testutil.New(t, storage.New)
	d := env.App.Deps
	running := api.StorageMigration{State: api.Running, Target: api.S3, TotalFiles: 3}
	if err := d.Settings.Set(context.Background(), "storage.migration", running); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.New(d); err != nil {
		t.Fatal(err)
	}
	var stored api.StorageMigration
	if err := d.Settings.Get(context.Background(), "storage.migration", &stored); err != nil {
		t.Fatal(err)
	}
	if stored.State != api.Failed || stored.Error == nil || stored.FinishedAt == nil {
		t.Fatalf("stored: %+v", stored)
	}
}
