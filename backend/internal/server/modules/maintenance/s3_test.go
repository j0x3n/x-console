package maintenance

import (
	"context"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakes3"
)

func TestAttachmentCleanupDeletesS3Object(t *testing.T) {
	ctx := context.Background()
	db := cleanerDB(t)
	fake := fakes3.New(t, "bucket")
	remote, err := files.NewS3(files.S3Config{Endpoint: fake.URL, Region: "us-east-1", Bucket: "bucket", AccessKeyID: "key", SecretAccessKey: "secret", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	d := testDeps(db, t.TempDir())
	d.Files.Swap(remote)
	c := AttachmentCleaner{Deps: d, Notes: true}
	if _, err = db.Exec("INSERT INTO notes VALUES(1,'',0)"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO note_attachments VALUES(1,1,'a',3,'hash',?)", time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	fake.Set("notes/attachments/1", []byte("abc"))
	items, err := c.Scan(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("scan %+v %v", items, err)
	}
	result, err := c.Clean(ctx, []string{items[0].ID})
	if err != nil || result.Deleted != 1 || result.Bytes != 3 {
		t.Fatalf("clean %+v %v", result, err)
	}
	if len(fake.Objects()) != 0 {
		t.Fatalf("S3 object retained: %v", fake.Objects())
	}
}
