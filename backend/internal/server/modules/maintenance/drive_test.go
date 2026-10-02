package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDriveOrphanRechecksVersionsAndUploads(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	for _, query := range []string{"CREATE TABLE drive_items(id INTEGER PRIMARY KEY,parent_id INTEGER,name TEXT,is_dir INTEGER,sha256 TEXT,hidden INTEGER,size INTEGER,trashed_at DATETIME,s3_key TEXT)", "CREATE TABLE drive_file_versions(id INTEGER PRIMARY KEY,item_id INTEGER,sha256 TEXT,size INTEGER)", "CREATE TABLE drive_shares(id INTEGER PRIMARY KEY,item_id INTEGER,expires_at DATETIME,max_downloads INTEGER,downloads INTEGER,token TEXT)", "CREATE TABLE drive_s3_deletions(key TEXT UNIQUE,created_at DATETIME)"} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	d := testDeps(db, t.TempDir())
	var mu sync.Mutex
	c := DriveCleaner{Deps: d, Lock: func(string) func() { mu.Lock(); return mu.Unlock }}
	hash := strings.Repeat("a", 64)
	key := driveKey(hash)
	if err := d.Files.For("drive").Put(ctx, key, strings.NewReader("abc"), 3); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d.Config.FilesDir(), "drive", filepath.FromSlash(key))
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("scan %+v %v", items, err)
	}
	if _, err = db.Exec("INSERT INTO drive_file_versions VALUES(1,1,?,3)", hash); err != nil {
		t.Fatal(err)
	}
	result, err := c.Clean(ctx, []string{items[0].ID})
	if err != nil || result.Skipped != 1 {
		t.Fatalf("version protection %+v %v", result, err)
	}
	if _, err = db.Exec("DELETE FROM drive_file_versions"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	done := make(chan error, 1)
	go func() { _, e := c.Clean(ctx, []string{items[0].ID}); done <- e }()
	if _, err = db.Exec("INSERT INTO drive_items VALUES(1,NULL,'new',0,?,1,3,NULL,NULL)", hash); err != nil {
		t.Fatal(err)
	}
	mu.Unlock()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = d.Files.For("drive").Stat(ctx, key); err != nil {
		t.Fatal("concurrent upload deleted")
	}
}
