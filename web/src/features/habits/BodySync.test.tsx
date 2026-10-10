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
import BodySync from "./BodySync";
import BodyTrend from "./BodyTrend";
import type { PersonalDay } from "./personalApi";
import "./personalI18n";

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

const wrap = (ui: React.ReactElement) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
};

const day = (date: string, extra: Partial<PersonalDay>): PersonalDay => ({
  date,
  weight: "",
  waist: "",
  sleep: "",
  steps: "",
  restingHr: "",
  energy: "",
  back: "",
  note: "",
  english: "",
  food: "",
  sets: {},
  checks: {},
  ...extra,
});

describe("BodyTrend", () => {
  const days = [
    day("2026-10-10", { weight: "71", restingHr: "57" }),
    day("2026-10-07", { weight: "72" }),
    day("2026-10-01", { weight: "74", restingHr: "60" }),
  ];

  it("显示最近一次、平均、最低、最高和变化", () => {
    wrap(<BodyTrend days={days} end="2026-10-10" />);
    const stat = (name: string) =>
      screen.getByText(name).closest("div")!.querySelector("dd")!.textContent;
    expect(stat("最近一次")).toBe("71 kg");
    expect(stat("平均")).toBe("72.3 kg");
    expect(stat("最低")).toBe("71 kg");
    expect(stat("最高")).toBe("74 kg");
    expect(stat("范围内变化")).toBe("-3 kg");
    expect(screen.getByRole("img", { name: "体重" })).toBeTruthy();
  });

  it("切换指标；只有一天数据时提示至少记两天", () => {
    wrap(<BodyTrend days={days} end="2026-10-10" />);
    fireEvent.click(
      within(screen.getByRole("group", { name: "身体指标" })).getByText(
        "静息心率",
      ),
    );
    expect(
      screen.getByText("最近一次").closest("div")!.querySelector("dd")!
        .textContent,
    ).toBe("57 bpm");
    fireEvent.click(
      within(screen.getByRole("group", { name: "身体指标" })).getByText("睡眠"),
    );
    expect(screen.getByText("至少记两天才有趋势")).toBeTruthy();
  });
});

describe("BodySync", () => {
  it("生成令牌后显示一次示例命令，关闭后回到未开启", async () => {
    let enabled = false;
    api.routes.set("GET /habits/body/push", () => ({
      status: 200,
      body: {
        enabled,
        reportUrl: "https://x.test/api/v1/habits/body/report",
        ...(enabled ? { createdAt: "2026-10-10T00:00:00Z" } : {}),
      },
    }));
    api.routes.set("POST /habits/body/push/token", () => {
      enabled = true;
      return {
        status: 200,
        body: {
          token: "t".repeat(64),
          reportUrl: "https://x.test/api/v1/habits/body/report",
          example: "curl -H 'Authorization: Bearer tttt'",
        },
      };
    });
    api.routes.set("DELETE /habits/body/push", () => {
      enabled = false;
      return { status: 204 };
    });
    wrap(<BodySync />);
    await screen.findByText("还没有开启");
    fireEvent.click(screen.getByRole("button", { name: "生成上报令牌" }));
    await screen.findByText(/curl -H 'Authorization: Bearer tttt'/);
    await screen.findByText("还没有收到过上报");
    fireEvent.click(screen.getByRole("button", { name: "关闭" }));
    await waitFor(() => expect(screen.queryByText(/curl -H/)).toBeNull());
    await screen.findByText("还没有开启");
    expect(api.calls.map((c) => `${c.method} ${c.path}`)).toContain(
      "DELETE /habits/body/push",
    );
  });
});
