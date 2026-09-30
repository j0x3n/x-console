// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 组件之前替换。
const api = vi.hoisted(() => {
  type Reply = { status: number; body?: unknown };
  const routes = new Map<string, (body: unknown, url: URL) => Reply>();
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
    calls.push({ method: req.method, path, body });
    const handler = routes.get(`${req.method} ${path}`);
    const reply = handler
      ? handler(body, url)
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
import { MemoryRouter } from "react-router";
import AiUsageTab from "./AiUsageTab";

const totals = (calls: number, input: number, cached: number) => ({
  calls,
  errors: 0,
  inputTokens: input,
  cachedInputTokens: cached,
  cacheWriteTokens: 0,
  outputTokens: 10,
  reasoningTokens: 0,
  avgDurationMs: 100,
  cacheHitRate: input ? cached / input : undefined,
  cost: 0.5,
});

function summary(groupBy: string) {
  const groups =
    groupBy === "day"
      ? [
          { key: "2026-03-01", totals: totals(1, 100, 80) },
          { key: "2026-03-02", totals: totals(0, 0, 0) },
        ]
      : groupBy === "model"
        ? [{ key: "gpt-x", providerName: "OpenAI", totals: totals(1, 100, 80) }]
        : [{ key: "notes", totals: totals(1, 100, 80) }];
  return {
    from: "2026-03-01",
    to: "2026-03-02",
    groupBy,
    total: totals(1, 100, 80),
    previous: totals(1, 50, 0),
    groups,
  };
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("AI usage tab", () => {
  it("shows the hit rate, the tables and filters calls by source", async () => {
    api.routes.set("GET /ai/usage/summary", (_b, url) => ({
      status: 200,
      body: summary(url.searchParams.get("groupBy")!),
    }));
    api.routes.set("GET /ai/usage/records", (_b, url) => ({
      status: 200,
      body: {
        items: [
          {
            id: 1,
            at: "2026-03-01T04:00:00Z",
            providerName: "OpenAI",
            model: "gpt-x",
            purpose: "fast",
            source: url.searchParams.get("source") ?? "notes",
            ref: "",
            status: "ok",
            inputTokens: 100,
            cachedInputTokens: 80,
            cacheWriteTokens: 0,
            outputTokens: 10,
            reasoningTokens: 0,
            durationMs: 1200,
            cost: 0.5,
          },
        ],
      },
    }));
    {
      const qc = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      render(
        <MemoryRouter>
          <QueryClientProvider client={qc}>
            <AiUsageTab />
          </QueryClientProvider>
        </MemoryRouter>,
      );
      await screen.findByText("每天的输入 token");
      expect(screen.getAllByText("80%").length).toBeGreaterThan(0);
      expect(screen.getByText("↑100%")).toBeTruthy(); // 输入 100 比上一段 50
      expect(screen.getByText("gpt-x", { selector: "button" })).toBeTruthy();
      fireEvent.click(screen.getByText("笔记", { selector: "button" }));
      await waitFor(() =>
        expect(
          screen.getByRole("link", { name: /导出 CSV/ }).getAttribute("href"),
        ).toContain("source=notes"),
      );
    }
  });
});
