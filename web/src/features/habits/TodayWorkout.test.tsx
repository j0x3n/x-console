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

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import TodayWorkout from "./TodayWorkout";
import "./i18n";
import "./personalI18n";

const exercise = (id: string, name: string) => ({
  id,
  name,
  en: name,
  group: "下肢",
  equipment: "哑铃",
  target: "臀腿",
  dose: "3 组 × 8–12 次",
  rest: "90 秒",
  setup: ["站稳"],
  steps: [`${name}第一步`],
  errors: ["不憋气"],
  easier: "",
  progress: "",
  restored: false,
  image: "",
});

const library = {
  version: 1,
  sourceHash: "",
  exercises: [exercise("squat", "高脚杯深蹲"), exercise("birddog", "鸟狗")],
  sessions: [
    {
      id: "L3",
      name: "下肢 A · 蹲主导",
      focus: "臀腿",
      place: "家中",
      time: "45 分钟",
      items: [
        {
          exerciseId: "squat",
          sets: 3,
          prescription: "8–12 次",
          optional: false,
        },
        {
          exerciseId: "birddog",
          sets: 1,
          prescription: "每侧 5 次",
          optional: true,
        },
      ],
    },
  ],
  runLevels: [],
  phases: [],
  habits: [],
  articles: [],
  original: "",
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  vi.useRealTimers();
});

describe("今天练什么（B203）", () => {
  it("按阶段和星期列出今天的动作，点动作看要领", async () => {
    // 2026-10-19 是周一，第三阶段的周一是“下肢 A”
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-19T10:00:00+08:00"));
    api.routes.set("GET /habits/library", () => ({
      status: 200,
      body: library,
    }));
    api.routes.set("GET /habits/personal/profile", () => ({
      status: 200,
      body: {
        start: "2026-09-01",
        wake: "07:30",
        sleep: "23:30",
        phase: 3,
        baseline: 70,
        stepGoal: 8000,
        runLevel: 2,
        habitIds: {},
      },
    }));
    api.routes.set("GET /habits/schedule", () => ({
      status: 200,
      body: { timezone: "Asia/Shanghai" },
    }));
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <TodayWorkout />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByText(/今天练什么 · 下肢 A/)).toBeTruthy();
    expect(screen.getByText("3 组 · 8–12 次")).toBeTruthy();
    expect(screen.getByText(/可选动作/)).toBeTruthy();
    expect(
      screen.getByRole("link", { name: /开始跟练/ }).getAttribute("href"),
    ).toBe("/habits/plan?section=training&date=2026-10-19");
    expect(screen.getByText(/腿麻/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /高脚杯深蹲/ }));
    expect(await screen.findByText("高脚杯深蹲第一步")).toBeTruthy();
  });
});
