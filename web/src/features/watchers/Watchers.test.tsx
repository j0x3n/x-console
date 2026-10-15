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

describe("出口（B208）", () => {
  const agents = [
    {
      id: "vm1",
      name: "Unraid 虚拟机",
      kind: "server",
      os: "linux",
      arch: "amd64",
      hostname: "vm",
      version: "1",
      online: true,
      capabilities: ["egress"],
      createdAt: "2026-10-01T00:00:00Z",
    },
    {
      id: "old",
      name: "旧机器",
      kind: "server",
      os: "linux",
      arch: "amd64",
      hostname: "old",
      version: "1",
      online: true,
      capabilities: [],
      createdAt: "2026-10-01T00:00:00Z",
    },
  ];

  it("creates a watcher that goes online via a connected machine", async () => {
    api.routes.set("GET /watchers", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("GET /watchers/status", () => ({ status: 404 }));
    api.routes.set("GET /agents", () => ({ status: 200, body: agents }));
    api.routes.set("POST /watchers", (body) => ({
      status: 201,
      body: { ...base, ...(body as object), id: 9 },
    }));
    renderAt("/monitoring/watchers?new=1", <WatchersPage />);
    await screen.findByText("还没有盯梢任务");
    fireEvent.change(screen.getByLabelText(/^(Name|名称)$/), {
      target: { value: "全链通" },
    });
    fireEvent.change(screen.getByLabelText(/^(Address|地址)$/), {
      target: { value: "https://www.jszwfw.gov.cn/" },
    });
    fireEvent.change(screen.getByLabelText("要盯什么"), {
      target: { value: "看办理进度" },
    });
    fireEvent.click(screen.getByText(/^(More options|更多设置)$/));
    const select = screen.getByLabelText("出口") as HTMLSelectElement;
    await screen.findByRole("option", { name: "Unraid 虚拟机" });
    // 不支持出口的机器不列出来
    expect(screen.queryByRole("option", { name: "旧机器" })).toBeNull();
    expect(screen.queryByLabelText("出口代理")).toBeNull();
    fireEvent.change(select, { target: { value: "vm1" } });
    fireEvent.click(screen.getByText(/^(Save|保存)$/));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toMatchObject({
        egressAgentId: "vm1",
      }),
    );
    expect(
      (api.calls.find((c) => c.method === "POST")?.body as object) ?? {},
    ).not.toHaveProperty("proxy");
  });

  it("shows the proxy field only when asked, and switching clears the old exit", async () => {
    const watcher = { ...base, egressAgentId: "vm1" };
    api.routes.set("GET /watchers/3", () => ({
      status: 200,
      body: { ...watcher, changes: [], runs: [], recipe: [] },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: agents }));
    api.routes.set("PATCH /watchers/3", (body) => ({
      status: 200,
      body: { ...watcher, ...(body as object) },
    }));
    renderAt(
      "/monitoring/watchers/3",
      <WatcherPage />,
      "/monitoring/watchers/:watcherId",
    );
    await screen.findByText("Unraid 虚拟机");
    fireEvent.click(screen.getByRole("button", { name: /^(Edit|编辑|修改)$/ }));
    fireEvent.click(await screen.findByText(/^(More options|更多设置)$/));
    const select = screen.getByLabelText("出口") as HTMLSelectElement;
    await waitFor(() => expect(select.value).toBe("vm1"));
    fireEvent.change(select, { target: { value: ":proxy" } });
    const save = screen.getByText(/^(Save|保存)$/).closest("button")!;
    // 没填代理地址不能保存
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("出口代理"), {
      target: { value: "socks5://10.0.0.2:1080" },
    });
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toMatchObject({
        proxy: "socks5://10.0.0.2:1080",
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
