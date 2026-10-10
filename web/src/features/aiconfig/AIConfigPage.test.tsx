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
vi.mock("../../hooks/useToast", () => ({ toast: () => {} }));

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
import AIConfigPage from "./AIConfigPage";
import "./i18n";

function renderIt() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/coding/config"]}>
        <AIConfigPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const emptyTool = { rules: "", allow: [], ask: [], deny: [], mcp: [] };
const hosts = [
  { id: "h1", name: "hk-1", online: true, supported: true },
  { id: "h2", name: "办公室电脑", online: true, supported: true },
  { id: "h3", name: "旧服务器", online: true, supported: false },
];
const config = (patch: Record<string, unknown> = {}) => ({
  claude: {
    ...emptyTool,
    rules: "提交信息用中文。",
    allow: ["Bash(git status)"],
  },
  codex: emptyTool,
  hostIds: ["h1", "h2"],
  hosts,
  updatedAt: "2026-10-10T08:00:00Z",
  ...patch,
});
const status = {
  hosts: [
    {
      hostId: "h1",
      name: "hk-1",
      state: "ok",
      items: [{ tool: "claude", item: "rules", state: "ok" }],
      checkedAt: "2026-10-10T08:00:00Z",
    },
    {
      hostId: "h2",
      name: "办公室电脑",
      state: "conflict",
      items: [
        { tool: "claude", item: "rules", state: "drift", reason: "missing" },
        {
          tool: "claude",
          item: "mcp",
          state: "conflict",
          reason: "exists",
          names: ["fs"],
        },
      ],
      checkedAt: "2026-10-10T08:00:00Z",
    },
  ],
};

function serve(cfg = config()) {
  api.routes.set("GET /aiconfig", () => ({ status: 200, body: cfg }));
  api.routes.set("GET /aiconfig/status", () => ({ status: 200, body: status }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("AIConfigPage B121", () => {
  it("显示机器和每台的状态，冲突写清原因", async () => {
    serve();
    renderIt();
    await screen.findByText("hk-1");
    await screen.findByText("一致");
    expect(screen.getByText("冲突")).toBeTruthy();
    expect(
      screen.getByText(/已有同名服务器，不是面板写的，不会覆盖/),
    ).toBeTruthy();
    expect(screen.getByText(/（fs）/)).toBeTruthy();
    // 不支持的机器不能勾选
    const old = screen.getByRole("checkbox", { name: "旧服务器" });
    expect((old as HTMLInputElement).disabled).toBe(true);
  });

  it("改了内容要先保存才能下发，保存带上整理过的内容", async () => {
    serve();
    api.routes.set("PUT /aiconfig", (body) => ({
      status: 200,
      body: { ...config(), ...(body as object) },
    }));
    renderIt();
    await screen.findByText("hk-1");
    const deliver = screen.getByRole("button", { name: "全部下发" });
    expect((deliver as HTMLButtonElement).disabled).toBe(false);

    fireEvent.change(screen.getByLabelText("允许"), {
      target: { value: " Bash(git status) \n\nBash(ls)\nBash(ls)\n" },
    });
    expect(screen.getByText("有没保存的修改")).toBeTruthy();
    expect((deliver as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    const put = api.calls.find((c) => c.method === "PUT")!;
    expect(put.body).toMatchObject({
      claude: {
        allow: ["Bash(git status)", "Bash(ls)"],
        rules: "提交信息用中文。",
      },
      hostIds: ["h1", "h2"],
    });
    // Codex 没有权限设置
    expect((put.body as { codex: { allow: string[] } }).codex.allow).toEqual(
      [],
    );
    await waitFor(() =>
      expect(screen.queryByText("有没保存的修改")).toBeNull(),
    );
  });

  it("勾选机器，加 MCP 服务器，保存时带上", async () => {
    serve(config({ hostIds: ["h1"] }));
    api.routes.set("PUT /aiconfig", (body) => ({
      status: 200,
      body: { ...config(), ...(body as object) },
    }));
    renderIt();
    await screen.findByText("hk-1");
    fireEvent.click(screen.getByRole("checkbox", { name: "办公室电脑" }));

    const codex = screen.getByRole("region", { name: "Codex" });
    fireEvent.click(within(codex).getByRole("button", { name: /添加服务器/ }));
    fireEvent.change(within(codex).getByLabelText("服务器名称"), {
      target: { value: "fs" },
    });
    fireEvent.change(within(codex).getByLabelText("命令"), {
      target: { value: "npx" },
    });
    fireEvent.change(within(codex).getByLabelText("参数"), {
      target: { value: "-y\nserver-fs" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")!.body).toMatchObject({
      hostIds: ["h1", "h2"],
      codex: {
        mcp: [
          {
            name: "fs",
            transport: "stdio",
            command: "npx",
            args: ["-y", "server-fs"],
          },
        ],
      },
    });
  });

  it("远程服务器填地址，不填命令；可以移除", async () => {
    serve(
      config({
        claude: {
          ...emptyTool,
          mcp: [
            { name: "docs", transport: "http", url: "https://x.example/mcp" },
          ],
        },
      }),
    );
    renderIt();
    await screen.findByText("hk-1");
    const claude = screen.getByRole("region", { name: "Claude Code" });
    expect(within(claude).getByLabelText("地址")).toBeTruthy();
    expect(within(claude).queryByLabelText("命令")).toBeNull();
    fireEvent.click(
      within(claude).getByRole("button", { name: /移除服务器 docs/ }),
    );
    expect(within(claude).getByText("没有服务器。")).toBeTruthy();
  });

  it("下发前要确认，下发一台只发这一台", async () => {
    serve();
    api.routes.set("POST /aiconfig/apply", () => ({
      status: 200,
      body: {
        hosts: [
          {
            ...status.hosts[0],
            items: [
              { tool: "claude", item: "rules", state: "ok", changed: true },
            ],
          },
        ],
      },
    }));
    renderIt();
    await screen.findByText("hk-1");
    fireEvent.click(screen.getByRole("button", { name: "下发 hk-1" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/aiconfig/apply")).toBe(true),
    );
    expect(api.calls.find((c) => c.path === "/aiconfig/apply")!.body).toEqual({
      hostIds: ["h1"],
    });

    fireEvent.click(screen.getByRole("button", { name: "全部下发" }));
    await waitFor(() =>
      expect(
        api.calls.filter((c) => c.path === "/aiconfig/apply"),
      ).toHaveLength(2),
    );
    expect(
      api.calls.filter((c) => c.path === "/aiconfig/apply")[1].body,
    ).toEqual({});
  });

  it("没有配对的机器时引导去设置", async () => {
    serve(config({ hosts: [], hostIds: [] }));
    renderIt();
    await screen.findByText("还没有配对的机器");
    expect(api.calls.some((c) => c.path === "/aiconfig/status")).toBe(false);
  });
});
