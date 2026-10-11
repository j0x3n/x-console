// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => {
  type Reply = { status: number; body?: unknown };
  const routes = new Map<string, (body: unknown) => Reply>();
  const calls: Array<{ method: string; path: string }> = [];
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
    calls.push({ method: req.method, path: path + url.search });
    const handler = routes.get(`${req.method} ${path}`);
    const reply = handler
      ? handler(null)
      : { status: 404, body: { code: "not_found", message: "not found" } };
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

import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import TodayGoalsCard, { useTodayGoals } from "./TodayGoalsCard";
import { currentPeriod } from "./format";
import "./i18n";

function Probe() {
  const q = useTodayGoals();
  return <span data-testid="count">{q.data?.items.length ?? "none"}</span>;
}

function renderIt(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const base = {
  year: 2026,
  quarter: 4,
  note: "",
  status: "active",
  results: [],
  expected: 45,
  from: "2026-10-01",
  until: "2026-12-31",
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};
const goals = [
  { ...base, id: 1, title: "多读书", progress: 60, state: "on_track" },
  { ...base, id: 2, title: "练身体", progress: 20, state: "behind" },
  { ...base, id: 3, title: "学英语", progress: null, state: "on_track" },
  { ...base, id: 4, title: "存钱", progress: 90, state: "on_track" },
];

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("TodayGoalsCard B128", () => {
  it("只取本季度进行中的目标", async () => {
    api.routes.set("GET /goals", () => ({
      status: 200,
      body: { items: [], current: currentPeriod() },
    }));
    renderIt(<Probe />);
    expect(await screen.findByText("0")).toBeTruthy();
    const p = currentPeriod();
    expect(api.calls[0]!.path).toBe(
      `/goals?year=${p.year}&quarter=${p.quarter}&status=active`,
    );
  });

  it("接口还没上线时没有数据，今日页据此不显示卡片", async () => {
    renderIt(<Probe />);
    // 404 之后查询出错，items 取不到
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.getByTestId("count").textContent).toBe("none");
  });

  it("最多三行，落后的标出来，其余写个数", async () => {
    api.routes.set("GET /goals", () => ({
      status: 200,
      body: { items: goals, current: currentPeriod() },
    }));
    renderIt(<TodayGoalsCard />);
    expect(await screen.findByText("多读书")).toBeTruthy();
    expect(screen.getByText("60%")).toBeTruthy();
    expect(screen.getByText("练身体")).toBeTruthy();
    expect(screen.getByText("落后")).toBeTruthy();
    expect(screen.getByText("学英语")).toBeTruthy();
    expect(screen.getByText("–")).toBeTruthy();
    expect(screen.queryByText("存钱")).toBeNull();
    expect(screen.getByText("还有 1 个目标")).toBeTruthy();
  });
});
