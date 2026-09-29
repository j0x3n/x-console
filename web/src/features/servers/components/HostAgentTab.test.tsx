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
import type { HostDetail } from "../api";
import HostAgentTab from "./HostAgentTab";
import "../i18n";

const host = {
  id: "h1",
  name: "web-1",
  kind: "server",
  source: "agent",
  online: true,
  hostname: "web-1",
  os: "linux",
  arch: "amd64",
  capabilities: [],
} as unknown as HostDetail;

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <HostAgentTab host={host} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const at = "2026-09-29T08:00:00Z";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  vi.restoreAllMocks();
});

describe("host Agent tab", () => {
  it("says it is not live when the server has no host agent yet", async () => {
    api.routes.set("GET /ai/host-agent/h1/conversations", () => ({
      status: 501,
      body: { code: "not_live", message: "not live" },
    }));
    renderTab();
    expect(await screen.findByText("Agent还没上线")).toBeTruthy();
  });

  it("creates a conversation with the chosen permission, then sends", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    api.routes.set("GET /ai/host-agent/h1/conversations", () => ({
      status: 200,
      body: [],
    }));
    api.routes.set("POST /ai/host-agent/h1/conversations", () => ({
      status: 201,
      body: { id: 7, title: "", hostId: "h1", createdAt: at, updatedAt: at },
    }));
    api.routes.set("PUT /ai/conversations/7/permission", (body) => ({
      status: 200,
      body,
    }));
    api.routes.set("POST /ai/conversations/7/messages", () => ({
      status: 202,
    }));
    api.routes.set("GET /ai/conversations/7", () => ({
      status: 200,
      body: {
        conversation: {
          id: 7,
          title: "看磁盘",
          hostId: "h1",
          permission: "all_auto",
          createdAt: at,
          updatedAt: at,
        },
        messages: [
          {
            id: 1,
            seq: 1,
            role: "assistant",
            createdAt: at,
            content: [
              {
                type: "tool_use",
                id: "t1",
                name: "host__run_command",
                input: { command: "df -h", reason: "先看各分区的占用" },
              },
            ],
          },
          {
            id: 2,
            seq: 2,
            role: "user",
            createdAt: at,
            content: [
              {
                type: "tool_result",
                tool_use_id: "t1",
                content: "/dev/sda1 91%",
              },
            ],
          },
        ],
        pendingActions: [],
        running: false,
      },
    }));
    const view = renderTab();

    await screen.findByText("要我在这台机器上做什么？");
    fireEvent.change(screen.getByLabelText("权限"), {
      target: { value: "all_auto" },
    });
    await waitFor(() =>
      expect((screen.getByLabelText("权限") as HTMLSelectElement).value).toBe(
        "all_auto",
      ),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "看看为什么磁盘快满了" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "发送" }));

    expect(await screen.findByText("df -h")).toBeTruthy();
    const order = api.calls
      .filter((c) => c.method !== "GET")
      .map((c) => `${c.method} ${c.path}`);
    expect(order).toEqual([
      "POST /ai/host-agent/h1/conversations",
      "PUT /ai/conversations/7/permission",
      "POST /ai/conversations/7/messages",
    ]);
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      mode: "all_auto",
    });
    expect(
      api.calls.find((c) => c.path === "/ai/conversations/7/messages")?.body,
    ).toEqual({ text: "看看为什么磁盘快满了" });

    // 离开页面后“全部自动”改回每步确认。
    view.unmount();
    await waitFor(() =>
      expect(api.calls.filter((c) => c.method === "PUT").at(-1)?.body).toEqual({
        mode: "confirm",
      }),
    );
  });
});
