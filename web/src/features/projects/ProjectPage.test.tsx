// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch reads globalThis.fetch when the client is created, so the
// mock must exist before the modules are imported.
const { calls, state } = vi.hoisted(() => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  // live 为 true 时模拟 B36 后端已上线：有分类和检查清单接口。
  const state = {
    live: false,
    checklists: [] as {
      id: number;
      issueId: number;
      title: string;
      position: number;
      items: {
        id: number;
        checklistId: number;
        text: string;
        done: boolean;
        position: number;
      }[];
    }[],
  };
  // B46：一个看板，6 个状态各一个列表，列表 id = 100 + 状态序号。
  const statusList: Record<string, number> = {
    backlog: 101,
    todo: 102,
    in_progress: 103,
    in_review: 104,
    done: 105,
    canceled: 106,
  };
  const names: Record<string, string> = {
    backlog: "待规划",
    todo: "待办",
    in_progress: "进行中",
    in_review: "待审核",
    done: "已完成",
    canceled: "已取消",
  };
  const board = {
    id: 1,
    projectId: 1,
    name: "开发进程",
    icon: "🕹️",
    position: 1,
    starred: false,
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    lists: Object.entries(statusList).map(([status, id], i) => ({
      id,
      boardId: 1,
      name: names[status],
      position: i + 1,
      status,
      color: "",
      wipLimit: 0,
      collapsed: false,
      cardCount: 0,
    })),
  };
  const issue = (n: number, status: string, sortOrder: number) => ({
    id: n,
    boardId: 1,
    listId: statusList[status],
    key: `XC-${n}`,
    projectId: 1,
    projectKey: "XC",
    number: n,
    title: `Issue ${n}`,
    description: "",
    status,
    priority: 0,
    sortOrder,
    labels: [],
    externalSource: "",
    externalId: "",
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
  });
  const issues: (ReturnType<typeof issue> & {
    categoryId?: number;
    checklistDone?: number;
    checklistTotal?: number;
  })[] = [
    issue(1, "todo", 1),
    issue(2, "todo", 2),
    issue(3, "done", 1),
    issue(4, "in_progress", 1),
  ];
  const categories = [
    { id: 10, projectId: 1, name: "后端", position: 1, issueCount: 0 },
    {
      id: 11,
      projectId: 1,
      parentId: 10,
      name: "云盘",
      position: 1,
      issueCount: 1,
    },
    { id: 20, projectId: 1, name: "前端", position: 2, issueCount: 1 },
  ];
  const project = {
    id: 1,
    key: "XC",
    name: "X Console",
    description: "",
    color: "",
    icon: "",
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    issueCount: 4,
    openCount: 3,
  };
  // Node's Request needs absolute URLs; the app uses relative ones.
  const NativeRequest = globalThis.Request;
  globalThis.Request = class extends NativeRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(
        typeof input === "string" && input.startsWith("/")
          ? `http://localhost${input}`
          : input,
        init,
      );
    }
  } as typeof Request;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof NativeRequest ? input : new Request(String(input), init);
    const url = new URL(req.url);
    const body =
      req.method === "GET"
        ? undefined
        : await req
            .clone()
            .json()
            .catch(() => undefined);
    calls.push({ method: req.method, url: url.pathname + url.search, body });
    const json = (data: unknown) =>
      new Response(JSON.stringify(data), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    const path = url.pathname.replace("/api/v1", "");
    const notLive = () =>
      new Response(JSON.stringify({ error: "not_live" }), {
        status: 501,
        headers: { "Content-Type": "application/json" },
      });
    if (path.endsWith("/categories"))
      return state.live ? json(categories) : notLive();
    if (path.endsWith("/checklists") && req.method === "GET")
      return state.live ? json(state.checklists) : notLive();
    if (path.includes("/checklist-items/") && req.method === "PATCH") {
      const id = Number(path.split("/")[4]);
      const item = state.checklists
        .flatMap((l) => l.items)
        .find((i) => i.id === id)!;
      Object.assign(item, body);
      return json(item);
    }
    if (path === "/projects")
      return json(url.searchParams.get("archived") ? [] : [project]);
    if (path === "/projects/1/boards") return json([board]);
    if (path === "/boards/starred") return json([]);
    if (path === "/issues") {
      const statuses = url.searchParams.getAll("status");
      const items = state.live
        ? issues.map((i) =>
            i.key === "XC-1"
              ? { ...i, categoryId: 11, checklistDone: 3, checklistTotal: 5 }
              : i.key === "XC-2"
                ? { ...i, categoryId: 20 }
                : i,
          )
        : issues;
      return json({
        items: statuses.length
          ? items.filter((item) => statuses.includes(item.status))
          : items,
      });
    }
    if (path.endsWith("/comments") || path.endsWith("/links")) return json([]);
    if (path.startsWith("/issues/") && req.method === "GET")
      return json(issues.find((i) => i.key === path.split("/")[2]));
    if (path.endsWith("/labels") || path.endsWith("/milestones"))
      return json([]);
    if (path.endsWith("/move")) {
      const move = body as {
        listId: number;
        afterKey?: string;
        beforeKey?: string;
      };
      const target = issues.find((i) => i.key === path.split("/")[2])!;
      const after = issues.find((i) => i.key === move.afterKey);
      const before = issues.find((i) => i.key === move.beforeKey);
      target.listId = move.listId;
      target.status =
        Object.entries(statusList).find(([, id]) => id === move.listId)?.[0] ??
        target.status;
      target.sortOrder = after
        ? after.sortOrder + 1
        : before
          ? before.sortOrder - 1
          : 0;
      return json(target);
    }
    if (path.startsWith("/issues/") && req.method === "PATCH") {
      const key = path.split("/")[2];
      return json({
        ...issues.find((i) => i.key === key),
        ...(body as object),
      });
    }
    return json({});
  }) as typeof fetch;
  return { calls, state };
});

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { routes } from "./routes";

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

afterEach(() => {
  cleanup();
  calls.length = 0;
  state.live = false;
  state.checklists = [];
});

describe("ProjectsPage", () => {
  it("shows in-progress issues in their own summary card", async () => {
    renderAt("/projects");
    const stats = await screen.findByRole("region", { name: "项目" });
    await within(stats).findByText("3");
    const cards = stats.querySelectorAll(".xc-stat");
    expect(cards).toHaveLength(5);
    const inProgress = within(stats).getByText("进行中").closest(".xc-stat");
    expect(inProgress?.querySelector(".xc-stat-value")?.textContent).toBe("1");
  });
});

describe("ProjectPage", () => {
  it("renders the board and changes status with the keyboard", async () => {
    renderAt("/projects/XC");
    expect(await screen.findByText("Issue 1")).toBeTruthy();
    expect(screen.getByText("待办")).toBeTruthy(); // column title in Chinese
    // J selects the first card, 3 moves it to "In progress".
    fireEvent.keyDown(document.body, { key: "j" });
    fireEvent.keyDown(document.body, { key: "3" });
    await waitFor(() =>
      expect(calls.find((c) => c.method === "PATCH")).toMatchObject({
        url: "/api/v1/issues/XC-1",
        body: { status: "in_progress" },
      }),
    );
  });

  it("opens a side panel with Enter on the board", async () => {
    const router = renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    fireEvent.keyDown(document.body, { key: "j" });
    fireEvent.keyDown(document.body, { key: "j" });
    fireEvent.keyDown(document.body, { key: "Enter" });
    expect(await screen.findByRole("dialog", { name: "XC-2" })).toBeTruthy();
    expect(router.state.location.pathname).toBe("/projects/XC");
  });

  it("opens the new issue dialog with C", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    fireEvent.keyDown(document.body, { key: "c" });
    expect(
      await screen.findByRole("dialog", { name: "新建卡片" }),
    ).toBeTruthy();
  });
});

describe("Board drag and drop", () => {
  it("moves a card below another one, optimistically", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    const card = (key: string) =>
      document.querySelector(`[data-issue-key="${key}"]`)!;
    const data = new Map<string, string>();
    const dataTransfer = {
      setData: (k: string, v: string) => data.set(k, v),
      getData: (k: string) => data.get(k) ?? "",
      effectAllowed: "",
      dropEffect: "",
    };
    // Drag XC-1 onto the lower half of XC-2 (jsdom has no layout, so every
    // pointer position counts as the lower half).
    fireEvent.dragStart(card("XC-1"), { dataTransfer });
    fireEvent.dragOver(card("XC-2"), { dataTransfer });
    fireEvent.drop(card("XC-2"), { dataTransfer });
    await waitFor(() =>
      expect(calls.find((c) => c.url.endsWith("/move"))).toMatchObject({
        url: "/api/v1/issues/XC-1/move",
        body: { listId: 102, afterKey: "XC-2" },
      }),
    );
    const todo = [...document.querySelectorAll(".projects-lane")][1];
    const order = [...todo.querySelectorAll("[data-issue-key]")].map((el) =>
      el.getAttribute("data-issue-key"),
    );
    expect(order).toEqual(["XC-2", "XC-1"]);
  });
});

describe("IssuePage", () => {
  it("shows the issue, a coding link, and edits with the keyboard", async () => {
    renderAt("/projects/XC/1");
    expect(
      await screen.findByRole("heading", { name: "Issue 1" }),
    ).toBeTruthy();
    // B55：“手动建编码任务”收进了右栏的“…”菜单
    fireEvent.click(screen.getByRole("button", { name: "更多：XC-1" }));
    expect(
      screen.getByRole("menuitem", { name: /手动建编码任务/ }),
    ).toBeTruthy();
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.keyDown(document.body, { key: "4" });
    await waitFor(() =>
      expect(calls.find((c) => c.method === "PATCH")).toMatchObject({
        body: { status: "in_review" },
      }),
    );
    fireEvent.keyDown(document.body, { key: "e" });
    const input = (await screen.findByLabelText("标题")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Renamed" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(calls.filter((c) => c.method === "PATCH").at(-1)).toMatchObject({
        body: { title: "Renamed" },
      }),
    );
  });
});

describe("B36 before the backend is live", () => {
  it("keeps the old fields and says checklists are not live", async () => {
    renderAt("/projects/XC/1");
    await screen.findByRole("heading", { name: "Issue 1" });
    expect(await screen.findByText("检查清单还没上线。")).toBeTruthy();
    expect(screen.queryByLabelText("分类")).toBeNull();
    expect(screen.queryByLabelText("截止时刻")).toBeNull();
    fireEvent.change(screen.getByLabelText("截止日期"), {
      target: { value: "2026-10-03" },
    });
    await waitFor(() =>
      expect(calls.find((c) => c.method === "PATCH")?.body).toEqual({
        dueDate: "2026-10-03",
      }),
    );
  });
});

describe("B46 boards", () => {
  it("shows the board tab and its lists, and adds a card inline", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    const tab = screen.getByRole("tab", { name: /开发进程/ });
    expect(tab.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("heading", { name: "待审核" })).toBeTruthy();
    fireEvent.click(screen.getAllByText("添加卡片")[1]);
    const input =
      await screen.findByPlaceholderText("卡片标题，可以直接粘贴图片");
    fireEvent.change(input, { target: { value: "新卡片" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(
        calls.find((c) => c.method === "POST" && c.url.endsWith("/issues")),
      ).toMatchObject({ body: { title: "新卡片", listId: 102 } }),
    );
  });

  it("filters to my cards with Q", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    fireEvent.keyDown(document.body, { key: "q" });
    await waitFor(() => expect(screen.queryByText("Issue 1")).toBeNull());
  });
});

describe("B36 with the backend live", () => {
  it("shows checklist progress on the card", async () => {
    state.live = true;
    renderAt("/projects/XC");
    const card = await screen.findByText("Issue 1");
    const article = card.closest("article")!;
    expect(within(article).getByText("3/5")).toBeTruthy();
  });

  it("ticks a checklist item and saves the due time with minutes", async () => {
    state.live = true;
    state.checklists = [
      {
        id: 1,
        issueId: 1,
        title: "上线前",
        position: 1,
        items: [
          { id: 7, checklistId: 1, text: "写迁移", done: true, position: 1 },
          { id: 8, checklistId: 1, text: "补测试", done: false, position: 2 },
        ],
      },
    ];
    renderAt("/projects/XC/1");
    const box = (await screen.findByLabelText("补测试")) as HTMLInputElement;
    expect(screen.getAllByText("1/2").length).toBeGreaterThan(0);
    fireEvent.click(box);
    await waitFor(() =>
      expect(
        calls.find((c) => c.url.includes("/checklist-items/")),
      ).toMatchObject({
        method: "PATCH",
        url: "/api/v1/issues/XC-1/checklist-items/8",
        body: { done: true },
      }),
    );
    fireEvent.change(screen.getByLabelText("截止日期"), {
      target: { value: "2026-10-03" },
    });
    await waitFor(() =>
      expect(
        calls.find(
          (c) => c.url === "/api/v1/issues/XC-1" && c.method === "PATCH",
        )?.body,
      ).toEqual({ dueAt: new Date("2026-10-03T23:59:00").toISOString() }),
    );
  });
});
