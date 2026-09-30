package files_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/files"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

type uploaded struct {
	Id  int64  `json:"id"`
	Url string `json:"url"`
}

func upload(t *testing.T, env *testutil.Env, scope, name string, data []byte) (int, uploaded) {
	t.Helper()
	var body bytes.Buffer
	part := multipart.NewWriter(&body)
	f, err := part.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := part.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, env.URL("/files?scope="+scope), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", part.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out uploaded
	if resp.StatusCode < 300 && json.Unmarshal(raw, &out) != nil {
		t.Fatalf("decode: %s", raw)
	}
	return resp.StatusCode, out
}

func picture(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUploadReadClaimAndCleanup(t *testing.T) {
	env := testutil.New(t)
	if status, _ := upload(t, env, "projects", "fake.png", []byte("plain text")); status != 415 {
		t.Fatalf("type: %d", status)
	}
	if status, _ := upload(t, env, "projects", "large.png", bytes.Repeat([]byte("a"), 20<<20+1)); status != 413 {
		t.Fatalf("size: %d", status)
	}
	if status, _ := upload(t, env, "wrong", "a.png", picture(t, 1, 1)); status != 400 {
		t.Fatalf("scope: %d", status)
	}
	status, item := upload(t, env, "projects", "../photo.png", picture(t, 1601, 2))
	if status != 201 || item.Id == 0 || item.Url == "" {
		t.Fatalf("upload: %d %+v", status, item)
	}
	resp, err := http.Get(env.Server.URL + item.Url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("anonymous read: %d", resp.StatusCode)
	}
	for _, suffix := range []string{"", "?thumb=1"} {
		resp, err = env.Client.Get(env.Server.URL + item.Url + suffix)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		want := "image/png"
		if suffix != "" {
			want = "image/jpeg"
		}
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != want || !strings.Contains(resp.Header.Get("Cache-Control"), "private") {
			t.Fatalf("read %s: %d %s", suffix, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	service, ok := module.Lookup[contracts.Files](env.App.Deps.Registry, contracts.FilesKey)
	if !ok {
		t.Fatal("files service missing")
	}
	if err := service.Claim(context.Background(), "issue", 42, "![image]("+item.Url+")"); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := env.App.Deps.DB.QueryRow("SELECT owner_kind FROM uploaded_files WHERE id=?", item.Id).Scan(&kind); err != nil || kind != "issue" {
		t.Fatalf("claim: %s %v", kind, err)
	}
	if err := service.Claim(context.Background(), "comment", 99, "![image]("+item.Url+")"); err != nil {
		t.Fatal(err)
	}
	if err := env.App.Deps.DB.QueryRow("SELECT owner_kind FROM uploaded_files WHERE id=?", item.Id).Scan(&kind); err != nil || kind != "issue" {
		t.Fatalf("stolen: %s %v", kind, err)
	}
	if _, err := env.App.Deps.DB.Exec("UPDATE uploaded_files SET created_at=? WHERE id=?", time.Now().Add(-48*time.Hour), item.Id); err != nil {
		t.Fatal(err)
	}
	if err := service.(*files.Module).CleanupForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do("GET", "/files/"+strings.TrimPrefix(item.Url, "/api/v1/files/"), nil, nil); status != 200 {
		t.Fatalf("claimed removed: %d", status)
	}
	if err := service.DeleteOwned(context.Background(), "issue", 42); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do("GET", "/files/"+strings.TrimPrefix(item.Url, "/api/v1/files/"), nil, nil); status != 404 {
		t.Fatalf("delete owner: %d", status)
	}
	_, orphan := upload(t, env, "projects", "orphan.png", picture(t, 1, 1))
	if _, err := env.App.Deps.DB.Exec("UPDATE uploaded_files SET created_at=? WHERE id=?", time.Now().Add(-48*time.Hour), orphan.Id); err != nil {
		t.Fatal(err)
	}
	if err := service.(*files.Module).CleanupForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do("GET", "/files/"+strings.TrimPrefix(orphan.Url, "/api/v1/files/"), nil, nil); status != 404 {
		t.Fatalf("orphan: %d", status)
	}
}

func TestIssueDescriptionAndCommentImagesRemoved(t *testing.T) {
	env := testutil.New(t)
	_, issueImage := upload(t, env, "projects", "issue.png", picture(t, 2, 2))
	_, commentImage := upload(t, env, "projects", "comment.png", picture(t, 2, 2))
	var project struct{ Id int64 }
	env.MustDo("POST", "/projects", map[string]any{"key": "IMG", "name": "图片项目"}, &project)
	var issue struct {
		Id  int64
		Key string
	}
	env.MustDo("POST", "/projects/"+strconv.FormatInt(project.Id, 10)+"/issues", map[string]any{"title": "图片", "description": "![a](" + issueImage.Url + ")"}, &issue)
	var comment struct{ Id int64 }
	env.MustDo("POST", "/issues/"+issue.Key+"/comments", map[string]any{"body": "![b](" + commentImage.Url + ")"}, &comment)
	for _, item := range []uploaded{issueImage, commentImage} {
		var kind string
		if err := env.App.Deps.DB.QueryRow("SELECT owner_kind FROM uploaded_files WHERE id=?", item.Id).Scan(&kind); err != nil || kind == "" {
			t.Fatalf("unclaimed %d: %s %v", item.Id, kind, err)
		}
	}
	env.MustDo("DELETE", "/issues/"+issue.Key, nil, nil)
	for _, item := range []uploaded{issueImage, commentImage} {
		if status, _ := env.Do("GET", "/files/"+strconv.FormatInt(item.Id, 10), nil, nil); status != 404 {
			t.Fatalf("remaining %d: %d", item.Id, status)
		}
	}
}

func TestCodingTaskImagesRemovedWithRepo(t *testing.T) {
	env := testutil.New(t)
	agentID := env.Agent("server", nil, nil)
	_, image := upload(t, env, "coding", "task.png", picture(t, 2, 2))
	now := time.Now().UTC()
	res, err := env.App.Deps.DB.Exec("INSERT INTO coding_repos(agent_id,path,name,created_at) VALUES(?,?,?,?)", agentID, "/src/app", "app", now)
	if err != nil {
		t.Fatal(err)
	}
	repoID, _ := res.LastInsertId()
	res, err = env.App.Deps.DB.Exec("INSERT INTO coding_tasks(repo_id,executor,prompt,status,created_at,updated_at) VALUES(?,?,?,?,?,?)", repoID, "claude", "![a]("+image.Url+")", "review", now, now)
	if err != nil {
		t.Fatal(err)
	}
	taskID, _ := res.LastInsertId()
	claimer, _ := module.Lookup[contracts.Files](env.App.Deps.Registry, contracts.FilesKey)
	if err := claimer.Claim(context.Background(), "coding", taskID, "![a]("+image.Url+")"); err != nil {
		t.Fatal(err)
	}
	env.MustDo("DELETE", "/coding/repos/"+strconv.FormatInt(repoID, 10), nil, nil)
	if status, _ := env.Do("GET", "/files/"+strconv.FormatInt(image.Id, 10), nil, nil); status != 404 {
		t.Fatalf("task image after repo delete: %d", status)
	}
}
