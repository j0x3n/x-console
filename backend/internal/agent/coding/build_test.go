package coding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func buildDone(t *testing.T, rec *recorder) protocol.CodingBuildDone {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	last := rec.events[len(rec.events)-1]
	if last.Kind != protocol.CodingEventDone {
		t.Fatalf("last event: %+v", last)
	}
	var d protocol.CodingBuildDone
	if err := json.Unmarshal(last.Data, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func (r *recorder) texts() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for _, ev := range r.events {
		if ev.Kind == protocol.CodingEventText {
			b.WriteString(ev.Text + "\n")
		}
	}
	return b.String()
}

func TestBuildStepsAndArtifacts(t *testing.T) {
	needGit(t)
	repo, _ := newRepo(t)
	ctx := context.Background()
	wt := WorktreePath(repo, 3)
	base, err := createWorktree(ctx, repo, wt, "xc/3-build", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(wt, "leak.md")); err != nil {
		t.Fatal(err)
	}
	s := New(Config{})
	tp := protocol.CodingTaskParams{TaskID: 3, RepoPath: repo, Branch: "xc/3-build", BaseCommit: base}

	rec := &recorder{}
	s.Build(ctx, protocol.CodingBuildParams{CodingTaskParams: tp, Steps: []protocol.CodingBuildStep{
		{Name: "test", Command: "echo testing; echo careful >&2"},
		{Name: "build", Command: "mkdir -p out/sub && echo bin > out/app.bin && echo x > out/sub/x.txt", Artifacts: []string{"out/**", "*.md", "../*", "/etc/*"}},
	}}, rec.emit, nil)
	d := buildDone(t, rec)
	if d.Reason != protocol.CodingBuildPassed || d.FailedStep != -1 {
		t.Fatalf("done: %+v", d)
	}
	var names []string
	for _, a := range d.Artifacts {
		names = append(names, a.Name)
		if !filepath.IsAbs(a.Path) {
			t.Fatalf("artifact path: %+v", a)
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "README.md,out/app.bin,out/sub/x.txt" {
		t.Fatalf("artifacts: %v", names)
	}
	if out := rec.texts(); !strings.Contains(out, "testing") || !strings.Contains(out, "careful") {
		t.Fatalf("output: %q", out)
	}
	if _, ok := rec.find(protocol.CodingEventStatus, "build_step_done"); !ok {
		t.Fatal("no build_step_done")
	}

	// A failing step stops the build.
	rec = &recorder{}
	s.Build(ctx, protocol.CodingBuildParams{CodingTaskParams: tp, Steps: []protocol.CodingBuildStep{
		{Name: "ok", Command: "true"}, {Name: "bad", Command: "echo broken >&2; exit 4"}, {Name: "never", Command: "touch never"},
	}}, rec.emit, nil)
	d = buildDone(t, rec)
	if d.Reason != protocol.CodingBuildFailed || d.FailedStep != 1 || exists(filepath.Join(wt, "never")) {
		t.Fatalf("failed build: %+v", d)
	}

	// Timeout.
	KillGrace = 200 * time.Millisecond
	t.Cleanup(func() { KillGrace = 10 * time.Second })
	rec = &recorder{}
	s.Build(ctx, protocol.CodingBuildParams{CodingTaskParams: tp, Steps: []protocol.CodingBuildStep{
		{Name: "slow", Command: "sleep 30", TimeoutSeconds: 1},
	}}, rec.emit, nil)
	if d = buildDone(t, rec); d.Reason != protocol.CodingBuildTimeout || d.FailedStep != 0 {
		t.Fatalf("timeout: %+v", d)
	}

	// No worktree.
	rec = &recorder{}
	tp.TaskID = 99
	s.Build(ctx, protocol.CodingBuildParams{CodingTaskParams: tp, Steps: []protocol.CodingBuildStep{{Name: "x", Command: "true"}}}, rec.emit, nil)
	if d = buildDone(t, rec); d.Reason != protocol.CodingBuildError {
		t.Fatalf("no worktree: %+v", d)
	}
}

func TestRunContinueInWorktree(t *testing.T) {
	needGit(t)
	repo, _ := newRepo(t)
	s := fakeService(t)
	ctx := context.Background()
	rec := &recorder{}
	s.Run(ctx, protocol.CodingRunParams{TaskID: 5, RepoPath: repo, Executor: "claude", Prompt: "first", BaseBranch: "main", Branch: "xc/5-a"}, rec.emit, nil)
	_, d := rec.done(t)
	if d.Reason != protocol.CodingEndExited {
		t.Fatalf("first run: %+v", d)
	}
	rec = &recorder{}
	s.Run(ctx, protocol.CodingRunParams{TaskID: 5, RepoPath: repo, Executor: "claude", Prompt: "fix the build", Branch: "xc/5-a",
		Continue: true, BaseCommit: d.BaseCommit}, rec.emit, nil)
	code, d2 := rec.done(t)
	if code != 0 || d2.Reason != protocol.CodingEndExited || d2.BaseCommit != d.BaseCommit || len(d2.Files) == 0 {
		t.Fatalf("continue: %d %+v", code, d2)
	}
	raw, _ := os.ReadFile(filepath.Join(WorktreePath(repo, 5), "hello.txt"))
	if !strings.Contains(string(raw), "fix the build") {
		t.Fatalf("hello.txt: %q", raw)
	}
	// Without a worktree, continue fails to start.
	rec = &recorder{}
	s.Run(ctx, protocol.CodingRunParams{TaskID: 6, RepoPath: repo, Executor: "claude", Prompt: "x", Branch: "xc/6-a", Continue: true, BaseCommit: d.BaseCommit}, rec.emit, nil)
	if _, d3 := rec.done(t); d3.Reason != protocol.CodingEndStartFailed {
		t.Fatalf("continue without worktree: %+v", d3)
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
