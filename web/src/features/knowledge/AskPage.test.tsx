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
import AskPage from "./AskPage";
import "./i18n";

const status = {
  enabled: true,
  available: true,
  syncing: false,
  docs: 12,
  chunks: 40,
  sources: [
    { source: "notes", docs: 10, chunks: 30 },
    { source: "journal", docs: 2, chunks: 10 },
  ],
  lastSyncAt: "2026-11-11T02:00:00Z",
};
const settings = {
  enabled: true,
  model: "",
  sources: ["notes", "readlater", "journal", "cards"],
};
const answer = {
  answer: "把 80 端口让出来。[1]",
  sources: [
    {
      source: "notes",
      title: "Caddy 证书",
      link: "/notes/3",
      snippet: "Caddy 续证书失败，原因是 80 端口被占用。",
      score: 0.83,
    },
    {
      source: "readlater",
      title: "ACME 验证",
      link: "https://example.com/acme",
      snippet: "HTTP-01 验证走 80 端口。",
      score: 0.61,
    },
  ],
};

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AskPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function routes(s: unknown = status) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /knowledge/status", () => ({ status: 200, body: s }));
  api.routes.set("GET /knowledge/settings", () => ({
    status: 200,
    body: settings,
  }));
}

beforeEach(() => routes());
afterEach(cleanup);

describe("资料问答页", () => {
  it("没开启时说明会发送什么，并给出开启入口", async () => {
    routes({ ...status, enabled: false });
    renderPage();
    expect(await screen.findByText("资料问答还没开启")).toBeTruthy();
    expect(screen.getByText(/转成向量/)).toBeTruthy();
    expect(screen.queryByLabelText("问题")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "去开启" }));
    expect(await screen.findByLabelText("开启资料问答")).toBeTruthy();
  });

  it("开启后显示索引数量和各来源", async () => {
    renderPage();
    expect(await screen.findByText("12")).toBeTruthy();
    expect(screen.getByText(/40 段/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "立即同步" })).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("提问：显示回答、出处和核对提示，网页出处在新标签页打开", async () => {
    api.routes.set("POST /knowledge/ask", () => ({
      status: 200,
      body: answer,
    }));
    renderPage();
    fireEvent.change(await screen.findByLabelText("问题"), {
      target: { value: "证书续不上怎么办" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    const result = await screen.findByRole("region", { name: "回答" });
    expect(within(result).getByText("把 80 端口让出来。[1]")).toBeTruthy();
    expect(within(result).getByText(/可能有错/)).toBeTruthy();
    expect(within(result).getByText("83%")).toBeTruthy();
    const links = within(result).getAllByRole("link", { name: /打开/ });
    expect(links[0].getAttribute("href")).toBe("/notes/3");
    expect(links[0].getAttribute("target")).toBeNull();
    expect(links[1].getAttribute("href")).toBe("https://example.com/acme");
    expect(links[1].getAttribute("target")).toBe("_blank");
    expect(links[1].getAttribute("rel")).toContain("noopener");
    const call = api.calls.find((c) => c.path === "/knowledge/ask");
    expect(call?.body).toEqual({ question: "证书续不上怎么办" });
  });

  it("没有 AI 回答时说明，仍然列出片段", async () => {
    api.routes.set("POST /knowledge/ask", () => ({
      status: 200,
      body: { sources: answer.sources },
    }));
    renderPage();
    fireEvent.change(await screen.findByLabelText("问题"), {
      target: { value: "证书" },
    });
    fireEvent.click(screen.getByRole("button", { name: "提问" }));
    expect(await screen.findByText(/没有可用的 AI 写回答/)).toBeTruthy();
    expect(screen.getByText("Caddy 证书")).toBeTruthy();
  });

  it("同步出错和没有供应商时显示原因", async () => {
    routes({
      ...status,
      available: false,
      lastError: "嵌入接口出错：404",
    });
    renderPage();
    expect(await screen.findByText(/还没有配置 AI 供应商/)).toBeTruthy();
    expect(screen.getByText(/嵌入接口出错：404/)).toBeTruthy();
  });

  it("同步中按钮不能点", async () => {
    routes({ ...status, syncing: true });
    renderPage();
    const button = await screen.findByRole("button", { name: "正在同步…" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it("保存设置：只提交勾选的来源", async () => {
    api.routes.set("PUT /knowledge/settings", (body) => ({
      status: 200,
      body,
    }));
    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /资料问答设置/ }),
    );
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByLabelText("稍后读"));
    fireEvent.change(within(dialog).getByLabelText("嵌入模型"), {
      target: { value: "bge-m3" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      enabled: true,
      model: "bge-m3",
      sources: ["notes", "journal", "cards"],
    });
  });

  it("嵌入可以选别的供应商（B197）", async () => {
    api.routes.set("GET /ai/providers", () => ({
      status: 200,
      body: [
        {
          id: 3,
          name: "Google",
          baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai",
          apiStyle: "chat",
          hasApiKey: true,
          modelCount: 0,
          createdAt: "2026-10-13T00:00:00Z",
        },
      ],
    }));
    api.routes.set("PUT /knowledge/settings", (body) => ({
      status: 200,
      body,
    }));
    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /资料问答设置/ }),
    );
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByRole("option", { name: "Google" });
    fireEvent.change(within(dialog).getByLabelText(/嵌入用的供应商/), {
      target: { value: "3" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toMatchObject({
      providerId: 3,
    });
  });

  it("接口还没上线时显示还没上线", async () => {
    api.routes.clear();
    renderPage();
    expect(
      await screen.findByText(/还没上线|尚未上线|还没有上线/),
    ).toBeTruthy();
  });
});
