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
    const path = new URL(req.url).pathname.replace("/api/v1", "");
    const text = await req.text();
    const body = text ? JSON.parse(text) : null;
    calls.push({ method: req.method, path, body });
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

vi.mock("../../../api/events", () => ({
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

vi.mock("../../../components/ui/ConfirmDialog", () => ({
  confirmAction: async () => true,
}));

import MemoryCard from "./MemoryCard";
import "../i18n";

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <MemoryCard />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const list = {
  enabled: true,
  items: [
    {
      id: 1,
      text: "主路由是 OpenWrt",
      source: "ai",
      createdAt: "2026-10-01T00:00:00Z",
      updatedAt: "2026-10-01T00:00:00Z",
    },
  ],
  usedChars: 12,
  limitChars: 4000,
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("记忆卡片（B61）", () => {
  it("adds, edits and deletes through the API", async () => {
    api.routes.set("GET /ai/memories", () => ({ status: 200, body: list }));
    api.routes.set("POST /ai/memories", (body) => ({
      status: 201,
      body: { ...list.items[0], id: 2, source: "user", ...(body as object) },
    }));
    api.routes.set("PATCH /ai/memories/1", (body) => ({
      status: 200,
      body: { ...list.items[0], ...(body as object) },
    }));
    api.routes.set("DELETE /ai/memories/1", () => ({ status: 204 }));
    renderCard();
    await screen.findByText("AI 记的");
    expect(screen.getByText("12 / 4000 字")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("添加一条记忆"), {
      target: { value: "周报周五写" },
    });
    fireEvent.click(screen.getByRole("button", { name: "添加" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
      text: "周报周五写",
    });

    fireEvent.click(screen.getByRole("button", { name: "主路由是 OpenWrt" }));
    const input = screen.getByLabelText("编辑记忆");
    fireEvent.change(input, { target: { value: "主路由是 OpenWrt 23.05" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );

    fireEvent.click(
      screen.getByRole("button", { name: "删除: 主路由是 OpenWrt" }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });

  it("shows the server's message when the limit is reached", async () => {
    api.routes.set("GET /ai/memories", () => ({ status: 200, body: list }));
    api.routes.set("POST /ai/memories", () => ({
      status: 400,
      body: {
        code: "validation_failed",
        message: "记忆总共最多 4000 字，先删掉一些",
      },
    }));
    renderCard();
    fireEvent.change(await screen.findByLabelText("添加一条记忆"), {
      target: { value: "再加一条" },
    });
    fireEvent.click(screen.getByRole("button", { name: "添加" }));
    await screen.findByText("记忆总共最多 4000 字，先删掉一些");
  });

  it("says it is not live when the server has no memory yet", async () => {
    renderCard();
    await screen.findByText("还没上线");
  });
});
