package drive_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestDriveVersionKeepsOldBlobUntilPermanentDelete(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "历史.txt", "old", false)
	digest := sha256.Sum256([]byte("old"))
	hash := hex.EncodeToString(digest[:])
	resp, body := saveContent(t, env, file.Id, "new", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", resp.StatusCode, body)
	}
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_file_versions WHERE item_id=? AND size=3 AND sha256=?", file.Id, hash).Scan(&count); err != nil || count != 1 {
		t.Fatalf("version row: %d %v", count, err)
	}
	store := env.App.Deps.Files.For("drive")
	if _, err := store.Stat(context.Background(), "blobs/"+hash[:2]+"/"+hash); err != nil {
		t.Fatalf("old blob removed: %v", err)
	}
	resp, body = saveContent(t, env, file.Id, "newer", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second save: %d %s", resp.StatusCode, body)
	}
	env.Elevate()
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(file.Id)+"?permanent=true", nil, nil)
	if _, err := store.Stat(context.Background(), "blobs/"+hash[:2]+"/"+hash); err != files.ErrNotFound {
		t.Fatalf("old blob after permanent delete: %v", err)
	}
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_file_versions WHERE item_id=?", file.Id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("version rows after permanent delete: %d %v", count, err)
	}
}

func TestDriveActionWriteTextCreatesVersion(t *testing.T) {
	env := testutil.New(t, drive.New)
	ctx := context.Background()
	input := json.RawMessage(`{"name":"action.txt","text":"one"}`)
	if _, err := env.App.Deps.Actions.Run(ctx, "drive.write_text", input); err != nil {
		t.Fatal(err)
	}
	input = json.RawMessage(`{"name":"action.txt","text":"two"}`)
	if _, err := env.App.Deps.Actions.Run(ctx, "drive.write_text", input); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_file_versions v JOIN drive_items i ON i.id=v.item_id WHERE i.name='action.txt'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("action version count: %d %v", count, err)
	}
}

func TestUploadKeepsSharedContentWhenOldCopyIsDeleted(t *testing.T) {
	env := testutil.New(t, drive.New)
	old := upload(t, env, "旧.txt", "same content", false)
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(old.Id), nil, nil)
	env.Elevate()
	// Hold a second upload of the same content after it found the stored
	// content and before it inserted its row.
	paused, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	restore := drive.SetAfterBlobPutForTest(func() {
		once.Do(func() {
			close(paused)
			<-resume
		})
	})
	defer restore()
	uploaded := make(chan api.DriveItem)
	go func() { uploaded <- upload(t, env, "新.txt", "same content", false) }()
	<-paused
	deleted := make(chan int)
	go func() {
		status, _ := env.Do(http.MethodDelete, "/drive/items/"+itoa(old.Id)+"?permanent=true", nil, nil)
		deleted <- status
	}()
	time.Sleep(100 * time.Millisecond) // the delete counts references now, or waits for the upload
	close(resume)
	item := <-uploaded
	if status := <-deleted; status != http.StatusNoContent {
		t.Fatalf("permanent delete: %d", status)
	}
	status, body := env.Do(http.MethodGet, "/drive/items/"+itoa(item.Id)+"/content", nil, nil)
	if status != http.StatusOK || string(body) != "same content" {
		t.Fatalf("new upload lost its content: %d %q", status, body)
	}
}
