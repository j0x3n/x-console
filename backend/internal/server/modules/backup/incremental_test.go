package backup_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakes3"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func TestIncrementalSettingsSnapshotsChangesAndCheck(t *testing.T) {
	env, m := setup(t)
	srv := fakes3.New(t, bucket)
	env.Elevate()
	var defaults map[string]any
	env.MustDo(http.MethodGet, "/backups/settings", nil, &defaults)
	if defaults["mode"] != "incremental" {
		t.Fatal(defaults)
	}
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"target": "custom", "s3": s3Settings(srv), "mode": "incremental", "retention": map[string]int{"last": 7, "daily": 14, "weekly": 8, "monthly": 12}}, nil)
	putFile(t, env, "projects/a", "one")
	putFile(t, env, "notes/private", "hidden")
	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	if j := waitJob(t, env); j.State != api.Done {
		t.Fatalf("backup: %+v", j)
	}
	var list struct {
		Items []struct {
			ID            string `json:"id"`
			UploadedBytes int64  `json:"uploadedBytes"`
		}
		Stats repo.Stats
	}
	env.MustDo(http.MethodGet, "/backups/snapshots", nil, &list)
	if len(list.Items) != 1 || list.Stats.Snapshots != 1 {
		t.Fatal(list)
	}
	first := list.Items[0].ID
	before := map[string]int{}
	for key := range srv.Objects() {
		if strings.Contains(key, "/blobs/") {
			before[key] = 1
		}
	}
	putFile(t, env, "projects/a", "two")
	if err := env.App.Deps.Files.Store().Delete(context.Background(), "notes/private"); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	if j := waitJob(t, env); j.State != api.Done {
		t.Fatalf("backup: %+v", j)
	}
	env.MustDo(http.MethodGet, "/backups/snapshots", nil, &list)
	if len(list.Items) != 2 || list.Items[0].ID == first {
		t.Fatal(list)
	}
	var changes struct {
		Items     []repo.Change
		Truncated bool
	}
	env.MustDo(http.MethodGet, "/backups/snapshots/"+list.Items[0].ID+"/changes", nil, &changes)
	for _, c := range changes.Items {
		if strings.HasPrefix(c.Path, "notes/") {
			t.Fatal("hidden path leaked")
		}
	}
	if len(changes.Items) != 1 || changes.Items[0].Kind != "modified" {
		t.Fatal(changes)
	}
	env.MustDo(http.MethodPost, "/backups/check", nil, nil)
	if j := waitJob(t, env); j.State != api.Done {
		t.Fatalf("check: %+v", j)
	}
	for key := range srv.Objects() {
		if !strings.HasPrefix(key, "site/backups/x-console-repo/") {
			t.Fatal(key)
		}
	}
	var stops atomic.Int32
	backup.SetExit(m, func() { stops.Add(1) })
	env.MustDo(http.MethodPost, "/backups/snapshots/"+first+"/restore", map[string]string{"confirm": "恢复"}, nil)
	deadline := time.Now().Add(15 * time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if stops.Load() != 1 || !backup.Pending(env.App.Deps.Config.RestoreDir()) {
		t.Fatal("snapshot not staged")
	}
	if got := readStore(t, env, "projects/a"); got != "two" {
		t.Fatal("changed before restart", got)
	}
}

func TestOldBackupSettingsStayFull(t *testing.T) {
	env, _ := setup(t)
	ctx := context.Background()
	if err := env.App.Deps.Settings.Set(ctx, "backup.settings", map[string]any{"enabled": false, "frequency": "daily", "time": "03:00", "weekday": 0, "keep": 14, "target": "storage"}); err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	env.MustDo(http.MethodGet, "/backups/settings", nil, &s)
	if s["mode"] != "full" {
		t.Fatal(s)
	}
}

func TestIncrementalRepoNeverBacksUpItselfAndMissingIsExplicit(t *testing.T) {
	env, _ := setup(t)
	srv := fakes3.New(t, bucket)
	env.Elevate()
	env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"target": "custom", "s3": s3Settings(srv), "mode": "incremental"}, nil)
	if code, _ := env.Do(http.MethodPost, "/backups/check", nil, nil); code != http.StatusAccepted {
		t.Fatal(code)
	}
	if j := waitJob(t, env); j.State != api.Failed {
		t.Fatal(j)
	}
	putFile(t, env, "backups/do-not-include", "archive")
	env.MustDo(http.MethodPost, "/backups/run", nil, nil)
	if j := waitJob(t, env); j.State != api.Done {
		t.Fatal(j)
	}
	s3, err := files.NewS3(files.S3Config{Endpoint: srv.URL, Region: "us-east-1", Bucket: bucket, Prefix: "site", AccessKeyID: "AK", SecretAccessKey: "SK", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	r, err := repo.Open(context.Background(), files.Scoped(files.Scoped(s3, "backups"), "x-console-repo"), env.App.Deps.Config.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	all, err := r.ListSnapshots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all[0].Files {
		if files.Module(f.Path) == "backups" {
			t.Fatal("repo included")
		}
	}
	if _, err := r.ReadSnapshot(context.Background(), "bad"); !errors.Is(err, files.ErrBadKey) {
		t.Fatal(err)
	}
}
