package maintenance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type fakeCleaner struct {
	items   []contracts.CleanupItem
	cleaned []string
}

func (f *fakeCleaner) Scan(context.Context) ([]contracts.CleanupItem, error) { return f.items, nil }
func (f *fakeCleaner) Clean(_ context.Context, ids []string) (contracts.CleanupResult, error) {
	f.cleaned = append(f.cleaned, ids...)
	return contracts.CleanupResult{Deleted: int64(len(ids))}, nil
}

func TestScanKeepsCompleteSnapshot(t *testing.T) {
	r := module.NewRegistry()
	f := &fakeCleaner{}
	for i := range 75 {
		f.items = append(f.items, contracts.CleanupItem{ID: strings.Repeat("x", i+1), Kind: "orphan_files", Bytes: 2})
	}
	f.items = append(f.items, contracts.CleanupItem{ID: "hidden", Kind: "orphan_files", Hidden: true}, contracts.CleanupItem{ID: "missing", Kind: "missing_records"}, contracts.CleanupItem{ID: "audit", Kind: "audit_logs"})
	module.Provide[contracts.Cleaner](r, contracts.MaintenanceCleanerPrefix+"fake", f)
	m := &Module{d: &module.Deps{Registry: r}, scan: Job{State: "running"}, snapshot: map[string][]contracts.CleanupItem{}, consumed: map[string]bool{}}
	m.runScan(context.Background())
	out := m.job(context.Background(), false)
	if m.scan.State != "done" || len(m.snapshot[contracts.MaintenanceCleanerPrefix+"fake"]) != 78 {
		t.Fatalf("snapshot lost candidates: %+v", m.scan)
	}
	for _, g := range out.Groups {
		if g.Kind == "orphan_files" && (g.Count != 75 || g.Bytes != 150 || len(g.Items) != 50) {
			t.Fatalf("preview: %+v", g)
		}
		if (g.Kind == "audit_logs" || g.Kind == "missing_records") && g.Selected {
			t.Fatalf("unsafe default: %+v", g)
		}
	}
	m.cleanup = Job{State: "running"}
	ids := []string{}
	for _, item := range m.snapshot[contracts.MaintenanceCleanerPrefix+"fake"] {
		if !item.Hidden {
			ids = append(ids, item.ID)
		}
	}
	m.runCleanup(context.Background(), map[string][]string{contracts.MaintenanceCleanerPrefix + "fake": ids})
	if len(f.cleaned) != 77 || m.cleanup.Result.Deleted != 77 {
		t.Fatalf("cleanup truncated: %+v", m.cleanup)
	}
}

func TestCleanupRequiresElevation(t *testing.T) {
	m := &Module{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/maintenance/cleanup", strings.NewReader(`{"kinds":["audit_logs"]}`))
	m.cleanupHandler(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

func TestTemporaryRechecksActiveAndFreshFiles(t *testing.T) {
	dir := t.TempDir()
	r := module.NewRegistry()
	c := temporaryCleaner{dir, r}
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{"old", "active", "fresh"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	items, err := c.Scan(context.Background())
	if err != nil || len(items) != 3 {
		t.Fatalf("scan %v %v", items, err)
	}
	release := contracts.TrackTemporaryFile(filepath.Join(dir, "active"))
	defer release()
	if err = os.Chtimes(filepath.Join(dir, "fresh"), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	result, err := c.Clean(context.Background(), ids)
	if err != nil || result.Deleted != 1 || result.Bytes != 3 || result.Skipped != 2 {
		t.Fatalf("result %+v %v", result, err)
	}
	if _, err = c.Clean(context.Background(), []string{candidateID(temporaryCandidate{Path: "../escape"})}); err == nil {
		t.Fatal("accepted traversal")
	}
}
