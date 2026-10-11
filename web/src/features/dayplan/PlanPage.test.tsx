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
import PlanPage from "./PlanPage";
import TodayPlanCard, { useHasTodayPlan } from "./TodayPlanCard";
import "./i18n";

const settings = {
  auto: true,
  generateTime: "07:30",
  useAi: true,
  workStart: "09:00",
  workEnd: "18:00",
  review: true,
  reviewTime: "20:30",
};
const item = (
  id: number,
  title: string,
  start: string,
  end: string,
  done = false,
) => ({
  id,
  position: id,
  title,
  kind: "issue",
  ref: `XC-${id}`,
  start,
  end,
  reason: `因为${title}`,
  done,
});
const plan = (
  status: "draft" | "confirmed",
  items = [
    item(1, "写周报", "09:00", "10:00"),
    item(2, "改登录页", "11:30", "12:30"),
  ],
) => ({
  date: "2026-11-11",
  status,
  source: "ai",
  generatedAt: "2026-11-11T00:00:00Z",
  items,
});
const slots = [
  { start: "09:00", end: "10:00" },
  { start: "11:30", end: "18:00" },
];

function routes(p: unknown) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /dayplan", () => ({
    status: 200,
    body: { date: "2026-11-11", plan: p ?? undefined, slots },
  }));
  api.routes.set("GET /dayplan/settings", () => ({
    status: 200,
    body: settings,
  }));
}
function renderPage(node: React.ReactNode = <PlanPage />) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => routes(plan("draft")));
afterEach(cleanup);

describe("今日计划页", () => {
  it("显示空档、事项、时间和理由，草稿有标记", async () => {
    renderPage();
    expect(await screen.findByDisplayValue("写周报")).toBeTruthy();
    expect(screen.getByText("09:00–10:00")).toBeTruthy();
    expect(screen.getByText("11:30–18:00")).toBeTruthy();
    expect(screen.getByText("因为写周报")).toBeTruthy();
    expect(screen.getByText("AI 排的")).toBeTruthy();
  });

  it("今天和明天以服务端的日期为准，不看浏览器的日期", async () => {
    // 服务端的今天是 2026-11-11，浏览器的日期不一样也不影响请求和保存。
    api.routes.set("POST /dayplan/generate", () => ({
      status: 200,
      body: plan("draft"),
    }));
    renderPage();
    await screen.findByDisplayValue("写周报");
    fireEvent.click(screen.getByRole("button", { name: "明天" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/dayplan?date=2026-11-12")).toBe(
        true,
      ),
    );
    // 今天的请求不带日期
    expect(api.calls.some((c) => c.path === "/dayplan")).toBe(true);
  });

  it("没有计划时提示，点“排一下”请求生成", async () => {
    routes(null);
    api.routes.set("POST /dayplan/generate", () => ({
      status: 200,
      body: plan("draft"),
    }));
    renderPage();
    expect(await screen.findByText(/还没有计划/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "排一下" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/dayplan/generate")).toBe(true),
    );
    expect(
      api.calls.find((c) => c.path === "/dayplan/generate")?.body,
    ).toMatchObject({ replace: false });
  });

  it("改时间后保存，带上全部事项", async () => {
    api.routes.set("PUT /dayplan/2026-11-11", () => ({
      status: 200,
      body: plan("draft"),
    }));
    renderPage();
    const start = await screen.findByLabelText("开始 写周报");
    fireEvent.change(start, { target: { value: "09:30" } });
    fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    const body = api.calls.find((c) => c.method === "PUT")?.body as {
      items: Array<{ title: string; start: string; end: string }>;
    };
    expect(body.items).toHaveLength(2);
    expect(body.items[0]).toMatchObject({
      title: "写周报",
      start: "09:30",
      end: "10:00",
    });
  });

  it("时间重叠时不提交，说明原因", async () => {
    renderPage();
    const start = await screen.findByLabelText("开始 改登录页");
    fireEvent.change(start, { target: { value: "09:30" } });
    fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
    expect(await screen.findByText("两件事的时间重叠了")).toBeTruthy();
    expect(api.calls.some((c) => c.method === "PUT")).toBe(false);
  });

  it("确认计划", async () => {
    api.routes.set("POST /dayplan/2026-11-11/confirm", () => ({
      status: 200,
      body: plan("confirmed"),
    }));
    renderPage();
    await screen.findByDisplayValue("写周报");
    fireEvent.click(screen.getByRole("button", { name: "确认计划" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.endsWith("/confirm"))).toBe(true),
    );
  });

  it("已确认的计划：勾选做完了；重新排要带 replace", async () => {
    routes(plan("confirmed"));
    api.routes.set("PUT /dayplan/items/1/done", () => ({
      status: 200,
      body: plan("confirmed", [
        item(1, "写周报", "09:00", "10:00", true),
        item(2, "改登录页", "11:30", "12:30"),
      ]),
    }));
    api.routes.set("POST /dayplan/generate", () => ({
      status: 200,
      body: plan("draft"),
    }));
    renderPage();
    const check = await screen.findByRole("checkbox", {
      name: "做完了 写周报",
    });
    expect(screen.queryByRole("button", { name: "确认计划" })).toBeNull();
    fireEvent.click(check);
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/dayplan/items/1/done")).toBe(
        true,
      ),
    );
    expect(
      api.calls.find((c) => c.path === "/dayplan/items/1/done")?.body,
    ).toEqual({ done: true });
    fireEvent.click(screen.getByRole("button", { name: "重新排" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/dayplan/generate")).toBe(true),
    );
    expect(
      api.calls.find((c) => c.path === "/dayplan/generate")?.body,
    ).toMatchObject({ replace: true });
  });

  it("加一件、去掉一件", async () => {
    renderPage();
    await screen.findByDisplayValue("写周报");
    fireEvent.click(screen.getByRole("button", { name: /加一件/ }));
    expect(screen.getAllByLabelText("事项名称")).toHaveLength(3);
    fireEvent.click(screen.getByRole("button", { name: "去掉这件 写周报" }));
    expect(screen.getAllByLabelText("事项名称")).toHaveLength(2);
  });

  it("设置：保存时带上新的工作时段", async () => {
    api.routes.set("PUT /dayplan/settings", (body) => ({ status: 200, body }));
    renderPage();
    await screen.findByDisplayValue("写周报");
    fireEvent.click(screen.getByRole("button", { name: "计划设置" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("工作开始"), {
      target: { value: "08:30" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "PUT" && c.path === "/dayplan/settings",
        ),
      ).toBe(true),
    );
    expect(
      api.calls.find(
        (c) => c.path === "/dayplan/settings" && c.method === "PUT",
      )?.body,
    ).toMatchObject({ workStart: "08:30", useAi: true });
  });
});

function Probe() {
  return <p>{useHasTodayPlan() ? "有计划" : "没有计划"}</p>;
}

describe("今日页的计划卡片", () => {
  it("列出事项和进度", async () => {
    routes(
      plan("confirmed", [
        item(1, "写周报", "09:00", "10:00", true),
        item(2, "改登录页", "", ""),
      ]),
    );
    renderPage(<TodayPlanCard />);
    expect(await screen.findByText("写周报")).toBeTruthy();
    expect(screen.getByText("1 / 2 已完成", { exact: false })).toBeTruthy();
    expect(
      (
        screen.getByRole("checkbox", {
          name: "做完了 写周报",
        }) as HTMLInputElement
      ).checked,
    ).toBe(true);
  });
  it("没有计划时判断为没有", async () => {
    routes(null);
    renderPage(<Probe />);
    expect(await screen.findByText("没有计划")).toBeTruthy();
  });
  it("有计划时判断为有", async () => {
    renderPage(<Probe />);
    expect(await screen.findByText("有计划")).toBeTruthy();
  });
});
