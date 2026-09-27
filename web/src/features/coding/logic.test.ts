import { describe, expect, it } from "vitest";
import {
  actionsFor,
  buildBlocks,
  filterTasks,
  formatDuration,
  hasGap,
  mergeEvents,
  parseDiff,
  sortTasks,
  type Task,
  type TaskEvent,
} from "./logic";

function task(id: number, status: Task["status"]): Task {
  return {
    id,
    status,
    repoId: 1,
    repoName: "demo",
    agentId: "a",
    executor: "claude",
    prompt: "p",
    title: `t${id}`,
    baseBranch: "main",
    branch: `xc/${id}-p`,
    error: "",
    commitSha: "",
    prUrl: "",
    changedFiles: [],
    timeoutMinutes: 60,
    createdAt: "2026-09-27T00:00:00Z",
    updatedAt: "2026-09-27T00:00:00Z",
  };
}

const ev = (
  seq: number,
  kind: TaskEvent["kind"],
  text = "",
  data?: Record<string, unknown>,
): TaskEvent => ({
  seq,
  kind,
  text,
  at: "2026-09-27T00:00:00Z",
  data,
});

describe("tasks", () => {
  it("puts running first, then queued oldest first, then newest", () => {
    const tasks = [
      task(1, "review"),
      task(2, "queued"),
      task(3, "running"),
      task(4, "failed"),
      task(5, "queued"),
    ];
    expect(sortTasks(tasks).map((t) => t.id)).toEqual([3, 2, 5, 4, 1]);
  });

  it("filters by status group", () => {
    const tasks = [
      task(1, "review"),
      task(2, "queued"),
      task(3, "pr_opened"),
      task(4, "canceled"),
    ];
    expect(filterTasks(tasks, "active").map((t) => t.id)).toEqual([2]);
    expect(filterTasks(tasks, "done").map((t) => t.id)).toEqual([3]);
    expect(filterTasks(tasks, "failed").map((t) => t.id)).toEqual([4]);
    expect(filterTasks(tasks, "all")).toHaveLength(4);
  });

  it("allows the actions the server allows", () => {
    expect(actionsFor({ status: "running" }, true)).toMatchObject({
      cancel: true,
      commit: false,
      diff: false,
    });
    expect(actionsFor({ status: "review" }, true)).toMatchObject({
      commit: true,
      discard: true,
      pr: false,
      diff: true,
    });
    expect(actionsFor({ status: "committed" }, true)).toMatchObject({
      push: true,
      pr: true,
      discard: true,
    });
    expect(actionsFor({ status: "committed" }, false).pr).toBe(false);
    expect(actionsFor({ status: "pr_opened" }, true)).toMatchObject({
      pr: false,
      discard: false,
      diff: true,
    });
  });

  it("formats durations", () => {
    expect(formatDuration(5_400)).toBe("5s");
    expect(formatDuration(125_000)).toBe("2m 5s");
    expect(formatDuration(3_720_000)).toBe("1h 2m");
  });
});

describe("events", () => {
  it("merges by seq and spots gaps", () => {
    const a = [ev(1, "text"), ev(2, "text")];
    expect(
      mergeEvents(a, [ev(2, "text"), ev(3, "text")]).map((e) => e.seq),
    ).toEqual([1, 2, 3]);
    expect(mergeEvents(a, [ev(1, "text")])).toBe(a);
    expect(hasGap(a, [ev(3, "text")])).toBe(false);
    expect(hasGap(a, [ev(5, "text")])).toBe(true);
    expect(hasGap([], [ev(1, "text")])).toBe(false);
  });

  it("groups tool calls and pairs results", () => {
    const blocks = buildBlocks([
      ev(1, "status", "session started", {
        code: "session_started",
        model: "m",
      }),
      ev(2, "text", "Looking."),
      ev(3, "tool", "Read: a.go", {
        id: "t1",
        name: "Read",
        input: { file_path: "a.go" },
      }),
      ev(4, "tool", "Edit: a.go", { id: "t2", name: "Edit" }),
      ev(5, "tool", "ok", { id: "t1", result: true }),
      ev(6, "tool", "boom", { id: "t2", result: true, isError: true }),
      ev(7, "text", "warn", { stream: "stderr" }),
      ev(8, "tool", "Bash: ls", { id: "t3", name: "Bash" }),
      ev(9, "done", "exited", { reason: "exited" }),
    ]);
    expect(blocks.map((b) => b.type)).toEqual([
      "status",
      "text",
      "tools",
      "text",
      "tools",
      "done",
    ]);
    const tools = blocks[2];
    if (tools.type !== "tools") throw new Error("not tools");
    expect(tools.calls).toHaveLength(2);
    expect(tools.calls[0]).toMatchObject({
      summary: "Read: a.go",
      result: "ok",
      done: true,
      isError: false,
    });
    expect(tools.calls[1]).toMatchObject({ result: "boom", isError: true });
    expect(blocks[3]).toMatchObject({ stderr: true });
    expect(blocks[5]).toMatchObject({ reason: "exited" });
  });
});

describe("parseDiff", () => {
  it("splits files and numbers lines", () => {
    const diff = [
      "diff --git a/README.md b/README.md",
      "index 1..2 100644",
      "--- a/README.md",
      "+++ b/README.md",
      "@@ -1,2 +1,3 @@",
      " # demo",
      "-old",
      "+new",
      "+more",
      "diff --git a/hello.txt b/hello.txt",
      "new file mode 100644",
      "--- /dev/null",
      "+++ b/hello.txt",
      "@@ -0,0 +1 @@",
      "+hi",
      "\\ No newline at end of file",
      "diff --git a/logo.png b/logo.png",
      "Binary files /dev/null and b/logo.png differ",
      "",
    ].join("\n");
    const files = parseDiff(diff);
    expect(files.map((f) => f.path)).toEqual([
      "README.md",
      "hello.txt",
      "logo.png",
    ]);
    expect(files[0]).toMatchObject({ additions: 2, deletions: 1 });
    expect(files[0].lines[1]).toMatchObject({
      kind: "ctx",
      oldNo: 1,
      newNo: 1,
    });
    expect(files[0].lines[2]).toMatchObject({ kind: "del", oldNo: 2 });
    expect(files[0].lines[4]).toMatchObject({ kind: "add", newNo: 3 });
    expect(files[1].lines.at(-1)).toMatchObject({ kind: "meta" });
    expect(files[2].binary).toBe(true);
  });
});
