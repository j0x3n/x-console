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
    const url = new URL(req.url);
    const path = url.pathname.replace("/api/v1", "");
    const text = await req.text();
    const body = text ? JSON.parse(text) : null;
    calls.push({ method: req.method, path, body });
    const handler = routes.get(`${req.method} ${path}`);
    const reply = handler
      ? handler(body)
      : { status: 404, body: { code: "not_found", message: "not found" } };
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
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import XSettingsTab from "./XSettingsTab";

function renderIt() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <XSettingsTab />
    </QueryClientProvider>,
  );
}

const none = {
  configured: false,
  status: "none",
  fxtwitter: false,
  queryId: "",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("设置 → 稍后阅读（X 推文）", () => {
  it("没配置时显示状态，只填一个 Cookie 值不发请求", async () => {
    api.routes.set("GET /readlater/x-auth", () => ({
      status: 200,
      body: none,
    }));
    renderIt();
    expect(await screen.findByText("还没有 Cookie")).toBeTruthy();
    fireEvent.change(screen.getByLabelText(/auth_token 的值/), {
      target: { value: "tok-ABC123def456" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    // 提示走全局 toast，这里只检查没有发请求
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(api.calls.some((c) => c.method === "PUT")).toBe(false);
  });

  it("保存时带上两个值和备用开关，保存后输入框清空", async () => {
    api.routes.set("GET /readlater/x-auth", () => ({
      status: 200,
      body: none,
    }));
    api.routes.set("PUT /readlater/x-auth", () => ({
      status: 200,
      body: { ...none, configured: true, status: "unverified" },
    }));
    renderIt();
    await screen.findByText("还没有 Cookie");
    fireEvent.change(screen.getByLabelText(/auth_token 的值/), {
      target: { value: "tok-ABC123def456" },
    });
    fireEvent.change(screen.getByLabelText(/ct0 的值/), {
      target: { value: "csrf-XYZ789abc012" },
    });
    fireEvent.click(screen.getByLabelText(/最后一步用第三方转换服务/));
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PUT")?.body).toEqual({
        fxtwitter: true,
        queryId: "",
        authToken: "tok-ABC123def456",
        ct0: "csrf-XYZ789abc012",
      }),
    );
    expect(await screen.findByText("还没检查")).toBeTruthy();
    expect(
      (screen.getByLabelText(/auth_token 的值/) as HTMLInputElement).value,
    ).toBe("");
  });

  it("过期时显示原因，测试和清除调用对应接口", async () => {
    const expired = {
      configured: true,
      status: "expired",
      message: "X 拒绝了登录 Cookie，可能过期了，请重新填写",
      fxtwitter: false,
      queryId: "",
    };
    api.routes.set("GET /readlater/x-auth", () => ({
      status: 200,
      body: expired,
    }));
    api.routes.set("POST /readlater/x-auth/test", () => ({
      status: 200,
      body: { ...expired, status: "ok", message: undefined },
    }));
    api.routes.set("DELETE /readlater/x-auth", () => ({
      status: 200,
      body: none,
    }));
    renderIt();
    expect(await screen.findByText("可能过期了")).toBeTruthy();
    expect(screen.getByText(/请重新填写/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "测试 Cookie" }));
    expect(await screen.findByText("可用")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "清除 Cookie" }));
    expect(await screen.findByText("还没有 Cookie")).toBeTruthy();
    expect(api.calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "GET /readlater/x-auth",
      "POST /readlater/x-auth/test",
      "DELETE /readlater/x-auth",
    ]);
  });
});
