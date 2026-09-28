package drive_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func upload(t *testing.T, env *testutil.Env, name, content string, hidden bool) api.DriveItem {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := "/drive/upload"
	if hidden {
		path += "?hidden=true"
	}
	req, err := http.NewRequest(http.MethodPost, env.URL(path), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Items []api.DriveItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("upload result: %s", raw)
	}
	return out.Items[0]
}

func TestFilesFoldersAndHidden(t *testing.T) {
	env := testutil.New(t, drive.New)
	var folder api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "项目资料"}, &folder)
	if !folder.IsDir {
		t.Fatal("folder flag")
	}
	first := upload(t, env, "a.txt", "same content", false)
	second := upload(t, env, "a.txt", "same content", false)
	if second.Name != "a (1).txt" {
		t.Fatalf("duplicate name: %q", second.Name)
	}
	blobs, err := filepath.Glob(filepath.Join(env.App.Deps.Config.DataDir, "drive", "blobs", "*", "*"))
	if err != nil || len(blobs) != 1 {
		t.Fatalf("blob dedup: %v %v", blobs, err)
	}
	raw, err := os.ReadFile(blobs[0])
	if err != nil || string(raw) != "same content" {
		t.Fatalf("blob: %s %v", raw, err)
	}
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(first.Id), map[string]any{"parentId": folder.Id}, nil)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	events, cancel := env.App.Deps.Bus.Subscribe("drive_item.", 4)
	defer cancel()
	var hidden api.DriveItem
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(first.Id), map[string]any{"hidden": true}, &hidden)
	select {
	case event := <-events:
		payload, ok := event.Data.(map[string]any)
		if !ok || len(payload) != 2 || payload["id"] != first.Id || payload["hidden"] != true {
			t.Fatalf("hidden event leaked: %#v", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("hidden event missing")
	}
	if hidden.RestoreTo == nil || *hidden.RestoreTo != "/项目资料" {
		t.Fatalf("restoreTo: %+v", hidden.RestoreTo)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ := env.Do(http.MethodGet, "/drive/items/"+itoa(first.Id), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked get: %d", status)
	}
	var found struct {
		Items []api.DriveItem `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/items?q=a", nil, &found)
	if len(found.Items) != 1 || found.Items[0].Id != second.Id {
		t.Fatalf("locked search: %+v", found.Items)
	}
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "secret123"}, nil)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(first.Id), map[string]any{"hidden": false}, &hidden)
	if hidden.ParentId == nil || *hidden.ParentId != folder.Id {
		t.Fatalf("restored parent: %+v", hidden.ParentId)
	}
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(first.Id), nil, nil)
	env.MustDo(http.MethodPost, "/drive/items/"+itoa(first.Id)+"/restore", nil, nil)
	req, err := http.NewRequest(http.MethodGet, env.URL("/drive/items/"+itoa(second.Id)+"/content?inline=true"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-3")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "same" {
		t.Fatalf("range: %d %q", resp.StatusCode, body)
	}
	status, _ = env.Do(http.MethodDelete, "/drive/items/"+itoa(second.Id)+"?permanent=true", nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("permanent without elevation: %d", status)
	}
	env.Elevate()
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(second.Id)+"?permanent=true", nil, nil)
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(folder.Id)+"?permanent=true", nil, nil)
	status, _ = env.Do(http.MethodGet, "/drive/items/"+itoa(first.Id), nil, nil)
	if status != 404 {
		t.Fatalf("child survived folder deletion: %d", status)
	}
	blobs, err = filepath.Glob(filepath.Join(env.App.Deps.Config.DataDir, "drive", "blobs", "*", "*"))
	if err != nil || len(blobs) != 0 {
		t.Fatalf("unreferenced blob remains: %v %v", blobs, err)
	}
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func TestHiddenTreeRestoreWhenOriginalFolderIsTrashed(t *testing.T) {
	env := testutil.New(t, drive.New)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	var original, secret, child, collision api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "原目录"}, &original)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "秘密", "parentId": original.Id}, &secret)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "子目录", "parentId": secret.Id}, &child)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(secret.Id), map[string]any{"hidden": true}, &secret)
	if secret.RestoreTo == nil || *secret.RestoreTo != "/原目录" {
		t.Fatalf("restore path: %+v", secret.RestoreTo)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ := env.Do(http.MethodGet, "/drive/items/"+itoa(child.Id), nil, nil)
	if status != 404 {
		t.Fatalf("hidden child visible: %d", status)
	}
	var listing struct {
		Items []api.DriveItem `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/items?q=子目录", nil, &listing)
	if len(listing.Items) != 0 {
		t.Fatalf("hidden child in search: %+v", listing.Items)
	}
	env.MustDo(http.MethodPost, "/vault/unlock", map[string]string{"password": "secret123"}, nil)
	env.MustDo(http.MethodGet, "/drive/items?hidden=true&parent="+itoa(secret.Id), nil, &listing)
	if len(listing.Items) != 1 || listing.Items[0].Id != child.Id {
		t.Fatalf("hidden tree: %+v", listing.Items)
	}
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(original.Id), nil, nil)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "秘密"}, &collision)
	secretID := secret.Id
	secret = api.DriveItem{}
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(secretID), map[string]any{"hidden": false}, &secret)
	if secret.ParentId != nil || secret.Name != "秘密 (1)" {
		t.Fatalf("fallback restore: %+v", secret)
	}
	env.MustDo(http.MethodGet, "/drive/items?parent="+itoa(secret.Id), nil, &listing)
	if len(listing.Items) != 1 || listing.Items[0].Hidden {
		t.Fatalf("restored descendants: %+v", listing.Items)
	}
}

func TestS3SyncAndActions(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	var sources []string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/testbucket" && r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/testbucket/")
		if key == r.URL.Path {
			http.Error(w, "bucket", 404)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			if source := r.Header.Get("X-Amz-Copy-Source"); source != "" {
				sources = append(sources, source)
				source = strings.TrimPrefix(strings.TrimPrefix(source, "/"), "testbucket/")
				source, _ = url.PathUnescape(source)
				data, ok := objects[source]
				if !ok {
					http.Error(w, "missing source", 404)
					return
				}
				objects[key] = bytes.Clone(data)
				w.Header().Set("Content-Type", "application/xml")
				io.WriteString(w, `<CopyObjectResult><ETag>"copied"</ETag><LastModified>2026-09-28T00:00:00Z</LastModified></CopyObjectResult>`)
				return
			}
			data, _ := io.ReadAll(r.Body)
			objects[key] = data
			w.Header().Set("ETag", `"test"`)
			w.WriteHeader(200)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(204)
		case http.MethodHead:
			if _, ok := objects[key]; !ok {
				http.Error(w, "missing", 404)
				return
			}
			w.WriteHeader(200)
		default:
			http.Error(w, "unexpected", 405)
		}
	}))
	defer fake.Close()
	env := testutil.New(t, drive.New)
	status, _ := env.Do(http.MethodPut, "/drive/s3", map[string]any{"enabled": true}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("S3 config without elevation: %d", status)
	}
	env.Elevate()
	var cfg api.S3Config
	env.MustDo(http.MethodPut, "/drive/s3", map[string]any{"endpoint": fake.URL, "region": "us-east-1", "bucket": "testbucket", "prefix": "backup", "accessKeyId": "key", "secretAccessKey": "secret", "pathStyle": true, "enabled": true}, &cfg)
	if !cfg.HasSecret {
		t.Fatal("secret wasn't stored")
	}
	var public api.S3Config
	env.MustDo(http.MethodGet, "/drive/s3", nil, &public)
	if public.HasSecret != true {
		t.Fatal("secret flag missing")
	}
	var stored string
	var encrypted int
	if err := env.App.Deps.DB.QueryRow("SELECT value,encrypted FROM settings WHERE key='drive.s3.secret'").Scan(&stored, &encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted != 1 || strings.Contains(stored, "secret") {
		t.Fatal("S3 secret was not encrypted")
	}
	var test struct {
		Ok bool `json:"ok"`
	}
	env.MustDo(http.MethodPost, "/drive/s3/test", nil, &test)
	if !test.Ok {
		t.Fatal("S3 test failed")
	}
	var folder api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]string{"name": "项目"}, &folder)
	item := upload(t, env, "one.txt", "hello S3", false)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(item.Id), map[string]any{"parentId": folder.Id}, nil)
	waitObject := func(key string, want bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			data, ok := objects[key]
			mu.Unlock()
			if ok == want && (!want || bytes.Contains(data, []byte("hello S3"))) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		var s3key, s3error any
		env.App.Deps.DB.QueryRow("SELECT s3_key,s3_error FROM drive_items WHERE id=?", item.Id).Scan(&s3key, &s3error)
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("object %s want %v: %+v; copy=%v row key=%v error=%v", key, want, objects, sources, s3key, s3error)
	}
	waitObject("backup/项目/one.txt", true)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(item.Id), map[string]string{"name": "two.txt"}, nil)
	waitObject("backup/项目/two.txt", true)
	waitObject("backup/项目/one.txt", false)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(item.Id), map[string]any{"hidden": true}, nil)
	waitObject("backup/项目/two.txt", false)
	env.MustDo(http.MethodPut, "/drive/s3", map[string]any{"includeHidden": true}, nil)
	waitObject("backup/.hidden/two.txt", true)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(item.Id), map[string]any{"hidden": false}, nil)
	waitObject("backup/项目/two.txt", true)
	waitObject("backup/.hidden/two.txt", false)
	ctx := context.Background()
	calls := []struct{ name, input string }{
		{"drive.list", `{}`}, {"drive.search", `{"q":"two"}`}, {"drive.read_text", `{"id":` + itoa(item.Id) + `}`},
		{"drive.create_folder", `{"name":"action-folder"}`}, {"drive.write_text", `{"name":"action.txt","text":"hello"}`},
		{"drive.move", `{"id":` + itoa(item.Id) + `,"parentId":0}`}, {"drive.rename", `{"id":` + itoa(item.Id) + `,"name":"three.txt"}`},
	}
	for _, call := range calls {
		if _, err := env.App.Deps.Actions.Run(ctx, call.name, json.RawMessage(call.input)); err != nil {
			t.Fatalf("action %s: %v", call.name, err)
		}
	}
	waitObject("backup/three.txt", true)
	if _, err := env.App.Deps.Actions.Run(ctx, "drive.delete", json.RawMessage(`{"id":`+itoa(item.Id)+`}`)); err != nil {
		t.Fatal(err)
	}
	waitObject("backup/three.txt", true)
	env.MustDo(http.MethodDelete, "/drive/items/"+itoa(item.Id)+"?permanent=true", nil, nil)
	waitObject("backup/three.txt", false)
}
