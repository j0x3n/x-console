// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

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

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { routes } from "./routes";

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

const at = "2026-09-27T08:00:00Z";
const catalog = {
  topics: [{ topic: "monitor.down", title: "网站挂了" }],
  actions: [
    {
      name: "notify.send",
      title: "发通知",
      effect: "write",
      input: {
        type: "object",
        properties: { title: { type: "string" }, body: { type: "string" } },
        required: ["title"],
      },
    },
    {
      name: "scripts.run",
      title: "运行脚本",
      effect: "dangerous",
      input: { type: "object", properties: { id: { type: "integer" } } },
    },
  ],
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("automations", () => {
  it("says it is not live when the server has no automations", async () => {
    renderAt("/automations");
    expect(await screen.findByText("自动化还没上线")).toBeTruthy();
  });

  it("lists rules with their trigger and actions", async () => {
    api.routes.set("GET /automations/catalog", () => ({
      status: 200,
      body: catalog,
    }));
    api.routes.set("GET /automations", () => ({
      status: 200,
      body: [
        {
          id: 1,
          name: "CPU 太高",
          enabled: true,
          authorized: false,
          trigger: { type: "metric", metric: "cpu", op: ">", value: 90 },
          conditions: [],
          actions: [{ action: "notify.send", input: { title: "CPU" } }],
          cooldownSeconds: 600,
          createdAt: at,
          updatedAt: at,
        },
      ],
    }));
    renderAt("/automations");
    expect(await screen.findByText("CPU 太高")).toBeTruthy();
    expect(screen.getByText(/任意服务器 CPU > 90% → 发通知/)).toBeTruthy();
  });

  it("builds a rule from the form and saves it", async () => {
    api.routes.set("GET /automations/catalog", () => ({
      status: 200,
      body: catalog,
    }));
    api.routes.set("POST /automations", (body) => ({
      status: 201,
      body: {
        ...(body as object),
        id: 3,
        authorized: false,
        createdAt: at,
        updatedAt: at,
      },
    }));
    api.routes.set("GET /automations/3", () => ({
      status: 404,
      body: { code: "not_found", message: "x" },
    }));
    const router = renderAt("/automations/new");
    fireEvent.change(await screen.findByLabelText("名称"), {
      target: { value: "网站挂了就通知" },
    });
    fireEvent.click(screen.getByRole("radio", { name: "事件" }));
    fireEvent.change(screen.getByLabelText(/^事件主题/), {
      target: { value: "monitor.down" },
    });

    // 没有动作时不让保存。
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    expect(await screen.findByText("至少加一个动作")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /加动作/ }));
    fireEvent.change(screen.getByLabelText("动作 1"), {
      target: { value: "notify.send" },
    });
    fireEvent.change(screen.getByLabelText(/^title/), {
      target: { value: "{{trigger.data.url}} 挂了" },
    });
    fireEvent.click(screen.getByRole("button", { name: /加条件/ }));
    fireEvent.change(screen.getByLabelText("字段 1"), {
      target: { value: "data.status" },
    });
    fireEvent.change(screen.getByLabelText("值 1"), {
      target: { value: "500" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() =>
      expect(
        api.calls.find((c) => c.method === "POST" && c.path === "/automations")
          ?.body,
      ).toEqual({
        name: "网站挂了就通知",
        enabled: true,
        trigger: { type: "event", topic: "monitor.down" },
        conditions: [{ field: "data.status", op: "==", value: "500" }],
        actions: [
          {
            action: "notify.send",
            input: { title: "{{trigger.data.url}} 挂了" },
          },
        ],
        cooldownSeconds: 0,
      }),
    );
    await waitFor(() =>
      expect(router.state.location.pathname).toBe("/automations/3"),
    );
  });

  it("warns about dangerous actions", async () => {
    api.routes.set("GET /automations/catalog", () => ({
      status: 200,
      body: catalog,
    }));
    renderAt("/automations/new");
    fireEvent.click(await screen.findByRole("button", { name: /加动作/ }));
    fireEvent.change(screen.getByLabelText("动作 1"), {
      target: { value: "scripts.run" },
    });
    expect(screen.getByText(/高危动作，保存时要再验证一次/)).toBeTruthy();
  });
});
