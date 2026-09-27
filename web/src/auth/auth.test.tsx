// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";

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

import AuthGate from "./AuthGate";
import ElevationDialog from "./ElevationDialog";
import { useElevationStore } from "./elevation";
import "../features/settings/i18n";
import SecurityTab from "../features/settings/SecurityTab";
import { passwordFormError } from "../features/settings/security";

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const fail = (status: number, code: string, message = code) => ({
  status,
  body: { code, message },
});

function setStatus(status: Record<string, unknown>) {
  api.routes.set("GET /auth/status", () => ({ status: 200, body: status }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  useElevationStore.setState({ open: false });
});

describe("login", () => {
  it("asks for the code only after the server says totp_required", async () => {
    let loggedIn = false;
    api.routes.set("GET /auth/status", () => ({
      status: 200,
      body: loggedIn
        ? { setupRequired: false, authenticated: true, username: "jo" }
        : { setupRequired: false, authenticated: false },
    }));
    api.routes.set("POST /auth/login", (body) => {
      const { code } = body as { code?: string };
      if (!code) return fail(401, "totp_required");
      loggedIn = true;
      return { status: 204 };
    });
    wrap(<AuthGate>面板内容</AuthGate>);

    fireEvent.change(await screen.findByLabelText("用户名"), {
      target: { value: "jo" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "0123456789" },
    });
    expect(screen.queryByLabelText(/^两步验证码/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    const code = await screen.findByLabelText(/^两步验证码/);
    expect(api.calls.find((c) => c.path === "/auth/login")?.body).toEqual({
      username: "jo",
      password: "0123456789",
    });
    fireEvent.change(code, { target: { value: "123456" } });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByText("面板内容")).toBeTruthy();
    expect(
      api.calls.filter((c) => c.path === "/auth/login").at(-1)?.body,
    ).toEqual({ username: "jo", password: "0123456789", code: "123456" });
  });

  it("logs in with only a password when two-step is off", async () => {
    let loggedIn = false;
    api.routes.set("GET /auth/status", () => ({
      status: 200,
      body: { setupRequired: false, authenticated: loggedIn },
    }));
    api.routes.set("POST /auth/login", () => {
      loggedIn = true;
      return { status: 204 };
    });
    wrap(<AuthGate>面板内容</AuthGate>);
    fireEvent.change(await screen.findByLabelText("用户名"), {
      target: { value: "jo" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "0123456789" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByText("面板内容")).toBeTruthy();
  });

  it("shows the server message for a wrong password", async () => {
    setStatus({ setupRequired: false, authenticated: false });
    api.routes.set("POST /auth/login", () =>
      fail(401, "invalid_credentials", "用户名、密码或验证码不正确"),
    );
    wrap(<AuthGate>面板内容</AuthGate>);
    fireEvent.change(await screen.findByLabelText("用户名"), {
      target: { value: "jo" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "wrong-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByText("用户名、密码或验证码不正确")).toBeTruthy();
    expect(screen.queryByLabelText(/^两步验证码/)).toBeNull();
  });
});

describe("setup", () => {
  async function createAccount() {
    wrap(<AuthGate>面板内容</AuthGate>);
    fireEvent.change(await screen.findByLabelText("用户名"), {
      target: { value: "jo" },
    });
    fireEvent.change(screen.getByLabelText(/^密码/), {
      target: { value: "0123456789" },
    });
    fireEvent.change(screen.getByLabelText("再输一次密码"), {
      target: { value: "0123456789" },
    });
    fireEvent.click(screen.getByRole("button", { name: "下一步" }));
    return screen.findByRole("button", { name: "跳过，以后再开" });
  }

  it("can skip two-step verification", async () => {
    let done = false;
    api.routes.set("GET /auth/status", () => ({
      status: 200,
      body: { setupRequired: !done, authenticated: done },
    }));
    api.routes.set("POST /auth/setup", () => ({
      status: 200,
      body: { secret: "ABC", otpauthUrl: "otpauth://totp/x?secret=ABC" },
    }));
    api.routes.set("POST /auth/setup/skip-totp", () => {
      done = true;
      return { status: 204 };
    });
    fireEvent.click(await createAccount());
    expect(await screen.findByText("面板内容")).toBeTruthy();
  });

  it("says so when skipping is not live yet", async () => {
    setStatus({ setupRequired: true, authenticated: false });
    api.routes.set("POST /auth/setup", () => ({
      status: 200,
      body: { secret: "ABC", otpauthUrl: "otpauth://totp/x?secret=ABC" },
    }));
    api.routes.set("POST /auth/setup/skip-totp", () =>
      fail(501, "not_ready", "功能还没上线"),
    );
    fireEvent.click(await createAccount());
    expect(await screen.findByText("功能还没上线")).toBeTruthy();
    expect(screen.getByText("ABC")).toBeTruthy();
  });
});

describe("elevation dialog", () => {
  it("asks for the password when two-step is off", async () => {
    setStatus({
      setupRequired: false,
      authenticated: true,
      totpEnabled: false,
    });
    api.routes.set("POST /auth/elevate", () => ({
      status: 200,
      body: { elevatedUntil: "2026-09-27T12:00:00Z" },
    }));
    wrap(<ElevationDialog />);
    const done = useElevationStore.getState().request();
    const input = await screen.findByLabelText("密码");
    expect(screen.getByText("输入登录密码。")).toBeTruthy();
    fireEvent.change(input, { target: { value: "0123456789" } });
    fireEvent.click(screen.getByRole("button", { name: "验证" }));
    await done;
    expect(api.calls.find((c) => c.path === "/auth/elevate")?.body).toEqual({
      password: "0123456789",
    });
  });

  it("asks for the code when the server does not say (older server)", async () => {
    setStatus({ setupRequired: false, authenticated: true });
    wrap(<ElevationDialog />);
    void useElevationStore
      .getState()
      .request()
      .catch(() => {});
    expect(await screen.findByLabelText("验证码")).toBeTruthy();
  });
});

describe("security tab", () => {
  it("turns two-step on: enroll, scan, confirm", async () => {
    setStatus({
      setupRequired: false,
      authenticated: true,
      totpEnabled: false,
    });
    api.routes.set("POST /auth/totp/enroll", () => ({
      status: 200,
      body: { secret: "SECRET1", otpauthUrl: "otpauth://totp/x?secret=S" },
    }));
    api.routes.set("POST /auth/totp/confirm", () => ({ status: 204 }));
    wrap(<SecurityTab />);
    expect(await screen.findByText("未开启")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "开启两步验证" }));
    expect(await screen.findByText("SECRET1")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("验证码"), {
      target: { value: "654321" },
    });
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === "/auth/totp/confirm")?.body,
      ).toEqual({ code: "654321" }),
    );
  });

  it("turning off needs the password and the code", async () => {
    setStatus({ setupRequired: false, authenticated: true, totpEnabled: true });
    api.routes.set("POST /auth/totp/disable", () =>
      fail(401, "invalid_credentials", "密码或验证码不正确"),
    );
    wrap(<SecurityTab />);
    expect(await screen.findByText("已开启")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "关闭两步验证" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(dialog.querySelector('input[type="password"]')!, {
      target: { value: "0123456789" },
    });
    fireEvent.change(dialog.querySelector("input.xc-mono")!, {
      target: { value: "111111" },
    });
    fireEvent.submit(dialog.querySelector("form")!);
    expect(await screen.findByText("密码或验证码不正确")).toBeTruthy();
    expect(
      api.calls.find((c) => c.path === "/auth/totp/disable")?.body,
    ).toEqual({ password: "0123456789", code: "111111" });
  });

  it("checks the new password before sending it", async () => {
    setStatus({ setupRequired: false, authenticated: true, totpEnabled: true });
    wrap(<SecurityTab />);
    fireEvent.change(await screen.findByLabelText("当前密码"), {
      target: { value: "0123456789" },
    });
    fireEvent.change(screen.getByLabelText(/^新密码/), {
      target: { value: "abcdefghijk" },
    });
    fireEvent.change(screen.getByLabelText("再输一次新密码"), {
      target: { value: "abcdefghijX" },
    });
    fireEvent.click(screen.getByRole("button", { name: "修改密码" }));
    expect(await screen.findByText("两次输入的新密码不一致")).toBeTruthy();
    expect(api.calls.some((c) => c.path === "/auth/password")).toBe(false);
  });
});

describe("passwordFormError", () => {
  it("checks length, match and change", () => {
    const ok = {
      oldPassword: "old-password",
      newPassword: "new-password",
      confirm: "new-password",
    };
    expect(passwordFormError(ok)).toBe("");
    expect(
      passwordFormError({ ...ok, newPassword: "short", confirm: "short" }),
    ).toMatch("至少 10 位");
    expect(passwordFormError({ ...ok, confirm: "other" })).toMatch("不一致");
    expect(passwordFormError({ ...ok, oldPassword: "new-password" })).toMatch(
      "一样",
    );
  });
});
