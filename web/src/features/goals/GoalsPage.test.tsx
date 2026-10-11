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
import GoalsPage from "./GoalsPage";
import { currentPeriod, shiftPeriod } from "./format";
import "./i18n";

function renderIt(url = "/goals") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <GoalsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const period = currentPeriod();
const now = "2026-10-11T00:00:00Z";

const manualResult = {
  id: 11,
  goalId: 1,
  name: "读完几本书",
  target: 4,
  unit: "本",
  startValue: 0,
  source: "manual",
  ref: "",
  refLabel: "",
  value: 1,
  progress: 25,
  state: "behind",
  unavailable: "",
  updatedAt: now,
};
const habitResult = {
  ...manualResult,
  id: 12,
  name: "读书打卡",
  target: 80,
  unit: "次",
  source: "habit_checkins",
  ref: "1",
  refLabel: "读书",
  value: 41.5,
  progress: 51.9,
  state: "on_track",
};
const hiddenResult = {
  ...manualResult,
  id: 13,
  name: "提交",
  source: "github_commits",
  value: null,
  progress: null,
  unavailable: "hidden",
};
const goal = {
  id: 1,
  title: "多读书",
  year: period.year,
  quarter: period.quarter,
  note: "每天睡前读",
  status: "active",
  results: [manualResult, habitResult, hiddenResult],
  progress: 38.5,
  expected: 45.1,
  state: "behind",
  from: "2026-10-01",
  until: "2026-12-31",
  createdAt: now,
  updatedAt: now,
};
const quiet = {
  ...goal,
  id: 2,
  title: "练身体",
  note: "",
  results: [],
  progress: null,
  state: "on_track",
};

const metrics = {
  items: [
    {
      source: "habit_checkins",
      refKind: "habit",
      options: [
        { value: "1", label: "读书" },
        { value: "2", label: "跑步" },
      ],
    },
    { source: "workouts", refKind: "none", options: [] },
    {
      source: "screen_minutes",
      refKind: "category",
      options: [
        { value: "", label: "全部分类" },
        { value: "coding", label: "编码" },
      ],
    },
  ],
};

function serve(items: unknown[] = [goal, quiet]) {
  api.routes.set("GET /goals", () => ({
    status: 200,
    body: { items, current: period },
  }));
  api.routes.set("GET /goals/metrics", () => ({ status: 200, body: metrics }));
}

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("GoalsPage B128", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("目标还没上线")).toBeTruthy();
  });

  it("显示进度、按时间应到的进度、落后和关键结果", async () => {
    serve();
    renderIt();
    expect(await screen.findByText("多读书")).toBeTruthy();
    expect(screen.getByText("每天睡前读")).toBeTruthy();
    expect(screen.getByText("进度 38.5% · 按时间应到 45.1%")).toBeTruthy();
    expect(screen.getByText("1 / 4 本")).toBeTruthy();
    expect(screen.getByText("41.5 / 80 次")).toBeTruthy();
    expect(screen.getByText("来源已隐藏")).toBeTruthy();
    // 概要：2 个进行中、平均进度只算有数的、1 个落后
    const stat = (label: string) =>
      screen.getByText(label, { selector: ".xc-stat *" }).closest(".xc-stat")!
        .textContent;
    expect(stat("进行中的目标")).toContain("2");
    expect(stat("平均进度")).toContain("38.5%");
    expect(stat("落后")).toContain("1");
    expect(stat("季度已过去")).toContain("45.1%");
    // 没有关键结果的目标有提示
    expect(screen.getByText("还没有关键结果。编辑目标来添加。")).toBeTruthy();
    // 落后的目标和关键结果都有标记
    expect(screen.getAllByText("落后").length).toBeGreaterThanOrEqual(3);
  });

  it("只看进行中会隐藏已完成和已放弃的", async () => {
    serve([goal, { ...quiet, status: "done" }]);
    renderIt();
    await screen.findByText("多读书");
    expect(screen.getByText("练身体")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "只看进行中" }));
    expect(screen.queryByText("练身体")).toBeNull();
    expect(screen.getByText("多读书")).toBeTruthy();
  });

  it("手动关键结果可以直接更新数值", async () => {
    serve();
    api.routes.set("POST /goals/results/11/value", () => ({
      status: 200,
      body: { ...manualResult, value: 3, progress: 75 },
    }));
    renderIt();
    await screen.findByText("多读书");
    // 自动统计的关键结果没有更新按钮
    expect(screen.getAllByRole("button", { name: "更新数值" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "更新数值" }));
    const input = screen.getByLabelText(
      "读完几本书 当前值",
    ) as HTMLInputElement;
    expect(input.value).toBe("1");
    fireEvent.change(input, { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    expect(api.calls.some((c) => c.method === "POST")).toBe(false);
    fireEvent.change(input, { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({
        value: 3,
      }),
    );
  });

  it("切换季度会按新的季度取数", async () => {
    serve();
    renderIt();
    await screen.findByText("多读书");
    fireEvent.click(screen.getByRole("button", { name: "上一个季度" }));
    const prev = shiftPeriod(period, -1);
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.path === `/goals?year=${prev.year}&quarter=${prev.quarter}`,
        ),
      ).toBe(true),
    );
    expect(screen.getByRole("button", { name: "本季度" })).toBeTruthy();
  });

  it("这个季度没有目标时引导新建，保存时带上手动和自动的关键结果", async () => {
    serve([]);
    api.routes.set("POST /goals", () => ({ status: 201, body: goal }));
    renderIt();
    expect(await screen.findByText("这个季度还没有目标")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: /新建目标/ })[0]!);
    const dialog = screen.getByRole("dialog");

    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText("请填写目标名称")).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("目标名称"), {
      target: { value: "多读书" },
    });

    fireEvent.click(
      within(dialog).getByRole("button", { name: /添加关键结果/ }),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(
      await within(dialog).findByText("每个关键结果都要有名称"),
    ).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("关键结果名称"), {
      target: { value: "读完几本书" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText(/请填目标值/)).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("读完几本书 目标值"), {
      target: { value: "4" },
    });

    fireEvent.click(
      within(dialog).getByRole("button", { name: /添加关键结果/ }),
    );
    const names = within(dialog).getAllByLabelText("关键结果名称");
    fireEvent.change(names[1]!, { target: { value: "读书打卡" } });
    // 自动来源在接口返回之后才有，选中后自动带出第一个习惯和单位
    await waitFor(() =>
      expect(
        within(dialog).getAllByRole("option", { name: "习惯打卡次数" }),
      ).toHaveLength(2),
    );
    const sources = within(dialog).getAllByLabelText(/数据来源$/);
    fireEvent.change(sources[1]!, { target: { value: "habit_checkins" } });
    expect(within(dialog).getByRole("option", { name: "跑步" })).toBeTruthy();
    fireEvent.change(within(dialog).getByLabelText("读书打卡 目标值"), {
      target: { value: "80" },
    });

    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === "POST" && c.path === "/goals"),
      ).toBe(true),
    );
    const body = api.calls.find((c) => c.method === "POST")!.body as {
      title: string;
      year: number;
      quarter: number;
      results: Array<Record<string, unknown>>;
    };
    expect(body).toMatchObject({
      title: "多读书",
      year: period.year,
      quarter: period.quarter,
    });
    expect(body.results[0]).toMatchObject({
      name: "读完几本书",
      source: "manual",
      target: 4,
      startValue: 0,
    });
    expect(body.results[1]).toMatchObject({
      name: "读书打卡",
      source: "habit_checkins",
      ref: "1",
      target: 80,
      unit: "次",
    });
  });

  it("编辑目标时带上原来的关键结果编号，删除前要确认", async () => {
    serve();
    api.routes.set("PUT /goals/1", () => ({ status: 200, body: goal }));
    api.routes.set("DELETE /goals/2", () => ({ status: 204 }));
    renderIt();
    await screen.findByText("多读书");
    fireEvent.click(screen.getByRole("button", { name: "更多: 多读书" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "编辑" }));
    const dialog = screen.getByRole("dialog");
    expect(
      (within(dialog).getByLabelText("目标名称") as HTMLInputElement).value,
    ).toBe("多读书");
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    const body = api.calls.find((c) => c.method === "PUT")!.body as {
      results: Array<{ id: number; source: string }>;
    };
    expect(body.results.map((r) => r.id)).toEqual([11, 12, 13]);
    // 来源现在用不了的关键结果保存时仍然是原来的来源
    expect(body.results[2]!.source).toBe("github_commits");

    fireEvent.click(screen.getByRole("button", { name: "更多: 练身体" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "删除" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });
});
