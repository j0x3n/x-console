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
vi.mock("../../auth/elevation", () => ({
  withElevation: <T,>(fn: () => Promise<T>) => fn(),
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
import MeSettingsTab from "./MeSettingsTab";
import type { MeSettings } from "./api";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

const wrap = (ui: React.ReactElement) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
};

const REPORT_URL = "https://x.test/api/v1/me/report";

function serve(
  opts: { haConnected?: boolean; settings?: Partial<MeSettings> } = {},
) {
  let settings: MeSettings = {
    workHosts: [],
    hosts: [
      { id: "pc1", name: "办公室电脑", online: true, state: "active" },
      { id: "pc2", name: "家里电脑", online: false, state: "offline" },
    ],
    guessSleep: true,
    haHome: "",
    haSleep: "",
    ...opts.settings,
  };
  let enabled = false;
  api.routes.set("GET /me/settings", () => ({ status: 200, body: settings }));
  api.routes.set("PUT /me/settings", (body) => {
    const b = body as Partial<MeSettings>;
    settings = { ...settings, ...b } as MeSettings;
    return { status: 200, body: settings };
  });
  api.routes.set("GET /me/report", () => ({
    status: 200,
    body: { enabled, reportUrl: REPORT_URL },
  }));
  api.routes.set("POST /me/report/token", () => {
    enabled = true;
    return {
      status: 200,
      body: {
        token: "k".repeat(64),
        reportUrl: REPORT_URL,
        example: "curl -H 'Authorization: Bearer kkkk'",
      },
    };
  });
  api.routes.set("DELETE /me/report", () => {
    enabled = false;
    return { status: 204 };
  });
  api.routes.set("GET /ha/status", () =>
    opts.haConnected === false
      ? {
          status: 412,
          body: { code: "integration_not_configured", message: "x" },
        }
      : {
          status: 200,
          body: {
            configured: true,
            connected: true,
            mode: "direct",
            entityCount: 3,
          },
        },
  );
}

const puts = () => api.calls.filter((c) => c.method === "PUT");

describe("MeSettingsTab", () => {
  it("列出能报空闲时间的电脑，勾选后保存", async () => {
    serve();
    wrap(<MeSettingsTab />);
    await screen.findByText("办公室电脑");
    expect(screen.getByText("正在使用")).toBeTruthy();
    expect(screen.getByText("离线")).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "办公室电脑" }));
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0].path).toBe("/me/settings");
    expect(puts()[0].body).toEqual({
      workHosts: ["pc1"],
      guessSleep: true,
      haHome: "",
      haSleep: "",
    });
    await waitFor(() =>
      expect(
        (
          screen.getByRole("checkbox", {
            name: "办公室电脑",
          }) as HTMLInputElement
        ).checked,
      ).toBe(true),
    );
  });

  it("按作息推测睡觉可以关掉，旁边有作息链接", async () => {
    serve();
    wrap(<MeSettingsTab />);
    const sw = await screen.findByRole("checkbox", { name: "推测我在睡觉" });
    expect((sw as HTMLInputElement).checked).toBe(true);
    fireEvent.click(sw);
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect((puts()[0].body as MeSettings).guessSleep).toBe(false);
    const link = screen.getByRole("link", { name: "改作息" });
    expect(link.getAttribute("href")).toBe("/habits?schedule=1");
  });

  it("写清楚推荐的分工和快捷指令步骤", async () => {
    serve();
    wrap(<MeSettingsTab />);
    await screen.findByText("办公室电脑");
    expect(screen.getByText(/手机 App 在后台定位更稳/)).toBeTruthy();
    expect(screen.getByText(/分不出是不是睡眠模式/)).toBeTruthy();
    expect(screen.getByText(/以最后收到的为准/)).toBeTruthy();
    const steps = screen
      .getByText("iPhone 快捷指令的配置步骤")
      .closest("section")!;
    expect(within(steps).getAllByRole("listitem")).toHaveLength(5);
    expect(within(steps).getByText(/点“自动化”，点右上角的“\+”/)).toBeTruthy();
    expect(
      within(steps).getByText(/sleep\.end.*home\.arrive.*home\.leave/),
    ).toBeTruthy();
  });

  it("生成令牌后只显示一次，关闭后回到未开启", async () => {
    serve();
    wrap(<MeSettingsTab />);
    await screen.findByText("还没有开启");
    expect(screen.getByText(REPORT_URL)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "生成上报令牌" }));
    await screen.findByText("k".repeat(64));
    expect(
      screen.getByText(/curl -H 'Authorization: Bearer kkkk'/),
    ).toBeTruthy();
    await screen.findByText("还没有收到过上报");
    expect(screen.getByRole("button", { name: "重新生成令牌" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    await waitFor(() => expect(screen.queryByText("k".repeat(64))).toBeNull());
    await screen.findByText("还没有开启");
    expect(api.calls.map((c) => `${c.method} ${c.path}`)).toContain(
      "DELETE /me/report",
    );
  });

  it("Home Assistant 没连上时只提示先去连接", async () => {
    serve({ haConnected: false });
    wrap(<MeSettingsTab />);
    await screen.findByText("先在设置里连接 Home Assistant。");
    expect(screen.queryByPlaceholderText("person.me")).toBeNull();
  });

  it("填 Home Assistant 的两个实体，写错时不能保存，睡眠实体旁边有提醒", async () => {
    serve();
    wrap(<MeSettingsTab />);
    const home = await screen.findByPlaceholderText("person.me");
    const sleep = screen.getByPlaceholderText("binary_sensor.iphone_focus");
    expect(screen.getByText(/白天也会开别的专注模式的话，不要填/)).toBeTruthy();
    const save = () => {
      const card = home.closest("section")!;
      return within(card).getByRole("button", {
        name: "保存",
      }) as HTMLButtonElement;
    };
    expect(save().disabled).toBe(true);
    fireEvent.change(home, { target: { value: "light.desk" } });
    expect(save().disabled).toBe(true);
    fireEvent.change(home, { target: { value: "person.me" } });
    fireEvent.change(sleep, { target: { value: "binary_sensor.me_focus" } });
    expect(save().disabled).toBe(false);
    fireEvent.click(save());
    await waitFor(() => expect(puts()).toHaveLength(1));
    expect(puts()[0].body).toEqual({
      workHosts: [],
      guessSleep: true,
      haHome: "person.me",
      haSleep: "binary_sensor.me_focus",
    });
  });

  it("接口还没上线时显示还没上线", async () => {
    wrap(<MeSettingsTab />);
    await screen.findByText(/还没上线/);
  });
});
