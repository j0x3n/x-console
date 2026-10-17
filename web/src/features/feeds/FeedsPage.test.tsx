// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 组件之前替换。
const api = vi.hoisted(() => {
  type Reply = { status: number; body?: unknown };
  const routes = new Map<
    string,
    (search: URLSearchParams, body: unknown) => Reply
  >();
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
      ? handler(url.searchParams, body)
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
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import NewsPage, { readNewsFilter } from "./NewsPage";
import TweetsPage, { handleOf } from "./TweetsPage";
import "./i18n";

function renderAt(url: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route path="/feeds" element={<NewsPage />} />
          <Route path="/feeds/tweets" element={<TweetsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const configured = {
  agentId: "vm",
  baseUrl: "http://127.0.0.1:8790",
  hasToken: true,
  configured: true,
};

function news(id: number, extra: Record<string, unknown> = {}) {
  return {
    id,
    title: `Title ${id}`,
    titleCn: `标题 ${id}`,
    summaryCn: `摘要 ${id}`,
    url: `https://example.com/${id}`,
    source: "Example",
    category: "model",
    tags: ["agent"],
    importance: 4,
    pushed: id % 2 === 0,
    fetchedAt: "2026-10-17T08:00:00+00:00",
    ...extra,
  };
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("资讯的筛选条件", () => {
  it("从地址栏读筛选，认不出的天数按全部", () => {
    const f = readNewsFilter(
      new URLSearchParams("q=gpt&tag=agent&days=7&important=1"),
    );
    expect(f).toEqual({
      q: "gpt",
      tag: "agent",
      category: "",
      days: 7,
      important: true,
      pushed: false,
    });
    expect(readNewsFilter(new URLSearchParams("days=3")).days).toBe(0);
  });

  it("作者栏取账号来筛选", () => {
    expect(handleOf("Sam Altman @sama")).toBe("@sama");
    expect(handleOf("没有账号")).toBe("没有账号");
  });
});

describe("新闻页", () => {
  it("没配置时提示去设置，不去请求新闻", async () => {
    api.routes.set("GET /feeds/settings", () => ({
      status: 200,
      body: { ...configured, agentId: "", hasToken: false, configured: false },
    }));
    renderAt("/feeds");
    await screen.findByText("资讯还没配置");
    expect(screen.getByRole("link", { name: "去设置" })).toBeTruthy();
    expect(api.calls.some((c) => c.path.startsWith("/feeds/news"))).toBe(false);
  });

  it("列出新闻，翻到下一页，筛选写进请求", async () => {
    api.routes.set("GET /feeds/settings", () => ({
      status: 200,
      body: configured,
    }));
    api.routes.set("GET /feeds/news/facets", () => ({
      status: 200,
      body: { categories: ["model", "tool"], tags: [] },
    }));
    api.routes.set("GET /feeds/news", (q) =>
      q.get("before") === "2"
        ? { status: 200, body: { items: [news(1)] } }
        : { status: 200, body: { items: [news(3), news(2)], next: "2" } },
    );
    renderAt("/feeds?important=1");
    await screen.findByText("标题 3");
    expect(screen.getByText("摘要 3")).toBeTruthy();
    const first = api.calls.find((c) => c.path.startsWith("/feeds/news?"));
    expect(first?.path).toContain("minImportance=4");
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await screen.findByText("标题 1");
    expect(api.calls.some((c) => c.path.includes("before=2"))).toBe(true);
  });

  it("机器不在线时说清楚是哪台", async () => {
    api.routes.set("GET /feeds/settings", () => ({
      status: 200,
      body: configured,
    }));
    api.routes.set("GET /feeds/news/facets", () => ({
      status: 200,
      body: { categories: [], tags: [] },
    }));
    api.routes.set("GET /feeds/news", () => ({
      status: 503,
      body: { code: "agent_offline", message: "虚拟机不在线：hermes" },
    }));
    renderAt("/feeds");
    await screen.findByText("虚拟机不在线");
    expect(screen.getByText("虚拟机不在线：hermes")).toBeTruthy();
  });

  it("打开详情，在译文和原文之间切换，没有正文时说明", async () => {
    api.routes.set("GET /feeds/settings", () => ({
      status: 200,
      body: configured,
    }));
    api.routes.set("GET /feeds/news/facets", () => ({
      status: 200,
      body: { categories: [], tags: [] },
    }));
    api.routes.set("GET /feeds/news", () => ({
      status: 200,
      body: { items: [news(5), news(6)] },
    }));
    api.routes.set("GET /feeds/news/5", () => ({
      status: 200,
      body: {
        ...news(5),
        content: {
          status: "ok",
          markdown: "English body",
          markdownCn: "中文正文",
        },
        media: [],
      },
    }));
    api.routes.set("GET /feeds/news/6", () => ({
      status: 200,
      body: { ...news(6), content: { status: "missing" }, media: [] },
    }));
    renderAt("/feeds?item=5");
    await screen.findByText("中文正文");
    fireEvent.click(screen.getByRole("button", { name: "原文" }));
    await screen.findByText("English body");
    cleanup();
    renderAt("/feeds?item=6");
    await screen.findByText("正文还没抓取");
  });

  it("存进稍后读用的是原文链接", async () => {
    api.routes.set("GET /feeds/settings", () => ({
      status: 200,
      body: configured,
    }));
    api.routes.set("GET /feeds/news/facets", () => ({
      status: 200,
      body: { categories: [], tags: [] },
    }));
    api.routes.set("GET /feeds/news", () => ({
      status: 200,
      body: { items: [news(7)] },
    }));
    api.routes.set("POST /readlater", () => ({
      status: 201,
      body: { item: { id: 1 }, duplicate: false },
    }));
    renderAt("/feeds");
    await screen.findByText("标题 7");
    fireEvent.click(screen.getByRole("button", { name: "存进稍后读 标题 7" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        url: "https://example.com/7",
      }),
    );
  });
});

describe("推文页", () => {
  it("按作者筛选", async () => {
    api.routes.set("GET /feeds/settings", () => ({
      status: 200,
      body: configured,
    }));
    api.routes.set("GET /feeds/tweets", (q) => ({
      status: 200,
      body: {
        items:
          q.get("author") === "@sama"
            ? [
                {
                  id: "2",
                  author: "Sam @sama",
                  time: "2026-10-17T08:00:00Z",
                  url: "https://x.com/i/web/status/2",
                  text: "第一行\n第二行",
                },
              ]
            : [
                {
                  id: "1",
                  author: "Other @other",
                  time: "2026-10-17T07:00:00Z",
                  url: "https://x.com/i/web/status/1",
                  text: "别的推文",
                },
              ],
      },
    }));
    renderAt("/feeds/tweets");
    await screen.findByText("别的推文");
    fireEvent.change(screen.getByLabelText("按作者筛选"), {
      target: { value: "@sama" },
    });
    await screen.findByText(/第一行/, undefined, { timeout: 2000 });
    expect(api.calls.some((c) => c.path.includes("author=%40sama"))).toBe(true);
  });
});
