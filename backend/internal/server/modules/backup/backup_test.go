package backup_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files/fakes3"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/store"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

const bucket = "site"

func setup(t *testing.T) (*testutil.Env, *backup.Module) {
	t.Helper()
	env := testutil.New(t, storage.New, backup.New)
	m, ok := module.Lookup[*backup.Module](env.App.Deps.Registry, backup.ServiceKey)
	if !ok {
		t.Fatal("backup module not registered")
	}
	return env, m
}

func putFile(t *testing.T, env *testutil.Env, key, content string) {
	t.Helper()
	if err := env.App.Deps.Files.Store().Put(context.Background(), key, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
}

func waitJob(t *testing.T, env *testutil.Env) api.BackupJob {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var job api.BackupJob
		env.MustDo(http.MethodGet, "/backups/job", nil, &job)
		if job.State != api.Running {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return api.BackupJob{}
}

func exportOne(t *testing.T, env *testutil.Env) string {
	t.Helper()
	env.Elevate()
	env.MustDo(http.MethodPost, "/backups/export", nil, nil)
	job := waitJob(t, env)
	if job.State != api.Done || job.BackupId == nil {
		t.Fatalf("export: %+v", job)
	}
	return *job.BackupId
}

func rawGet(t *testing.T, env *testutil.Env, path string) (int, []byte) {
	t.Helper()
	resp, err := env.Client.Get(env.URL(path))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// pack is a package taken apart.
type pack struct {
	manifest map[string]any
	db       []byte
	files    map[string]string
}

func unpack(t *testing.T, data []byte) pack {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	p := pack{files: map[string]string{}}
	first := true
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		switch {
		case hdr.Name == "manifest.json":
			if !first {
				t.Fatal("manifest is not the first entry")
			}
			if err := json.Unmarshal(body, &p.manifest); err != nil {
				t.Fatal(err)
			}
		case hdr.Name == "x-console.db":
			p.db = body
		case strings.HasPrefix(hdr.Name, "files/"):
			p.files[strings.TrimPrefix(hdr.Name, "files/")] = string(body)
		}
		first = false
	}
	return p
}

func makePackage(t *testing.T, manifest map[string]any, db []byte, fileMap map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name string, body []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(body)
	}
	raw, _ := json.Marshal(manifest)
	add("manifest.json", raw)
	if db != nil {
		add("x-console.db", db)
	}
	for k, v := range fileMap {
		add("files/"+k, []byte(v))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func upload(t *testing.T, env *testutil.Env, content []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "x.tar.gz")
	part.Write(content)
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, env.URL("/backups/upload"), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func TestExportListDownloadAndDelete(t *testing.T) {
	env, _ := setup(t)
	ctx := context.Background()
	if err := env.App.Deps.Settings.Set(ctx, "test.marker", "hello"); err != nil {
		t.Fatal(err)
	}
	putFile(t, env, "notes/attachments/1", "picture bytes")
	putFile(t, env, "drive/blobs/ab/abc", "drive bytes")

	if code, _ := env.Do(http.MethodPost, "/backups/export", nil, nil); code != http.StatusForbidden {
		t.Fatalf("export without elevation: %d", code)
	}
	id := exportOne(t, env)
	if !strings.HasPrefix(id, "x-console-") || !strings.HasSuffix(id, ".tar.gz") {
		t.Fatalf("name: %q", id)
	}

	var list struct{ Items []api.Backup }
	env.MustDo(http.MethodGet, "/backups", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Id != id || list.Items[0].Kind != api.BackupKindManual ||
		list.Items[0].Location != api.Local || list.Items[0].Files == nil || *list.Items[0].Files != 2 {
		t.Fatalf("list: %+v", list.Items)
	}

	status, raw := rawGet(t, env, "/backups/"+id+"/download")
	if status != http.StatusOK {
		t.Fatalf("download: %d %s", status, raw)
	}
	p := unpack(t, raw)
	if p.manifest["format"] != float64(1) || p.manifest["files"] != float64(2) || p.manifest["dbSha256"] == "" || p.manifest["migration"] == "" {
		t.Fatalf("manifest: %v", p.manifest)
	}
	if p.files["notes/attachments/1"] != "picture bytes" || p.files["drive/blobs/ab/abc"] != "drive bytes" || len(p.files) != 2 {
		t.Fatalf("files: %v", p.files)
	}
	dbPath := filepath.Join(t.TempDir(), "copy.db")
	if err := os.WriteFile(dbPath, p.db, 0o600); err != nil {
		t.Fatal(err)
	}
	copyDB, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var marker string
	if err := copyDB.QueryRow(`SELECT value FROM settings WHERE key = 'test.marker'`).Scan(&marker); err != nil || marker != `"hello"` {
		t.Fatalf("marker in database copy: %q %v", marker, err)
	}

	// The sidecar file is not a backup of its own.
	if _, err := os.Stat(filepath.Join(env.App.Deps.Config.BackupsDir(), id+".json")); err != nil {
		t.Fatalf("sidecar: %v", err)
	}
	env.MustDo(http.MethodDelete, "/backups/"+id, nil, nil)
	env.MustDo(http.MethodGet, "/backups", nil, &list)
	if len(list.Items) != 0 {
		t.Fatalf("after delete: %+v", list.Items)
	}
	if _, err := os.Stat(filepath.Join(env.App.Deps.Config.BackupsDir(), id+".json")); err == nil {
		t.Fatal("sidecar left behind")
	}
}

func TestOldDeployBackupsAreNotListed(t *testing.T) {
	env, _ := setup(t)
	dir := env.App.Deps.Config.BackupsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x-console-20260928T010203Z-sha-1.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	var list struct{ Items []api.Backup }
	env.MustDo(http.MethodGet, "/backups", nil, &list)
	if len(list.Items) != 0 {
		t.Fatalf("list: %+v", list.Items)
	}
}

func TestBadIDsAndElevation(t *testing.T) {
	env, _ := setup(t)
	for _, id := range []string{"notes.txt", "..%2Fx-console.db", "a%2Fb.tar.gz", ".hidden.tar.gz"} {
		code, _ := rawGet(t, env, "/backups/"+id+"/download")
		if code != http.StatusForbidden && code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Fatalf("id %q: %d", id, code)
		}
	}
	env.Elevate()
	for _, id := range []string{"notes.txt", "..%2Fx-console.db", ".hidden.tar.gz"} {
		code, _ := rawGet(t, env, "/backups/"+id+"/download")
		if code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Fatalf("elevated id %q: %d", id, code)
		}
		if code, _ := env.Do(http.MethodDelete, "/backups/"+id, nil, nil); code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Fatalf("delete id %q: %d", id, code)
		}
	}
	if code, _ := env.Do(http.MethodDelete, "/backups/missing.tar.gz", nil, nil); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}
}

func TestUploadChecksThePackage(t *testing.T) {
	env, _ := setup(t)
	if code, _ := upload(t, env, []byte("hello")); code != http.StatusForbidden {
		t.Fatalf("without elevation: %d", code)
	}
	env.Elevate()
	var junk bytes.Buffer
	gz := gzip.NewWriter(&junk)
	gz.Write([]byte("just some text, not a tar archive"))
	gz.Close()
	for name, body := range map[string][]byte{"text": []byte("hello"), "gzip of text": junk.Bytes(), "empty": {}} {
		if code, raw := upload(t, env, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}
	entries, _ := os.ReadDir(env.App.Deps.Config.BackupsDir())
	if len(entries) != 0 {
		t.Fatalf("rejected uploads left files: %v", entries)
	}

	good := makePackage(t, map[string]any{"format": 1, "version": "dev", "dbSha256": "ab", "migration": "1", "files": 0}, []byte("x"), nil)
	code, raw := upload(t, env, good)
	if code != http.StatusCreated {
		t.Fatalf("good package: %d %s", code, raw)
	}
	var got api.Backup
	if err := json.Unmarshal(raw, &got); err != nil || got.Kind != api.BackupKindUploaded || !strings.HasPrefix(got.Id, "uploaded-") {
		t.Fatalf("uploaded: %+v %v", got, err)
	}
}

func TestRestoreChecks(t *testing.T) {
	env, _ := setup(t)
	env.Elevate()
	newer := makePackage(t, map[string]any{"format": 1, "version": "future", "dbSha256": "ab", "migration": "99999999999999", "files": 0}, []byte("x"), nil)
	code, raw := upload(t, env, newer)
	if code != http.StatusCreated {
		t.Fatalf("upload: %d %s", code, raw)
	}
	var item api.Backup
	json.Unmarshal(raw, &item)
	if code, _ := env.Do(http.MethodPost, "/backups/"+item.Id+"/restore", map[string]string{"confirm": "yes"}, nil); code != http.StatusBadRequest {
		t.Fatalf("wrong confirm: %d", code)
	}
	code, raw = env.Do(http.MethodPost, "/backups/"+item.Id+"/restore", map[string]string{"confirm": "恢复"}, nil)
	if code != http.StatusBadRequest || !strings.Contains(string(raw), "更新的版本") {
		t.Fatalf("newer package: %d %s", code, raw)
	}
	if code, _ := env.Do(http.MethodPost, "/backups/missing.tar.gz/restore", map[string]string{"confirm": "恢复"}, nil); code != http.StatusNotFound {
		t.Fatalf("missing: %d", code)
	}
	if job := waitJob(t, env); job.State != api.Idle {
		t.Fatalf("a refused restore started a job: %+v", job)
	}
}

// TestRestoreOnAnotherInstall moves a backup from one site to another: the
// restore is staged, the "restart" applies the database, and the file phase
// leaves exactly the files of the backup.
func TestRestoreOnAnotherInstall(t *testing.T) {
	src, _ := setup(t)
	ctx := context.Background()
	if err := src.App.Deps.Settings.Set(ctx, "test.marker", "from the first site"); err != nil {
		t.Fatal(err)
	}
	if err := src.App.Deps.Settings.SetSecret(ctx, "test.token", "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	putFile(t, src, "notes/attachments/1", "picture bytes")
	putFile(t, src, "drive/blobs/ab/abc", "drive bytes")
	id := exportOne(t, src)
	status, pkg := rawGet(t, src, "/backups/"+id+"/download")
	if status != http.StatusOK {
		t.Fatalf("download: %d", status)
	}

	dst, m := setup(t)
	putFile(t, dst, "notes/attachments/old", "left over of the second site")
	dst.Elevate()
	var stops atomic.Int32
	backup.SetExit(m, func() { stops.Add(1) })
	code, raw := upload(t, dst, pkg)
	if code != http.StatusCreated {
		t.Fatalf("upload: %d %s", code, raw)
	}
	var item api.Backup
	json.Unmarshal(raw, &item)
	dst.MustDo(http.MethodPost, "/backups/"+item.Id+"/restore", map[string]string{"confirm": "恢复"}, nil)
	deadline := time.Now().Add(15 * time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stops.Load() != 1 {
		t.Fatalf("server was not asked to stop: %d", stops.Load())
	}
	restoreDir := dst.App.Deps.Config.RestoreDir()
	if !backup.Pending(restoreDir) {
		t.Fatal("restore is not staged")
	}
	// The current data was saved first.
	var list struct{ Items []api.Backup }
	dst.MustDo(http.MethodGet, "/backups", nil, &list)
	kinds := map[api.BackupKind]int{}
	for _, b := range list.Items {
		kinds[b.Kind]++
	}
	if kinds[api.BackupKindPreRestore] != 1 || kinds[api.BackupKindUploaded] != 1 {
		t.Fatalf("backups after staging: %+v", list.Items)
	}
	// Nothing has changed yet.
	if got := readStore(t, dst, "notes/attachments/old"); got != "left over of the second site" {
		t.Fatalf("data changed before the restart: %q", got)
	}

	// The restart: the database is put in place before it is opened.
	dataDir := dst.App.Deps.Config.DataDir
	applied, err := backup.ApplyPending(dataDir)
	if err != nil || !applied {
		t.Fatalf("apply: %v %v", applied, err)
	}
	if applied, err = backup.ApplyPending(dataDir); err != nil || !applied {
		t.Fatalf("apply twice must be harmless: %v %v", applied, err)
	}
	restored, err := store.Open(ctx, filepath.Join(dataDir, "x-console.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var marker string
	if err := restored.QueryRow(`SELECT value FROM settings WHERE key = 'test.marker'`).Scan(&marker); err != nil || marker != `"from the first site"` {
		t.Fatalf("restored marker: %q %v", marker, err)
	}
	// Same master key: everything can be read. Another key: the token cannot.
	key := bytes.Repeat([]byte{7}, 32)
	same, _ := secrets.NewBox(key)
	if keys, err := backup.UnreadableSecrets(ctx, restored, same); err != nil || len(keys) != 0 {
		t.Fatalf("same key: %v %v", keys, err)
	}
	other, _ := secrets.NewBox(bytes.Repeat([]byte{9}, 32))
	keys, err := backup.UnreadableSecrets(ctx, restored, other)
	if err != nil || !contains(keys, "test.token") {
		t.Fatalf("other key: %v %v", keys, err)
	}

	// The file phase.
	if err := backup.FinishRestore(m, ctx); err != nil {
		t.Fatal(err)
	}
	if got := readStore(t, dst, "notes/attachments/1"); got != "picture bytes" {
		t.Fatalf("restored file: %q", got)
	}
	if got := readStore(t, dst, "drive/blobs/ab/abc"); got != "drive bytes" {
		t.Fatalf("restored file: %q", got)
	}
	if _, _, err := dst.App.Deps.Files.Store().Get(ctx, "notes/attachments/old"); err == nil {
		t.Fatal("a file that is not in the backup is still there")
	}
	if backup.Pending(restoreDir) {
		t.Fatal("restore folder was not cleaned up")
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func readStore(t *testing.T, env *testutil.Env, key string) string {
	t.Helper()
	rc, _, err := env.App.Deps.Files.Store().Get(context.Background(), key)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	defer rc.Close()
	raw, _ := io.ReadAll(rc)
	return string(raw)
}

func s3Settings(srv *fakes3.Server) map[string]any {
	return map[string]any{"endpoint": srv.URL, "region": "us-east-1", "bucket": bucket, "prefix": "site",
		"accessKeyId": "AK", "secretAccessKey": "SK", "pathStyle": true}
}

func TestSettingsValidation(t *testing.T) {
	env, _ := setup(t)
	var s api.BackupSettings
	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if s.Enabled || s.Frequency != api.BackupSettingsFrequencyDaily || s.Time != "03:00" || s.Keep != 14 ||
		s.Target != "storage" || s.NextRunAt != nil || len(s.LastRuns) != 0 {
		t.Fatalf("defaults: %+v", s)
	}
	if code, _ := env.Do(http.MethodPut, "/backups/settings", map[string]any{"keep": 3}, nil); code != http.StatusForbidden {
		t.Fatalf("without elevation: %d", code)
	}
	env.Elevate()
	for name, body := range map[string]map[string]any{
		"bad time":             {"time": "3am"},
		"bad hour":             {"time": "25:00"},
		"bad keep":             {"keep": 0},
		"bad weekday":          {"weekday": 9},
		"bad target":           {"target": "somewhere"},
		"no storage S3":        {"enabled": true, "target": "storage"},
		"custom without setup": {"enabled": true, "target": "custom"},
	} {
		if code, raw := env.Do(http.MethodPut, "/backups/settings", body, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}
	// Turned off, incomplete settings are fine.
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"keep": 3, "time": "04:30", "frequency": "weekly", "weekday": 2}, &s)
	if s.Keep != 3 || s.Time != "04:30" || s.Frequency != api.BackupSettingsFrequencyWeekly || s.Weekday != 2 {
		t.Fatalf("saved: %+v", s)
	}
}

func TestAutomaticBackupToS3KeepsTheNewest(t *testing.T) {
	env, m := setup(t)
	srv := fakes3.New(t, bucket)
	putFile(t, env, "notes/attachments/1", "picture")
	env.Elevate()

	clock := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC).In(env.App.Deps.Config.Location) // 10:00 in Shanghai
	backup.SetNow(m, func() time.Time { return clock })
	body := map[string]any{"enabled": true, "target": "custom", "keep": 2, "time": "03:00", "frequency": "daily",
		"s3": s3Settings(srv)}
	var s api.BackupSettings
	env.MustDo(http.MethodPut, "/backups/settings", body, &s)
	if s.S3 == nil || !s.S3.HasSecret || s.S3.Bucket != bucket || s.NextRunAt == nil {
		t.Fatalf("settings: %+v", s)
	}
	// Switching it on at 10:00 does not run the 03:00 backup of today.
	if err := backup.Tick(m, context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(srv.Objects()); n != 0 {
		t.Fatalf("ran when switched on: %d objects", n)
	}

	for day := 2; day <= 4; day++ {
		clock = time.Date(2026, 10, day, 3, 5, 0, 0, env.App.Deps.Config.Location)
		if err := backup.Tick(m, context.Background()); err != nil {
			t.Fatal(err)
		}
		if job := waitJob(t, env); job.State != api.Done || job.Kind == nil || *job.Kind != api.BackupJobKindAuto {
			t.Fatalf("day %d: %+v error=%v", day, job, job.Error)
		}
		// Asked again the same day, it does nothing.
		before := srv.Puts
		if err := backup.Tick(m, context.Background()); err != nil {
			t.Fatal(err)
		}
		if srv.Puts != before {
			t.Fatalf("day %d: ran twice", day)
		}
	}

	var list struct{ Items []api.Backup }
	env.MustDo(http.MethodGet, "/backups", nil, &list)
	if len(list.Items) != 2 {
		t.Fatalf("kept: %+v", list.Items)
	}
	for _, b := range list.Items {
		if b.Location != api.S3 || b.Kind != api.BackupKindAuto {
			t.Fatalf("item: %+v", b)
		}
	}
	// Newest first: the ones of day 4 and day 3.
	if !list.Items[0].CreatedAt.After(list.Items[1].CreatedAt) {
		t.Fatalf("order: %+v", list.Items)
	}
	if !list.Items[1].CreatedAt.After(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day 2 was not the one removed: %+v", list.Items)
	}
	for key := range srv.Objects() {
		if !strings.HasPrefix(key, "site/backups/") {
			t.Fatalf("object outside the backups folder: %s", key)
		}
	}

	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if len(s.LastRuns) != 3 || !s.LastRuns[0].Ok {
		t.Fatalf("history: %+v", s.LastRuns)
	}

	// The package in S3 can be downloaded through the server and has the files.
	status, raw := rawGet(t, env, "/backups/"+list.Items[0].Id+"/download")
	if status != http.StatusOK {
		t.Fatalf("download from S3: %d", status)
	}
	if p := unpack(t, raw); p.files["notes/attachments/1"] != "picture" {
		t.Fatalf("S3 package: %v", p.files)
	}
}

func TestBackupToStorageS3AndFailureIsRecorded(t *testing.T) {
	env, m := setup(t)
	srv := fakes3.New(t, bucket)
	env.Elevate()
	// The storage settings have no S3 yet: a run is refused.
	if code, _ := env.Do(http.MethodPost, "/backups/run", nil, nil); code != http.StatusPreconditionFailed {
		t.Fatalf("run without S3: %d", code)
	}
	env.MustDo(http.MethodPut, "/storage/s3", s3Settings(srv), nil)
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"enabled": true, "target": "storage"}, nil)

	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	if job := waitJob(t, env); job.State != api.Done {
		t.Fatalf("run: %+v", *job.Error)
	}
	found := false
	for key := range srv.Objects() {
		if strings.HasPrefix(key, "site/backups/") && strings.HasSuffix(key, ".tar.gz") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no package in the bucket: %v", srv.Objects())
	}

	// A bucket that fails: the run ends as failed and the history says why.
	srv.Fail = func(method, key string) int {
		if method == http.MethodPut && strings.HasSuffix(key, ".tar.gz") {
			return http.StatusForbidden
		}
		return 0
	}
	backup.SetNow(m, func() time.Time { return time.Now().Add(time.Hour) })
	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	job := waitJob(t, env)
	if job.State != api.Failed || job.Error == nil {
		t.Fatalf("failing run: %+v", job)
	}
	var s api.BackupSettings
	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if len(s.LastRuns) != 2 || s.LastRuns[0].Ok || s.LastRuns[0].Error == nil || !s.LastRuns[1].Ok {
		t.Fatalf("history: %+v", s.LastRuns)
	}
}

func TestOnlyOneJobAtATime(t *testing.T) {
	env, _ := setup(t)
	env.Elevate()
	// Hold the first export by making the folder unusable is fragile, so start
	// two right after each other: the second is either refused or runs after
	// the first has ended.
	env.MustDo(http.MethodPost, "/backups/export", nil, nil)
	code, _ := env.Do(http.MethodPost, "/backups/export", nil, nil)
	if code != http.StatusAccepted && code != http.StatusConflict {
		t.Fatalf("second export: %d", code)
	}
	waitJob(t, env)
}

func TestLocalKeepsFiveManualBackups(t *testing.T) {
	env, m := setup(t)
	env.Elevate()
	clock := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	backup.SetNow(m, func() time.Time { return clock })
	for i := 0; i < 7; i++ {
		clock = clock.Add(time.Hour)
		env.MustDo(http.MethodPost, "/backups/export", nil, nil)
		if job := waitJob(t, env); job.State != api.Done {
			t.Fatalf("export %d: %+v", i, job)
		}
	}
	var list struct{ Items []api.Backup }
	env.MustDo(http.MethodGet, "/backups", nil, &list)
	if len(list.Items) != 5 {
		t.Fatalf("kept %d", len(list.Items))
	}
	if !list.Items[4].CreatedAt.Equal(time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("oldest kept: %v", list.Items[4].CreatedAt)
	}
}
