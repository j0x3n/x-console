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
import RoiPage from "./RoiPage";
import "./i18n";

function renderIt(url = "/coding/roi") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <RoiPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const zero = {
  merged: 0,
  reverted: 0,
  canceled: 0,
  failed: 0,
  waiting: 0,
  active: 0,
  costMissing: 0,
  tokenMissing: 0,
  runMissing: 0,
  reviewMissing: 0,
  rounds: 0,
};

const base = {
  runs: 1,
  costPartial: false,
  reviewState: "none",
  status: "pr_opened",
  executor: "claude",
  createdAt: "2026-10-06T01:00:00Z",
  startedAt: "2026-10-06T01:00:00Z",
};

const report = {
  period: "week",
  from: "2026-10-05",
  to: "2026-10-11",
  groupBy: "repo",
  screenAvailable: true,
  totals: {
    ...zero,
    key: "",
    label: "",
    tasks: 3,
    merged: 1,
    reverted: 1,
    active: 1,
    mergeRate: 0.5,
    cost: 0.5,
    costMissing: 1,
    inputTokens: 1000,
    outputTokens: 200,
    runSeconds: 2100,
    reviewMinutes: 35,
    reviewMissing: 1,
    rounds: 1,
    costPerMerge: 0.5,
    runSecondsPerMerge: 2100,
    reviewMinutesPerMerge: 35,
  },
  groups: [
    {
      ...zero,
      key: "x-console",
      label: "x-console",
      tasks: 2,
      merged: 1,
      reverted: 1,
      mergeRate: 0.5,
      cost: 0.5,
      costMissing: 1,
      runSeconds: 2100,
      reviewMinutes: 35,
      rounds: 1,
      costPerMerge: 0.5,
      runSecondsPerMerge: 2100,
      reviewMinutesPerMerge: 35,
    },
    {
      ...zero,
      key: "blog",
      label: "blog",
      tasks: 1,
      active: 1,
      reviewMissing: 1,
    },
  ],
  tasks: [
    {
      ...base,
      id: 1,
      title: "修复溢出",
      repoName: "x-console",
      outcome: "merged",
      cost: 0.5,
      inputTokens: 1000,
      outputTokens: 200,
      runSeconds: 1200,
      rounds: 0,
      reviewMinutes: 30,
      reviewState: "done",
      reviewPerRun: 1.5,
      agentName: "Builder",
    },
    {
      ...base,
      id: 2,
      title: "Codex 的任务",
      repoName: "x-console",
      executor: "codex",
      status: "discarded",
      outcome: "reverted",
      runs: 2,
      rounds: 1,
      runSeconds: 900,
      inputTokens: 1000,
      outputTokens: 200,
      reviewMinutes: 5,
      reviewState: "done",
    },
    {
      ...base,
      id: 3,
      title: "等你看",
      repoName: "blog",
      status: "review",
      outcome: "active",
      rounds: 0,
      runSeconds: 600,
      reviewState: "no_data",
    },
    {
      ...base,
      id: 4,
      title: "排队中",
      repoName: "blog",
      status: "queued",
      outcome: "active",
      runs: 0,
      startedAt: undefined,
    },
  ],
};

function serve(body: unknown = report) {
  api.routes.set("GET /codingroi/report", () => ({ status: 200, body }));
}

const reportCalls = () =>
  api.calls.filter((c) => c.path.startsWith("/codingroi/report"));

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
});

describe("RoiPage B129", () => {
  it("接口还没上线时显示还没上线", async () => {
    renderIt();
    expect(await screen.findByText("投入产出还没上线")).toBeTruthy();
  });

  it("出错时可以重试", async () => {
    api.routes.set("GET /codingroi/report", () => ({
      status: 500,
      body: { code: "internal", message: "坏了" },
    }));
    renderIt();
    expect(await screen.findByRole("button", { name: /重试/ })).toBeTruthy();
  });

  it("没有任务时说明按创建日期归期", async () => {
    serve({ ...report, tasks: [], groups: [] });
    renderIt();
    expect(await screen.findByText("这段时间没有编码任务")).toBeTruthy();
    expect(screen.getByText(/任务按创建的日期/)).toBeTruthy();
  });

  it("页面上写明看自己的数据，审查时间是估算", async () => {
    serve();
    renderIt();
    expect(
      await screen.findByText(/AI 划不划算，只能看你自己的数据/),
    ).toBeTruthy();
    expect(await screen.findByText(/审查时间是估算值/)).toBeTruthy();
    // 没有写研究的名字
    expect(document.body.textContent).not.toMatch(/METR|Brynjolfsson/);
  });

  it("概要卡片只加有记录的，缺的任务数写在下面", async () => {
    serve();
    renderIt();
    const strip = await screen.findByRole("region", { name: "投入产出" });
    expect(within(strip).getByText("$0.50")).toBeTruthy();
    expect(within(strip).getByText(/1\.2k Token/)).toBeTruthy();
    expect(within(strip).getAllByText(/另有 1 个无记录/).length).toBe(2);
    expect(within(strip).getByText("合并率 50%")).toBeTruthy();
  });

  it("没有记录的格子写无记录，不写 0", async () => {
    serve();
    renderIt();
    await screen.findByText("任务明细");
    const tasks = document.querySelector(".roi-tasks") as HTMLElement;
    const rows = within(tasks).getAllByRole("row");
    // 第 2 行是任务 1，第 3 行任务 2，之后任务 3、任务 4，按创建时间排，同时间保持原顺序
    const task2 = rows.find((r) => r.textContent?.includes("Codex 的任务"))!;
    expect(within(task2).getByText("无记录")).toBeTruthy(); // 费用
    const task3 = rows.find((r) => r.textContent?.includes("等你看"))!;
    // 费用、Token 和审查时间都没有记录
    expect(within(task3).getAllByText("无记录").length).toBe(3);
    // 还没开始的任务不是“没记录”
    const task4 = rows.find((r) => r.textContent?.includes("排队中"))!;
    expect(within(task4).queryByText("无记录")).toBeNull();
  });

  it("分组表有合计行，合计里缺记录的数字前写至少", async () => {
    serve();
    renderIt();
    await screen.findByText("分组汇总");
    const groups = document.querySelector(".roi-groups") as HTMLElement;
    const total = groups.querySelector(".roi-total") as HTMLElement;
    expect(within(total).getByText("合计")).toBeTruthy();
    expect(within(total).getAllByText(/至少/).length).toBeGreaterThan(0);
  });

  it("点列名排序，再点反过来，没有记录的排最后", async () => {
    serve();
    renderIt();
    await screen.findByText("分组汇总");
    const groups = document.querySelector(".roi-groups") as HTMLElement;
    const names = () =>
      Array.from(groups.querySelectorAll("tbody .roi-name strong")).map(
        (x) => x.textContent,
      );
    // 默认按任务数从多到少
    expect(names()).toEqual(["x-console", "blog"]);
    fireEvent.click(within(groups).getByRole("button", { name: "费用" }));
    expect(names()).toEqual(["x-console", "blog"]); // blog 没有费用，排最后
    fireEvent.click(within(groups).getByRole("button", { name: "费用" }));
    expect(names()).toEqual(["x-console", "blog"]); // 升序时也一样
    fireEvent.click(within(groups).getByRole("button", { name: "任务" }));
    fireEvent.click(within(groups).getByRole("button", { name: "任务" }));
    expect(names()).toEqual(["blog", "x-console"]);
  });

  it("切换分组和范围时重新取数，导出链接带同样的条件", async () => {
    serve();
    renderIt();
    await screen.findByText("分组汇总");
    expect(reportCalls()[0].path).toContain("period=week");
    expect(reportCalls()[0].path).toContain("groupBy=repo");
    fireEvent.click(screen.getByRole("button", { name: "工具" }));
    await waitFor(() =>
      expect(reportCalls().some((c) => c.path.includes("groupBy=tool"))).toBe(
        true,
      ),
    );
    const link = screen.getByRole("link", { name: /导出 CSV/ });
    expect(link.getAttribute("href")).toContain("/api/v1/codingroi/report.csv");
    expect(link.getAttribute("href")).toContain("groupBy=tool");
    fireEvent.click(screen.getByRole("button", { name: "月" }));
    await waitFor(() =>
      expect(reportCalls().some((c) => c.path.includes("period=month"))).toBe(
        true,
      ),
    );
    expect(
      screen.getByRole("link", { name: /导出 CSV/ }).getAttribute("href"),
    ).toContain("period=month");
  });

  it("翻页用服务端给的起始日", async () => {
    serve();
    renderIt();
    await screen.findByText("分组汇总");
    fireEvent.click(screen.getByRole("button", { name: "上一段" }));
    await waitFor(() =>
      expect(
        reportCalls().some((c) => c.path.includes("date=2026-09-28")),
      ).toBe(true),
    );
  });

  it("电脑时间不可用时说明为什么审查时间没有记录", async () => {
    serve({ ...report, screenAvailable: false });
    renderIt();
    expect(await screen.findByText(/电脑时间没有记录/)).toBeTruthy();
  });

  it("任务标题链接到任务页", async () => {
    serve();
    renderIt();
    const link = await screen.findByRole("link", { name: "修复溢出" });
    expect(link.getAttribute("href")).toBe("/coding/1");
  });
});
