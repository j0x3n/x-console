package drive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"golang.org/x/text/encoding/simplifiedchinese"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func makeExtractArchive(t *testing.T, kind string) []byte {
	t.Helper()
	var out bytes.Buffer
	if kind == "zip" {
		zw := zip.NewWriter(&out)
		for _, file := range []struct{ name, content string }{{"nested/子文件.txt", "hello"}, {"空目录/", ""}} {
			w, err := zw.Create(file.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, file.content); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	var writer io.Writer = &out
	var gz *gzip.Writer
	if kind == "tar.gz" || kind == "tgz" {
		gz = gzip.NewWriter(&out)
		writer = gz
	}
	tw := tar.NewWriter(writer)
	for _, file := range []struct {
		name, content string
		dir           bool
	}{{"nested/子文件.txt", "hello", false}, {"空目录/", "", true}} {
		header := &tar.Header{Name: file.name, Mode: 0o644, Size: int64(len(file.content)), Typeflag: tar.TypeReg}
		if file.dir {
			header.Typeflag = tar.TypeDir
			header.Mode = 0o755
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if !file.dir {
			if _, err := io.WriteString(tw, file.content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if gz != nil {
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

func extractTask(t *testing.T, env *testutil.Env, id int64, body any) api.DriveTask {
	t.Helper()
	var task api.DriveTask
	status, raw := env.Do(http.MethodPost, "/drive/items/"+strconv.FormatInt(id, 10)+"/extract", body, &task)
	if status != http.StatusAccepted {
		t.Fatalf("extract start: %d %s", status, raw)
	}
	return waitTransfer(t, env, task.Id)
}

func waitFailedExtract(t *testing.T, env *testutil.Env, id string, code string) api.DriveTask {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var out struct {
			Items []api.DriveTask `json:"items"`
		}
		env.MustDo(http.MethodGet, "/drive/tasks", nil, &out)
		for _, task := range out.Items {
			if task.Id == id && task.FinishedAt != nil {
				if task.State != api.DriveTaskStateFailed || task.ErrorCode == nil || *task.ErrorCode != code {
					t.Fatalf("extract failure: %+v", task)
				}
				return task
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("extract failure timed out")
	return api.DriveTask{}
}

func TestDriveExtractFormatsAndImplicitDirectories(t *testing.T) {
	for _, kind := range []string{"zip", "tar", "tar.gz", "tgz"} {
		t.Run(kind, func(t *testing.T) {
			env := testutil.New(t, drive.New)
			archive := upload(t, env, "资料."+kind, string(makeExtractArchive(t, kind)), false)
			task := extractTask(t, env, archive.Id, nil)
			if task.ResultId == nil || task.TargetId == nil || *task.TargetId != *task.ResultId || task.DoneBytes != 5 || task.DoneItems != 2 {
				t.Fatalf("extract progress: %+v", task)
			}
			root := listTransfer(t, env, *task.ResultId)
			if len(root) != 2 || root[0].IsDir == false || root[1].IsDir == false {
				t.Fatalf("extracted folders: %+v", root)
			}
			for _, folder := range root {
				if folder.Name != "nested" {
					continue
				}
				children := listTransfer(t, env, folder.Id)
				if len(children) != 1 || children[0].Name != "子文件.txt" || string(archiveBytes(t, env, children[0].Id)) != "hello" {
					t.Fatalf("extracted content: %+v", children)
				}
			}
		})
	}
}

func TestDriveExtractConflictsAndUnsupportedFormat(t *testing.T) {
	env := testutil.New(t, drive.New)
	var target api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "目标"}, &target)
	existing := upload(t, env, "same.txt", "old", false)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(existing.Id, 10), map[string]any{"parentId": target.Id}, nil)
	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	w, err := zw.Create("same.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "new"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := upload(t, env, "同名.zip", raw.String(), false)
	task := extractTask(t, env, archive.Id, map[string]any{"targetId": target.Id, "conflict": "skip"})
	if task.Skipped == nil || *task.Skipped != 1 || len(listTransfer(t, env, target.Id)) != 1 {
		t.Fatalf("skip conflict: %+v", task)
	}
	task = extractTask(t, env, archive.Id, map[string]any{"targetId": target.Id, "conflict": "rename"})
	if task.DoneItems != 1 || len(listTransfer(t, env, target.Id)) != 2 {
		t.Fatalf("rename conflict: %+v", task)
	}
	task = extractTask(t, env, archive.Id, map[string]any{"targetId": target.Id, "conflict": "overwrite"})
	var oldTrashed int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_items WHERE id=? AND trashed_at IS NOT NULL", existing.Id).Scan(&oldTrashed); err != nil || oldTrashed != 1 {
		t.Fatalf("overwritten file should enter trash: %d %v", oldTrashed, err)
	}
	if task.State != api.DriveTaskStateDone {
		t.Fatalf("overwrite task: %+v", task)
	}
	unsupported := upload(t, env, "不支持.rar", "rar", false)
	status, _ := env.Do(http.MethodPost, "/drive/items/"+strconv.FormatInt(unsupported.Id, 10)+"/extract", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unsupported extension: %d", status)
	}
	wrongMagic := upload(t, env, "错误.zip", "not a zip", false)
	status, _ = env.Do(http.MethodPost, "/drive/items/"+strconv.FormatInt(wrongMagic.Id, 10)+"/extract", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unsupported magic: %d", status)
	}
}

func TestDriveExtractRejectsTraversalBeforeWritingAndCleansPartialFailure(t *testing.T) {
	env := testutil.New(t, drive.New)
	var unsafe bytes.Buffer
	zw := zip.NewWriter(&unsafe)
	for _, name := range []string{"good.txt", "../escape.txt"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, "data"); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := upload(t, env, "危险.zip", unsafe.String(), false)
	var task api.DriveTask
	env.MustDo(http.MethodPost, "/drive/items/"+strconv.FormatInt(archive.Id, 10)+"/extract", nil, &task)
	waitFailedExtract(t, env, task.Id, "archive_unsafe_path")
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_items WHERE name='危险'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unsafe archive wrote rows: %d %v", count, err)
	}
	var partial bytes.Buffer
	zw = zip.NewWriter(&partial)
	for _, file := range []struct{ name, value string }{{"first.txt", "first"}, {"bad.txt", "BROKEN_CONTENT"}} {
		header := &zip.FileHeader{Name: file.name, Method: zip.Store}
		w, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, file.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Replace(partial.Bytes(), []byte("BROKEN_CONTENT"), []byte("XROKEN_CONTENT"), 1)
	archive = upload(t, env, "半成品.zip", string(corrupt), false)
	env.MustDo(http.MethodPost, "/drive/items/"+strconv.FormatInt(archive.Id, 10)+"/extract", nil, &task)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var out struct {
			Items []api.DriveTask `json:"items"`
		}
		env.MustDo(http.MethodGet, "/drive/tasks", nil, &out)
		for _, current := range out.Items {
			if current.Id == task.Id && current.FinishedAt != nil {
				if current.State != api.DriveTaskStateFailed {
					t.Fatalf("corrupt archive: %+v", current)
				}
				if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_items WHERE name IN ('半成品','first.txt')").Scan(&count); err != nil || count != 0 {
					t.Fatalf("partial extraction remained: %d %v", count, err)
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("partial failure timed out")
}

func TestDriveExtractSkipsLinks(t *testing.T) {
	env := testutil.New(t, drive.New)
	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	header := &zip.FileHeader{Name: "link", Method: zip.Store}
	header.SetMode(os.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "elsewhere"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := upload(t, env, "链接.zip", raw.String(), false)
	task := extractTask(t, env, archive.Id, nil)
	if task.Skipped == nil || *task.Skipped != 1 || task.ResultId == nil || len(listTransfer(t, env, *task.ResultId)) != 0 {
		t.Fatalf("skipped link: %+v", task)
	}
}

func TestDriveExtractGBKZipName(t *testing.T) {
	env := testutil.New(t, drive.New)
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("中文.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	header := &zip.FileHeader{Name: string(encoded), Method: zip.Store, NonUTF8: true}
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "gbk"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := upload(t, env, "旧文件.zip", raw.String(), false)
	task := extractTask(t, env, archive.Id, nil)
	if task.ResultId == nil {
		t.Fatalf("missing result folder: %+v", task)
	}
	items := listTransfer(t, env, *task.ResultId)
	if len(items) != 1 || items[0].Name != "中文.txt" || string(archiveBytes(t, env, items[0].Id)) != "gbk" {
		t.Fatalf("GBK filename: %+v", items)
	}
}

func TestDriveExtractHiddenArchiveStaysHidden(t *testing.T) {
	env := testutil.New(t, drive.New)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	archive := upload(t, env, "秘密.zip", string(makeExtractArchive(t, "zip")), true)
	task := extractTask(t, env, archive.Id, nil)
	if task.ResultId == nil {
		t.Fatalf("missing hidden folder: %+v", task)
	}
	var hidden int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_items WHERE id=? AND hidden=1", *task.ResultId).Scan(&hidden); err != nil || hidden != 1 {
		t.Fatalf("folder hidden flag: %d %v", hidden, err)
	}
	var visibleChildren int
	if err := env.App.Deps.DB.QueryRow("WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT count(*) FROM drive_items WHERE id IN (SELECT id FROM subtree) AND hidden=0", *task.ResultId).Scan(&visibleChildren); err != nil || visibleChildren != 0 {
		t.Fatalf("visible extracted children: %d %v", visibleChildren, err)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ := env.Do(http.MethodGet, "/drive/items/"+strconv.FormatInt(*task.ResultId, 10), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked extracted folder: %d", status)
	}
}
