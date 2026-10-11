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
import ReviewPage from "./ReviewPage";
import "./i18n";

function renderIt(url = "/goals/review") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <ReviewPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const weekGoal = {
  goalId: 1,
  title: "多读书",
  year: 2026,
  quarter: 4,
  progress: 47.6,
  progressBefore: 36.2,
  change: 11.4,
  expected: 45.1,
  state: "on_track",
  results: [
    {
      id: 11,
      name: "读完几本书",
      unit: "本",
      source: "manual",
      target: 4,
      value: 2,
      change: 1,
      progress: 50,
      state: "on_track",
      unavailable: "",
    },
    {
      id: 12,
      name: "读书打卡",
      unit: "次",
      source: "habit_checkins",
      target: 80,
      value: 41.5,
      change: 2.5,
      progress: 51.9,
      state: "on_track",
      unavailable: "",
    },
    {
      id: 13,
      name: "提交",
      unit: "个",
      source: "github_commits",
      target: 50,
      value: null,
      change: null,
      progress: null,
      state: "on_track",
      unavailable: "missing",
    },
  ],
};
const behindGoal = {
  ...weekGoal,
  goalId: 2,
  title: "练身体",
  progress: 20,
  progressBefore: 18,
  change: 2,
  state: "behind",
  results: [],
};

function review(over: Record<string, unknown> = {}) {
  return {
    weekStart: "2026-11-09",
    weekEnd: "2026-11-15",
    current: true,
    saved: false,
    wentWell: "",
    fellShort: "",
    nextWeek: "",
    lastPlan: "",
    goals: [weekGoal, behindGoal],
    updatedAt: null,
    ...over,
  };
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("ReviewPage B128", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("每周复盘还没上线")).toBeTruthy();
  });

  it("列出每个目标这周的变化和偏离", async () => {
    api.routes.set("GET /goals/reviews/current", () => ({
      status: 200,
      body: review(),
    }));
    api.routes.set("GET /goals/reviews", () => ({
      status: 200,
      body: { items: [] },
    }));
    renderIt();
    expect(await screen.findByText("多读书")).toBeTruthy();
    expect(
      screen.getAllByText("11 月 9 日到 11 月 15 日").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("本周")).toBeTruthy();
    expect(screen.getByText("+11.4 %")).toBeTruthy();
    expect(screen.getByText("进度 47.6% · 按时间应到 45.1%")).toBeTruthy();
    // 落后的目标写出落后多少
    expect(
      screen.getByText("进度 20% · 按时间应到 45.1% · 落后 25.1%"),
    ).toBeTruthy();
    expect(screen.getByText("本周 +1 本")).toBeTruthy();
    expect(screen.getByText("本周 +2.5 次")).toBeTruthy();
    expect(screen.getByText("没有数据")).toBeTruthy();
    expect(screen.getByText("保存过的复盘会列在这里。")).toBeTruthy();
    // 本周不能再往后翻
    expect(
      (screen.getByRole("button", { name: "下一周" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });

  it("写三个回答并保存，保存的是这一周", async () => {
    api.routes.set("GET /goals/reviews/current", () => ({
      status: 200,
      body: review({ lastPlan: "周二晚上八点读书一小时" }),
    }));
    api.routes.set("GET /goals/reviews", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("PUT /goals/reviews/2026-11-09", () => ({
      status: 200,
      body: review({ saved: true, wentWell: "跑了三次" }),
    }));
    renderIt();
    await screen.findByText("多读书");
    expect(screen.getByText("上周写的下周安排")).toBeTruthy();
    expect(screen.getByText("周二晚上八点读书一小时")).toBeTruthy();
    fireEvent.change(screen.getAllByLabelText("这周哪里做得好？")[0]!, {
      target: { value: "跑了三次" },
    });
    fireEvent.change(
      screen.getAllByLabelText("下周具体在什么时候做什么？")[0]!,
      {
        target: { value: "周二晚上八点读书" },
      },
    );
    fireEvent.click(screen.getByRole("button", { name: "保存复盘" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      wentWell: "跑了三次",
      fellShort: "",
      nextWeek: "周二晚上八点读书",
    });
  });

  it("往期复盘列在右边，点一下看那一周", async () => {
    api.routes.set("GET /goals/reviews/current", () => ({
      status: 200,
      body: review(),
    }));
    api.routes.set("GET /goals/reviews/2026-11-02", () => ({
      status: 200,
      body: review({
        weekStart: "2026-11-02",
        weekEnd: "2026-11-08",
        current: false,
        saved: true,
        wentWell: "上周做得好",
        updatedAt: "2026-11-08T12:00:00Z",
      }),
    }));
    api.routes.set("GET /goals/reviews", () => ({
      status: 200,
      body: {
        items: [
          {
            weekStart: "2026-11-02",
            weekEnd: "2026-11-08",
            answered: true,
            progress: 40,
            updatedAt: "2026-11-08T12:00:00Z",
          },
        ],
      },
    }));
    renderIt();
    const history = await screen.findByRole("complementary", {
      name: "往期复盘",
    });
    const row = await within(history).findByRole("button", {
      name: /11 月 2 日到 11 月 8 日/,
    });
    expect(row.textContent).toContain("40% · 已写");
    fireEvent.click(row);
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path === "/goals/reviews/2026-11-02"),
      ).toBe(true),
    );
    expect(await screen.findByDisplayValue("上周做得好")).toBeTruthy();
    // 看过去的周，可以回到本周
    expect(screen.getByRole("button", { name: "本周" })).toBeTruthy();
  });

  it("提醒设置可以关掉并改时间", async () => {
    api.routes.set("GET /goals/reviews/current", () => ({
      status: 200,
      body: review({ goals: [] }),
    }));
    api.routes.set("GET /goals/reviews", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("GET /goals/settings", () => ({
      status: 200,
      body: { reviewReminder: true, reviewTime: "20:00" },
    }));
    api.routes.set("PUT /goals/settings", (body) => ({ status: 200, body }));
    renderIt();
    expect(await screen.findByText("这周没有进行中的目标")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /提醒设置/ }));
    const dialog = await screen.findByRole("dialog");
    const time = (await within(dialog).findByDisplayValue(
      "20:00",
    )) as HTMLInputElement;
    fireEvent.change(time, { target: { value: "18:30" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      reviewReminder: true,
      reviewTime: "18:30",
    });
  });
});
