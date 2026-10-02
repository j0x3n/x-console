package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArtifactCleanupRechecksFinishedReferences(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	for _, query := range []string{"ALTER TABLE coding_tasks ADD COLUMN id INTEGER", "ALTER TABLE coding_tasks ADD COLUMN artifacts TEXT DEFAULT '[]'", "ALTER TABLE coding_tasks ADD COLUMN build_status TEXT DEFAULT ''"} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	d := testDeps(db, t.TempDir())
	c := ArtifactCleaner{d}
	key := "artifacts/1/output"
	if err := d.Files.For("coding").Put(ctx, key, strings.NewReader("abc"), 3); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(d.Config.FilesDir(), "coding", filepath.FromSlash(key)), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO coding_tasks(id,artifacts,build_status) VALUES(1,'[]','running')"); err != nil {
		t.Fatal(err)
	}
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("running %+v %v", items, err)
	}
	if _, err = db.Exec("UPDATE coding_tasks SET build_status='passed'"); err != nil {
		t.Fatal(err)
	}
	items, err = c.Scan(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("orphan %+v %v", items, err)
	}
	if _, err = db.Exec(`UPDATE coding_tasks SET artifacts='[{"key":"artifacts/1/output"}]'`); err != nil {
		t.Fatal(err)
	}
	result, err := c.Clean(ctx, []string{items[0].ID})
	if err != nil || result.Skipped != 1 {
		t.Fatalf("reference %+v %v", result, err)
	}
	if _, err = db.Exec("UPDATE coding_tasks SET artifacts='[]'"); err != nil {
		t.Fatal(err)
	}
	result, err = c.Clean(ctx, []string{items[0].ID})
	if err != nil || result.Deleted != 1 || result.Bytes != 3 {
		t.Fatalf("cleanup %+v %v", result, err)
	}
}
