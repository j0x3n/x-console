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
vi.mock("../../components/ui/ConfirmDialog", () => ({
  confirmAction: async () => true,
}));
vi.mock("../../auth/elevation", () => ({
  withElevation: <T,>(fn: () => Promise<T>) => fn(),
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
import ScreenTimePage from "./ScreenTimePage";
import TodayScreenTimeCard from "./TodayScreenTimeCard";
import "./i18n";

function renderIt(url = "/screentime", ui = <ScreenTimePage />) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const day = {
  range: "day",
  from: "2026-10-10",
  to: "2026-10-10",
  state: "ok",
  minutes: 200,
  categories: [
    { category: "coding", minutes: 120 },
    { category: "web", minutes: 50 },
    { category: "entertainment", minutes: 30 },
  ],
  apps: [
    { app: "Code.exe", category: "coding", minutes: 120 },
    { app: "chrome.exe", category: "web", minutes: 80 },
  ],
  days: [],
};
const week = {
  ...day,
  range: "week",
  from: "2026-10-05",
  to: "2026-10-11",
  days: [
    { date: "2026-10-05", minutes: 120, categories: { coding: 120 } },
    { date: "2026-10-06", minutes: 80, categories: { web: 50, ai: 30 } },
  ],
};
const settings = {
  enabled: true,
  keepTitles: false,
  hosts: [
    {
      id: "h1",
      name: "办公室电脑",
      online: true,
      supported: true,
      enabled: true,
    },
    {
      id: "h2",
      name: "家里电脑",
      online: false,
      supported: true,
      enabled: true,
    },
  ],
};

function serve(summary: unknown = day) {
  api.routes.set("GET /screentime/summary", () => ({
    status: 200,
    body: summary,
  }));
  api.routes.set("GET /screentime/settings", () => ({
    status: 200,
    body: settings,
  }));
  api.routes.set("GET /screentime/rules", () => ({
    status: 200,
    body: { items: [] },
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("ScreenTimePage B116", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("时间去向还没上线")).toBeTruthy();
  });

  it("显示总时长、各类别占比和用得最多的程序", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("时间花在哪")).toBeTruthy();
    // 编码 120 分钟：类别条和程序列表各出现一次
    expect(
      screen.getAllByText("2 小时", { selector: ".screentime-time" }).length,
    ).toBe(2);
    expect(screen.getByText("60%")).toBeTruthy();
    expect(screen.getByText("Code")).toBeTruthy();
    expect(screen.getByText("chrome")).toBeTruthy();
    // 概要卡片：200 分钟写成 3.3 小时
    expect(screen.getByText("3.3")).toBeTruthy();
    // 日视图没有每天的图
    expect(screen.queryByText("每天的时间")).toBeNull();
  });

  it("周视图多一张每天的图，翻页用服务端给的起始日", async () => {
    serve(week);
    renderIt("/screentime?range=week");
    expect(await screen.findByText("每天的时间")).toBeTruthy();
    expect(screen.getAllByRole("img").length).toBe(2);
    fireEvent.click(screen.getByRole("button", { name: "上一段" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) =>
            c.path.startsWith("/screentime/summary") &&
            c.path.includes("date=2026-09-28"),
        ),
      ).toBe(true),
    );
  });

  it("超过一台电脑时可以只看一台", async () => {
    serve();
    renderIt();
    const select = await screen.findByLabelText("哪台电脑");
    fireEvent.change(select, { target: { value: "h2" } });
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.includes("hostId=h2"))).toBe(true),
    );
  });

  it("说明为什么没有数据", async () => {
    serve({ ...day, state: "no_agent", minutes: 0, categories: [], apps: [] });
    renderIt();
    expect(await screen.findByText("还没有能记录的电脑")).toBeTruthy();
    cleanup();
    serve({ ...day, state: "waiting", minutes: 0, categories: [], apps: [] });
    renderIt();
    expect(await screen.findByText("等第一分钟的记录")).toBeTruthy();
    cleanup();
    serve({ ...day, minutes: 0, categories: [], apps: [] });
    renderIt();
    expect(await screen.findByText("这段时间没有记录")).toBeTruthy();
  });

  it("总开关关着时引导去设置", async () => {
    serve({ ...day, state: "disabled", minutes: 0, categories: [], apps: [] });
    renderIt();
    expect(await screen.findByText("记录已关闭")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "打开设置" }));
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });
});

describe("设置 B116", () => {
  it("保存标题默认关，打开时带上其他开关一起保存", async () => {
    serve();
    api.routes.set("PUT /screentime/settings", (body) => ({
      status: 200,
      body: { ...settings, ...(body as object) },
    }));
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: /设置/ }));
    const dialog = await screen.findByRole("dialog");
    const keep = await within(dialog).findByLabelText(/保存窗口标题/);
    expect((keep as HTMLInputElement).checked).toBe(false);
    await waitFor(() =>
      expect((keep as HTMLInputElement).disabled).toBe(false),
    );
    fireEvent.click(keep);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        enabled: true,
        keepTitles: true,
        disabledHosts: [],
      }),
    );
  });

  it("单独关掉一台电脑", async () => {
    serve();
    api.routes.set("PUT /screentime/settings", () => ({
      status: 200,
      body: settings,
    }));
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: /设置/ }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(await within(dialog).findByLabelText("家里电脑"));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toMatchObject({
        disabledHosts: ["h2"],
      }),
    );
  });

  it("添加一条分类规则", async () => {
    serve();
    api.routes.set("POST /screentime/rules", (body) => ({
      status: 201,
      body: { id: 1, createdAt: "2026-10-10T00:00:00Z", ...(body as object) },
    }));
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: /设置/ }));
    const dialog = await screen.findByRole("dialog");
    const add = within(dialog).getByRole("button", { name: "添加规则" });
    expect((add as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("要匹配的文字"), {
      target: { value: " Foo.exe " },
    });
    fireEvent.change(within(dialog).getByLabelText("时间花在哪"), {
      target: { value: "office" },
    });
    fireEvent.click(add);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        field: "app",
        pattern: "Foo.exe",
        category: "office",
      }),
    );
  });

  it("清空记录先确认", async () => {
    serve();
    api.routes.set("DELETE /screentime/data", () => ({ status: 204 }));
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: /设置/ }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: /清空全部记录/ }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });
});

describe("今日页卡片 B116", () => {
  it("显示今天的总时长和最多的几个类别", async () => {
    serve();
    renderIt("/", <TodayScreenTimeCard />);
    expect(await screen.findByText("3 小时 20 分钟")).toBeTruthy();
    expect(screen.getByText("编码")).toBeTruthy();
    expect(screen.getByText("网页")).toBeTruthy();
  });
});
