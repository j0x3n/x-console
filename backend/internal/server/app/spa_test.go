package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// 部署后旧页面请求已经不在的打包文件时要回 404，不能回首页的 HTML。
func TestSPAMissingAssetIs404(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "a.js"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := spa(dir)
	for path, want := range map[string]int{"/assets/a.js": 200, "/assets/old.js": 404, "/notes/12": 200} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Fatalf("%s: %d, want %d", path, rec.Code, want)
		}
	}
}
