// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import ForecastsPage from "./ForecastsPage";
import "./i18n";

const open1 = {
  id: 1,
  kind: "prediction",
  statement: "这个功能两天做完",
  probability: 70,
  dueDate: "2026-11-09",
  reason: "有现成代码",
  note: "",
  overdue: true,
  createdAt: "2026-11-07T02:00:00Z",
};
const open2 = {
  id: 2,
  kind: "decision",
  statement: "改用新的部署方式",
  dueDate: "2026-12-01",
  reason: "",
  note: "",
  overdue: false,
  createdAt: "2026-11-07T02:00:00Z",
};
const done1 = {
  id: 3,
  kind: "prediction",
  statement: "上周的判断",
  probability: 60,
  dueDate: "2026-11-01",
  reason: "",
  note: "差了一天",
  outcome: "no",
  overdue: false,
  createdAt: "2026-10-20T02:00:00Z",
  resolvedAt: "2026-11-02T02:00:00Z",
};
const stats = {
  count: 3,
  brier: 0.25,
  bias: 12.3,
  small: true,
  bins: [
    { label: "1–20", count: 0, meanProbability: 0, observed: 0, few: true },
    { label: "21–40", count: 0, meanProbability: 0, observed: 0, few: true },
    { label: "41–60", count: 1, meanProbability: 60, observed: 0, few: true },
    { label: "61–80", count: 2, meanProbability: 70, observed: 100, few: true },
    { label: "81–99", count: 0, meanProbability: 0, observed: 0, few: true },
  ],
};

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ForecastsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function routes(items: unknown[], s: unknown = stats) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /forecasts", () => ({
    status: 200,
    body: { items, today: "2026-11-11" },
  }));
  api.routes.set("GET /forecasts/stats", () => ({ status: 200, body: s }));
  api.routes.set("GET /forecasts/settings", () => ({
    status: 200,
    body: { remind: true, remindTime: "09:00" },
  }));
}

beforeEach(() => routes([open1, open2, done1]));
afterEach(cleanup);

describe("预测页", () => {
  it("到期没填的标红，提示条数，已有结果的收在折叠里", async () => {
    renderPage();
    expect(await screen.findByText("这个功能两天做完")).toBeTruthy();
    expect(screen.getByText("改用新的部署方式")).toBeTruthy();
    expect(screen.getByText("把握 70%")).toBeTruthy();
    expect(screen.getByText("条到期没填")).toBeTruthy();
    expect(screen.getByText(/已有结果 \(1\)/)).toBeTruthy();
  });

  it("预测和决策的结果按钮文字不同", async () => {
    renderPage();
    await screen.findByText("这个功能两天做完");
    expect(screen.getAllByRole("button", { name: "成了" }).length).toBe(1);
    expect(screen.getAllByRole("button", { name: "做对了" }).length).toBe(1);
    expect(screen.getAllByRole("button", { name: "说不清" }).length).toBe(1);
  });

  it("填结果：带上备注", async () => {
    api.routes.set("POST /forecasts/1/resolve", () => ({
      status: 200,
      body: { ...open1, outcome: "yes", overdue: false },
    }));
    renderPage();
    await screen.findByText("这个功能两天做完");
    fireEvent.change(screen.getAllByLabelText("结果备注（可不填）")[0], {
      target: { value: "按时完成" },
    });
    fireEvent.click(screen.getByRole("button", { name: "成了" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/forecasts/1/resolve")).toBe(
        true,
      ),
    );
    const call = api.calls.find((c) => c.path === "/forecasts/1/resolve");
    expect(call?.body).toEqual({ outcome: "yes", note: "按时完成" });
  });

  it("记一条：把握留空时不提交把握", async () => {
    api.routes.set("POST /forecasts", () => ({ status: 201, body: open2 }));
    renderPage();
    await screen.findByText("这个功能两天做完");
    fireEvent.click(screen.getByRole("button", { name: "决策" }));
    const add = screen.getByRole("button", { name: "记下来" });
    expect((add as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("内容"), {
      target: { value: "换工具" },
    });
    fireEvent.change(screen.getByLabelText("到期日"), {
      target: { value: "2026-12-01" },
    });
    expect((add as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(add);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    const body = api.calls.find((c) => c.method === "POST")?.body as Record<
      string,
      unknown
    >;
    expect(body.kind).toBe("decision");
    expect(body.statement).toBe("换工具");
    expect(body.dueDate).toBe("2026-12-01");
    expect(body.probability).toBeUndefined();
  });

  it("统计：Brier、偏差说明、样本少和分档", async () => {
    renderPage();
    expect(await screen.findByText("0.250")).toBeTruthy();
    expect(screen.getByText("+12.3")).toBeTruthy();
    expect(screen.getByText(/偏乐观/)).toBeTruthy();
    expect(screen.getByText("样本少，只供参考")).toBeTruthy();
    const rows = screen.getAllByRole("row");
    expect(rows.length).toBe(6);
    expect(within(rows[4]).getByText("100%")).toBeTruthy();
  });

  it("没有可统计的记录时给说明，不画表", async () => {
    routes([open2], { ...stats, count: 0, bias: 0, brier: 0 });
    renderPage();
    expect(await screen.findByText(/还没有可统计的记录/)).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("接口还没上线时显示还没上线", async () => {
    api.routes.clear();
    renderPage();
    expect(
      await screen.findByText(/还没上线|尚未上线|还没有上线/),
    ).toBeTruthy();
  });
});
