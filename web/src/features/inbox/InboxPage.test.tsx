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
import InboxPage from "./InboxPage";
import "./i18n";

const settings = {
  suggest: true,
  reviewReminder: true,
  reviewTime: "21:00",
  transcribeModel: "",
  language: "",
  cardProjectId: 0,
};
const base = {
  id: 1,
  text: "明天下午三点给医生打电话",
  source: "web",
  status: "open",
  createdAt: "2026-11-11T02:00:00Z",
};
const suggested = {
  ...base,
  suggestState: "ready",
  suggestion: {
    kind: "reminder",
    title: "给医生打电话",
    body: "",
    at: "2026-11-12T15:00:00+08:00",
    url: "",
    reason: "有明确的时间",
  },
};

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <InboxPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function routes(items: unknown[]) {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /inbox", () => ({
    status: 200,
    body: { items, openCount: items.length },
  }));
  api.routes.set("GET /inbox/settings", () => ({
    status: 200,
    body: settings,
  }));
  api.routes.set("GET /projects", () => ({
    status: 200,
    body: [{ id: 3, key: "XC", name: "X Console" }],
  }));
}

beforeEach(() => routes([suggested]));
afterEach(cleanup);

describe("收集箱页", () => {
  it("显示条目、AI 的建议和理由", async () => {
    renderPage();
    const card = await screen.findByRole("article");
    expect(within(card).getByText("明天下午三点给医生打电话")).toBeTruthy();
    expect(within(card).getByText(/有明确的时间/)).toBeTruthy();
    expect(
      (within(card).getByDisplayValue("给医生打电话") as HTMLInputElement)
        .value,
    ).toBe("给医生打电话");
    expect(
      (
        within(card).getByRole("combobox", {
          name: "归到",
        }) as HTMLSelectElement
      ).value,
    ).toBe("reminder");
  });

  it("归档：提醒带上标题和 RFC 3339 时间", async () => {
    api.routes.set("POST /inbox/1/file", () => ({
      status: 200,
      body: {
        ...suggested,
        status: "filed",
        filedKind: "reminder",
        filedRef: "9",
      },
    }));
    renderPage();
    const card = await screen.findByRole("article");
    fireEvent.click(within(card).getByRole("button", { name: "归档" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/inbox/1/file")).toBe(true),
    );
    const body = api.calls.find((c) => c.path === "/inbox/1/file")?.body as {
      kind: string;
      title: string;
      at: string;
    };
    expect(body.kind).toBe("reminder");
    expect(body.title).toBe("给医生打电话");
    expect(new Date(body.at).getTime()).toBe(
      new Date("2026-11-12T15:00:00+08:00").getTime(),
    );
  });

  it("提醒没有时间时不提交，提示补上", async () => {
    routes([{ ...suggested, suggestion: { ...suggested.suggestion, at: "" } }]);
    renderPage();
    const card = await screen.findByRole("article");
    fireEvent.click(within(card).getByRole("button", { name: "归档" }));
    expect(await within(card).findByText("提醒要有时间")).toBeTruthy();
    expect(api.calls.some((c) => c.path.endsWith("/file"))).toBe(false);
  });

  it("没有建议时给出说明，类型默认笔记，用第一行做标题", async () => {
    routes([{ ...base, id: 2, text: "买牛奶\n还有鸡蛋", suggestState: "off" }]);
    renderPage();
    const card = await screen.findByRole("article");
    expect(within(card).getByText("没有建议。自己选一个地方。")).toBeTruthy();
    expect(
      (
        within(card).getByRole("combobox", {
          name: "归到",
        }) as HTMLSelectElement
      ).value,
    ).toBe("note");
    expect(
      (within(card).getByDisplayValue("买牛奶") as HTMLInputElement).value,
    ).toBe("买牛奶");
  });

  it("回车保存一条，Shift+回车不保存", async () => {
    api.routes.set("POST /inbox", (body) => ({
      status: 201,
      body: {
        ...base,
        id: 5,
        text: (body as { text: string }).text,
        suggestState: "pending",
      },
    }));
    renderPage();
    const box = await screen.findByRole("textbox", { name: "写一条" });
    fireEvent.change(box, { target: { value: "新想法" } });
    fireEvent.keyDown(box, { key: "Enter", shiftKey: true });
    expect(
      api.calls.some((c) => c.method === "POST" && c.path === "/inbox"),
    ).toBe(false);
    fireEvent.keyDown(box, { key: "Enter" });
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === "POST" && c.path === "/inbox"),
      ).toBe(true),
    );
    expect(
      api.calls.find((c) => c.method === "POST" && c.path === "/inbox")?.body,
    ).toEqual({ text: "新想法" });
  });

  it("忽略和删除", async () => {
    api.routes.set("POST /inbox/1/dismiss", () => ({
      status: 200,
      body: { ...suggested, status: "dismissed" },
    }));
    api.routes.set("DELETE /inbox/1", () => ({ status: 204 }));
    renderPage();
    const card = await screen.findByRole("article");
    fireEvent.click(within(card).getByRole("button", { name: "忽略" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/inbox/1/dismiss")).toBe(true),
    );
    fireEvent.click(within(card).getByRole("button", { name: /^删除/ }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });

  it("空的时候给出说明", async () => {
    routes([]);
    renderPage();
    expect(await screen.findByText("收集箱是空的")).toBeTruthy();
  });

  it("设置：关掉 AI 建议后保存", async () => {
    api.routes.set("PUT /inbox/settings", (body) => ({ status: 200, body }));
    renderPage();
    await screen.findByRole("article");
    fireEvent.click(screen.getByRole("button", { name: "收集箱设置" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("checkbox", { name: /用 AI 建议/ }),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toMatchObject({
      suggest: false,
      reviewTime: "21:00",
    });
  });
});
