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
import "./i18n";
import "../settings/i18n";
import RemoteAccessTab, { setupSnippets } from "./RemoteAccessTab";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("setupSnippets", () => {
  it("puts the address and the secret in every client config", () => {
    const s = setupSnippets("https://x.example/api/v1/mcp", "xc_abc");
    expect(s.claude).toContain("--transport http");
    expect(s.claude).toContain("Bearer xc_abc");
    expect(s.codex).toContain('url = "https://x.example/api/v1/mcp"');
    expect(JSON.parse(s.json).mcpServers.xconsole.headers.Authorization).toBe(
      "Bearer xc_abc",
    );
  });
});

describe("RemoteAccessTab", () => {
  it("creates a token and shows the secret once", async () => {
    const tokens: unknown[] = [];
    api.routes.set("GET /api-tokens", () => ({ status: 200, body: tokens }));
    api.routes.set("GET /api-tokens/calls", () => ({ status: 200, body: [] }));
    api.routes.set("GET /api-tokens/tools", () => ({
      status: 200,
      body: {
        modules: ["notes", "projects"],
        tools: [{ name: "notes_search", title: "搜笔记", effect: "read" }],
      },
    }));
    api.routes.set("POST /api-tokens", (body) => {
      const token = {
        id: 1,
        prefix: "xc_abcde",
        modules: [],
        createdAt: "2026-10-01T00:00:00Z",
        ...(body as object),
      };
      tokens.push(token);
      return { status: 201, body: { token, secret: "xc_abcdefsecret" } };
    });
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <RemoteAccessTab />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: /新建令牌/ }));
    fireEvent.change(await screen.findByPlaceholderText(/笔记本上/), {
      target: { value: "笔记本" },
    });
    fireEvent.click(screen.getByLabelText(/只读/));
    fireEvent.click(await screen.findByRole("button", { name: "notes" }));
    fireEvent.click(screen.getByRole("button", { name: "创建令牌" }));
    await screen.findByText("xc_abcdefsecret");
    const post = api.calls.find((c) => c.method === "POST");
    expect(post?.body).toEqual({
      name: "笔记本",
      access: "read",
      modules: ["notes"],
      expiresInDays: 90,
    });
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    await waitFor(() =>
      expect(screen.queryByText("xc_abcdefsecret")).toBeNull(),
    );
    await screen.findByText("笔记本");
  });
});
