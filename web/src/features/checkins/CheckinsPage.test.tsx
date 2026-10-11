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
import CheckinsPage from "./CheckinsPage";
import "./i18n";

const slots = [
  {
    questionId: 1,
    label: "现在精力怎么样",
    lowLabel: "很低",
    highLabel: "很好",
    slot: "10:00",
    status: "answered",
    value: 4,
  },
  {
    questionId: 1,
    label: "现在精力怎么样",
    lowLabel: "很低",
    highLabel: "很好",
    slot: "15:00",
    status: "pending",
  },
  {
    questionId: 2,
    label: "现在压力大不大",
    lowLabel: "很轻松",
    highLabel: "很大",
    slot: "20:00",
    status: "upcoming",
  },
];
const questions = [
  {
    id: 1,
    label: "现在精力怎么样",
    lowLabel: "很低",
    highLabel: "很好",
    times: ["10:00", "15:00"],
    enabled: true,
    sortOrder: 1,
  },
  {
    id: 2,
    label: "现在压力大不大",
    lowLabel: "很轻松",
    highLabel: "很大",
    times: ["20:00"],
    enabled: true,
    sortOrder: 2,
  },
];
let enabled = false;

function setRoutes() {
  api.routes.clear();
  api.calls.length = 0;
  const ok = (body: unknown, status = 200) => ({ status, body });
  api.routes.set("GET /checkins/today", () =>
    ok({ date: "2026-11-11", slots }),
  );
  api.routes.set("GET /checkins/questions", () => ok({ items: questions }));
  api.routes.set("GET /checkins/settings", () => ok({ enabled }));
  api.routes.set("PUT /checkins/settings", (body) => {
    enabled = (body as { enabled: boolean }).enabled;
    return ok({ enabled });
  });
  api.routes.set("GET /checkins/series", () =>
    ok({
      from: "2026-11-01",
      to: "2026-11-11",
      items: [
        {
          questionId: 1,
          label: "现在精力怎么样",
          average: 3.5,
          count: 2,
          points: [
            { date: "2026-11-10", average: 3, count: 1 },
            { date: "2026-11-11", average: 4, count: 1 },
          ],
        },
        {
          questionId: 2,
          label: "现在压力大不大",
          average: 0,
          count: 0,
          points: [],
        },
      ],
    }),
  );
  api.routes.set("PUT /checkins/answers", (body) =>
    ok({ id: 1, ...(body as object) }),
  );
  api.routes.set("POST /checkins/questions", (body) =>
    ok({ id: 3, sortOrder: 3, ...(body as object) }, 201),
  );
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <CheckinsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  enabled = false;
  setRoutes();
});
afterEach(() => {
  cleanup();
});

describe("状态打卡页", () => {
  it("显示今天的时间点和状态，已回答的分数高亮", async () => {
    renderPage();
    await screen.findByRole("group", { name: "现在精力怎么样 15:00" });
    const group = screen.getByRole("group", { name: "现在精力怎么样 10:00" });
    expect(
      within(group)
        .getByRole("button", { name: "现在精力怎么样 10:00 4" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.getByText("等你回答")).toBeTruthy();
    expect(screen.getByText("还没到")).toBeTruthy();
    // 还没到的时间点不能提前答
    const upcoming = screen.getByRole("group", {
      name: "现在压力大不大 20:00",
    });
    const button = within(upcoming).getByRole("button", {
      name: "现在压力大不大 20:00 3",
    }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it("点分数补答，带上问题和时间点", async () => {
    renderPage();
    await screen.findByRole("group", { name: "现在精力怎么样 15:00" });
    fireEvent.click(
      screen.getByRole("button", { name: "现在精力怎么样 15:00 5" }),
    );
    await waitFor(() =>
      expect(api.calls.some((c) => c.path === "/checkins/answers")).toBe(true),
    );
    const put = api.calls.find((c) => c.path === "/checkins/answers");
    expect(put?.method).toBe("PUT");
    expect(put?.body).toEqual({ questionId: 1, slot: "15:00", value: 5 });
  });

  it("总开关写回设置", async () => {
    renderPage();
    await screen.findByRole("group", { name: "现在精力怎么样 15:00" });
    fireEvent.click(screen.getByRole("checkbox", { name: "每天问我" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "PUT" && c.path === "/checkins/settings",
        ),
      ).toBe(true),
    );
    expect(
      api.calls.find(
        (c) => c.path === "/checkins/settings" && c.method === "PUT",
      )?.body,
    ).toEqual({ enabled: true });
  });

  it("趋势显示平均分，没有回答的问题给出说明", async () => {
    renderPage();
    const card = await screen.findByRole("article", { name: "现在精力怎么样" });
    expect(within(card).getByText("3.5")).toBeTruthy();
    const empty = screen.getByRole("article", { name: "现在压力大不大" });
    expect(within(empty).getByText("这段时间没有回答。")).toBeTruthy();
  });

  it("新建问题：保存时带上时间点", async () => {
    renderPage();
    await screen.findByRole("group", { name: "现在精力怎么样 15:00" });
    fireEvent.click(screen.getAllByRole("button", { name: /新建问题/ })[0]);
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(
      within(dialog).getByPlaceholderText("例如 现在心情怎么样"),
      {
        target: { value: "现在心情怎么样" },
      },
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "POST" && c.path === "/checkins/questions",
        ),
      ).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "POST")?.body).toMatchObject({
      label: "现在心情怎么样",
      times: ["10:00"],
      enabled: true,
    });
  });
});
