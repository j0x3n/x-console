package drive_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func readContent(t *testing.T, env *testutil.Env, id int64) (string, string) {
	t.Helper()
	resp, err := env.Client.Get(env.URL("/drive/items/" + itoa(id) + "/content?inline=1"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read: %d %s", resp.StatusCode, raw)
	}
	return string(raw), resp.Header.Get("ETag")
}

func saveContent(t *testing.T, env *testutil.Env, id int64, text, ifMatch string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, env.URL("/drive/items/"+itoa(id)+"/content"), strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("X-Requested-With", "x-console")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func TestSaveTextContent(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "config.yaml", "a: 1\n", false)
	text, tag := readContent(t, env, file.Id)
	if text != "a: 1\n" || tag == "" {
		t.Fatalf("before save: %q %q", text, tag)
	}

	resp, body := saveContent(t, env, file.Id, "a: 2\n", tag)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", resp.StatusCode, body)
	}
	newTag := resp.Header.Get("ETag")
	if newTag == "" || newTag == tag || !strings.Contains(body, `"size":5`) {
		t.Fatalf("save result: %q %s", newTag, body)
	}
	text, readTag := readContent(t, env, file.Id)
	if text != "a: 2\n" || readTag != newTag {
		t.Fatalf("after save: %q %q", text, readTag)
	}

	// 用旧版本号再存一次：别的标签页的情况。
	resp, body = saveContent(t, env, file.Id, "a: 3\n", tag)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "version_conflict") {
		t.Fatalf("stale save: %d %s", resp.StatusCode, body)
	}
	// 不带版本号就是覆盖。
	resp, body = saveContent(t, env, file.Id, "a: 3\n", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overwrite: %d %s", resp.StatusCode, body)
	}
	if text, _ = readContent(t, env, file.Id); text != "a: 3\n" {
		t.Fatalf("after overwrite: %q", text)
	}

	resp, _ = saveContent(t, env, file.Id, "\xff\xfe", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non utf-8: %d", resp.StatusCode)
	}
	resp, _ = saveContent(t, env, file.Id, strings.Repeat("x", 10<<20+1), "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("too large: %d", resp.StatusCode)
	}
	var folder struct{ Id int64 }
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "dir"}, &folder)
	resp, _ = saveContent(t, env, folder.Id, "x", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("folder: %d", resp.StatusCode)
	}
}
