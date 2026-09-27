// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 页面之前替换。
const responses = vi.hoisted(() => {
  const routes = new Map<string, unknown>();
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
  // Node 的 Request 不接受相对地址，浏览器会按当前页面补全。
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
    calls.push({
      method: req.method,
      path,
      body: text ? JSON.parse(text) : null,
    });
    if (req.method !== "GET") return new Response(null, { status: 204 });
    const body = routes.get(path);
    return new Response(JSON.stringify(body ?? {}), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  return { routes, calls };
});

import "./i18n";
import HomePage from "./HomePage";

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <HomePage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  responses.routes.clear();
  responses.calls.length = 0;
});

describe("HomePage", () => {
  it("shows setup guidance when Home Assistant is not configured", async () => {
    responses.routes.set("/ha/status", {
      configured: false,
      connected: false,
      mode: "direct",
      entityCount: 0,
    });
    renderPage();
    const link = await screen.findByRole("link", { name: /去设置/ });
    expect(link.getAttribute("href")).toBe("/settings/homeassistant");
  });

  it("toggles a favorite light with one tap", async () => {
    responses.routes.set("/ha/status", {
      configured: true,
      connected: true,
      mode: "direct",
      entityCount: 1,
      version: "2026.9.1",
    });
    responses.routes.set("/ha/favorites", [
      {
        entityId: "light.kitchen",
        alias: "",
        sortOrder: 0,
        state: {
          entityId: "light.kitchen",
          state: "off",
          attributes: { friendly_name: "厨房灯" },
          lastChanged: "2026-09-27T00:00:00Z",
        },
      },
    ]);
    renderPage();
    const card = await screen.findByRole("button", { name: /厨房灯/ });
    expect(card.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(card);
    await waitFor(() =>
      expect(responses.calls).toContainEqual({
        method: "POST",
        path: "/ha/services/light/turn_on",
        body: { entityId: "light.kitchen" },
      }),
    );
  });
});
