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
import { MemoryRouter, Route, Routes } from "react-router";
import ExperimentsPage from "./ExperimentsPage";
import ExperimentDetailPage from "./ExperimentDetailPage";
import "./i18n";

const metric = {
  key: "body:sleep",
  label: "睡眠",
  unit: "小时",
  subjective: false,
};
const experiment = {
  id: 7,
  title: "下午不喝咖啡",
  change: "两点后不喝咖啡",
  baselineStart: "2026-11-01",
  changeStart: "2026-11-08",
  endDate: "2026-11-15",
  note: "",
  conclusion: "",
  stopped: false,
  phase: "finished",
  retrospective: false,
  metrics: [metric],
  createdAt: "2026-11-01T00:00:00Z",
};
const stats = (n: number, mean: number) => ({ days: 7, n, mean, sd: 0.5 });

function result(verdict: string, extra: object = {}) {
  return {
    metric,
    verdict,
    baseline: stats(7, 6.2),
    change: stats(7, 7.1),
    diff: 0.9,
    ciLow: 0.3,
    ciHigh: 1.5,
    notes: [],
    points: [
      { date: "2026-11-02", value: 6.2, phase: "baseline" },
      { date: "2026-11-09", value: 7.1, phase: "change" },
    ],
    ...extra,
  };
}

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/experiments" element={<ExperimentsPage />} />
          <Route path="/experiments/:id" element={<ExperimentDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  api.routes.clear();
  api.calls.length = 0;
  api.routes.set("GET /experiments", () => ({
    status: 200,
    body: { items: [experiment] },
  }));
  api.routes.set("GET /experiments/metrics", () => ({
    status: 200,
    body: {
      items: [
        { ...metric, group: "body" },
        {
          key: "checkin:1",
          label: "现在精力怎么样",
          unit: "分",
          subjective: true,
          group: "checkin",
        },
      ],
    },
  }));
});
afterEach(cleanup);

describe("自我实验列表", () => {
  it("显示实验、阶段和指标", async () => {
    renderAt("/experiments");
    const link = await screen.findByRole("link", { name: /下午不喝咖啡/ });
    expect(within(link).getByText("已结束")).toBeTruthy();
    expect(within(link).getByText(/睡眠/)).toBeTruthy();
  });

  it("没有实验时给出说明", async () => {
    api.routes.set("GET /experiments", () => ({
      status: 200,
      body: { items: [] },
    }));
    renderAt("/experiments");
    expect(await screen.findByText("还没有实验")).toBeTruthy();
  });

  it("新建：至少选一个指标，保存时带上日期和指标", async () => {
    api.routes.set("POST /experiments", (body) => ({
      status: 201,
      body: { ...experiment, id: 8, ...(body as object) },
    }));
    renderAt("/experiments");
    await screen.findByRole("link", { name: /下午不喝咖啡/ });
    fireEvent.click(screen.getAllByRole("button", { name: /新建实验/ })[0]);
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByPlaceholderText("例如 下午不喝咖啡"), {
      target: { value: "早睡" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    expect(await within(dialog).findByText("至少选一个指标")).toBeTruthy();
    fireEvent.click(
      await within(dialog).findByRole("checkbox", { name: "睡眠" }),
    );
    // 选了自己打的分，出现提示
    fireEvent.click(
      within(dialog).getByRole("checkbox", { name: "现在精力怎么样" }),
    );
    expect(within(dialog).getByText(/容易受预期影响/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.method === "POST" && c.path === "/experiments"),
      ).toBe(true),
    );
    const post = api.calls.find((c) => c.method === "POST");
    expect(post?.body).toMatchObject({
      title: "早睡",
      metrics: ["body:sleep", "checkin:1"],
    });
  });
});

describe("自我实验详情", () => {
  it("有差异：先说方向和区间，再列两段的数字", async () => {
    api.routes.set("GET /experiments/7", () => ({
      status: 200,
      body: { experiment, results: [result("higher")] },
    }));
    renderAt("/experiments/7");
    const card = await screen.findByRole("article", { name: "睡眠" });
    expect(within(card).getByText(/改变期更高，平均高 0.9 小时/)).toBeTruthy();
    expect(within(card).getByText("+0.9 小时", { exact: false })).toBeTruthy();
    expect(within(card).getByRole("img", { name: /睡眠/ })).toBeTruthy();
  });

  it("样本太少只给数字并说不能下结论", async () => {
    api.routes.set("GET /experiments/7", () => ({
      status: 200,
      body: {
        experiment,
        results: [
          result("too_few", {
            baseline: stats(3, 6.2),
            notes: ["sparse_data", "subjective"],
          }),
        ],
      },
    }));
    renderAt("/experiments/7");
    const card = await screen.findByRole("article", { name: "睡眠" });
    expect(within(card).getByText(/天数太少，不能下结论/)).toBeTruthy();
    expect(within(card).getByText(/有数据的天数不到六成/)).toBeTruthy();
    expect(within(card).getByText(/自己打的分/)).toBeTruthy();
  });

  it("区间含 0：写成在随机波动范围内", async () => {
    api.routes.set("GET /experiments/7", () => ({
      status: 200,
      body: {
        experiment,
        results: [
          result("inconclusive", { diff: 0.1, ciLow: -0.5, ciHigh: 0.7 }),
        ],
      },
    }));
    renderAt("/experiments/7");
    expect(await screen.findByText(/差异在随机波动范围内/)).toBeTruthy();
  });

  it("写结论并保存，保存时带上原有的设置", async () => {
    api.routes.set("GET /experiments/7", () => ({
      status: 200,
      body: { experiment, results: [result("higher")] },
    }));
    api.routes.set("PUT /experiments/7", (body) => ({
      status: 200,
      body: { ...experiment, ...(body as object) },
    }));
    renderAt("/experiments/7");
    const box = await screen.findByRole("textbox", { name: "你的结论" });
    fireEvent.change(box, { target: { value: "继续不喝" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PUT")).toBe(true),
    );
    expect(api.calls.find((c) => c.method === "PUT")?.body).toMatchObject({
      title: "下午不喝咖啡",
      conclusion: "继续不喝",
      metrics: ["body:sleep"],
      baselineStart: "2026-11-01",
    });
  });
});
