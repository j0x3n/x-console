// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

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
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    const text = await req.text();
    const body = text ? JSON.parse(text) : null;
    calls.push({ method: req.method, path, body });
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

import VaultPanel from "./VaultPanel";
import { BRAND_TAP_EVENT, markVaultSession } from "./logic";

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <main id="main" className="main-panel" />
      <VaultPanel />
    </QueryClientProvider>,
  );
}

function tap(times: number) {
  act(() => {
    for (let i = 0; i < times; i++)
      window.dispatchEvent(new Event(BRAND_TAP_EVENT));
  });
}

const status = (body: Record<string, unknown>) => () => ({
  status: 200,
  body,
});

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  markVaultSession(false);
  vi.useRealTimers();
});

describe("vault panel", () => {
  it("shows nothing until the logo is tapped five times", async () => {
    api.routes.set(
      "GET /vault/status",
      status({ configured: true, unlocked: false }),
    );
    renderPanel();
    await waitFor(() => expect(api.calls.length).toBe(1));
    tap(4);
    expect(screen.queryByRole("dialog")).toBeNull();
    tap(1);
    expect(await screen.findByText("输入隐藏密码。")).toBeTruthy();
  });

  it("says the feature is not live when the server has no vault", async () => {
    renderPanel();
    await waitFor(() => expect(api.calls.length).toBe(1));
    tap(5);
    expect(await screen.findByText("这个功能还没上线。")).toBeTruthy();
  });

  it("sets a password the first time, then shows the bar and locks", async () => {
    let unlocked = false;
    api.routes.set("GET /vault/status", () => ({
      status: 200,
      body: { configured: unlocked, unlocked },
    }));
    api.routes.set("POST /vault/setup", () => {
      unlocked = true;
      return { status: 200, body: { configured: true, unlocked: true } };
    });
    api.routes.set("POST /vault/lock", () => {
      unlocked = false;
      return { status: 204 };
    });
    renderPanel();
    await waitFor(() => expect(api.calls.length).toBe(1));
    tap(5);
    fireEvent.change(await screen.findByLabelText("隐藏密码"), {
      target: { value: "123456" },
    });
    fireEvent.change(screen.getByLabelText("再输一次密码"), {
      target: { value: "123456" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    const bar = await screen.findByText("隐藏内容已显示");
    // 提示栏放在主面板里。
    expect(bar.closest("#main")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "锁定" }));
    await waitFor(() =>
      expect(screen.queryByText("隐藏内容已显示")).toBeNull(),
    );
    expect(api.calls.some((c) => c.path === "/vault/lock")).toBe(true);
  });

  it("locks right away when the page is reopened while unlocked", async () => {
    api.routes.set(
      "GET /vault/status",
      status({ configured: true, unlocked: true }),
    );
    api.routes.set("POST /vault/lock", () => ({ status: 204 }));
    renderPanel();
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/vault/lock")).toBe(true),
    );
  });

  it("stays unlocked after a refresh in the same tab", async () => {
    markVaultSession(true);
    api.routes.set(
      "GET /vault/status",
      status({ configured: true, unlocked: true }),
    );
    renderPanel();
    expect(await screen.findByText("隐藏内容已显示")).toBeTruthy();
    expect(api.calls.some((c) => c.path === "/vault/lock")).toBe(false);
  });

  it("locks after 15 minutes without activity", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    markVaultSession(true);
    api.routes.set(
      "GET /vault/status",
      status({ configured: true, unlocked: true }),
    );
    api.routes.set("POST /vault/lock", () => ({ status: 204 }));
    renderPanel();
    await screen.findByText("隐藏内容已显示");
    await act(async () => {
      vi.advanceTimersByTime(14 * 60 * 1000);
    });
    expect(api.calls.some((c) => c.path === "/vault/lock")).toBe(false);
    await act(async () => {
      vi.advanceTimersByTime(60 * 1000 + 15_000);
    });
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/vault/lock")).toBe(true),
    );
  });
});
