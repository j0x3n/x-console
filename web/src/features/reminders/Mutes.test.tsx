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
vi.mock("../../auth/elevation", () => ({
  withElevation: <T,>(fn: () => Promise<T>) => fn(),
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
import AccountDialog from "../mail/components/AccountDialog";
import MutesCard from "./MutesCard";
import "../mail/i18n";
import "./i18n";

function renderIt(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const IPHONE =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1";
const WINDOWS =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36";

const subs = [
  {
    id: 1,
    endpoint: "https://push.example/1",
    service: "apple",
    userAgent: IPHONE,
    createdAt: "2026-10-01T00:00:00Z",
    lastOkAt: new Date().toISOString(),
  },
  {
    id: 2,
    endpoint: "https://push.example/2",
    service: "google",
    userAgent: WINDOWS,
    createdAt: "2026-10-01T00:00:00Z",
  },
];
const channels = [
  { name: "webpush", configured: true, fields: [], subscriptions: 2 },
  { name: "telegram", configured: false, fields: [] },
  { name: "bark", configured: true, fields: [] },
  { name: "serverchan", configured: false, fields: [] },
];
const gmail = {
  id: 7,
  name: "个人 Gmail",
  email: "me@gmail.com",
  provider: "gmail",
  imapHost: "imap.gmail.com",
  imapPort: 993,
  username: "me@gmail.com",
  notify: true,
  status: "ok",
  unread: 0,
};

function commonRoutes() {
  api.routes.set("GET /notify/channels", () => ({
    status: 200,
    body: channels,
  }));
  api.routes.set("GET /notify/webpush/subscriptions", () => ({
    status: 200,
    body: subs,
  }));
  api.routes.set("GET /mail/accounts", () => ({ status: 200, body: [gmail] }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("MutesCard B113", () => {
  it("列出规则，邮箱和设备显示名字，已删除的邮箱有标注", async () => {
    commonRoutes();
    api.routes.set("GET /notify/mutes", () => ({
      status: 200,
      body: {
        items: [
          {
            id: 1,
            kindPattern: "mail.new",
            scope: "mail:7",
            target: "webpush:1",
            createdAt: "2026-10-06T00:00:00Z",
          },
          {
            id: 2,
            kindPattern: "mail.new",
            scope: "mail:99",
            target: "bark",
            createdAt: "2026-10-06T00:00:00Z",
          },
          {
            id: 3,
            kindPattern: "github.*",
            scope: "",
            target: "webpush:2",
            createdAt: "2026-10-06T00:00:00Z",
          },
        ],
      },
    }));
    api.routes.set("DELETE /notify/mutes/3", () => ({ status: 204 }));
    renderIt(<MutesCard />);
    expect(await screen.findByText("个人 Gmail → Safari · iOS")).toBeTruthy();
    expect(screen.getByText("已删除的邮箱 → Bark")).toBeTruthy();
    expect(screen.getByText("全部 → Chrome · Windows")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "删除 仓库" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "DELETE" && c.path === "/notify/mutes/3",
        ),
      ).toBe(true),
    );
  });

  it("添加规则：邮件类型可以选邮箱，保存带上范围和目标", async () => {
    commonRoutes();
    api.routes.set("GET /notify/mutes", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /notify/mutes", (body) => ({
      status: 201,
      body: {
        id: 9,
        createdAt: "2026-10-06T00:00:00Z",
        scope: "",
        ...(body as object),
      },
    }));
    renderIt(<MutesCard />);
    expect(await screen.findByText("还没有静音规则")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "添加静音规则" }));
    const dialog = screen.getByRole("dialog");
    const save = within(dialog).getByRole("button", { name: "保存" });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("邮箱"), {
      target: { value: "mail:7" },
    });
    await within(dialog).findByRole("option", { name: "Safari · iOS" });
    fireEvent.change(within(dialog).getByLabelText("不发送到"), {
      target: { value: "webpush:1" },
    });
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "POST")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
      kindPattern: "mail.*",
      scope: "mail:7",
      target: "webpush:1",
    });
  });

  it("自定义类型写错时不能保存", async () => {
    commonRoutes();
    api.routes.set("GET /notify/mutes", () => ({
      status: 200,
      body: { items: [] },
    }));
    renderIt(<MutesCard />);
    fireEvent.click(
      await screen.findByRole("button", { name: "添加静音规则" }),
    );
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("类型"), {
      target: { value: "__custom__" },
    });
    await within(dialog).findByRole("option", { name: "Bark" });
    fireEvent.change(within(dialog).getByLabelText("不发送到"), {
      target: { value: "bark" },
    });
    const save = within(dialog).getByRole("button", { name: "保存" });
    fireEvent.change(within(dialog).getByLabelText("自定义类型"), {
      target: { value: "Bad Kind" },
    });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(dialog).getByLabelText("自定义类型"), {
      target: { value: "host.alert*" },
    });
    expect((save as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("邮箱弹窗的“通知发到” B113", () => {
  it("取消手机那一项，保存时先改邮箱，再保存静音目标", async () => {
    commonRoutes();
    api.routes.set("GET /notify/mutes", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("PATCH /mail/accounts/7", () => ({
      status: 200,
      body: gmail,
    }));
    api.routes.set("PUT /notify/mutes/scope", (body) => ({
      status: 200,
      body: {
        items: (body as { targets: string[] }).targets.map((t, i) => ({
          id: i + 1,
          kindPattern: "mail.new",
          scope: "mail:7",
          target: t,
          createdAt: "2026-10-06T00:00:00Z",
        })),
      },
    }));
    renderIt(
      <AccountDialog open onClose={() => {}} account={gmail as never} />,
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText("手机上装了 Gmail 的话，取消手机那一项就行。"),
    ).toBeTruthy();
    const phone = (await within(dialog).findByLabelText(
      /Safari · iOS/,
    )) as HTMLInputElement;
    const bell = within(dialog).getByLabelText(
      /面板通知铃/,
    ) as HTMLInputElement;
    expect(bell.checked).toBe(true);
    expect(bell.disabled).toBe(true);
    expect(phone.checked).toBe(true);
    fireEvent.click(phone);
    expect(phone.checked).toBe(false);
    fireEvent.click(within(dialog).getByRole("button", { name: "连接" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
      kindPattern: "mail.new",
      scope: "mail:7",
      targets: ["webpush:1"],
    });
    const order = api.calls.map((c) => `${c.method} ${c.path}`);
    expect(order.indexOf("PATCH /mail/accounts/7")).toBeLessThan(
      order.indexOf("PUT /notify/mutes/scope"),
    );
  });

  it("已有的静音规则显示成没勾，没改时不请求保存", async () => {
    commonRoutes();
    api.routes.set("GET /notify/mutes?scope=mail%3A7", () => ({
      status: 200,
      body: {
        items: [
          {
            id: 1,
            kindPattern: "mail.new",
            scope: "mail:7",
            target: "webpush:1",
            createdAt: "2026-10-06T00:00:00Z",
          },
        ],
      },
    }));
    api.routes.set("GET /notify/mutes", () => ({
      status: 200,
      body: {
        items: [
          {
            id: 1,
            kindPattern: "mail.new",
            scope: "mail:7",
            target: "webpush:1",
            createdAt: "2026-10-06T00:00:00Z",
          },
        ],
      },
    }));
    api.routes.set("PATCH /mail/accounts/7", () => ({
      status: 200,
      body: gmail,
    }));
    renderIt(
      <AccountDialog open onClose={() => {}} account={gmail as never} />,
    );
    const dialog = await screen.findByRole("dialog");
    const phone = (await within(dialog).findByLabelText(
      /Safari · iOS/,
    )) as HTMLInputElement;
    await waitFor(() => expect(phone.checked).toBe(false));
    const desktop = within(dialog).getByLabelText(
      /Chrome · Windows/,
    ) as HTMLInputElement;
    expect(desktop.checked).toBe(true);
    fireEvent.click(within(dialog).getByRole("button", { name: "连接" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    await new Promise((r) => setTimeout(r, 50));
    expect(api.calls.some((c) => c.method === "PUT")).toBe(false);
  });

  it("推送开关关着时，不列目标", async () => {
    commonRoutes();
    api.routes.set("GET /notify/mutes", () => ({
      status: 200,
      body: { items: [] },
    }));
    renderIt(
      <AccountDialog
        open
        onClose={() => {}}
        account={{ ...gmail, notify: false } as never}
      />,
    );
    expect(
      await screen.findByText("这个邮箱的外部通知已整体关闭。"),
    ).toBeTruthy();
    expect(screen.queryByLabelText(/Safari · iOS/)).toBeNull();
  });
});
