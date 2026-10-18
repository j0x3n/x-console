// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 组件之前替换。
const api = vi.hoisted(() => {
  const state = { tasks: [] as unknown[] };
  const calls: Array<{ method: string; path: string }> = [];
  const BaseRequest = globalThis.Request;
  globalThis.Request = class extends BaseRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(
        typeof input === "string" && input.startsWith("/")
          ? `http://localhost${input}`
          : input,
        init,
      );
    }
  } as typeof Request;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const req = input as Request;
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    calls.push({ method: req.method, path });
    const json = (body: unknown) =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    if (req.method === "GET" && path === "/coding/tasks")
      return json({ items: state.tasks });
    if (req.method === "POST" && /^\/coding\/tasks\/\d+\/cancel$/.test(path))
      return json({ ...(state.tasks[0] as object), status: "canceled" });
    return new Response(null, { status: 404 });
  }) as typeof fetch;
  return { state, calls };
});

vi.mock("../../api/events", () => ({
  onServerEvent: () => () => {},
  invalidateOn: () => {},
  useServerEvent: () => {},
  useEventTopic: () => {},
}));
const confirm = vi.hoisted(() => ({ answer: true, titles: [] as string[] }));
vi.mock("../../components/ui/ConfirmDialog", () => ({
  confirmAction: async (o: { title: string }) => {
    confirm.titles.push(o.title);
    return confirm.answer;
  },
}));

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import BackgroundButton from "./BackgroundButton";
import {
  registerHandle,
  removeSession,
  useTerminals,
  type TermInfo,
} from "./terminal/store";

const task = (id: number, status: string) => ({
  id,
  repoId: 1,
  repoName: "x-console",
  agentId: "a",
  executor: "codex",
  prompt: "",
  title: `任务${id}`,
  baseBranch: "main",
  branch: "",
  status,
  error: "",
  commitSha: "",
  prUrl: "",
  changedFiles: [],
  timeoutMinutes: 30,
  createdAt: new Date(Date.now() - 5 * 60000).toISOString(),
  startedAt: new Date(Date.now() - 12 * 60000).toISOString(),
  updatedAt: new Date().toISOString(),
});

const closed: string[] = [];
function park(hostId: string, minutesAgo: number) {
  const info: TermInfo = {
    hostId,
    hostName: `机器${hostId}`,
    path: `/servers/${hostId}/terminal`,
    status: "open",
    title: "",
    parkedAt: Date.now() - minutesAgo * 60000,
  };
  useTerminals.setState((s) => ({
    sessions: { ...s.sessions, [hostId]: info },
  }));
  registerHandle(hostId, {
    close: () => {
      closed.push(hostId);
      removeSession(hostId);
    },
    lastOutputAt: () => 0,
  });
}

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <QueryClientProvider client={qc}>
            <BackgroundButton />
          </QueryClientProvider>
        ),
      },
    ],
    { initialEntries: ["/notes"] },
  );
  render(<RouterProvider router={router} />);
  return router;
}

beforeEach(() => {
  api.state.tasks = [];
  api.calls.length = 0;
  closed.length = 0;
  confirm.answer = true;
  confirm.titles = [];
  useTerminals.setState({ sessions: {} });
});
afterEach(cleanup);

describe("BackgroundButton (B249)", () => {
  it("is hidden when nothing runs in the background", async () => {
    setup();
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/coding/tasks")).toBe(true),
    );
    expect(screen.queryByRole("button", { name: /后台/ })).toBeNull();
  });

  it("counts background terminals and running agent tasks", async () => {
    park("h1", 12);
    api.state.tasks = [task(7, "running"), task(8, "queued")];
    setup();
    const button = await screen.findByRole("button", { name: "后台 3 个" });
    expect(button.textContent).toBe("3");
    fireEvent.click(button);
    expect(screen.getByText("机器h1")).toBeTruthy();
    expect(screen.getByText("终端 · 12 分钟")).toBeTruthy();
    expect(screen.getByText("任务7")).toBeTruthy();
    expect(screen.getByText("x-console · 运行中 · 12 分钟")).toBeTruthy();
    expect(screen.getByText("任务8")).toBeTruthy();
  });

  it("jumps back to the terminal or the task", async () => {
    park("h1", 1);
    api.state.tasks = [task(7, "running")];
    const router = setup();
    fireEvent.click(await screen.findByRole("button", { name: "后台 2 个" }));
    fireEvent.click(screen.getByText("机器h1"));
    expect(router.state.location.pathname).toBe("/servers/h1/terminal");
    expect(screen.queryByText("任务7")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "后台 2 个" }));
    fireEvent.click(screen.getByText("任务7"));
    expect(router.state.location.pathname).toBe("/coding/7");
  });

  it("confirms before disconnecting a terminal", async () => {
    park("h1", 1);
    park("h2", 2);
    setup();
    fireEvent.click(await screen.findByRole("button", { name: "后台 2 个" }));
    confirm.answer = false;
    fireEvent.click(
      screen.getByRole("button", { name: "断开 机器h1 的终端？" }),
    );
    await act(() => Promise.resolve());
    expect(confirm.titles).toEqual(["断开 机器h1 的终端？"]);
    expect(closed).toEqual([]);

    confirm.answer = true;
    fireEvent.click(
      screen.getByRole("button", { name: "断开 机器h1 的终端？" }),
    );
    await waitFor(() => expect(closed).toEqual(["h1"]));
    expect(screen.getByRole("button", { name: "后台 1 个" })).toBeTruthy();
  });

  it("confirms before canceling a task", async () => {
    api.state.tasks = [task(7, "running")];
    setup();
    fireEvent.click(await screen.findByRole("button", { name: "后台 1 个" }));
    fireEvent.click(screen.getByRole("button", { name: "取消任务 任务7？" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "POST" && c.path === "/coding/tasks/7/cancel",
        ),
      ).toBe(true),
    );
    expect(confirm.titles).toEqual(["取消任务 任务7？"]);
  });

  it("shows only a dot above 9", async () => {
    for (let i = 0; i < 4; i++) park(`h${i}`, i);
    api.state.tasks = Array.from({ length: 6 }, (_, i) =>
      task(i + 1, "running"),
    );
    setup();
    const button = await screen.findByRole("button", { name: "后台 10 个" });
    expect(button.textContent).toBe("");
    expect(button.querySelector(".bg-count.dot")).toBeTruthy();
  });
});
