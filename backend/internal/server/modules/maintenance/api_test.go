package maintenance_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/maintenance"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestMaintenanceMainFlow(t *testing.T) {
	env := testutil.New(t, maintenance.New)
	var overview map[string]any
	env.MustDo(http.MethodGet, "/maintenance/overview?refresh=true", nil, &overview)
	for _, key := range []string{"version", "builtAt", "startedAt", "goVersion", "dataDir", "process", "machine", "storage"} {
		if _, ok := overview[key]; !ok {
			t.Fatalf("missing overview field %s", key)
		}
	}
	status, _ := env.Do(http.MethodPost, "/maintenance/cleanup", map[string]any{"kinds": []string{"audit_logs"}}, nil)
	if status != 403 {
		t.Fatalf("cleanup without elevation: %d", status)
	}
	tmpPath := filepath.Join(env.App.Deps.Config.TmpDir(), "old-e2e.tmp")
	if err := os.WriteFile(tmpPath, []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(tmpPath, old, old); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodPost, "/maintenance/scan", nil, nil)
	var scan maintenance.Job
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		env.MustDo(http.MethodGet, "/maintenance/scan", nil, &scan)
		if scan.State != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if scan.State != "done" {
		t.Fatalf("scan failed %+v", scan)
	}
	env.Elevate()
	status, _ = env.Do(http.MethodPost, "/maintenance/cleanup", map[string]any{"kinds": []string{"unknown"}}, nil)
	if status != 400 {
		t.Fatalf("unknown kind: %d", status)
	}
	env.MustDo(http.MethodPost, "/maintenance/cleanup", map[string]any{"kinds": []string{"temporary_files"}, "scanId": scan.ID}, nil)
	var cleanup maintenance.Job
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		env.MustDo(http.MethodGet, "/maintenance/cleanup", nil, &cleanup)
		if cleanup.State != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cleanup.State != "done" || cleanup.Result.Deleted != 1 || cleanup.Result.Bytes != 7 {
		t.Fatalf("cleanup %+v", cleanup)
	}
	var count int
	if err := env.App.Deps.DB.QueryRowContext(context.Background(), "SELECT count(*) FROM audit_log WHERE action='maintenance.cleanup'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit %d %v", count, err)
	}
	env.MustDo(http.MethodPost, "/maintenance/vacuum", nil, nil)
}
