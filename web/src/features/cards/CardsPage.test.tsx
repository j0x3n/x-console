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
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import CardsPage from "./CardsPage";
import "./i18n";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

function renderIt(url = "/cards") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <CardsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const summary = {
  due: 2,
  dueReview: 1,
  dueNew: 1,
  reviewedToday: 3,
  learnedToday: 1,
  total: 12,
  newCards: 4,
  suspended: 1,
  nextDue: "2026-10-14T01:00:00Z",
  recall30: 0.86,
};

const card = (id: number, extra: Record<string, unknown> = {}) => ({
  id,
  deck: "后端",
  question: `问题 ${id}`,
  answer: `答案 ${id}`,
  sourceTitle: "",
  sourceLink: "",
  suspended: false,
  state: "review",
  due: "2026-10-12T00:00:00Z",
  stability: 3,
  difficulty: 5,
  reps: id,
  lapses: 0,
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
  ...extra,
});

const intervals = { again: 1, hard: 2, good: 5, easy: 40 };

function serve(data = summary) {
  api.routes.set("GET /cards/summary", () => ({ status: 200, body: data }));
  api.routes.set("GET /cards/decks", () => ({
    status: 200,
    body: [{ name: "后端", total: 12, due: 1, newCards: 4 }],
  }));
  api.routes.set("GET /cards/queue", () => ({
    status: 200,
    body: {
      items: [
        {
          card: card(1, { sourceTitle: "WAL 笔记", sourceLink: "/notes/3" }),
          intervals,
        },
        { card: card(2, { state: "new", reps: 0 }), intervals },
      ],
      summary: data,
    },
  }));
  api.routes.set("POST /cards/1/review", () => ({
    status: 200,
    body: { card: card(1), intervalDays: 5, summary: data },
  }));
  api.routes.set("POST /cards/2/review", () => ({
    status: 200,
    body: { card: card(2), intervalDays: 40, summary: data },
  }));
}

const key = (k: string) => fireEvent.keyDown(window, { key: k });
const reviews = () => api.calls.filter((c) => c.path.endsWith("/review"));

describe("cards page", () => {
  it("shows the day at a glance and reviews with the keyboard", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("开始复习")).toBeTruthy();
    expect(screen.getByText("今天待复习")).toBeTruthy();
    expect(screen.getByText("86%")).toBeTruthy();
    expect(screen.getByText(/1 张旧卡 · 1 张新卡/)).toBeTruthy();

    fireEvent.click(screen.getByText("开始复习"));
    expect(await screen.findByText("问题 1")).toBeTruthy();
    expect(screen.getByText("1 / 2")).toBeTruthy();
    // The answer is hidden and the number keys do nothing yet.
    expect(screen.queryByText("答案 1")).toBeNull();
    key("3");
    expect(reviews()).toHaveLength(0);

    // Space shows the answer, with the next interval under each rating.
    key(" ");
    expect(await screen.findByText("答案 1")).toBeTruthy();
    expect(screen.getByText("来源")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "WAL 笔记" }).getAttribute("href"),
    ).toBe("/notes/3");
    expect(screen.getByText("5 天")).toBeTruthy();
    expect(screen.getByText("1 个月")).toBeTruthy();

    // Key 3 rates "Good" and moves on to the next card.
    key("3");
    expect(await screen.findByText("问题 2")).toBeTruthy();
    expect(reviews()).toEqual([
      { method: "POST", path: "/cards/1/review", body: { rating: 3, reps: 1 } },
    ]);

    // Space twice: show the answer, then "Good". The last card ends the review.
    key(" ");
    await screen.findByText("答案 2");
    key("4");
    expect(await screen.findByText("复习完成")).toBeTruthy();
    expect(reviews().map((c) => c.body)).toEqual([
      { rating: 3, reps: 1 },
      { rating: 4, reps: 0 },
    ]);
    expect(
      document.querySelector(".cards-summary-main strong")?.textContent,
    ).toBe("2");
    expect(screen.getByText("良好")).toBeTruthy();
    fireEvent.click(screen.getByText("返回"));
    expect(await screen.findByText("开始复习")).toBeTruthy();
  });

  it("rates with the buttons and skips a card that was answered elsewhere", async () => {
    serve();
    api.routes.set("POST /cards/1/review", () => ({
      status: 409,
      body: { code: "card_stale", message: "这张卡已经答过了" },
    }));
    renderIt();
    fireEvent.click(await screen.findByText("开始复习"));
    await screen.findByText("问题 1");
    fireEvent.click(screen.getByText("显示答案"));
    fireEvent.click((await screen.findByText("简单")).closest("button")!);
    // The stale card is skipped, the next one shows up.
    expect(await screen.findByText("问题 2")).toBeTruthy();
    // No answer was counted for it.
    fireEvent.click(screen.getByText("结束"));
    expect(await screen.findByText("开始复习")).toBeTruthy();
  });

  it("does not take shortcut keys while typing", async () => {
    serve();
    renderIt();
    fireEvent.click(await screen.findByText("开始复习"));
    await screen.findByText("问题 1");
    const input = document.createElement("input");
    document.body.appendChild(input);
    fireEvent.keyDown(input, { key: " " });
    expect(screen.queryByText("答案 1")).toBeNull();
    input.remove();
  });

  it("has an empty state without cards and one for a finished day", async () => {
    serve({ ...summary, due: 0, dueReview: 0, dueNew: 0, total: 0 });
    renderIt();
    expect(await screen.findByText("还没有卡片")).toBeTruthy();
    cleanup();
    serve({ ...summary, due: 0, dueReview: 0, dueNew: 0 });
    renderIt();
    expect(await screen.findByText("今天的卡片都复习完了")).toBeTruthy();
    expect(screen.queryByText("开始复习")).toBeNull();
  });

  it("lists, searches, pauses and deletes cards", async () => {
    serve();
    api.routes.set("GET /cards", () => ({
      status: 200,
      body: { items: [card(1), card(2, { suspended: true })], total: 2 },
    }));
    api.routes.set("PATCH /cards/1", () => ({
      status: 200,
      body: card(1, { suspended: true }),
    }));
    api.routes.set("DELETE /cards/2", () => ({ status: 204 }));
    renderIt("/cards?tab=cards");
    expect(await screen.findByText("问题 1")).toBeTruthy();
    expect(screen.getByText("已暂停")).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText("搜索卡片"), {
      target: { value: "wal" },
    });
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.includes("q=wal"))).toBe(true),
    );

    const menus = screen.getAllByRole("button", { name: "更多" });
    fireEvent.click(menus[0]);
    fireEvent.click(await screen.findByText("暂停"));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
        suspended: true,
      }),
    );

    fireEvent.click(screen.getAllByRole("button", { name: "更多" })[1]);
    fireEvent.click(await screen.findByText("删除"));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === "DELETE" && c.path === "/cards/2"),
      ).toBe(true),
    );
  });

  it("creates a card by hand and saves settings", async () => {
    serve();
    api.routes.set("POST /cards", () => ({ status: 201, body: card(9) }));
    api.routes.set("GET /cards/settings", () => ({
      status: 200,
      body: {
        newPerDay: 10,
        reviewPerDay: 100,
        retention: 0.9,
        pushEnabled: true,
        pushTime: "20:00",
        aiEnabled: true,
        aiAvailable: false,
      },
    }));
    api.routes.set("PUT /cards/settings", (body) => ({
      status: 200,
      body: {
        ...(body as object),
        aiEnabled: true,
        aiAvailable: false,
        pushEnabled: true,
        pushTime: "20:00",
      },
    }));
    renderIt("/cards?new=1");
    const boxes = await screen.findAllByRole("textbox");
    fireEvent.change(boxes[0], { target: { value: "什么是 FSRS？" } });
    fireEvent.change(boxes[1], { target: { value: "一种间隔重复算法。" } });
    fireEvent.click(screen.getByText("保存"));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.method === "POST" && c.path === "/cards")?.body,
      ).toEqual({
        question: "什么是 FSRS？",
        answer: "一种间隔重复算法。",
      }),
    );

    cleanup();
    renderIt("/cards?tab=settings");
    await screen.findByText("目标记忆保持率（%）");
    const retention = document.querySelector('input[min="70"]')!;
    fireEvent.change(retention, { target: { value: "85" } });
    fireEvent.click(screen.getAllByText("保存")[0]);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        newPerDay: 10,
        reviewPerDay: 100,
        retention: 0.85,
      }),
    );
    expect(
      screen.getByText(/选中的文字和笔记或文章的标题会发给你的 AI 供应商/),
    ).toBeTruthy();
  });
});
