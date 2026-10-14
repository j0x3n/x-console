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
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryRouter,
  MemoryRouter,
  Route,
  RouterProvider,
  Routes,
} from "react-router";
import CookPage from "./CookPage";
import DiscoverPage from "./DiscoverPage";
import RecipeEditPage from "./RecipeEditPage";
import RecipesPage from "./RecipesPage";
import "./i18n";

const client = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } });

const chicken = {
  id: 1,
  title: "临沂炒鸡",
  summary: "先煸后炖，最后收汁",
  servings: "一只散养公鸡",
  totalMinutes: 90,
  ingredients: [
    { name: "散养公鸡", amount: "1500 克", note: "剁小块" },
    { name: "花椒", amount: "1 汤匙" },
    { name: "八角", amount: "2 颗" },
  ],
  steps: [
    {
      title: "泡血水",
      text: "用冷水浸泡半小时，中途换一次水。",
      timers: [{ label: "泡血水", seconds: 1800 }],
      ingredients: [0],
    },
    {
      title: "炒香料",
      text: "放入花椒、八角炒香。",
      timers: [],
      ingredients: [],
    },
  ],
  tips: "",
  tags: ["辣", "鲁菜"],
  sourceNoteId: null,
  sourceUrl: "",
  nutrition: ["high_protein"],
  cookedCount: 2,
  lastCookedOn: "2026-10-10",
  avgRating: 4.5,
  createdAt: "2026-10-14T00:00:00Z",
  updatedAt: "2026-10-14T00:00:00Z",
};
const eggs = {
  ...chicken,
  id: 2,
  title: "番茄炒蛋",
  summary: "",
  ingredients: [{ name: "鸡蛋", amount: "3 个" }],
  tags: ["家常"],
  nutrition: ["veg_rich"],
  cookedCount: 0,
  avgRating: null,
};

afterEach(() => {
  cleanup();
  api.routes.clear();
  api.calls.length = 0;
  localStorage.clear();
});

describe("菜单", () => {
  it("列出菜谱，能按标签和关键字筛选", async () => {
    api.routes.set("GET /recipes", () => ({
      status: 200,
      body: {
        items: [chicken, eggs],
        tags: [
          { tag: "辣", count: 1 },
          { tag: "鲁菜", count: 1 },
          { tag: "家常", count: 1 },
        ],
      },
    }));
    api.routes.set("GET /cook-timers", () => ({
      status: 200,
      body: { items: [] },
    }));
    render(
      <QueryClientProvider client={client()}>
        <MemoryRouter initialEntries={["/recipes"]}>
          <RecipesPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("临沂炒鸡")).toBeTruthy();
    expect(screen.getByText("番茄炒蛋")).toBeTruthy();

    fireEvent.click(screen.getByRole("tab", { name: "家常" }));
    expect(screen.queryByText("临沂炒鸡")).toBeNull();
    expect(screen.getByText("番茄炒蛋")).toBeTruthy();

    fireEvent.click(screen.getByRole("tab", { name: "全部" }));
    fireEvent.change(screen.getByPlaceholderText("搜索菜谱"), {
      target: { value: "花椒" },
    });
    expect(screen.getByText("临沂炒鸡")).toBeTruthy();
    expect(screen.queryByText("番茄炒蛋")).toBeNull();
  });

  it("没有菜谱时给新建和导入的按钮", async () => {
    api.routes.set("GET /recipes", () => ({
      status: 200,
      body: { items: [], tags: [] },
    }));
    render(
      <QueryClientProvider client={client()}>
        <MemoryRouter initialEntries={["/recipes"]}>
          <RecipesPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("还没有菜谱")).toBeTruthy();
    expect(screen.getAllByText("从文字导入").length).toBeGreaterThan(0);
  });
});

describe("B210 菜单筛选", () => {
  it("填了我手头有，只列能做的菜，写出还缺什么", async () => {
    api.routes.set("GET /recipes", () => ({
      status: 200,
      body: { items: [chicken, eggs], tags: [] },
    }));
    api.routes.set("GET /cook-timers", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("GET /recipes/settings", () => ({
      status: 200,
      body: { habitId: null, dietGoal: "none", staples: ["花椒"] },
    }));
    render(
      <QueryClientProvider client={client()}>
        <MemoryRouter initialEntries={["/recipes"]}>
          <RecipesPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await screen.findByText("番茄炒蛋");
    const input = screen.getByLabelText("我手头有");
    fireEvent.change(input, { target: { value: "鸡" } });
    fireEvent.keyDown(input, { key: "Enter" });
    // “鸡”对上散养公鸡和鸡蛋；临沂炒鸡还缺八角（花椒是常备调料）
    await screen.findByText("还缺：八角");
    expect(screen.getByText("都有了")).toBeTruthy();
    // 只要高蛋白的
    fireEvent.click(screen.getByRole("button", { name: "高蛋白" }));
    expect(screen.queryByText("番茄炒蛋")).toBeNull();
    expect(screen.getByText("临沂炒鸡")).toBeTruthy();
    expect(screen.getByText(/做过 2 次 · 4.5 分/)).toBeTruthy();
  });
});

describe("做菜页", () => {
  it("显示这一步的料，点计时按钮开始计时，能切到下一步", async () => {
    api.routes.set("GET /recipes/1", () => ({ status: 200, body: chicken }));
    api.routes.set("GET /cook-timers", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /cook-timers", (body) => ({
      status: 201,
      body: {
        id: 9,
        recipeTitle: "临沂炒鸡",
        state: "running",
        endsAt: "2026-10-14T01:00:00Z",
        remainingSeconds: 1800,
        createdAt: "2026-10-14T00:30:00Z",
        ...(body as object),
      },
    }));
    render(
      <QueryClientProvider client={client()}>
        <MemoryRouter initialEntries={["/recipes/1/cook"]}>
          <Routes>
            <Route path="/recipes/:id/cook" element={<CookPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("第 1 / 2 步")).toBeTruthy();
    expect(screen.getByText("散养公鸡")).toBeTruthy();
    expect(screen.getByText("1500 克")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /计时 30 分钟/ }));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.method === "POST" && c.path === "/cook-timers")
          ?.body,
      ).toEqual({ label: "泡血水", seconds: 1800, recipeId: 1, stepIndex: 0 }),
    );

    fireEvent.click(screen.getByRole("button", { name: /下一步/ }));
    expect(screen.getByText("第 2 / 2 步")).toBeTruthy();
    // 第二步没标食材，按名字在正文里找到花椒和八角
    expect(screen.getByText("花椒")).toBeTruthy();
    expect(screen.getByText("八角")).toBeTruthy();
    expect(localStorage.getItem("recipes.cook.1")).toBe("1");
    expect(screen.getByRole("button", { name: /做完了/ })).toBeTruthy();
  });

  it("做完了弹出记一顿，记下来是自己做的，打了分", async () => {
    localStorage.setItem("recipes.cook.1", "1");
    api.routes.set("GET /recipes/1", () => ({ status: 200, body: chicken }));
    api.routes.set("GET /recipes", () => ({
      status: 200,
      body: { items: [chicken], tags: [] },
    }));
    api.routes.set("GET /cook-timers", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /meals", (body) => ({
      status: 201,
      body: { id: 5, createdAt: "", ...(body as object) },
    }));
    render(
      <QueryClientProvider client={client()}>
        <MemoryRouter initialEntries={["/recipes/1/cook"]}>
          <Routes>
            <Route path="/recipes/:id/cook" element={<CookPage />} />
            <Route path="/recipes/:id" element={<div>详情</div>} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: /做完了/ }));
    await screen.findByText("做完了，记一下这顿吗？");
    fireEvent.click(screen.getByRole("radio", { name: "4 分" }));
    fireEvent.click(screen.getByRole("button", { name: "记下来" }));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.method === "POST" && c.path === "/meals")?.body,
      ).toMatchObject({
        recipeId: 1,
        title: "临沂炒鸡",
        cooked: true,
        rating: 4,
      }),
    );
    expect(await screen.findByText("详情")).toBeTruthy();
    expect(localStorage.getItem("recipes.cook.1")).toBeNull();
  });
});

describe("编辑页", () => {
  it("删掉食材后保存，步骤里的下标跟着改", async () => {
    api.routes.set("GET /recipes/1", () => ({ status: 200, body: chicken }));
    api.routes.set("PATCH /recipes/1", (body) => ({
      status: 200,
      body: { ...chicken, ...(body as object) },
    }));
    const router = createMemoryRouter(
      [
        { path: "/recipes/:id/edit", element: <RecipeEditPage /> },
        { path: "/recipes/:id", element: <div>详情</div> },
      ],
      { initialEntries: ["/recipes/1/edit"] },
    );
    render(
      <QueryClientProvider client={client()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    await screen.findByDisplayValue("临沂炒鸡");
    // 第二步原来没标食材，先标上八角（下标 2）
    const picks = screen.getAllByRole("button", { name: "八角" });
    fireEvent.click(picks[picks.length - 1]);
    // 删掉第一个食材“散养公鸡”
    fireEvent.click(screen.getAllByRole("button", { name: "移除" })[0]);
    fireEvent.click(screen.getAllByRole("button", { name: /保存/ })[0]);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === "PATCH")).toBe(true),
    );
    const body = api.calls.find((c) => c.method === "PATCH")!.body as {
      ingredients: { name: string }[];
      steps: { ingredients: number[] }[];
    };
    expect(body.ingredients.map((i) => i.name)).toEqual(["花椒", "八角"]);
    expect(body.steps.map((s) => s.ingredients)).toEqual([[], [1]]);
    expect(await screen.findByText("详情")).toBeTruthy();
  });
});

describe("B211 找新菜谱", () => {
  const profile = {
    tags: ["辣", "鲁菜"],
    nutrition: ["high_protein"],
    dietGoal: "none",
    webSearch: "brave",
    runsLeft: 9,
  };
  const candidate = {
    id: 7,
    draft: {
      title: "辣子鸡",
      summary: "重庆家常做法",
      ingredients: [{ name: "鸡腿", amount: "500 克" }],
      steps: [{ text: "鸡腿切丁腌 15 分钟" }],
      tags: ["辣"],
      nutrition: ["high_protein"],
      sourceUrl: "https://example.com/lazi",
    },
    reason: "你常做辣的菜",
    sources: [{ title: "辣子鸡做法", url: "https://example.com/lazi" }],
    status: "pending",
    recipeId: null,
    createdAt: "2026-10-14T00:00:00Z",
  };
  const renderDiscover = () =>
    render(
      <QueryClientProvider client={client()}>
        <MemoryRouter initialEntries={["/recipes/discover"]}>
          <DiscoverPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

  it("联网搜索关着时提示去设置", async () => {
    api.routes.set("GET /recipe-candidates", () => ({
      status: 200,
      body: { items: [], profile: { ...profile, webSearch: "off" } },
    }));
    renderDiscover();
    expect(await screen.findByText("联网搜索还没打开")).toBeTruthy();
    expect(screen.getByRole("link", { name: "去设置" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /帮我找/ })).toBeNull();
  });

  it("按要求找菜，候选能不要", async () => {
    api.routes.set("GET /recipe-candidates", () => ({
      status: 200,
      body: { items: [candidate], profile },
    }));
    api.routes.set("POST /recipes/discover", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("PATCH /recipe-candidates/7", (body) => ({
      status: 200,
      body: { ...candidate, ...(body as object) },
    }));
    renderDiscover();
    expect(await screen.findByText("辣子鸡")).toBeTruthy();
    expect(screen.getByText("你常做辣的菜")).toBeTruthy();
    expect(screen.getByText(/今天还能找：9/)).toBeTruthy();

    fireEvent.change(screen.getByLabelText("这次想吃什么"), {
      target: { value: "清淡的汤" },
    });
    fireEvent.click(screen.getByRole("button", { name: /帮我找/ }));
    await waitFor(() =>
      expect(
        api.calls.find((c) => c.path === "/recipes/discover")?.body,
      ).toEqual({ hint: "清淡的汤", count: 3 }),
    );

    fireEvent.click(screen.getByRole("button", { name: /不要/ }));
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toMatchObject({
        status: "dismissed",
      }),
    );
  });

  it("收进菜单：保存后把候选标成已收下", async () => {
    api.routes.set("POST /recipes", (body) => ({
      status: 201,
      body: { ...chicken, ...(body as object), id: 9 },
    }));
    api.routes.set("PATCH /recipe-candidates/7", (body) => ({
      status: 200,
      body: { ...candidate, ...(body as object) },
    }));
    const router = createMemoryRouter(
      [
        { path: "/recipes/new", element: <RecipeEditPage /> },
        { path: "/recipes/:id", element: <div>详情</div> },
      ],
      {
        initialEntries: [
          {
            pathname: "/recipes/new",
            state: { draft: candidate.draft, candidateId: 7 },
          },
        ],
      },
    );
    render(
      <QueryClientProvider client={client()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    await screen.findByDisplayValue("辣子鸡");
    fireEvent.click(screen.getAllByRole("button", { name: /保存/ })[0]);
    await waitFor(() =>
      expect(api.calls.find((c) => c.method === "PATCH")?.body).toEqual({
        status: "accepted",
        recipeId: 9,
      }),
    );
    expect(
      (
        api.calls.find((c) => c.path === "/recipes")?.body as {
          sourceUrl: string;
        }
      ).sourceUrl,
    ).toBe("https://example.com/lazi");
  });
});
