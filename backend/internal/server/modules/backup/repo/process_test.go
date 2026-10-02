package repo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOperatingSystemLockAcrossProcesses(t *testing.T) {
	if path := os.Getenv("XC_REPO_LOCK_TEST"); path != "" {
		f, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := lockFile(f); err == nil {
			unlockFile(f)
			t.Fatal("second process acquired held lock")
		}
		return
	}
	path := filepath.Join(t.TempDir(), "held.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		t.Fatal(err)
	}
	defer unlockFile(f)
	cmd := exec.Command(os.Args[0], "-test.run=^TestOperatingSystemLockAcrossProcesses$")
	cmd.Env = append(os.Environ(), "XC_REPO_LOCK_TEST="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %s %v", out, err)
	}
}

func TestRuntimeLeaseExcludesSecondServer(t *testing.T) {
	dir := t.TempDir()
	first, err := LockRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockRuntime(dir); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := LockRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
}
