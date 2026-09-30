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
import ElevationModeCard from "./ElevationModeCard";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("ElevationModeCard", () => {
  it("saves the picked mode and warns when it is off", async () => {
    let mode = "always";
    api.routes.set("GET /auth/elevation-mode", () => ({
      status: 200,
      body: { mode },
    }));
    api.routes.set("PUT /auth/elevation-mode", (body) => {
      mode = (body as { mode: string }).mode;
      return { status: 200, body: { mode } };
    });
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <ElevationModeCard />
      </QueryClientProvider>,
    );
    const always = await screen.findByLabelText(/每次/);
    await waitFor(() =>
      expect((always as HTMLInputElement).checked).toBe(true),
    );
    expect(screen.queryByRole("note")).toBeNull();
    fireEvent.click(screen.getByLabelText(/关闭/));
    await screen.findByRole("note");
    expect(api.calls.some((c) => c.method === "PUT")).toBe(true);
    expect(mode).toBe("off");
  });
});
