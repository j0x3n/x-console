// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import "./i18n";
import OutputView from "./components/OutputView";
import DiffView from "./components/DiffView";
import type { TaskEvent } from "./logic";

afterEach(cleanup);

const ev = (seq: number, kind: TaskEvent["kind"], text = "", data?: Record<string, unknown>, exitCode?: number): TaskEvent => ({
  seq,
  kind,
  text,
  at: "2026-09-27T00:00:00Z",
  data,
  exitCode,
});

describe("OutputView", () => {
  it("shows text, collapses tool calls and translates status lines", () => {
    render(
      <OutputView
        running={false}
        events={[
          ev(1, "status", "worktree ready", { code: "worktree_ready", branch: "xc/1-demo" }),
          ev(2, "text", "Working on it."),
          ev(3, "tool", "Write: hello.txt", { id: "t1", name: "Write", input: { file_path: "hello.txt" } }),
          ev(4, "tool", "File written", { id: "t1", result: true }),
          ev(5, "done", "exited", { reason: "exited" }, 0),
        ]}
      />,
    );
    expect(screen.getByText("Working on it.")).toBeTruthy();
    expect(screen.getByText(/worktree 已建好，分支 xc\/1-demo/)).toBeTruthy();
    expect(screen.getByText("退出码 0")).toBeTruthy();
    // 工具调用默认折叠，只显示数量和最后一个调用。
    const group = screen.getByRole("button", { name: /1 次工具调用/ });
    expect(group.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("File written")).toBeNull();
    fireEvent.click(group);
    fireEvent.click(screen.getByRole("button", { name: /Write: hello.txt/ }));
    expect(screen.getByText("File written")).toBeTruthy();
  });
});

describe("DiffView", () => {
  it("lists files and renders lines", () => {
    render(
      <DiffView
        diff={{
          truncated: false,
          files: [{ path: "a.txt", status: "M", additions: 1, deletions: 1 }],
          diff: "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old line\n+new line\n",
        }}
      />,
    );
    expect(screen.getByText("+1")).toBeTruthy();
    expect(screen.getByText("new line")).toBeTruthy();
    expect(screen.getByText("old line")).toBeTruthy();
  });
});
