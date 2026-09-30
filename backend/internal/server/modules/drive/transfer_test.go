package drive_test

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func waitTransfer(t *testing.T, env *testutil.Env, id string) api.DriveTask {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var out struct {
			Items []api.DriveTask `json:"items"`
		}
		env.MustDo(http.MethodGet, "/drive/tasks", nil, &out)
		for _, task := range out.Items {
			if task.Id == id && task.FinishedAt != nil {
				if task.State != api.DriveTaskStateDone {
					t.Fatalf("transfer task: %+v", task)
				}
				return task
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("transfer task timed out")
	return api.DriveTask{}
}

func transferItems(t *testing.T, env *testutil.Env, kind string, ids []int64, target int64, conflict string) api.DriveTask {
	t.Helper()
	var task api.DriveTask
	status, raw := env.Do(http.MethodPost, "/drive/batch/"+kind, map[string]any{"ids": ids, "targetId": target, "conflict": conflict}, &task)
	if status != http.StatusAccepted {
		t.Fatalf("start transfer: %d %s", status, raw)
	}
	return waitTransfer(t, env, task.Id)
}

func listTransfer(t *testing.T, env *testutil.Env, parent int64) []api.DriveItem {
	t.Helper()
	var out struct {
		Items []api.DriveItem `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/items?parent="+strconv.FormatInt(parent, 10), nil, &out)
	return out.Items
}

func TestDriveTransferCopyMoveAndConflicts(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "记录.txt", "five!", false)
	same := transferItems(t, env, "copy", []int64{file.Id}, 0, "skip")
	if same.Skipped == nil || *same.Skipped != 1 {
		t.Fatalf("copy to same folder with skip: %+v", same)
	}
	var source, target api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "来源"}, &source)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "目标"}, &target)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(file.Id, 10), map[string]any{"parentId": source.Id}, nil)

	got := transferItems(t, env, "copy", []int64{source.Id, file.Id}, target.Id, "rename")
	if got.DoneItems != 2 || got.DoneBytes != 5 || got.TotalItems != 2 || got.TargetId == nil || *got.TargetId != target.Id {
		t.Fatalf("copy progress: %+v", got)
	}
	children := listTransfer(t, env, target.Id)
	if len(children) != 1 || children[0].Name != "来源" {
		t.Fatalf("copied folder: %+v", children)
	}
	copiedFile := listTransfer(t, env, children[0].Id)
	if len(copiedFile) != 1 || copiedFile[0].Name != file.Name || copiedFile[0].Id == file.Id {
		t.Fatalf("copied content: %+v", copiedFile)
	}
	got = transferItems(t, env, "copy", []int64{source.Id}, target.Id, "skip")
	if got.Skipped == nil || *got.Skipped != 1 || len(listTransfer(t, env, target.Id)) != 1 {
		t.Fatalf("skip conflict: %+v", got)
	}
	got = transferItems(t, env, "copy", []int64{source.Id}, target.Id, "rename")
	if got.DoneItems != 2 || len(listTransfer(t, env, target.Id)) != 2 {
		t.Fatalf("rename conflict: %+v", got)
	}
	got = transferItems(t, env, "copy", []int64{source.Id}, target.Id, "overwrite")
	if got.DoneItems != 2 || len(listTransfer(t, env, target.Id)) != 2 {
		t.Fatalf("overwrite conflict: %+v", got)
	}
	var trashed int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_items WHERE id=? AND trashed_at IS NOT NULL", children[0].Id).Scan(&trashed); err != nil || trashed != 1 {
		t.Fatalf("overwritten folder should enter trash: %d, %v", trashed, err)
	}
	transferItems(t, env, "move", []int64{file.Id}, target.Id, "rename")
	if len(listTransfer(t, env, source.Id)) != 0 || len(listTransfer(t, env, target.Id)) != 3 {
		t.Fatalf("move to target: %+v", listTransfer(t, env, target.Id))
	}
}

func TestDriveTransferCopyAcrossTransactionBatches(t *testing.T) {
	env := testutil.New(t, drive.New)
	var source, target api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "批量来源"}, &source)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "批量目标"}, &target)
	for i := 0; i < 205; i++ {
		_, err := env.App.Deps.DB.Exec("INSERT INTO drive_items(parent_id,name,is_dir,size,mime,sha256,hidden,created_at,updated_at) VALUES(?,?,1,0,'','',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)", source.Id, fmt.Sprintf("子目录-%03d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	got := transferItems(t, env, "copy", []int64{source.Id}, target.Id, "rename")
	if got.DoneItems != 206 || got.TotalItems != 206 {
		t.Fatalf("batch progress: %+v", got)
	}
	newSource := listTransfer(t, env, target.Id)
	if len(newSource) != 1 || len(listTransfer(t, env, newSource[0].Id)) != 205 {
		t.Fatalf("copied subtree: %+v", newSource)
	}
}

func TestDriveTransferRejectsInvalidTargetAndHiddenMismatch(t *testing.T) {
	env := testutil.New(t, drive.New)
	var folder, child api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "上级"}, &folder)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "下级", "parentId": folder.Id}, &child)
	for _, kind := range []string{"copy", "move"} {
		status, _ := env.Do(http.MethodPost, "/drive/batch/"+kind, map[string]any{"ids": []int64{folder.Id}, "targetId": child.Id, "conflict": "rename"}, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("%s descendant target: %d", kind, status)
		}
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	hidden := upload(t, env, "秘密.txt", "secret", true)
	status, _ := env.Do(http.MethodPost, "/drive/batch/copy", map[string]any{"ids": []int64{hidden.Id}, "targetId": folder.Id, "conflict": "rename"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("hidden mismatch: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ = env.Do(http.MethodPost, "/drive/batch/copy", map[string]any{"ids": []int64{hidden.Id}, "targetId": 0, "conflict": "rename"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked hidden source: %d", status)
	}
}
