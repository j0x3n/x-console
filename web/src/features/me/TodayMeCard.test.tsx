// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

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

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import TodayMeCard from "./TodayMeCard";
import type { MeState } from "./api";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

const wrap = (ui: React.ReactElement) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
};

const local = (h: number, m = 0, daysAgo = 0) =>
  new Date(
    new Date().getFullYear(),
    new Date().getMonth(),
    new Date().getDate() - daysAgo,
    h,
    m,
  ).toISOString();

const state = (extra: Partial<MeState>): MeState => ({
  state: "working",
  since: local(9, 5),
  guessed: false,
  manual: false,
  source: "pc",
  ...extra,
});

const history = {
  days: [
    {
      date: "2026-10-11",
      segments: [
        {
          state: "working",
          startedAt: local(9),
          guessed: false,
          manual: false,
        },
      ],
      sleptAt: local(3, 40),
      wokeAt: local(11, 55),
      sleepMinutes: 495,
      sleepGuessed: false,
    },
    { date: "2026-10-10", segments: [] },
    {
      date: "2026-10-09",
      segments: [],
      sleptAt: local(2, 30, 2),
      wokeAt: local(11, 0, 2),
      sleepMinutes: 510,
      sleepGuessed: true,
    },
  ],
};

describe("TodayMeCard", () => {
  it("显示状态和从几点开始", async () => {
    api.routes.set("GET /me", () => ({ status: 200, body: state({}) }));
    wrap(<TodayMeCard />);
    await screen.findByText("工作中");
    expect(screen.getByText("从 09:05 开始")).toBeTruthy();
    expect(screen.queryByText("推测")).toBeNull();
    expect(screen.queryByText(/^手动/)).toBeNull();
  });

  it("推测的标“推测”，手动的标“手动”和到期时间", async () => {
    api.routes.set("GET /me", () => ({
      status: 200,
      body: state({ state: "sleeping", guessed: true, source: "schedule" }),
    }));
    wrap(<TodayMeCard />);
    await screen.findByText("睡觉");
    expect(screen.getByText("推测")).toBeTruthy();
    cleanup();
    api.routes.set("GET /me", () => ({
      status: 200,
      body: state({
        state: "out",
        manual: true,
        source: "manual",
        manualUntil: local(16, 30),
      }),
    }));
    wrap(<TodayMeCard />);
    await screen.findByText("外出");
    expect(screen.getByText("手动 · 到 16:30")).toBeTruthy();
  });

  it("点开后手动设定，并显示最近的睡觉时间", async () => {
    let current = state({});
    api.routes.set("GET /me", () => ({ status: 200, body: current }));
    api.routes.set("GET /me/history", () => ({ status: 200, body: history }));
    api.routes.set("PUT /me/manual", (body) => {
      const b = body as { state: MeState["state"] };
      current = state({
        state: b.state,
        manual: true,
        source: "manual",
        manualUntil: local(17),
      });
      return { status: 200, body: current };
    });
    wrap(<TodayMeCard />);
    await screen.findByText("工作中");
    fireEvent.click(screen.getByRole("button", { name: /改状态或看历史/ }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(/03:40 → 11:55 · 8 小时 15 分钟/);
    expect(
      within(dialog).getByText(/02:30 → 11:00 · 8 小时 30 分钟/),
    ).toBeTruthy();
    expect(within(dialog).getAllByText("推测")).toHaveLength(1);
    expect(api.calls.find((c) => c.path.startsWith("/me/history"))?.path).toBe(
      "/me/history?days=7",
    );
    // 默认选中现在的状态；改成睡觉，选 2 小时
    expect(
      within(dialog)
        .getByRole("button", { name: "工作中" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    fireEvent.click(within(dialog).getByRole("button", { name: "睡觉" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "设定" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    const put = api.calls.find((c) => c.method === "PUT")!;
    expect(put.path).toBe("/me/manual");
    expect(put.body).toEqual({ state: "sleeping", duration: "2h" });
    await within(dialog).findByRole("button", { name: "改回自动" });
  });

  it("改回自动", async () => {
    let current = state({ manual: true, source: "manual", state: "home" });
    api.routes.set("GET /me", () => ({ status: 200, body: current }));
    api.routes.set("GET /me/history", () => ({
      status: 200,
      body: { days: [] },
    }));
    api.routes.set("DELETE /me/manual", () => {
      current = state({});
      return { status: 200, body: current };
    });
    wrap(<TodayMeCard />);
    await screen.findByText("在家");
    fireEvent.click(screen.getByRole("button", { name: /改状态或看历史/ }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      await within(dialog).findByRole("button", { name: "改回自动" }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
    await waitFor(() =>
      expect(
        within(dialog).queryByRole("button", { name: "改回自动" }),
      ).toBeNull(),
    );
  });

  it("手动设定里没有“不确定”，选择时长有四项", async () => {
    api.routes.set("GET /me", () => ({
      status: 200,
      body: state({ state: "unknown", source: "" }),
    }));
    api.routes.set("GET /me/history", () => ({
      status: 200,
      body: { days: [] },
    }));
    wrap(<TodayMeCard />);
    await screen.findAllByText("不确定");
    fireEvent.click(screen.getByRole("button", { name: /改状态或看历史/ }));
    const dialog = await screen.findByRole("dialog");
    const group = within(dialog).getByRole("group", { name: "状态" });
    expect(within(group).getAllByRole("button")).toHaveLength(5);
    expect(within(group).queryByText("不确定")).toBeNull();
    const options = within(dialog)
      .getAllByRole("option")
      .map((o) => o.textContent);
    expect(options).toEqual([
      "1 小时",
      "2 小时",
      "到下个起床时间",
      "一直到我改回来",
    ]);
  });
});
