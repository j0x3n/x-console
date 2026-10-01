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
import RouterPage from "./RouterPage";
import RouterSettingsTab from "./RouterSettingsTab";
import "./i18n";

function renderIt(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const status = {
  hostname: "OpenWrt",
  model: "Xiaomi AX3600",
  firmware: "OpenWrt 24.10.0",
  uptimeSeconds: 90000,
  interfaces: [
    {
      name: "lan",
      up: true,
      ipv4: ["192.168.1.1/24"],
      ipv6: [],
      uptimeSeconds: 90000,
    },
    {
      name: "wan",
      up: true,
      proto: "pppoe",
      ipv4: ["100.64.1.2/32"],
      ipv6: [],
      uptimeSeconds: 3600,
    },
  ],
  wan: {
    name: "wan",
    up: true,
    proto: "pppoe",
    ipv4: ["100.64.1.2/32"],
    ipv6: [],
    uptimeSeconds: 3600,
  },
  rxRate: 2 * 1024 * 1024,
  txRate: 300 * 1024,
  clientCount: 2,
  checkedAt: "2026-10-01T12:00:00Z",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  localStorage.clear();
});

describe("RouterPage B65", () => {
  it("没配置时引导去设置", async () => {
    api.routes.set("GET /router/status", () => ({
      status: 412,
      body: { code: "integration_not_configured", message: "集成还没有配置" },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("连接 OpenWrt 路由器")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "设置路由器" }).getAttribute("href"),
    ).toBe("/settings/router");
  });

  it("显示概要、接口、在线设备，重启接口调接口", async () => {
    api.routes.set("GET /router/status", () => ({ status: 200, body: status }));
    api.routes.set("GET /router/clients", () => ({
      status: 200,
      body: {
        items: [
          { mac: "AA:BB:CC:00:00:03", name: "phone", ip: "192.168.1.3" },
          { mac: "AA:BB:CC:00:00:02", name: "nas", ip: "192.168.1.20" },
        ],
      },
    }));
    api.routes.set("POST /router/interfaces/wan/restart", () => ({
      status: 204,
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("在线")).toBeTruthy();
    expect(screen.getByText("100.64.1.2")).toBeTruthy();
    expect(screen.getByText("↓ 2.0 MB/s")).toBeTruthy();
    expect(await screen.findByText("phone")).toBeTruthy();
    expect(screen.getByText("AA:BB:CC:00:00:02")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "重启接口 wan" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path === "/router/interfaces/wan/restart"),
      ).toBe(true),
    );
  });

  it("流量标签按范围取数据", async () => {
    localStorage.setItem("router.tab", "traffic");
    api.routes.set("GET /router/status", () => ({ status: 200, body: status }));
    api.routes.set("GET /router/traffic", () => ({
      status: 200,
      body: {
        range: "7d",
        stepSeconds: 3600,
        points: [],
        rxBytes: 0,
        txBytes: 0,
      },
    }));
    renderIt(<RouterPage />);
    expect(await screen.findByText("还没有流量数据")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "最近 7 天" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/router/traffic?range=7d")).toBe(
        true,
      ),
    );
  });
});

describe("RouterSettingsTab B65", () => {
  it("保存时带上表单，密码留空不传", async () => {
    api.routes.set("GET /router/config", () => ({
      status: 200,
      body: {
        url: "http://192.168.1.1",
        username: "xconsole",
        mode: "direct",
        hasPassword: true,
      },
    }));
    api.routes.set("GET /agents", () => ({ status: 200, body: [] }));
    api.routes.set("PUT /router/config", (body) => ({
      status: 200,
      body: { ...(body as object), hasPassword: true },
    }));
    renderIt(<RouterSettingsTab />);
    expect(await screen.findByDisplayValue("http://192.168.1.1")).toBeTruthy();
    expect(screen.getByPlaceholderText("留空表示不改")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "保存并检查" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    const put = api.calls.find((c) => c.method === "PUT");
    expect(put?.body).toEqual({
      url: "http://192.168.1.1",
      username: "xconsole",
      mode: "direct",
    });
  });
});
