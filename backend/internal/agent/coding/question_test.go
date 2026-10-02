package coding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuestionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".xc-question.md")
	if err := os.WriteFile(path, []byte(`{"title":"选哪个数据库？","options":["SQLite","Postgres"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	q, err := readQuestion(dir)
	if err != nil || q.Title != "选哪个数据库？" || len(q.Options) != 2 {
		t.Fatalf("%+v %v", q, err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("question not removed")
	}
	if err = os.WriteFile(path, make([]byte, 65537), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readQuestion(dir); err == nil {
		t.Fatal("oversized question accepted")
	}
	_ = os.Remove(path)
	if err = os.Symlink(filepath.Join(dir, "other"), path); err == nil {
		if _, err = readQuestion(dir); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}
