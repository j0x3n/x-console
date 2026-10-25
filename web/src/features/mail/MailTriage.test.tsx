// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 组件之前替换。
const api = vi.hoisted(() => {
  type Reply = { status: number; body?: unknown };
  const routes = new Map<string, (body: unknown) => Reply | Promise<Reply>>();
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
      ? await handler(body)
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
import MailPage from "./MailPage";
import MailSettingsTab from "./MailSettingsTab";
import "./i18n";

const account = (triage: boolean) => ({
  id: 1,
  name: "个人",
  email: "me@example.com",
  provider: "other",
  imapHost: "h",
  imapPort: 993,
  username: "me",
  notify: true,
  status: "ok",
  lastError: "",
  unread: 2,
  triage,
  createdAt: "2026-11-01T00:00:00Z",
});
const soon = new Date(Date.now() + 5 * 86400_000);
const pad = (n: number) => String(n).padStart(2, "0");
const soonDate = `${soon.getFullYear()}-${pad(soon.getMonth() + 1)}-${pad(soon.getDate())}`;
const row = (id: number, subject: string, triage?: unknown) => ({
  id,
  accountId: 1,
  from: { address: "a@example.com", name: "甲" },
  subject,
  snippet: `${subject}的开头`,
  date: "2026-11-10T08:00:00Z",
  unread: false,
  flagged: false,
  hasAttachments: false,
  ...(triage ? { triage } : {}),
});
const action = {
  category: "action",
  summary: "请在截止前签字",
  dates: [{ date: soonDate, text: "签字截止" }],
};

function setup(triage = true) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /mail/accounts", () => ({
    status: 200,
    body: [account(triage)],
  }));
  api.routes.set("GET /mail/messages", () => ({
    status: 200,
    body: {
      items: [
        row(1, "合同请签字", action),
        row(2, "促销来了", {
          category: "promo",
          summary: "商家促销",
          dates: [],
        }),
        row(4, "登录验证码", {
          category: "notice",
          code: true,
          summary: "登录验证码",
          dates: [],
        }),
        row(3, "没分拣的信"),
      ],
    },
  }));
  api.routes.set("GET /mail/messages/1", () => ({
    status: 200,
    body: {
      ...row(1, "合同请签字", action),
      to: [],
      cc: [],
      text: "正文",
      html: null,
      attachments: [],
      unread: false,
    },
  }));
}

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <MailPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => setup());
afterEach(cleanup);

describe("邮件分拣（B136）", () => {
  it("列表里要处理的邮件有标记，其他类别有小字，没分拣的照旧", async () => {
    renderAt("/mail");
    const list = await screen.findByRole("region", { name: "收件箱" });
    const rows = await within(list).findAllByRole("button", { name: /甲/ });
    expect(within(rows[0]).getByText("要处理")).toBeTruthy();
    expect(within(rows[0]).getByText("请在截止前签字")).toBeTruthy();
    expect(within(rows[1]).getByText("营销")).toBeTruthy();
    expect(within(rows[2]).getByText("验证码")).toBeTruthy();
    expect(within(rows[3]).getByText("没分拣的信的开头")).toBeTruthy();
  });

  it("有邮箱开了分拣时出现类别切换，点“要处理”带上 category", async () => {
    renderAt("/mail");
    await screen.findAllByRole("button", { name: /甲/ });
    fireEvent.click(screen.getByRole("button", { name: "要处理" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.includes("category=action"))).toBe(
        true,
      ),
    );
  });

  it("没有邮箱开分拣、也没选类别时不显示切换", async () => {
    setup(false);
    renderAt("/mail");
    await screen.findAllByRole("button", { name: /甲/ });
    expect(screen.queryByRole("button", { name: "营销" })).toBeNull();
  });

  it("阅读区：显示分拣结果，日期一键加提醒", async () => {
    api.routes.set("POST /reminders", (body) => ({
      status: 201,
      body: { id: 5, ...(body as object) },
    }));
    renderAt("/mail?m=1");
    const panel = await screen.findByRole("region", { name: "AI 分拣" });
    expect(within(panel).getByText("请在截止前签字")).toBeTruthy();
    const title = screen.getByRole("heading", {
      name: "合同请签字",
    }).parentElement!;
    expect(within(title).getByText("要处理")).toBeTruthy();
    expect(within(panel).queryByText("要处理")).toBeNull();
    expect(
      within(panel).queryByRole("button", { name: "重新分拣" }),
    ).toBeNull();
    fireEvent.click(
      within(panel).getByRole("button", {
        name: new RegExp(`提醒我 ${soonDate}`),
      }),
    );
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === "POST" && c.path === "/reminders"),
      ).toBe(true),
    );
    const body = api.calls.find(
      (c) => c.method === "POST" && c.path === "/reminders",
    )?.body as { title: string; at: string; link: string };
    expect(body.title).toBe("签字截止：合同请签字");
    expect(body.link).toBe("/mail?m=1");
    expect(new Date(body.at).getHours()).toBe(9);
  });

  it("阅读区：没分拣过的邮件用标题旁的图标请求分拣", async () => {
    api.routes.set("GET /mail/messages/1", () => ({
      status: 200,
      body: {
        ...row(1, "普通信"),
        to: [],
        cc: [],
        text: "正文",
        html: null,
        attachments: [],
        unread: false,
      },
    }));
    api.routes.set("POST /mail/messages/1/triage", () => ({
      status: 200,
      body: { category: "notice", summary: "账单", dates: [] },
    }));
    renderAt("/mail?m=1");
    const button = await screen.findByRole("button", { name: "重新分拣" });
    expect(button.title).toBe("重新分拣");
    expect(button.textContent).toBe("");
    expect(screen.queryByRole("region", { name: "AI 分拣" })).toBeNull();
    fireEvent.click(button);
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/mail/messages/1/triage")).toBe(
        true,
      ),
    );
  });

  it("阅读区：重新分拣等待期间禁用图标按钮", async () => {
    let finish!: (reply: { status: number; body: unknown }) => void;
    const reply = new Promise<{ status: number; body: unknown }>((resolve) => {
      finish = resolve;
    });
    api.routes.set("POST /mail/messages/1/triage", () => reply);
    renderAt("/mail?m=1");
    const button = (await screen.findByRole("button", {
      name: "重新分拣",
    })) as HTMLButtonElement;
    fireEvent.click(button);
    await waitFor(() => expect(button.disabled).toBe(true));
    expect(button.title).toBe("重新分拣");
    finish({ status: 200, body: action });
    await waitFor(() => expect(button.disabled).toBe(false));
    expect(
      api.calls.filter((c) => c.path === "/mail/messages/1/triage"),
    ).toHaveLength(1);
  });

  it("阅读区：验证码类型放在主题旁，摘要继续显示", async () => {
    api.routes.set("GET /mail/messages/1", () => ({
      status: 200,
      body: {
        ...row(1, "登录验证码", {
          category: "notice",
          code: true,
          summary: "验证码为 123456",
          dates: [],
        }),
        to: [],
        cc: [],
        text: "验证码为 123456",
        html: null,
        attachments: [],
      },
    }));
    renderAt("/mail?m=1");
    const panel = await screen.findByRole("region", { name: "AI 分拣" });
    const title = screen.getByRole("heading", {
      name: "登录验证码",
    }).parentElement!;
    expect(within(title).getByText("验证码")).toBeTruthy();
    expect(within(panel).getByText("验证码为 123456")).toBeTruthy();
    expect(within(panel).queryByText("验证码")).toBeNull();
  });
});

describe("邮件外部图片", () => {
  const withHtml = (triage?: object) => () => ({
    status: 200,
    body: {
      ...row(1, "带图的信", triage),
      to: [],
      cc: [],
      text: "",
      html: '<p>hi</p><img src="https://a/logo.png"><img src="https://t/o.gif" width="1" height="1">',
      attachments: [],
      unread: false,
    },
  });
  const frameDoc = async () =>
    (await screen.findByTitle("带图的信")).getAttribute("srcdoc") ?? "";

  it("不是营销邮件时默认显示，1 像素图片去掉", async () => {
    api.routes.set(
      "GET /mail/messages/1",
      withHtml({ category: "notice", summary: "通知", dates: [] }),
    );
    renderAt("/mail?m=1");
    const doc = await frameDoc();
    expect(doc).toContain("https: http:");
    expect(doc).toContain("https://a/logo.png");
    expect(doc).not.toContain("https://t/o.gif");
    expect(screen.queryByRole("button", { name: "显示图片" })).toBeNull();
  });

  it("营销邮件先不加载，点“显示图片”后放开", async () => {
    api.routes.set(
      "GET /mail/messages/1",
      withHtml({ category: "promo", summary: "促销", dates: [] }),
    );
    renderAt("/mail?m=1");
    expect(await frameDoc()).not.toContain("https: http:");
    fireEvent.click(await screen.findByRole("button", { name: "显示图片" }));
    await waitFor(async () =>
      expect(await frameDoc()).toContain("https: http:"),
    );
  });
});

describe("右键菜单（XC-66）", () => {
  it("右键一封信可以单独标已读、归档、删除", async () => {
    api.routes.set("POST /mail/messages/batch", (body) => ({
      status: 200,
      body: { done: (body as { ids: number[] }).ids.length, errors: [] },
    }));
    renderAt("/mail");
    const rows = await screen.findAllByRole("button", { name: /甲/ });
    const batches = () =>
      api.calls.filter((c) => c.path === "/mail/messages/batch");

    fireEvent.contextMenu(rows[1]);
    let menu = await screen.findByRole("menu");
    expect(
      within(menu).getByRole("menuitem", { name: "标为未读" }),
    ).toBeTruthy();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "归档" }));
    await waitFor(() =>
      expect(batches().at(-1)?.body).toEqual({ ids: [2], action: "archive" }),
    );
    expect(screen.queryByRole("menu")).toBeNull();

    fireEvent.contextMenu(rows[2]);
    menu = await screen.findByRole("menu");
    fireEvent.click(within(menu).getByRole("menuitem", { name: "删除" }));
    await waitFor(() =>
      expect(batches().at(-1)?.body).toEqual({ ids: [4], action: "delete" }),
    );
  });
});

describe("邮件分拣与设置入口（B256）", () => {
  it("邮件页右上角有分拣设置入口链接", async () => {
    setup();
    renderAt("/mail");
    const link = await screen.findByRole("link", { name: "分拣设置" });
    expect(link.getAttribute("href")).toBe(
      "/settings/mail?section=mail-sorting",
    );
  });

  it("邮箱设置页显示分拣设置并能修改保存", async () => {
    setup();
    let saved: unknown = null;
    api.routes.set("GET /mail/sorting", () => ({
      status: 200,
      body: { markPromoRead: false, importantOnly: false },
    }));
    api.routes.set("PUT /mail/sorting", (body) => {
      saved = body;
      return { status: 200, body };
    });
    api.routes.set("GET /reminders/mutes", () => ({
      status: 200,
      body: [],
    }));
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={["/settings/mail"]}>
          <MailSettingsTab />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const promoSwitch = await screen.findByLabelText("营销邮件自动已读");
    const importantSwitch = await screen.findByLabelText("只推送重要通知");
    expect((promoSwitch as HTMLInputElement).checked).toBe(false);
    expect((importantSwitch as HTMLInputElement).checked).toBe(false);

    fireEvent.click(promoSwitch);
    await waitFor(() => {
      expect(saved).toEqual({ markPromoRead: true, importantOnly: false });
    });

    fireEvent.click(importantSwitch);
    await waitFor(() => {
      expect(saved).toEqual({ markPromoRead: true, importantOnly: true });
    });
  });
});
