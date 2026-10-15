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
import { MemoryRouter } from "react-router";
import DevicesTab from "../settings/DevicesTab";
import "./i18n";

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/settings/devices"]}>
        <DevicesTab />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const agent = (id: string, name: string, version: string) => ({
  id,
  name,
  kind: "server",
  os: "linux",
  arch: "amd64",
  hostname: `${name}.lan`,
  version,
  online: true,
  capabilities: [],
  createdAt: "2026-10-01T00:00:00Z",
});

function setup(items: unknown[]) {
  api.routes.set("GET /agents", () => ({
    status: 200,
    body: [
      agent("a1", "unraid-vm", "v1"),
      agent("a2", "old-box", "v0"),
      agent("a3", "fresh", "v2"),
    ],
  }));
  api.routes.set("GET /agent-updates", () => ({
    status: 200,
    body: { latest: "v2", items },
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("代理自动更新（B233）", () => {
  it("marks outdated agents, updates one and turns auto update off", async () => {
    setup([
      {
        agentId: "a1",
        version: "v1",
        autoUpdate: true,
        canUpdate: true,
        state: "idle",
      },
      {
        agentId: "a2",
        version: "v0",
        autoUpdate: true,
        canUpdate: false,
        state: "idle",
      },
      {
        agentId: "a3",
        version: "v2",
        autoUpdate: true,
        canUpdate: true,
        state: "idle",
      },
    ]);
    api.routes.set("POST /agent-updates/a1", () => ({ status: 202 }));
    api.routes.set("PUT /agent-updates/a1/auto", () => ({ status: 204 }));
    renderTab();
    await screen.findByText("unraid-vm");
    // 只有 a1 能更新：a2 太旧，a3 已经是最新
    await waitFor(() => expect(screen.getAllByText("可更新").length).toBe(1));
    expect(screen.getByText(/这台代理不能自己更新/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /更新$/ }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.path).toBe(
        "/agent-updates/a1",
      ),
    );
    const switches = screen.getAllByLabelText("这台机器自动更新 开关");
    expect(switches.length).toBe(3);
    // 太旧的代理不能开关
    expect((switches[1] as HTMLInputElement).disabled).toBe(true);
    fireEvent.click(switches[0]);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        on: false,
      }),
    );
  });

  it("shows an update in progress and a failure", async () => {
    setup([
      {
        agentId: "a1",
        version: "v1",
        autoUpdate: true,
        canUpdate: true,
        state: "updating",
      },
      {
        agentId: "a2",
        version: "v0",
        autoUpdate: false,
        canUpdate: true,
        state: "failed",
        error: "新程序跑不起来",
      },
    ]);
    renderTab();
    await screen.findByText("正在更新");
    expect(screen.getByText(/更新失败：新程序跑不起来/)).toBeTruthy();
  });
});
