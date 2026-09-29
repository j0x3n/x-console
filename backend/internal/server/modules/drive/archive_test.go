package drive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func archiveTask(t *testing.T, env *testutil.Env, body any) api.DriveTask {
	t.Helper()
	var task api.DriveTask
	status, raw := env.Do(http.MethodPost, "/drive/archive", body, &task)
	if status != http.StatusAccepted {
		t.Fatalf("archive start: %d %s", status, raw)
	}
	return waitTransfer(t, env, task.Id)
}

func archiveBytes(t *testing.T, env *testutil.Env, id int64) []byte {
	t.Helper()
	resp, err := env.Client.Get(env.URL("/drive/items/" + strconv.FormatInt(id, 10) + "/content"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("archive content: %d %v %s", resp.StatusCode, err, data)
	}
	return data
}

func TestDriveArchiveZIPAndTarGzip(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			env := testutil.New(t, drive.New)
			file := upload(t, env, "说明.txt", "hello", false)
			var folder, target api.DriveItem
			env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "资料"}, &folder)
			env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "保存处"}, &target)
			child := upload(t, env, "子文件.txt", "world", false)
			env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(child.Id, 10), map[string]any{"parentId": folder.Id}, nil)
			var empty api.DriveItem
			env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "空目录", "parentId": folder.Id}, &empty)
			task := archiveTask(t, env, map[string]any{"ids": []int64{file.Id, folder.Id, child.Id}, "name": "中文备份", "format": format, "parentId": target.Id})
			if task.ResultId == nil || task.TargetId == nil || *task.TargetId != target.Id || task.DoneItems != 4 || task.TotalItems != 4 || task.DoneBytes != 10 {
				t.Fatalf("archive progress: %+v", task)
			}
			out := listTransfer(t, env, target.Id)
			if len(out) != 1 || out[0].Id != *task.ResultId || out[0].Name != "中文备份."+format {
				t.Fatalf("archive output: %+v", out)
			}
			data := archiveBytes(t, env, *task.ResultId)
			contents := map[string]string{}
			if format == "zip" {
				zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range zr.File {
					r, err := entry.Open()
					if err != nil {
						t.Fatal(err)
					}
					value, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						t.Fatal(err)
					}
					contents[entry.Name] = string(value)
				}
			} else {
				gz, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				tr := tar.NewReader(gz)
				for {
					header, err := tr.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					value, err := io.ReadAll(tr)
					if err != nil {
						t.Fatal(err)
					}
					contents[header.Name] = string(value)
				}
			}
			if len(contents) != 4 || contents["说明.txt"] != "hello" || contents["资料/子文件.txt"] != "world" || contents["资料/"] != "" || contents["资料/空目录/"] != "" {
				t.Fatalf("archive contents: %+v", contents)
			}
			second := archiveTask(t, env, map[string]any{"ids": []int64{file.Id}, "name": "中文备份." + format, "format": format, "parentId": target.Id})
			if second.ResultId == nil || len(listTransfer(t, env, target.Id)) != 2 {
				t.Fatalf("archive rename: %+v", second)
			}
		})
	}
}

func TestDriveArchiveRejectsHiddenMismatchAndCleansFailedTemporaryFile(t *testing.T) {
	env := testutil.New(t, drive.New)
	plain := upload(t, env, "普通.txt", "plain", false)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	hidden := upload(t, env, "隐藏.txt", "hidden", true)
	status, _ := env.Do(http.MethodPost, "/drive/archive", map[string]any{"ids": []int64{plain.Id, hidden.Id}, "name": "错误", "format": "zip"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("mixed hidden: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ = env.Do(http.MethodPost, "/drive/archive", map[string]any{"ids": []int64{hidden.Id}, "name": "错误", "format": "zip"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked hidden: %d", status)
	}
	missingHash := strings.Repeat("a", 64)
	_, err := env.App.Deps.DB.Exec("UPDATE drive_items SET sha256=? WHERE id=?", missingHash, plain.Id)
	if err != nil {
		t.Fatal(err)
	}
	var task api.DriveTask
	env.MustDo(http.MethodPost, "/drive/archive", map[string]any{"ids": []int64{plain.Id}, "name": "失败包", "format": "zip"}, &task)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var out struct {
			Items []api.DriveTask `json:"items"`
		}
		env.MustDo(http.MethodGet, "/drive/tasks", nil, &out)
		for _, item := range out.Items {
			if item.Id == task.Id && item.FinishedAt != nil {
				if item.State != api.DriveTaskStateFailed {
					t.Fatalf("missing blob state: %+v", item)
				}
				files, err := filepath.Glob(filepath.Join(env.App.Deps.Config.TmpDir(), "archive-*"))
				if err != nil || len(files) != 0 {
					t.Fatalf("orphan temporary files: %v %v", files, err)
				}
				var count int
				if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_items WHERE name='失败包.zip'").Scan(&count); err != nil || count != 0 {
					t.Fatalf("failed archive inserted: %d %v", count, err)
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("archive failure timed out")
}
