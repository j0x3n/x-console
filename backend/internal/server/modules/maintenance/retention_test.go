package maintenance

import (
	"context"
	"testing"
	"time"
)

func TestRetentionRechecksFinishedRunsAndChangedShares(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	for _, query := range []string{"CREATE TABLE script_runs(id INTEGER PRIMARY KEY,finished_at DATETIME)", "CREATE TABLE ai_usage(id INTEGER PRIMARY KEY,created_at DATETIME)", "CREATE TABLE audit_log(id INTEGER PRIMARY KEY,at DATETIME)", "CREATE TABLE note_shares(note_id INTEGER PRIMARY KEY,expires_at DATETIME,token TEXT DEFAULT 'original')"} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-100 * 24 * time.Hour)
	for _, query := range []string{"INSERT INTO script_runs VALUES(1,?)", "INSERT INTO ai_usage VALUES(1,?)", "INSERT INTO audit_log VALUES(1,?)", "INSERT INTO note_shares(note_id,expires_at) VALUES(1,?)"} {
		if _, err := db.Exec(query, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO notes VALUES(1,'',0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO script_runs VALUES(2,NULL)"); err != nil {
		t.Fatal(err)
	}
	c := recordCleaner{db}
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 4 {
		t.Fatalf("scan %+v %v", items, err)
	}
	if _, err = db.Exec("UPDATE script_runs SET finished_at=? WHERE id=1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE note_shares SET expires_at=? WHERE note_id=1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	result, err := c.Clean(ctx, ids)
	if err != nil || result.Deleted != 2 || result.Skipped != 2 || result.Bytes != 0 {
		t.Fatalf("retention %+v %v", result, err)
	}
}
