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
import SkillsPage from "./SkillsPage";
import "./i18n";

const english = {
  id: 1,
  name: "英语口语",
  weeklyTarget: 120,
  archived: false,
  weekMinutes: 60,
  totalMinutes: 300,
  lastPracticedOn: "2026-11-11",
  weeks: [0, 30, 0, 45, 120, 90, 80, 60],
  createdAt: "2026-09-01T02:00:00Z",
};
const rust = {
  id: 2,
  name: "Rust",
  archived: false,
  weekMinutes: 0,
  totalMinutes: 0,
  weeks: [0, 0, 0, 0, 0, 0, 0, 0],
  createdAt: "2026-09-01T02:00:00Z",
};
const session = {
  id: 7,
  skillId: 1,
  practicedOn: "2026-11-11",
  minutes: 30,
  content: "对话练习",
  stuck: "时态总是错",
  createdAt: "2026-11-11T02:00:00Z",
};

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SkillsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function routes(items: unknown[]) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /skills", () => ({
    status: 200,
    body: { items, weekStart: "2026-11-09", today: "2026-11-11" },
  }));
  api.routes.set("GET /skills/1/sessions", () => ({
    status: 200,
    body: { items: [session] },
  }));
}

beforeEach(() => routes([english, rust]));
afterEach(cleanup);

describe("技能页", () => {
  it("显示本周分钟、目标进度和概要", async () => {
    renderPage();
    const card = await screen.findByRole("article", { name: "英语口语" });
    expect(within(card).getByText("1 小时")).toBeTruthy();
    const bar = within(card).getByRole("progressbar");
    expect(bar.getAttribute("aria-valuenow")).toBe("50");
    expect(within(card).getByText(/累计 5 小时/)).toBeTruthy();
    // 没设目标的技能没有进度条
    const other = screen.getByRole("article", { name: "Rust" });
    expect(within(other).queryByRole("progressbar")).toBeNull();
    expect(within(other).getByText(/还没练过/)).toBeTruthy();
    expect(screen.getByText("0 / 1")).toBeTruthy();
  });

  it("记一次：提交日期、分钟数、内容和卡住的地方", async () => {
    api.routes.set("POST /skills/1/sessions", () => ({
      status: 201,
      body: session,
    }));
    renderPage();
    const card = await screen.findByRole("article", { name: "英语口语" });
    fireEvent.click(within(card).getByRole("button", { name: "记一次" }));
    const dialog = await screen.findByRole("dialog");
    const save = within(dialog).getByRole("button", { name: "保存" });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("分钟数"), {
      target: { value: "40" },
    });
    fireEvent.change(within(dialog).getByLabelText("练了什么"), {
      target: { value: "听力" },
    });
    fireEvent.change(within(dialog).getByLabelText("哪里卡住"), {
      target: { value: "连读" },
    });
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    const call = api.calls.find((c) => c.method === "POST");
    expect(call?.path).toBe("/skills/1/sessions");
    expect(call?.body).toEqual({
      practicedOn: "2026-11-11",
      minutes: 40,
      content: "听力",
      stuck: "连读",
    });
  });

  it("展开练习记录，只看卡住的会带上参数", async () => {
    renderPage();
    const card = await screen.findByRole("article", { name: "英语口语" });
    fireEvent.click(within(card).getByRole("button", { name: "练习记录" }));
    expect(await within(card).findByText("对话练习")).toBeTruthy();
    expect(within(card).getByText(/时态总是错/)).toBeTruthy();
    fireEvent.click(within(card).getByLabelText("只看卡住的"));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) =>
            c.path.includes("/skills/1/sessions?") &&
            c.path.includes("stuck=true"),
        ),
      ).toBe(true),
    );
  });

  it("删除技能前要确认，确认后才请求", async () => {
    api.routes.set("DELETE /skills/2", () => ({ status: 204 }));
    renderPage();
    const card = await screen.findByRole("article", { name: "Rust" });
    fireEvent.click(within(card).getByRole("button", { name: /更多/ }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "删除" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "DELETE")?.path).toBe(
      "/skills/2",
    );
  });

  it("没有技能时给出说明", async () => {
    routes([]);
    renderPage();
    expect(await screen.findByText("还没有技能")).toBeTruthy();
  });

  it("接口还没上线时显示还没上线", async () => {
    api.routes.clear();
    renderPage();
    expect(
      await screen.findByText(/还没上线|尚未上线|还没有上线/),
    ).toBeTruthy();
  });
});
