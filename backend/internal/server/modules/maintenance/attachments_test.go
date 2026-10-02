package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	_ "modernc.org/sqlite"
)

func testDeps(db *sql.DB, dir string) *module.Deps {
	cfg := config.Config{DataDir: dir}
	return &module.Deps{DB: db, Config: cfg, Files: files.NewManager(files.Local{Root: cfg.FilesDir()}), Registry: module.NewRegistry()}
}

func cleanerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))+"?_txlock=immediate&_pragma=busy_timeout(5000)&_time_format=sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, query := range []string{
		"CREATE TABLE notes(id INTEGER PRIMARY KEY,body TEXT,hidden INTEGER)",
		"CREATE TABLE note_attachments(id INTEGER PRIMARY KEY,note_id INTEGER,name TEXT,size INTEGER,sha256 TEXT,created_at DATETIME)",
		"CREATE TABLE uploaded_files(id INTEGER PRIMARY KEY,scope TEXT,name TEXT,size INTEGER,sha256 TEXT,owner_kind TEXT,created_at DATETIME)",
		"CREATE TABLE projects(description TEXT)", "CREATE TABLE issues(description TEXT,cover_file_id INTEGER)", "CREATE TABLE issue_comments(body TEXT)", "CREATE TABLE calendar_events(description TEXT)", "CREATE TABLE reminders(body TEXT)", "CREATE TABLE coding_tasks(prompt TEXT)",
	} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestAttachmentCleanerProtectsHiddenAndNewReferences(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	store := files.Local{Root: t.TempDir()}
	d := &module.Deps{DB: db, Files: files.NewManager(store), Registry: module.NewRegistry()}
	c := AttachmentCleaner{Deps: d, Notes: true}
	if _, err := db.Exec("INSERT INTO notes VALUES(1,'/api/v1/notes/attachments/12',1),(2,'',0)"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{12, 123, 124} {
		if _, err := db.Exec("INSERT INTO note_attachments VALUES(?,2,'a',3,'hash',?)", id, time.Now().Add(-48*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := d.Files.For("notes").Put(ctx, c.key(id), strings.NewReader("abc"), 3); err != nil {
			t.Fatal(err)
		}
	}
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("scan %+v %v", items, err)
	}
	if _, err = db.Exec("UPDATE notes SET body='/api/v1/notes/attachments/123' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	result, err := c.Clean(ctx, ids)
	if err != nil || result.Deleted != 1 || result.Skipped != 1 {
		t.Fatalf("clean %+v %v", result, err)
	}
	for _, id := range []int64{12, 123} {
		if _, err = d.Files.For("notes").Stat(ctx, c.key(id)); err != nil {
			t.Fatalf("referenced attachment %d removed", id)
		}
	}
}

func TestPublicUploadChecksBusinessReferencesAndAge(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	d := &module.Deps{DB: db, Files: files.NewManager(files.Local{Root: t.TempDir()}), Registry: module.NewRegistry()}
	c := AttachmentCleaner{Deps: d}
	for _, id := range []int64{1, 2, 3} {
		at := time.Now().Add(-48 * time.Hour)
		if id == 3 {
			at = time.Now()
		}
		if _, err := db.Exec("INSERT INTO uploaded_files VALUES(?,'projects','a',3,'hash',NULL,?)", id, at); err != nil {
			t.Fatal(err)
		}
		if err := d.Files.For("projects").Put(ctx, c.key(id), strings.NewReader("abc"), 3); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO projects VALUES('/api/v1/files/1')"); err != nil {
		t.Fatal(err)
	}
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("scan %+v %v", items, err)
	}
	if _, err = db.Exec("INSERT INTO calendar_events VALUES('/api/v1/files/2')"); err != nil {
		t.Fatal(err)
	}
	result, err := c.Clean(ctx, []string{items[0].ID})
	if err != nil || result.Deleted != 0 || result.Skipped != 1 {
		t.Fatalf("cover removed %+v %v", result, err)
	}
}

type remoteAccess struct{ err error }

func (r remoteAccess) Stat(context.Context, string, string) (files.Info, error) {
	return files.Info{}, r.err
}
func (remoteAccess) Location() string { return "s3" }
func (remoteAccess) WithCleanup(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestRemoteFailureIsNotMissing(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	r := module.NewRegistry()
	d := &module.Deps{DB: db, Files: files.NewManager(files.Local{Root: t.TempDir()}), Registry: r}
	c := AttachmentCleaner{Deps: d, Notes: true}
	if _, err := db.Exec("INSERT INTO notes VALUES(1,'',0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO note_attachments VALUES(1,1,'a',3,'hash',?)", time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	module.Provide[contracts.MaintenanceStorage](r, contracts.MaintenanceStorageKey, remoteAccess{errors.New("remote timeout")})
	if items, err := c.Scan(ctx); err == nil || len(items) > 0 {
		t.Fatalf("remote error became missing: %+v %v", items, err)
	}
	module.Provide[contracts.MaintenanceStorage](r, contracts.MaintenanceStorageKey, remoteAccess{files.ErrNotFound})
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 1 || items[0].Kind != "missing_records" {
		t.Fatalf("missing %+v %v", items, err)
	}
	module.Provide[contracts.MaintenanceStorage](r, contracts.MaintenanceStorageKey, remoteAccess{errors.New("remote unavailable")})
	if _, err = c.Clean(ctx, []string{items[0].ID}); err == nil {
		t.Fatal("cleanup ignored remote failure")
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM note_attachments").Scan(&n); err != nil || n != 1 {
		t.Fatalf("row deleted %d %v", n, err)
	}
}
