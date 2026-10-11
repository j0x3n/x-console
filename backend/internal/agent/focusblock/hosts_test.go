package focusblock

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const sampleHosts = "# Copyright (c) Microsoft Corp.\n127.0.0.1 localhost\n::1 localhost\n\n# my own entry\n10.0.0.5 nas.home\n"

func TestWithBlockKeepsTheRestAndRoundTrips(t *testing.T) {
	for name, original := range map[string]string{
		"unix":    sampleHosts,
		"windows": strings.ReplaceAll(sampleHosts, "\n", "\r\n"),
		"empty":   "",
	} {
		t.Run(name, func(t *testing.T) {
			with := withBlock(original, []string{"youtube.com", "www.youtube.com"}, false)
			if !strings.HasPrefix(with, original) {
				t.Fatalf("original changed:\n%q", with)
			}
			if !strings.Contains(with, "127.0.0.1 youtube.com") || !strings.Contains(with, beginMarker) || !strings.Contains(with, endMarker) {
				t.Fatalf("block missing:\n%q", with)
			}
			if strings.Contains(original, "\r\n") && strings.Contains(strings.ReplaceAll(with, "\r\n", ""), "\n") {
				t.Fatalf("mixed line endings:\n%q", with)
			}
			back, removed := removeBlocks(with)
			if !removed || back != original {
				t.Fatalf("round trip: removed=%v\n%q\nwant\n%q", removed, back, original)
			}
		})
	}
}

func TestWithBlockAddsMissingFinalLineBreakOnly(t *testing.T) {
	original := "127.0.0.1 localhost" // no line break at the end
	with := withBlock(original, []string{"a.com"}, false)
	if !strings.HasPrefix(with, "127.0.0.1 localhost\n"+beginMarker) {
		t.Fatalf("got %q", with)
	}
	back, _ := removeBlocks(with)
	if back != original+"\n" {
		t.Fatalf("got %q", back)
	}
}

func TestWithBlockTwiceKeepsOneBlock(t *testing.T) {
	once := withBlock(sampleHosts, []string{"a.com"}, false)
	twice := withBlock(once, []string{"a.com"}, false)
	if once != twice {
		t.Fatalf("not idempotent:\n%q\n%q", once, twice)
	}
	other := withBlock(twice, []string{"b.com"}, true)
	if strings.Count(other, beginMarker) != 1 || strings.Contains(other, "a.com") || !strings.Contains(other, "::1 b.com") {
		t.Fatalf("got %q", other)
	}
	if got := withBlock(other, nil, false); got != sampleHosts {
		t.Fatalf("empty list should remove the block: %q", got)
	}
}

func TestRemoveBlocksLeavesLookalikeLinesAlone(t *testing.T) {
	// A user's own loopback lines outside the block stay, even for the same name.
	original := sampleHosts + "127.0.0.1 youtube.com\n"
	with := withBlock(original, []string{"youtube.com"}, false)
	back, _ := removeBlocks(with)
	if back != original {
		t.Fatalf("got %q", back)
	}
}

func TestRemoveBlocksUnclosedAndStray(t *testing.T) {
	// The end marker is missing (a crash while writing, or a hand edit): the
	// loopback lines after the begin marker go, the user's lines stay.
	broken := sampleHosts + beginMarker + "\n127.0.0.1 a.com\n::1 a.com\n10.0.0.9 keep.me\n"
	got, removed := removeBlocks(broken)
	if !removed || got != sampleHosts+"10.0.0.9 keep.me\n" {
		t.Fatalf("unclosed: %q", got)
	}
	// Two blocks and a stray end marker.
	two := beginMarker + "\n127.0.0.1 a.com\n" + endMarker + "\n127.0.0.1 localhost\n" + beginMarker + "\n127.0.0.1 b.com\n" + endMarker + "\n" + endMarker + "\n"
	got, removed = removeBlocks(two)
	if !removed || got != "127.0.0.1 localhost\n" {
		t.Fatalf("two: %q", got)
	}
	// Nothing to remove.
	if got, removed := removeBlocks(sampleHosts); removed || got != sampleHosts {
		t.Fatalf("clean file: %v %q", removed, got)
	}
}

func TestSyncHostsDoesNotTouchACleanFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(sampleHosts), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	wrote, err := syncHosts(path, nil, false)
	if err != nil || wrote {
		t.Fatalf("wrote=%v err=%v", wrote, err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Fatal("file was replaced")
	}
	// A missing file is a clean file.
	if wrote, err := syncHosts(filepath.Join(t.TempDir(), "none"), nil, false); err != nil || wrote {
		t.Fatalf("missing file: wrote=%v err=%v", wrote, err)
	}
}

func TestWriteAtomicKeepsModeAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte(sampleHosts), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v err %v", info.Mode(), err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new\n" {
		t.Fatalf("content %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestWriteAtomicFollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "hosts")
	if err := os.WriteFile(real, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks here")
	}
	if err := writeAtomic(link, []byte("b\n")); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced by a file")
	}
	if b, _ := os.ReadFile(real); string(b) != "b\n" {
		t.Fatalf("target %q", b)
	}
}

func TestWriteAtomicFailureLeavesTheFileAlone(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte(sampleHosts), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := syncHosts(path, []string{"a.com"}, false)
	if hostsResult(err) != protocol.FocusBlockHostsPermission {
		t.Fatalf("err %v maps to %q", err, hostsResult(err))
	}
	if b, _ := os.ReadFile(path); string(b) != sampleHosts {
		t.Fatalf("file changed: %q", b)
	}
}

func TestHostsResult(t *testing.T) {
	if hostsResult(nil) != "ok" || hostsResult(fs.ErrPermission) != "permission" || hostsResult(os.ErrInvalid) != "failed" {
		t.Fatal("wrong mapping")
	}
}

func TestDefaultHostsPath(t *testing.T) {
	if p := DefaultHostsPath(); !strings.HasSuffix(filepath.ToSlash(p), "etc/hosts") {
		t.Fatalf("got %q", p)
	}
}
