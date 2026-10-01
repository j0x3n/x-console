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
import "./i18n";

import AgentDialog from "./AgentDialog";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("AgentDialog", () => {
  it("creates a command line agent with its repositories", async () => {
    api.routes.set("GET /agents", () => ({
      status: 200,
      body: [
        {
          id: "pc1",
          name: "台式机",
          kind: "desktop",
          online: true,
          capabilities: ["coding"],
        },
        {
          id: "srv",
          name: "服务器",
          kind: "server",
          online: true,
          capabilities: [],
        },
      ],
    }));
    api.routes.set("GET /coding/repos", () => ({
      status: 200,
      body: [
        { id: 7, name: "demo", remoteRepo: "team/demo", agentName: "台式机" },
      ],
    }));
    let sent: unknown;
    api.routes.set("POST /ai-agents", (body) => {
      sent = body;
      return { status: 201, body: { id: 1, ...(body as object) } };
    });
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const onClose = vi.fn();
    render(
      <QueryClientProvider client={qc}>
        <AgentDialog onClose={onClose} />
      </QueryClientProvider>,
    );
    fireEvent.change(screen.getByPlaceholderText("比如：后端开发"), {
      target: { value: "后端" },
    });
    fireEvent.click(screen.getByLabelText(/^Codex/));
    // Only machines with Claude Code or Codex are offered.
    const machine = await screen.findByRole("option", { name: "台式机" });
    expect(screen.queryByRole("option", { name: "服务器" })).toBeNull();
    fireEvent.change(machine.closest("select")!, { target: { value: "pc1" } });
    fireEvent.click(await screen.findByLabelText(/team\/demo/));
    fireEvent.click(screen.getByLabelText(/^不限制/));
    expect(screen.getByRole("note").textContent).toContain("弄坏");
    fireEvent.change(screen.getByPlaceholderText("不限"), {
      target: { value: "12.5" },
    });
    fireEvent.click(screen.getByRole("button", { name: "创建 Agent" }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(sent).toMatchObject({
      name: "后端",
      kind: "codex",
      runnerAgentId: "pc1",
      repoIds: [7],
      cliPermission: "full",
      monthlyBudgetUsd: 12.5,
      autoBuild: true,
      buildRetries: 2,
    });
  });
});
