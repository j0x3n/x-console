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

vi.mock("../../api/events", () => ({
  onServerEvent: () => () => {},
  invalidateOn: () => {},
  useServerEvent: () => {},
}));

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import GitHubSettingsTab from "./GitHubSettingsTab";

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <GitHubSettingsTab />
    </QueryClientProvider>,
  );
}

const config = {
  hasToken: true,
  token: "••••x9Qa",
  repos: ["j0x3n/x-console"],
  apiUrl: "https://api.github.com",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("GitHub settings", () => {
  it("falls back to typing repositories when the list is not live", async () => {
    api.routes.set("GET /github/config", () => ({ status: 200, body: config }));
    api.routes.set("GET /github/available-repos", () => ({
      status: 501,
      body: { code: "not_live", message: "not live" },
    }));
    renderTab();
    const box = (await screen.findByLabelText(
      "关注的仓库",
    )) as HTMLTextAreaElement;
    await waitFor(() => expect(box.tagName).toBe("TEXTAREA"));
    expect(box.value).toBe("j0x3n/x-console");
  });

  it("picks repositories from the token's list and saves them", async () => {
    let saved: unknown = null;
    api.routes.set("GET /github/config", () => ({ status: 200, body: config }));
    api.routes.set("GET /github/available-repos", () => ({
      status: 200,
      body: {
        fetchedAt: "2026-09-29T08:00:00Z",
        repos: [
          { fullName: "j0x3n/x-console", private: true },
          { fullName: "j0x3n/dotfiles", private: false, description: "配置" },
          { fullName: "acme/api", private: true },
        ],
      },
    }));
    api.routes.set("PUT /github/config", (body) => {
      saved = body;
      return {
        status: 200,
        body: { ...config, repos: (body as { repos: string[] }).repos },
      };
    });
    renderTab();

    // 已选的不再出现在下拉里。
    await screen.findByRole("button", { name: "移除 j0x3n/x-console" });
    expect(
      screen.queryByRole("option", { name: /j0x3n\/x-console/ }),
    ).toBeNull();
    fireEvent.change(screen.getByPlaceholderText("搜索仓库"), {
      target: { value: "acme" },
    });
    fireEvent.click(await screen.findByRole("option", { name: /acme\/api/ }));
    fireEvent.click(
      screen.getByRole("button", { name: "移除 j0x3n/x-console" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(saved).not.toBeNull());
    expect(saved).toMatchObject({ repos: ["acme/api"] });
  });
});
