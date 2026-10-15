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

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import HeldNotices from "./HeldNotices";
import "./i18n";

function setPhone(phone: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: phone && query.includes("max-width"),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia;
}

function renderIt() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route path="/" element={<HeldNotices />} />
        <Route path="/mail" element={<p>邮件页</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

const held = {
  items: [
    {
      id: 7,
      kind: "mail.new",
      title: "不在时的信",
      body: "张三：周五开会",
      link: "/mail",
      createdAt: new Date(Date.now() - 600_000).toISOString(),
    },
  ],
};

describe("HeldNotices (B164)", () => {
  it("claims what waited and lists it on the phone", async () => {
    setPhone(true);
    api.routes.set("POST /notify/digest/claim", () => ({
      status: 200,
      body: held,
    }));
    renderIt();
    expect(await screen.findByText("你不在电脑前时的通知")).toBeTruthy();
    expect(screen.getByText("张三：周五开会")).toBeTruthy();
    fireEvent.click(screen.getByText("不在时的信"));
    expect(await screen.findByText("邮件页")).toBeTruthy();
  });

  it("does nothing on a computer", async () => {
    setPhone(false);
    api.routes.set("POST /notify/digest/claim", () => ({
      status: 200,
      body: held,
    }));
    renderIt();
    await new Promise((r) => setTimeout(r, 50));
    expect(api.calls).toHaveLength(0);
    await waitFor(() =>
      expect(screen.queryByText("你不在电脑前时的通知")).toBeNull(),
    );
  });
});
