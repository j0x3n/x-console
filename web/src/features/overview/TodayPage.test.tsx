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
const api = vi.hoisted(() => {
  const routes = new Map<string, { status: number; body: unknown }>();
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
  const empty: Record<string, unknown> = {
    "/auth/status": {
      setupRequired: false,
      authenticated: true,
      username: "jo",
    },
    "/issues": { items: [] },
    "/reminders": { items: [] },
    "/coding/tasks": { items: [] },
    "/habits/today": [],
    "/hosts": [],
    "/calendar/events": [],
    "/notifications": { items: [], unreadCount: 0 },
    "/ha/favorites": [],
    "/weather": {
      latitude: 0,
      longitude: 0,
      temperature: 21.4,
      weatherCode: 2,
      summary: "多云",
      high: 25,
      low: 16,
      precipitationChance: 10,
      fetchedAt: "2026-09-27T08:00:00Z",
    },
  };
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const req = input as Request;
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    const text = await req.text();
    calls.push({
      method: req.method,
      path,
      body: text ? JSON.parse(text) : null,
    });
    const hit = routes.get(`${req.method} ${path}`);
    if (hit)
      return new Response(JSON.stringify(hit.body), {
        status: hit.status,
        headers: { "Content-Type": "application/json" },
      });
    if (req.method === "GET" && path in empty)
      return new Response(JSON.stringify(empty[path]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    return new Response(
      JSON.stringify({ code: "not_found", message: "not found" }),
      {
        status: 404,
        headers: { "Content-Type": "application/json" },
      },
    );
  }) as typeof fetch;
  return { routes, calls };
});

import "../habits/i18n";
import "../home/i18n";
import "./i18n";
import TodayPage from "./TodayPage";

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TodayPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const fail = (status: number, code: string) => ({
  status,
  body: { code, message: code },
});

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("TodayPage", () => {
  it("opens with nothing configured and points to settings", async () => {
    api.routes.set("GET /weather", fail(412, "precondition_failed"));
    api.routes.set(
      "GET /ha/favorites",
      fail(412, "integration_not_configured"),
    );
    renderPage();
    expect(await screen.findByText(/，jo$/)).toBeTruthy();
    await waitFor(() => {
      const hrefs = screen
        .getAllByRole("link", { name: /去设置/ })
        .map((a) => a.getAttribute("href"));
      expect(hrefs).toEqual(
        expect.arrayContaining(["/settings/brief", "/settings/homeassistant"]),
      );
    });
    expect(await screen.findByText("今天没有待办")).toBeTruthy();
    expect(await screen.findByText("今天没有日程")).toBeTruthy();
  });

  it("keeps other cards working when one module fails", async () => {
    api.routes.set("GET /habits/today", fail(500, "internal"));
    renderPage();
    expect(await screen.findAllByText("internal")).not.toHaveLength(0);
    expect(await screen.findByText("今天没有日程")).toBeTruthy();
    expect(await screen.findByText("21°")).toBeTruthy();
  });

  it("hides cards from the saved layout and saves edits", async () => {
    api.routes.set("GET /dashboard/layout", {
      status: 200,
      body: { cards: [{ id: "weather", visible: false, order: 0 }] },
    });
    api.routes.set("PUT /dashboard/layout", {
      status: 200,
      body: { cards: [{ id: "weather", visible: true, order: 0 }] },
    });
    renderPage();
    await screen.findByText("今天没有日程");
    expect(screen.queryByText("天气")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /调整布局/ }));
    const show = await screen.findAllByRole("button", { name: "显示" });
    fireEvent.click(show[0]);
    fireEvent.click(screen.getByRole("button", { name: "完成" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "PUT" && c.path === "/dashboard/layout",
        ),
      ).toBe(true),
    );
    const put = api.calls.find((c) => c.method === "PUT")!;
    const weather = (
      put.body as { cards: { id: string; visible: boolean }[] }
    ).cards.find((c) => c.id === "weather");
    expect(weather?.visible).toBe(true);
  });

  it("falls back to the default layout when the layout API is missing", async () => {
    renderPage();
    await screen.findByText("今天没有日程");
    fireEvent.click(screen.getByRole("button", { name: /调整布局/ }));
    expect(await screen.findByText(/还没上线/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "隐藏" })[0]);
    fireEvent.click(screen.getByRole("button", { name: "完成" }));
    expect(api.calls.some((c) => c.method === "PUT")).toBe(false);
    expect(screen.queryByText("今日待办", { selector: "h2" })).toBeNull();
  });
});
