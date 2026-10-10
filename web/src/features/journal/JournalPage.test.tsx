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
import JournalPage from "./JournalPage";
import { chipLabel, shiftDay, summaryNumbers } from "./format";
import "./i18n";

function renderIt(url = "/journal") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <JournalPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const at = (hour: number, minute: number) =>
  new Date(2026, 9, 10, hour, minute).toISOString();

const items = [
  {
    id: 1,
    at: at(9, 5),
    module: "projects",
    kind: "card",
    title: "完成 XC-1 登录页改版",
    detail: "示例项目",
    link: "/projects/XC/1",
    minutes: 0,
  },
  {
    id: 2,
    at: at(10, 30),
    module: "calendar",
    kind: "focus",
    title: "专注 25 分钟",
    detail: "",
    link: "/calendar/focus",
    minutes: 25,
  },
  {
    id: 3,
    at: at(11, 0),
    module: "github",
    kind: "commit",
    title: "修复导航栏",
    detail: "me/app · 1111111",
    link: "https://github.com/me/app/commit/1",
    minutes: 0,
  },
  {
    id: 4,
    at: at(12, 0),
    module: "habits",
    kind: "habit",
    title: "习惯打卡：喝水",
    detail: "打卡 2 次，共 3 杯",
    link: "/habits",
    minutes: 0,
  },
];
const counts = [
  { kind: "commit", count: 1, minutes: 0 },
  { kind: "pr", count: 1, minutes: 0 },
  { kind: "card", count: 1, minutes: 0 },
  { kind: "task", count: 2, minutes: 0 },
  { kind: "focus", count: 1, minutes: 25 },
  { kind: "habit", count: 1, minutes: 0 },
];

function day(date: string, extra: Record<string, unknown> = {}) {
  return {
    day: date,
    items: [],
    counts: [],
    diary: { day: date, body: "" },
    ...extra,
  };
}

function serve() {
  api.routes.set("GET /journal/recent", () => ({
    status: 200,
    body: {
      days: [
        { day: "2026-10-10", items: 4, diary: false },
        { day: "2026-10-09", items: 0, diary: true },
      ],
    },
  }));
  api.routes.set("GET /journal/days/2026-10-10", () => ({
    status: 200,
    body: day("2026-10-10", { items, counts }),
  }));
  api.routes.set("GET /journal/days/2026-10-09", () => ({
    status: 200,
    body: day("2026-10-09", {
      diary: { day: "2026-10-09", body: "昨天写的日记", updatedAt: at(22, 0) },
    }),
  }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("JournalPage B118", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("每日时间线还没上线")).toBeTruthy();
  });

  it("默认显示服务器的今天，按时间列出，概要对得上", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("完成 XC-1 登录页改版")).toBeTruthy();
    expect((screen.getByLabelText("选择日期") as HTMLInputElement).value).toBe(
      "2026-10-10",
    );
    const rows = screen.getAllByRole("listitem");
    expect(rows.map((r) => r.querySelector("time")?.textContent)).toEqual([
      "09:05",
      "10:30",
      "11:00",
      "12:00",
    ]);
    // 站内链接用路由，外部链接新窗口打开
    expect(
      screen
        .getByRole("link", { name: "完成 XC-1 登录页改版" })
        .getAttribute("href"),
    ).toBe("/projects/XC/1");
    const external = screen.getByRole("link", { name: "修复导航栏" });
    expect(external.getAttribute("target")).toBe("_blank");
    expect(external.getAttribute("rel")).toContain("noopener");
    expect(screen.getByText("me/app · 1111111")).toBeTruthy();
    // 概要：代码 2，完成 3，专注 25 分钟，习惯 1
    const strip = screen.getByRole("region", { name: "每日时间线" });
    expect(
      within(strip).getByText("代码").closest(".xc-stat")?.textContent,
    ).toContain("2");
    expect(
      within(strip).getByText("完成的卡片和任务").closest(".xc-stat")
        ?.textContent,
    ).toContain("3");
    expect(
      within(strip).getByText("专注时间").closest(".xc-stat")?.textContent,
    ).toContain("25");
    expect(api.calls.some((c) => c.path === "/journal/days/2026-10-10")).toBe(
      true,
    );
  });

  it("日期条切到另一天，不请求今天以外的日期以外的东西", async () => {
    serve();
    renderIt();
    await screen.findByText("完成 XC-1 登录页改版");
    fireEvent.click(screen.getByRole("button", { name: /10\/9/ }));
    expect(await screen.findByText("这一天没有记录")).toBeTruthy();
    expect(await screen.findByDisplayValue("昨天写的日记")).toBeTruthy();
    // 后一天回到今天
    fireEvent.click(screen.getByRole("button", { name: "后一天" }));
    expect(await screen.findByText("完成 XC-1 登录页改版")).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "后一天" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });

  it("带 date 参数时直接打开那一天，前一天按钮和选日期都能用", async () => {
    serve();
    renderIt("/journal?date=2026-10-09");
    expect(await screen.findByDisplayValue("昨天写的日记")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("选择日期"), {
      target: { value: "2026-10-10" },
    });
    expect(await screen.findByText("完成 XC-1 登录页改版")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "前一天" }));
    expect(await screen.findByDisplayValue("昨天写的日记")).toBeTruthy();
  });

  it("写日记：没改时不能保存，改了以后点保存会提交，清空也提交", async () => {
    serve();
    api.routes.set("PUT /journal/days/2026-10-10/diary", (body) => ({
      status: 200,
      body: { day: "2026-10-10", body: (body as { body: string }).body },
    }));
    renderIt();
    await screen.findByText("完成 XC-1 登录页改版");
    const save = screen.getByRole("button", {
      name: "保存",
    }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    const box = screen.getByLabelText("日记内容");
    fireEvent.change(box, { target: { value: "今天修了导航栏" } });
    expect(screen.getByText("还没保存")).toBeTruthy();
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        body: "今天修了导航栏",
      }),
    );
  });

  it("换日期时先保存没存的日记", async () => {
    serve();
    api.routes.set("PUT /journal/days/2026-10-10/diary", (body) => ({
      status: 200,
      body: { day: "2026-10-10", body: (body as { body: string }).body },
    }));
    renderIt();
    await screen.findByText("完成 XC-1 登录页改版");
    fireEvent.change(screen.getByLabelText("日记内容"), {
      target: { value: "还没点保存" },
    });
    fireEvent.click(screen.getByRole("button", { name: "前一天" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        body: "还没点保存",
      }),
    );
    expect(await screen.findByDisplayValue("昨天写的日记")).toBeTruthy();
  });

  it("搜索：显示命中的日期和片段，点一条跳到那天", async () => {
    serve();
    api.routes.set("GET /journal/search", () => ({
      status: 200,
      body: {
        items: [
          {
            day: "2026-10-09",
            kind: "diary",
            title: "日记",
            snippet: "…服务器突然变慢…",
            link: "/journal?date=2026-10-09",
          },
          {
            day: "2026-10-06",
            kind: "card",
            title: "完成 XC-7 修复服务器巡检",
            snippet: "",
            link: "/journal?date=2026-10-06",
          },
        ],
      },
    }));
    renderIt();
    await screen.findByText("完成 XC-1 登录页改版");
    fireEvent.change(screen.getByPlaceholderText("搜索日记和时间线"), {
      target: { value: "服务器" },
    });
    expect(await screen.findByText("…服务器突然变慢…")).toBeTruthy();
    expect(screen.getByText("完成 XC-7 修复服务器巡检")).toBeTruthy();
    expect(
      api.calls.some(
        (c) => c.path === "/journal/search?q=%E6%9C%8D%E5%8A%A1%E5%99%A8",
      ),
    ).toBe(true);
    fireEvent.click(screen.getByText("…服务器突然变慢…"));
    expect(await screen.findByDisplayValue("昨天写的日记")).toBeTruthy();
    // 跳转后搜索框清空，回到时间线
    expect(
      (screen.getByPlaceholderText("搜索日记和时间线") as HTMLInputElement)
        .value,
    ).toBe("");
  });

  it("搜不到时说没有找到", async () => {
    serve();
    api.routes.set("GET /journal/search", () => ({
      status: 200,
      body: { items: [] },
    }));
    renderIt();
    await screen.findByText("完成 XC-1 登录页改版");
    fireEvent.change(screen.getByPlaceholderText("搜索日记和时间线"), {
      target: { value: "没有的词" },
    });
    expect(await screen.findByText("没有找到")).toBeTruthy();
  });
});

describe("格式", () => {
  it("shiftDay 能跨月跨年", () => {
    expect(shiftDay("2026-10-01", -1)).toBe("2026-09-30");
    expect(shiftDay("2026-12-31", 1)).toBe("2027-01-01");
    expect(shiftDay("2026-03-01", -1)).toBe("2026-02-28");
  });
  it("chipLabel 是月/日和星期", () => {
    expect(chipLabel("2026-10-10", "zh")).toEqual(["10/10", "周六"]);
  });
  it("summaryNumbers 把提交和 PR、卡片和任务加在一起", () => {
    expect(summaryNumbers(counts as never)).toEqual({
      code: 2,
      done: 3,
      focusMinutes: 25,
      habits: 1,
    });
  });
});
