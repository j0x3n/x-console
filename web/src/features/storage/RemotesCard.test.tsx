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

const toasts = vi.hoisted(() => [] as unknown[]);
vi.mock("../../hooks/useToast", () => ({
  toast: (t: unknown) => toasts.push(t),
}));

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
import RemotesCard from "./RemotesCard";

const accounts = [
  {
    id: 3,
    kind: "webdav",
    name: "坚果云",
    showInDrive: true,
    ready: true,
    usedByBackup: true,
    createdAt: "2026-10-01T00:00:00Z",
    webdav: {
      url: "https://dav.jianguoyun.com/dav/",
      username: "me",
      passwordSet: true,
    },
  },
  {
    id: 4,
    kind: "gdrive",
    name: "Google Drive",
    showInDrive: false,
    ready: false,
    usedByBackup: false,
    createdAt: "2026-10-01T00:00:00Z",
    gdrive: {
      clientId: "c",
      secretSet: true,
      authorized: false,
      limited: false,
      redirectUri:
        "https://x.example.com/api/v1/storage/remotes/gdrive/callback",
    },
  },
];

function show() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <MemoryRouter initialEntries={["/settings/storage"]}>
      <QueryClientProvider client={qc}>
        <RemotesCard />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  toasts.length = 0;
});

describe("drive accounts card (B69)", () => {
  it("lists accounts and starts the Google authorization", async () => {
    api.routes.set("GET /storage/remotes", () => ({
      status: 200,
      body: { items: accounts },
    }));
    api.routes.set("GET /storage/remotes/4/gdrive/auth", () => ({
      status: 500,
      body: { code: "internal", message: "stop here" },
    }));
    show();
    expect(await screen.findByText("坚果云")).toBeTruthy();
    expect(screen.getByText("备份在用")).toBeTruthy();
    expect(screen.getByText("云盘页不显示")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "授权" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path === "/storage/remotes/4/gdrive/auth"),
      ).toBe(true),
    );
  });

  it("adds a WebDAV account after testing it", async () => {
    api.routes.set("GET /storage/remotes", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /storage/remotes/test", () => ({
      status: 200,
      body: { ok: true, message: "连接正常，可以读写" },
    }));
    api.routes.set("POST /storage/remotes", () => ({
      status: 201,
      body: accounts[0],
    }));
    show();
    expect(await screen.findByText("还没有网盘账号")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /添加网盘账号/ })[0]);
    fireEvent.change(await screen.findByLabelText(/^地址/), {
      target: { value: " https://dav.jianguoyun.com/dav/ " },
    });
    fireEvent.change(screen.getByLabelText("用户名"), {
      target: { value: "me" },
    });
    fireEvent.change(screen.getByLabelText(/^密码/), {
      target: { value: "app-pw" },
    });
    fireEvent.click(screen.getByRole("button", { name: "测试连接" }));
    await screen.findByText("连接正常，可以读写");
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(
        api.calls.find(
          (c) => c.method === "POST" && c.path === "/storage/remotes",
        )?.body,
      ).toEqual({
        name: "",
        showInDrive: true,
        kind: "webdav",
        webdav: {
          url: "https://dav.jianguoyun.com/dav/",
          username: "me",
          password: "app-pw",
        },
      }),
    );
  });
});
