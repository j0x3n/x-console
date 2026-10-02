package maintenance

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTrashSnapshotAndFreshness(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	for _, query := range []string{"CREATE TABLE drive_items(id INTEGER PRIMARY KEY,parent_id INTEGER,sha256 TEXT,is_dir INTEGER,hidden INTEGER,trashed_at DATETIME,s3_key TEXT)", "CREATE TABLE drive_file_versions(id INTEGER PRIMARY KEY,item_id INTEGER,sha256 TEXT)", "CREATE TABLE drive_s3_deletions(key TEXT UNIQUE,created_at DATETIME)"} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	d := testDeps(db, t.TempDir())
	old := time.Now().Add(-40 * 24 * time.Hour)
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	hash := strings.Repeat("a", 64)
	if _, err := db.Exec("INSERT INTO drive_items VALUES(1,NULL,'',1,0,?,NULL),(2,1,?,0,0,?,'legacy')", old, hash, old); err != nil {
		t.Fatal(err)
	}
	if err := d.Files.For("drive").Put(ctx, driveKey(hash), strings.NewReader("abc"), 3); err != nil {
		t.Fatal(err)
	}
	lock := func(string) func() { return func() {} }
	if _, err := db.Exec("INSERT INTO drive_items VALUES(3,1,'',1,0,?,NULL)", old); err != nil {
		t.Fatal(err)
	}
	result, err := PurgeDriveTrash(ctx, d, 1, cutoff, []int64{1, 2}, lock)
	if err != nil || result.Skipped != 1 {
		t.Fatalf("new subtree %+v %v", result, err)
	}
	if _, err = db.Exec("DELETE FROM drive_items WHERE id=3"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE drive_items SET trashed_at=NULL WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	result, err = PurgeDriveTrash(ctx, d, 1, cutoff, []int64{1, 2}, lock)
	if err != nil || result.Skipped != 1 {
		t.Fatalf("restored descendant %+v %v", result, err)
	}
	if _, err = db.Exec("UPDATE drive_items SET trashed_at=? WHERE id=2", old); err != nil {
		t.Fatal(err)
	}
	result, err = PurgeDriveTrash(ctx, d, 1, cutoff, []int64{1, 2}, lock)
	if err != nil || result.Deleted != 2 || result.Bytes != 3 {
		t.Fatalf("purge %+v %v", result, err)
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM drive_s3_deletions WHERE key='legacy'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("legacy queue %d %v", n, err)
	}
}
