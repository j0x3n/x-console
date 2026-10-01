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
import BackupTab from "./BackupTab";

const settings = {
  enabled: false,
  frequency: "daily",
  time: "03:00",
  weekday: 0,
  keep: 14,
  target: "storage",
  lastRuns: [],
  webdav: { folder: "x-console-backups" },
  gdrive: { folderName: "X Console 备份" },
};

const accounts = [
  {
    id: 3,
    kind: "webdav",
    name: "坚果云",
    showInDrive: true,
    ready: true,
    usedByBackup: false,
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
    showInDrive: true,
    ready: false,
    usedByBackup: false,
    createdAt: "2026-10-01T00:00:00Z",
    gdrive: {
      clientId: "c",
      secretSet: true,
      authorized: false,
      limited: false,
      redirectUri: "x",
    },
  },
];

function serve() {
  api.routes.set("GET /backups", () => ({
    status: 200,
    body: {
      items: [
        {
          id: "a.tar.gz",
          name: "a.tar.gz",
          createdAt: "2026-10-01T03:00:00Z",
          sizeBytes: 1024,
          location: "gdrive",
          kind: "auto",
        },
      ],
    },
  }));
  api.routes.set("GET /backups/job", () => ({
    status: 200,
    body: { state: "idle" },
  }));
  api.routes.set("GET /backups/settings", () => ({
    status: 200,
    body: settings,
  }));
  api.routes.set("GET /storage/remotes", () => ({
    status: 200,
    body: { items: accounts },
  }));
}

function show(path = "/settings/backup") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={qc}>
        <BackupTab />
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

describe("backup tab (B63, B69)", () => {
  it("picks a drive account and sends only its folder", async () => {
    serve();
    api.routes.set("POST /backups/target/test", () => ({
      status: 200,
      body: { ok: true, message: "连接正常，可以读写" },
    }));
    show();
    const where = await screen.findByLabelText(/^备份到/);
    expect(
      await screen.findByRole("option", { name: "坚果云（WebDAV）" }),
    ).toBeTruthy();
    fireEvent.change(where, { target: { value: "remote:3" } });
    const folder = await screen.findByLabelText("文件夹");
    fireEvent.change(folder, { target: { value: "/my-backups/" } });
    fireEvent.click(screen.getByRole("button", { name: "测试连接" }));
    await screen.findByText("连接正常，可以读写");
    const sent = api.calls.find((c) => c.path === "/backups/target/test");
    expect(sent?.body).toEqual({
      target: "remote",
      remoteId: 3,
      webdav: { folder: "my-backups" },
    });

    // Google 账号没授权时提示去存储设置
    fireEvent.change(where, { target: { value: "remote:4" } });
    expect(await screen.findByLabelText(/^文件夹名称/)).toBeTruthy();
    expect(screen.getByText(/这个账号还没授权/)).toBeTruthy();
  });

  it("points to the storage settings when there is no account", async () => {
    serve();
    api.routes.set("GET /storage/remotes", () => ({
      status: 200,
      body: { items: [] },
    }));
    show();
    const link = await screen.findByRole("link", { name: "设置 → 存储" });
    expect(link.getAttribute("href")).toBe("/settings/storage");
  });
});
