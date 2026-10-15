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
import TodayWatchersCard from "./TodayWatchersCard";
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
    await screen.findByText("还没有网站自动化任务");
    fireEvent.change(screen.getByLabelText(/^(Name|名称)$/), {
      target: { value: "护照" },
    });
    fireEvent.change(screen.getByLabelText(/^(Address|地址)$/), {
      target: { value: "https://a.gov.cn/x" },
    });
    fireEvent.change(screen.getByLabelText("要 AI 做什么"), {
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
    await screen.findByText("还没有网站自动化任务");
    fireEvent.change(screen.getByLabelText(/^(Name|名称)$/), {
      target: { value: "全链通" },
    });
    fireEvent.change(screen.getByLabelText(/^(Address|地址)$/), {
      target: { value: "https://www.jszwfw.gov.cn/" },
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
    await screen.findByText("还没有网站自动化任务");
    fireEvent.change(screen.getByLabelText(/^(Name|名称)$/), {
      target: { value: "Alice" },
    });
    // B213：不用选类型，填 @账号 就认成 X
    fireEvent.change(screen.getByLabelText(/^(Address|地址)$/), {
      target: { value: "@alice" },
    });
    expect(screen.getByText("认出来是：X 账号")).toBeTruthy();
    expect(screen.queryByLabelText(/^(Password|密码)$/)).toBeNull();
    fireEvent.click(screen.getByText(/^(Save|保存)$/));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
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

describe("B234 cards, run history and screenshots", () => {
  const failedRun = (id: number) => ({
    id,
    kind: "explore",
    startedAt: `2026-10-14T0${id}:00:00Z`,
    outcome: "failed",
    detail: "今天已经提醒过你 3 次，明天再检查",
    steps: [
      { at: "2026-10-14T02:00:00Z", text: "打开网址" },
      { at: "2026-10-14T02:00:01Z", text: "等你处理：要登录", shotId: 40 + id },
    ],
    tokens: 120,
  });

  it("shows each watcher as a card with the latest value and change", async () => {
    api.routes.set("GET /watchers", () => ({
      status: 200,
      body: {
        items: [
          {
            ...base,
            lastChange: {
              summary: "办理状态变成审核中",
              important: true,
              createdAt: "2026-10-14T02:00:00Z",
            },
          },
          {
            ...base,
            id: 4,
            name: "公司注销",
            currentValue: "",
            status: "exploring",
          },
        ],
      },
    }));
    api.routes.set("GET /watchers/status", () => ({ status: 404 }));
    const { container } = renderAt("/monitoring/watchers", <WatchersPage />);
    await screen.findByText("签证进度");
    expect(container.querySelectorAll(".watchers-card")).toHaveLength(2);
    expect(screen.getByText("审核中")).toBeTruthy();
    expect(screen.getByText("办理状态变成审核中")).toBeTruthy();
    // 还没读到值时写现在在干什么
    expect(screen.getByText("AI 在找要盯的内容")).toBeTruthy();
  });

  it("folds the same failures, loads more and shows screenshots in the page", async () => {
    api.routes.set("GET /watchers/3", () => ({
      status: 200,
      body: {
        ...base,
        changes: [],
        runs: [],
        recipe: [],
        shots: { count: 4, bytes: 3 * 1024 * 1024 },
      },
    }));
    api.routes.set("GET /watchers/3/runs", () => {
      const before = api.calls.at(-1)?.path.match(/before=(\d+)/)?.[1];
      return before
        ? {
            status: 200,
            body: {
              items: [{ ...failedRun(2), outcome: "ok", detail: "找到了" }],
              hasMore: false,
            },
          }
        : {
            status: 200,
            body: {
              items: [failedRun(5), failedRun(4), failedRun(3)],
              hasMore: true,
            },
          };
    });
    renderAt(
      "/monitoring/watchers/3",
      <WatcherPage />,
      "/monitoring/watchers/:watcherId",
    );
    await screen.findByText("同样 3 次");
    expect(screen.getByText("截图 4 张，共 3.0 MB")).toBeTruthy();
    // 默认折叠，看不到步骤
    expect(screen.queryByText("等你处理：要登录")).toBeNull();

    fireEvent.click(screen.getByText("同样 3 次"));
    await screen.findByText("等你处理：要登录");
    fireEvent.click(screen.getByRole("button", { name: /看大图/ }));
    const box = await screen.findByRole("dialog", { name: "等你处理：要登录" });
    expect(box.querySelector("img")?.getAttribute("src")).toContain(
      "/watchers/shots/45",
    );
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "等你处理：要登录" }),
      ).toBeNull(),
    );

    fireEvent.click(screen.getByText(/^(Load more|加载更多)$/));
    await screen.findByText("找到了");
    expect(
      api.calls.some((c) => c.path === "/watchers/3/runs?before=3&limit=10"),
    ).toBe(true);
  });

  it("cleans up screenshots and deletes one run's screenshots", async () => {
    api.routes.set("GET /watchers/3", () => ({
      status: 200,
      body: {
        ...base,
        changes: [],
        runs: [],
        recipe: [],
        shots: { count: 4, bytes: 2048 },
      },
    }));
    api.routes.set("GET /watchers/3/runs", () => ({
      status: 200,
      body: { items: [failedRun(5)], hasMore: false },
    }));
    api.routes.set("POST /watchers/3/shots/cleanup", () => ({
      status: 200,
      body: { deleted: 3, bytes: 1536 },
    }));
    api.routes.set("DELETE /watchers/3/runs/5/shots", () => ({
      status: 200,
      body: { deleted: 1, bytes: 512 },
    }));
    renderAt(
      "/monitoring/watchers/3",
      <WatcherPage />,
      "/monitoring/watchers/:watcherId",
    );
    fireEvent.click(await screen.findByRole("button", { name: "清理截图" }));
    fireEvent.click(await screen.findByText("只留最近一次成功的"));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === "/watchers/3/shots/cleanup")?.body,
      ).toEqual({ keep: "latest" }),
    );

    fireEvent.click(screen.getByText("今天已经提醒过你 3 次，明天再检查"));
    fireEvent.click(await screen.findByText("删除截图"));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "DELETE" && c.path === "/watchers/3/runs/5/shots",
        ),
      ).toBe(true),
    );
  });

  it("lists watchers on the today page, the ones that need you first", async () => {
    const items = Array.from({ length: 8 }, (_, i) => ({
      ...base,
      id: i + 1,
      name: `任务 ${i + 1}`,
      status: i === 7 ? "waiting" : "ok",
    }));
    api.routes.set("GET /watchers", () => ({ status: 200, body: { items } }));
    const { container } = renderAt("/", <TodayWatchersCard />);
    await screen.findByText("任务 8");
    const rows = container.querySelectorAll("a.today-row");
    expect(rows[0].textContent).toContain("任务 8");
    expect(screen.getByText("还有 2 个")).toBeTruthy();
  });
});
