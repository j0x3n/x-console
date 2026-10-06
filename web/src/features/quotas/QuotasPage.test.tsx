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
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import QuotasPage from "./QuotasPage";
import TodayQuotasCard from "./TodayQuotasCard";
import "./i18n";

function renderIt(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const inMinutes = (m: number) =>
  new Date(Date.now() + m * 60_000).toISOString();

const claude = {
  id: 1,
  kind: "claude",
  name: "个人 Claude",
  hostId: "h1",
  hostName: "办公室电脑",
  hostOnline: true,
  home: "",
  keySet: false,
  status: "ok",
  plan: "max",
  user: "me@example.com",
  windows: [
    {
      name: "5 小时",
      usedPercent: 40,
      resetsAt: inMinutes(134),
      spanSecs: 18000,
    },
    { name: "7 天", usedPercent: 97, resetsAt: inMinutes(3 * 1440) },
  ],
  balances: [],
  readAt: new Date().toISOString(),
  createdAt: "2026-10-01T00:00:00Z",
};
const codexOffline = {
  id: 2,
  kind: "codex",
  name: "公司 Codex",
  hostId: "h2",
  hostName: "服务器",
  hostOnline: false,
  home: "/home/me/.codex-work",
  keySet: false,
  status: "error",
  errorCode: "offline",
  error: "机器离线",
  windows: [{ name: "5 小时", usedPercent: 10 }],
  balances: [],
  readAt: inMinutes(-12),
  createdAt: "2026-10-01T00:00:00Z",
};
const deepseek = {
  id: 3,
  kind: "deepseek",
  name: "DeepSeek 主号",
  hostId: "",
  home: "",
  keySet: true,
  status: "ok",
  windows: [],
  balances: [
    { currency: "CNY", amount: "110.00" },
    { currency: "USD", amount: "3.5" },
  ],
  readAt: new Date().toISOString(),
  createdAt: "2026-10-01T00:00:00Z",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("QuotasPage B111", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt(<QuotasPage />);
    expect(await screen.findByText("AI 额度还没上线")).toBeTruthy();
  });

  it("没有账号时引导添加，DeepSeek 账号带上 Key 创建", async () => {
    api.routes.set("GET /quotas", () => ({ status: 200, body: { items: [] } }));
    api.routes.set("POST /quotas", (body) => ({
      status: 201,
      body: { ...deepseek, name: (body as { name: string }).name },
    }));
    renderIt(<QuotasPage />);
    expect(await screen.findByText("还没有额度账号")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "添加账号" })[0]!);
    const dialog = screen.getByRole("dialog");
    const save = within(dialog).getByRole("button", { name: "保存" });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("服务"), {
      target: { value: "deepseek" },
    });
    fireEvent.change(within(dialog).getByLabelText("备注名"), {
      target: { value: "备用" },
    });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("DeepSeek API Key"), {
      target: { value: "sk-abc" },
    });
    expect((save as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
      kind: "deepseek",
      name: "备用",
      apiKey: "sk-abc",
    });
  });

  it("Claude 账号要选机器，目录可以留空", async () => {
    api.routes.set("GET /quotas", () => ({ status: 200, body: { items: [] } }));
    api.routes.set("GET /quotas/hosts", () => ({
      status: 200,
      body: {
        items: [
          { id: "h1", name: "办公室电脑", online: true },
          { id: "h2", name: "服务器", online: false },
        ],
      },
    }));
    api.routes.set("POST /quotas", () => ({ status: 201, body: claude }));
    renderIt(<QuotasPage />);
    await screen.findByText("还没有额度账号");
    fireEvent.click(screen.getAllByRole("button", { name: "添加账号" })[0]!);
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("备注名"), {
      target: { value: "个人" },
    });
    await within(dialog).findByRole("option", { name: "服务器 (离线)" });
    fireEvent.change(within(dialog).getByLabelText("机器"), {
      target: { value: "h2" },
    });
    fireEvent.change(within(dialog).getByLabelText("登录目录"), {
      target: { value: "~/.claude-work" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
      kind: "claude",
      name: "个人",
      hostId: "h2",
      home: "~/.claude-work",
    });
  });

  it("显示窗口剩余、重置倒计时、余额，离线的账号变灰并保留旧数字", async () => {
    api.routes.set("GET /quotas", () => ({
      status: 200,
      body: { items: [claude, codexOffline, deepseek] },
    }));
    renderIt(<QuotasPage />);
    const card1 = await screen.findByRole("article", { name: "个人 Claude" });
    expect(within(card1).getByText("剩余 60%")).toBeTruthy();
    expect(
      within(card1).getByText(/2 小时 14 分后重置|2 小时 13 分后重置/),
    ).toBeTruthy();
    expect(within(card1).getByText("剩余 3%")).toBeTruthy();
    expect(within(card1).getByText("max")).toBeTruthy();
    expect(within(card1).getByText(/办公室电脑 · me@example.com/)).toBeTruthy();

    const card2 = screen.getByRole("article", { name: "公司 Codex" });
    expect(card2.className).toContain("is-stale");
    expect(
      within(card2).getByText("机器离线", { selector: ".xc-badge" }),
    ).toBeTruthy();
    expect(within(card2).getByText("剩余 90%")).toBeTruthy();
    expect(
      within(card2).getByText("显示的是上一次读取成功的数字。"),
    ).toBeTruthy();

    const card3 = screen.getByRole("article", { name: "DeepSeek 主号" });
    expect(within(card3).getByText("110.00")).toBeTruthy();
    expect(within(card3).getByText("3.5")).toBeTruthy();

    // 概要：3 个账号，最少剩余 3%
    expect(screen.getByText("1 个有问题", { exact: false })).toBeTruthy();
    expect(screen.getAllByText("3%").length).toBeGreaterThan(0);
  });

  it("刷新一个账号，删除要先确认", async () => {
    api.routes.set("GET /quotas", () => ({
      status: 200,
      body: { items: [claude, deepseek] },
    }));
    api.routes.set("POST /quotas/1/refresh", () => ({
      status: 200,
      body: claude,
    }));
    api.routes.set("DELETE /quotas/3", () => ({ status: 204 }));
    renderIt(<QuotasPage />);
    const card = await screen.findByRole("article", { name: "个人 Claude" });
    fireEvent.click(within(card).getByRole("button", { name: "刷新" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/quotas/1/refresh")).toBe(true),
    );

    const ds = screen.getByRole("article", { name: "DeepSeek 主号" });
    fireEvent.click(within(ds).getByRole("button", { name: "更多" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "删除" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === "DELETE" && c.path === "/quotas/3"),
      ).toBe(true),
    );
  });

  it("全部刷新对每个账号请求一次", async () => {
    api.routes.set("GET /quotas", () => ({
      status: 200,
      body: { items: [claude, deepseek] },
    }));
    api.routes.set("POST /quotas/1/refresh", () => ({
      status: 200,
      body: claude,
    }));
    api.routes.set("POST /quotas/3/refresh", () => ({
      status: 200,
      body: deepseek,
    }));
    renderIt(<QuotasPage />);
    fireEvent.click(await screen.findByRole("button", { name: "全部刷新" }));
    await waitFor(() =>
      expect(api.calls.filter((c) => c.path.endsWith("/refresh")).length).toBe(
        2,
      ),
    );
  });
});

describe("TodayQuotasCard B111", () => {
  it("每个账号一行：最紧张的窗口，或余额", async () => {
    api.routes.set("GET /quotas", () => ({
      status: 200,
      body: { items: [claude, deepseek] },
    }));
    renderIt(<TodayQuotasCard />);
    expect(await screen.findByText("个人 Claude")).toBeTruthy();
    expect(screen.getByText("剩余 3%")).toBeTruthy();
    expect(screen.getByText("110.00 CNY")).toBeTruthy();
    for (const link of screen.getAllByRole("link"))
      expect(link.getAttribute("href")).toBe("/quotas");
  });
});
