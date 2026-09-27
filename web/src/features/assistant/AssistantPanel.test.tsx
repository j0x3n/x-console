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

// 事件流换成可以手动发事件的版本。
const events = vi.hoisted(() => {
  const listeners = new Set<
    (e: { topic: string; data: unknown; at: string }) => void
  >();
  return {
    listeners,
    emit: (topic: string, data: unknown) =>
      listeners.forEach((fn) => fn({ topic, data, at: "" })),
  };
});
vi.mock("../../api/events", () => ({
  onServerEvent: (
    fn: (e: { topic: string; data: unknown; at: string }) => void,
  ) => {
    events.listeners.add(fn);
    return () => events.listeners.delete(fn);
  },
  invalidateOn: () => {},
  useServerEvent: () => {},
}));

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import AssistantPanel from "./AssistantPanel";
import { useAssistant } from "./store";

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/projects/XC"]}>
        <main id="main">
          <div className="xc-page-title">
            <h1>XC 项目</h1>
          </div>
        </main>
        <AssistantPanel />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const at = "2026-09-27T08:00:00Z";
function live() {
  let messages: unknown[] = [];
  let running = false;
  api.routes.set("GET /ai/conversations", () => ({ status: 200, body: [] }));
  api.routes.set("GET /ai/settings", () => ({
    status: 200,
    body: {
      hasApiKey: true,
      model: "claude-opus-5-5",
      confirmAllWrites: false,
    },
  }));
  api.routes.set("GET /ai/tools", () => ({
    status: 200,
    body: [
      {
        name: "notes__delete",
        action: "notes.delete",
        title: "删除笔记",
        effect: "write",
      },
    ],
  }));
  api.routes.set("POST /ai/conversations", () => ({
    status: 201,
    body: { id: 5, title: "", createdAt: at, updatedAt: at },
  }));
  api.routes.set("POST /ai/conversations/5/messages", (body) => {
    running = true;
    messages = [
      {
        id: 1,
        seq: 1,
        role: "user",
        createdAt: at,
        content: [{ type: "text", text: (body as { text: string }).text }],
      },
    ];
    return { status: 202 };
  });
  api.routes.set("GET /ai/conversations/5", () => ({
    status: 200,
    body: {
      conversation: { id: 5, title: "删笔记", createdAt: at, updatedAt: at },
      messages,
      pendingActions: [],
      running,
    },
  }));
  return {
    finish: (more: unknown[], pendingActions: unknown[] = []) => {
      messages = [...messages, ...more];
      running = pendingActions.length > 0;
      api.routes.set("GET /ai/conversations/5", () => ({
        status: 200,
        body: {
          conversation: {
            id: 5,
            title: "删笔记",
            createdAt: at,
            updatedAt: at,
          },
          messages,
          pendingActions,
          running,
        },
      }));
    },
  };
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  localStorage.clear();
  useAssistant.setState({
    open: false,
    expanded: false,
    conversationId: null,
    streaming: {},
    errors: {},
  });
});

describe("assistant panel", () => {
  it("opens with ⌘J and says when the server has no assistant", async () => {
    renderPanel();
    fireEvent.keyDown(document, { key: "j", metaKey: true });
    expect(await screen.findByText("AI 助手还没上线")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    expect(screen.getByRole("button", { name: /AI 助手/ })).toBeTruthy();
  });

  it("asks for an API key first", async () => {
    live();
    api.routes.set("GET /ai/settings", () => ({
      status: 200,
      body: {
        hasApiKey: false,
        model: "claude-opus-5-5",
        confirmAllWrites: false,
      },
    }));
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /AI 助手/ }));
    expect(await screen.findByText("还没有填 API Key")).toBeTruthy();
  });

  it("sends with the page context, streams, then confirms an action", async () => {
    const server = live();
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /AI 助手/ }));
    const box = await screen.findByLabelText("消息");
    await screen.findByText("有什么要我做的？");
    fireEvent.change(box, { target: { value: "删掉周报笔记" } });
    fireEvent.keyDown(box, { key: "Enter" });

    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === "/ai/conversations/5/messages")?.body,
      ).toEqual({
        text: "删掉周报笔记",
        context: { path: "/projects/XC", title: "XC 项目" },
      }),
    );
    expect(await screen.findByText("删掉周报笔记")).toBeTruthy();
    expect(useAssistant.getState().conversationId).toBe(5);

    act(() => {
      events.emit("ai.delta", { conversationId: 5, text: "好的，" });
      events.emit("ai.delta", { conversationId: 5, text: "我来删。" });
    });
    expect(await screen.findByText("好的，我来删。")).toBeTruthy();

    server.finish(
      [
        {
          id: 2,
          seq: 2,
          role: "assistant",
          createdAt: at,
          content: [
            { type: "text", text: "好的，我来删。" },
            {
              type: "tool_use",
              id: "tu1",
              name: "notes__delete",
              input: { id: 7 },
            },
          ],
        },
      ],
      [
        {
          id: 11,
          conversationId: 5,
          toolUseId: "tu1",
          action: "notes.delete",
          input: { id: 7 },
          status: "pending",
        },
      ],
    );
    api.routes.set("POST /ai/actions/11/approve", () => ({ status: 204 }));
    act(() => events.emit("ai.action_pending", { conversationId: 5 }));

    expect(await screen.findByText("删除笔记")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "POST" && c.path === "/ai/actions/11/approve",
        ),
      ).toBe(true),
    );
  });

  it("does not send while the input method is composing", async () => {
    live();
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /AI 助手/ }));
    const box = await screen.findByLabelText("消息");
    fireEvent.change(box, { target: { value: "你好" } });
    fireEvent.keyDown(box, { key: "Enter", isComposing: true });
    fireEvent.keyDown(box, { key: "Enter", shiftKey: true });
    await new Promise((r) => setTimeout(r, 30));
    expect(api.calls.some((c) => c.path.endsWith("/messages"))).toBe(false);
  });
});
