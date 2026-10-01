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
  webdav: {
    url: "",
    username: "",
    folder: "x-console-backups",
    passwordSet: false,
  },
  gdrive: {
    clientId: "",
    folderName: "X Console 备份",
    secretSet: false,
    authorized: false,
  },
};

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

describe("backup tab (B63)", () => {
  it("shows the fields of each target and tests the form", async () => {
    serve();
    api.routes.set("POST /backups/target/test", () => ({
      status: 200,
      body: { ok: false, message: "用户名或密码不对" },
    }));
    show();
    const where = await screen.findByLabelText(/^备份到/);
    expect(
      screen.getByText("Google Drive", { selector: ".xc-badge" }),
    ).toBeTruthy();

    fireEvent.change(where, { target: { value: "webdav" } });
    fireEvent.change(screen.getByLabelText(/^地址/), {
      target: { value: "https://dav.jianguoyun.com/dav/" },
    });
    fireEvent.change(screen.getByLabelText(/^密码/), {
      target: { value: "pw" },
    });
    fireEvent.click(screen.getByRole("button", { name: "测试连接" }));
    await screen.findByText("用户名或密码不对");
    const call = api.calls.find((c) => c.path === "/backups/target/test");
    expect(call?.body).toMatchObject({
      target: "webdav",
      webdav: {
        url: "https://dav.jianguoyun.com/dav/",
        password: "pw",
        folder: "x-console-backups",
      },
    });

    fireEvent.change(where, { target: { value: "gdrive" } });
    expect(screen.getByLabelText(/^客户端 ID/)).toBeTruthy();
    expect(
      screen.getByText("http://localhost:3000/api/v1/backups/gdrive/callback"),
    ).toBeTruthy();
    expect(screen.queryByLabelText(/^地址/)).toBeNull();
  });

  it("authorizes Google Drive after saving the client", async () => {
    serve();
    api.routes.set("PUT /backups/settings", () => ({
      status: 200,
      body: { ...settings, gdrive: { ...settings.gdrive, secretSet: true } },
    }));
    api.routes.set("GET /backups/gdrive/auth", () => ({
      status: 200,
      body: { url: "#google", redirectUri: "x" },
    }));
    show();
    fireEvent.change(await screen.findByLabelText(/^备份到/), {
      target: { value: "gdrive" },
    });
    fireEvent.change(screen.getByLabelText(/^客户端 ID/), {
      target: { value: " id.apps " },
    });
    fireEvent.change(screen.getByLabelText(/^客户端密钥/), {
      target: { value: "s" },
    });
    fireEvent.click(screen.getByRole("button", { name: "授权" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/backups/gdrive/auth")).toBe(
        true,
      ),
    );
    const put = api.calls.find((c) => c.method === "PUT");
    // 只保存客户端，不改备份位置和开关。
    expect(put?.body).toEqual({
      gdrive: {
        clientId: "id.apps",
        clientSecret: "s",
        folderName: "X Console 备份",
      },
    });
  });

  it("says how the Google authorization went", async () => {
    serve();
    show("/settings/backup?gdrive=error&message=授权链接已失效");
    await screen.findByLabelText(/^备份到/);
    await waitFor(() =>
      expect(toasts).toContainEqual({
        message: "Google Drive 授权失败：授权链接已失效",
        tone: "error",
      }),
    );
  });

  it("shows the authorized account", async () => {
    api.routes.set("GET /backups/settings", () => ({
      status: 200,
      body: {
        ...settings,
        target: "gdrive",
        gdrive: {
          ...settings.gdrive,
          clientId: "id",
          secretSet: true,
          authorized: true,
          account: "me@gmail.com",
        },
      },
    }));
    api.routes.set("GET /backups", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("GET /backups/job", () => ({
      status: 200,
      body: { state: "idle" },
    }));
    show();
    await screen.findByText("已授权：me@gmail.com");
    expect(screen.getByRole("button", { name: "撤销授权" })).toBeTruthy();
    expect(screen.queryByLabelText(/^客户端 ID/)).toBeNull();
  });
});
