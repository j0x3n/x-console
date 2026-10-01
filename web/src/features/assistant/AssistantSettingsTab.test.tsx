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
import AssistantSettingsTab from "./AssistantSettingsTab";

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <AssistantSettingsTab />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

const notLive = () => ({
  status: 501,
  body: { code: "not_live", message: "not live" },
});

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("AI settings", () => {
  it("shows an error when providers cannot load without calling old settings", async () => {
    api.routes.set("GET /ai/providers", notLive);
    renderTab();
    expect(await screen.findByText("not live")).toBeTruthy();
    expect(api.calls.some((call) => call.path === "/ai/settings")).toBe(false);
  });

  it("shows providers, models and usage when live", async () => {
    let saved: unknown = null;
    api.routes.set("GET /ai/providers", () => ({
      status: 200,
      body: [
        {
          id: 1,
          name: "OpenAI",
          baseUrl: "https://api.openai.com/v1",
          hasApiKey: true,
          modelCount: 2,
          createdAt: "2026-09-01T00:00:00Z",
        },
      ],
    }));
    api.routes.set("GET /ai/models", () => ({
      status: 200,
      body: [
        {
          providerId: 1,
          id: "gpt-5",
          toolCall: true,
          reasoning: true,
          contextWindow: 400000,
          inputPrice: 1.25,
          outputPrice: 10,
          specSource: "exact",
        },
        {
          providerId: 1,
          id: "text-only",
          toolCall: false,
          specSource: "exact",
        },
      ],
    }));
    api.routes.set("GET /ai/model-settings", () => ({
      status: 200,
      body: {
        reasoningEffort: "medium",
        reasoningUnsupported: false,
        confirmAllWrites: false,
        legacyAnthropic: true,
      },
    }));
    api.routes.set("PUT /ai/model-settings", (body) => {
      saved = body;
      return {
        status: 200,
        body: {
          agent: { providerId: 1, model: "gpt-5" },
          reasoningEffort: "medium",
          reasoningUnsupported: false,
          confirmAllWrites: false,
        },
      };
    });
    api.routes.set("GET /ai/usage", () => ({
      status: 200,
      body: {
        month: "2026-09",
        calls: 3,
        inputTokens: 12000,
        outputTokens: 800,
        cost: 0.02,
        byModel: [
          {
            providerId: 1,
            providerName: "OpenAI",
            model: "gpt-5",
            calls: 3,
            inputTokens: 12000,
            outputTokens: 800,
            cost: 0.02,
          },
        ],
      },
    }));
    api.routes.set("GET /notes/ai-settings", notLive);
    renderTab();

    expect(await screen.findByText("OpenAI")).toBeTruthy();
    expect(await screen.findByText(/AI 现在改用 OpenAI 兼容接口/)).toBeTruthy();
    expect(await screen.findByText("本月用量")).toBeTruthy();

    // 选 Agent 模型：不支持工具调用的不能选。
    fireEvent.click(screen.getByRole("button", { name: "Agent 模型" }));
    const off = await screen.findByRole("option", { name: /text-only/ });
    expect(off.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(off);
    fireEvent.click(screen.getByRole("option", { name: /gpt-5/ }));
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() => expect(saved).not.toBeNull());
    expect(saved).toEqual({
      fast: null,
      agent: { providerId: 1, model: "gpt-5" },
      reasoningEffort: "medium",
      // B60：没选过默认权限时按原来的开关，关着就是“写入”
      defaultPermission: "write",
      confirmAllWrites: false,
    });
    // 没配笔记 AI 时不显示笔记卡片。
    expect(screen.queryByText("自动起标题")).toBeNull();
  });
});
