// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, recentApiErrors } from "../api/client";
import { useToastStore, toast } from "../hooks/useToast";
import {
  DEDUPE_MS,
  isIgnoredError,
  reportError,
  setErrorClock,
  useErrorStore,
} from "./errors";

let clock = 1_700_000_000_000;

function apiError(status: number, code: string, message: string) {
  const e = new ApiError(status, code, message);
  e.request = {
    method: "PATCH",
    path: "/api/v1/notes/12",
    body: JSON.stringify({ code, message, requestId: "req-1" }),
    requestId: "req-1",
    at: clock,
  };
  recentApiErrors.unshift(e);
  return e;
}

beforeEach(() => {
  vi.useFakeTimers();
  setErrorClock(() => clock);
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  useErrorStore.setState({ notices: [], history: [] });
  useToastStore.setState({ current: null });
  recentApiErrors.length = 0;
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("reportError", () => {
  it("keeps errors on screen but hides ok toasts after 5 seconds", () => {
    reportError(new Error("boom"));
    toast("已保存");
    vi.advanceTimersByTime(60_000);
    expect(useErrorStore.getState().notices).toHaveLength(1);
    expect(useToastStore.getState().current).toBeNull();
  });

  it("prints to the console", () => {
    reportError(new Error("boom"), { title: "保存失败" });
    expect(console.error).toHaveBeenCalledWith(
      "[X Console]",
      "保存失败",
      "boom",
      expect.any(Error),
    );
  });

  it("merges the same error within 10 seconds and counts it", () => {
    reportError(apiError(500, "internal", "database is locked"));
    clock += 3_000;
    reportError(apiError(500, "internal", "database is locked"));
    let notices = useErrorStore.getState().notices;
    expect(notices).toHaveLength(1);
    expect(notices[0].count).toBe(2);
    clock += DEDUPE_MS + 1;
    reportError(apiError(500, "internal", "database is locked"));
    notices = useErrorStore.getState().notices;
    expect(notices).toHaveLength(2);
  });

  it("merges requests that differ only in query string", () => {
    const a = apiError(503, "agent_offline", "代理不在线");
    a.request!.path = "/api/v1/reminders?view=today";
    const b = apiError(503, "agent_offline", "代理不在线");
    b.request!.path = "/api/v1/reminders?view=upcoming";
    reportError(a);
    reportError(b);
    expect(useErrorStore.getState().notices).toHaveLength(1);
  });

  it("builds a copy text with request, status, id and page", () => {
    reportError(apiError(500, "internal", "database is locked"), {
      title: "保存笔记失败",
    });
    const detail = useErrorStore.getState().notices[0].detail;
    expect(detail.split("\n")[0]).toBe("保存笔记失败");
    expect(detail).toContain("请求：PATCH /api/v1/notes/12");
    expect(detail).toContain("状态：500 internal");
    expect(detail).toContain("信息：database is locked");
    expect(detail).toContain("请求编号：req-1");
    expect(detail).toContain("页面：/");
    expect(detail).toContain("响应：");
  });

  it("ignores 401, cancelled requests and elevation prompts", () => {
    expect(isIgnoredError(new ApiError(401, "unauthorized", "x"))).toBe(true);
    expect(isIgnoredError(new DOMException("aborted", "AbortError"))).toBe(
      true,
    );
    expect(isIgnoredError(new ApiError(403, "elevation_required", "x"))).toBe(
      true,
    );
    reportError(new ApiError(401, "unauthorized", "x"));
    expect(useErrorStore.getState().notices).toHaveLength(0);
  });

  it("does not show silent errors", () => {
    reportError(new Error("boom"), { silent: true });
    expect(useErrorStore.getState().notices).toHaveLength(0);
  });

  it("turns error toasts into notices with the matching request details", () => {
    apiError(500, "internal", "database is locked");
    toast({
      message: "保存失败",
      subtitle: "database is locked",
      tone: "error",
    });
    const [notice] = useErrorStore.getState().notices;
    expect(useToastStore.getState().current).toBeNull();
    expect(notice.title).toBe("保存失败");
    expect(notice.message).toBe("database is locked");
    expect(notice.requestId).toBe("req-1");
  });

  it("renames the global notice when the page reports the same error", () => {
    const e = apiError(500, "internal", "database is locked");
    reportError(e);
    reportError(e, { title: "保存笔记失败" });
    const notices = useErrorStore.getState().notices;
    expect(notices).toHaveLength(1);
    expect(notices[0].title).toBe("保存笔记失败");
    expect(notices[0].detail.startsWith("保存笔记失败\n")).toBe(true);
  });

  it("keeps the latest 50 in history", () => {
    for (let i = 0; i < 60; i++) reportError(new Error(`e${i}`));
    expect(useErrorStore.getState().history).toHaveLength(50);
    expect(useErrorStore.getState().history[0].message).toBe("e59");
  });
});
