// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 组件之前替换。
const api = vi.hoisted(() => {
  type Reply = { status: number; body?: unknown };
  const routes = new Map<string, (body: unknown) => Reply>();
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
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
    const url = new URL(req.url);
    const path = url.pathname.replace("/api/v1", "");
    const text = await req.text();
    const body = text ? JSON.parse(text) : null;
    calls.push({ method: req.method, path: path + url.search, body });
    const handler = routes.get(`${req.method} ${path}`);
    const reply = handler
      ? handler(body)
      : { status: 404, body: { code: "not_found", message: "not found" } };
    if (reply.body === undefined)
      return new Response(null, { status: reply.status });
    return new Response(JSON.stringify(reply.body), {
      status: reply.status,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  return { routes, calls };
});

vi.mock("../../api/events", () => ({
  onServerEvent: () => () => {},
  invalidateOn: () => {},
  useServerEvent: () => {},
}));
vi.mock("../../components/ui/ConfirmDialog", () => ({
  confirmAction: async () => true,
}));
vi.mock("../../auth/elevation", () => ({
  withElevation: <T,>(fn: () => Promise<T>) => fn(),
}));

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import WatchersPage from "./WatchersPage";
import WatcherPage from "./WatcherPage";
import "./i18n";

function renderAt(path: string, node: React.ReactNode, pattern = "*") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path={pattern} element={node} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  id: 3,
  kind: "web",
  name: "签证进度",
  url: "https://example.gov.cn/status",
  goal: "盯办理状态",
  allowedHosts: ["example.gov.cn"],
  intervalMinutes: 60,
  keepaliveMinutes: 0,
  mode: "replay",
  enabled: true,
  status: "ok",
  statusDetail: "",
  currentValue: "审核中",
  hasLogin: true,
  hasProxy: false,
  hasRecipe: true,
  createdAt: "2026-10-14T01:00:00Z",
  lastCheckedAt: "2026-10-14T02:00:00Z",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("WatchersPage", () => {
  it("lists watchers, puts waiting ones first and explains the browser", async () => {
    api.routes.set("GET /watchers", () => ({
      status: 200,
      body: {
        items: [
          base,
          {
            ...base,
            id: 4,
            name: "社保",
            status: "waiting",
            currentValue: "",
            pending: {
              kind: "captcha",
              message: "请输入验证码",
              since: "2026-10-14T02:00:00Z",
              live: true,
            },
          },
        ],
      },
    }));
    api.routes.set("GET /watchers/status", () => ({
      status: 200,
      body: {
        browser: false,
        browserConfigured: true,
        ai: true,
        spentToday: 0.12,
        budgetUsd: 1,
      },
    }));
    renderAt("/monitoring/watchers", <WatchersPage />);
    await screen.findByText("签证进度");
    const names = screen
      .getAllByText(/签证进度|社保/)
      .map((el) => el.textContent);
    expect(names[0]).toBe("社保");
    expect(screen.getByText("去处理")).toBeTruthy();
    expect(screen.getByText("服务器上的浏览器没有启动")).toBeTruthy();
    expect(screen.getByText("$0.120")).toBeTruthy();

    api.routes.set("POST /watchers/3/run", () => ({ status: 202 }));
    fireEvent.click(screen.getByText(/Check now|立即检查/));
    await waitFor(() =>
      expect(api.calls.find((c) => c.path === "/watchers/3/run")?.body).toEqual(
        { kind: "check" },
      ),
    );
  });

  it("creates a watcher with login details", async () => {
    api.routes.set("GET /watchers", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("GET /watchers/status", () => ({ status: 404 }));
    api.routes.set("POST /watchers", (body) => ({
      status: 201,
      body: { ...base, ...(body as object), id: 9 },
    }));
    renderAt("/monitoring/watchers?new=1", <WatchersPage />);
    await screen.findByText("还没有盯梢任务");
    fireEvent.change(screen.getByLabelText(/^(Name|名称)$/), {
      target: { value: "护照" },
    });
    fireEvent.change(screen.getByLabelText(/^(Address|地址)$/), {
      target: { value: "https://a.gov.cn/x" },
    });
    fireEvent.change(screen.getByLabelText("要盯什么"), {
      target: { value: "看护照进度" },
    });
    fireEvent.change(screen.getByLabelText(/^(Username|用户名)$/), {
      target: { value: "alice" },
    });
    fireEvent.change(screen.getByLabelText(/^(Password|密码)$/), {
      target: { value: "pw" },
    });
    fireEvent.click(screen.getByText(/^(Save|保存)$/));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        name: "护照",
        url: "https://a.gov.cn/x",
        goal: "看护照进度",
        intervalMinutes: 60,
        keepaliveMinutes: 0,
        mode: "replay",
        username: "alice",
        password: "pw",
      }),
    );
  });
});

describe("X watchers", () => {
  it("creates an X watcher without login fields", async () => {
    api.routes.set("GET /watchers", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("GET /watchers/status", () => ({ status: 404 }));
    api.routes.set("POST /watchers", (body) => ({
      status: 201,
      body: { ...base, ...(body as object), id: 10 },
    }));
    renderAt("/monitoring/watchers?new=1", <WatchersPage />);
    await screen.findByText("还没有盯梢任务");
    fireEvent.change(screen.getByLabelText("盯什么"), {
      target: { value: "x" },
    });
    expect(screen.queryByLabelText(/^(Password|密码)$/)).toBeNull();
    fireEvent.change(screen.getByLabelText(/^(Name|名称)$/), {
      target: { value: "Alice" },
    });
    fireEvent.change(screen.getByLabelText("账号、推文或搜索词"), {
      target: { value: "@alice" },
    });
    fireEvent.click(screen.getByText(/^(Save|保存)$/));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        kind: "x",
        name: "Alice",
        url: "@alice",
        goal: "",
        intervalMinutes: 30,
      }),
    );
  });
});

describe("WatcherPage", () => {
  it("shows the value, changes, steps and the approval card", async () => {
    api.routes.set("GET /watchers/3", () => ({
      status: 200,
      body: {
        ...base,
        status: "waiting",
        pending: {
          kind: "approve",
          message: "AI 要点“撤销申请”，批准吗？",
          since: "2026-10-14T02:00:00Z",
          live: true,
        },
        changes: [
          {
            id: 1,
            oldValue: "待受理",
            newValue: "审核中",
            summary: "办理状态变成审核中",
            important: true,
            createdAt: "2026-10-14T02:00:00Z",
          },
        ],
        runs: [
          {
            id: 7,
            kind: "check",
            startedAt: "2026-10-14T02:00:00Z",
            outcome: "changed",
            detail: "当前值：审核中",
            steps: [{ at: "2026-10-14T02:00:00Z", text: "打开网址" }],
            tokens: 0,
          },
        ],
        recipe: [{ text: "打开网址" }, { text: "填用户名" }],
        targetNote: "办理状态那一行",
      },
    }));
    renderAt(
      "/monitoring/watchers/3",
      <WatcherPage />,
      "/monitoring/watchers/:watcherId",
    );
    await screen.findByText("办理状态变成审核中");
    expect(screen.getAllByText("审核中").length).toBeGreaterThan(0);
    expect(screen.getByText("填用户名")).toBeTruthy();
    api.routes.set("POST /watchers/3/answer", () => ({ status: 204 }));
    fireEvent.click(screen.getByText("不让点"));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === "/watchers/3/answer")?.body,
      ).toEqual({ answer: "deny" }),
    );
  });
});
