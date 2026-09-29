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
  const issue = (n: number, status: string, sortOrder: number) => ({
    id: n,
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
        status: string;
        afterKey?: string;
        beforeKey?: string;
      };
      const target = issues.find((i) => i.key === path.split("/")[2])!;
      const after = issues.find((i) => i.key === move.afterKey);
      const before = issues.find((i) => i.key === move.beforeKey);
      target.status = move.status;
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

  it("opens the issue page with Enter", async () => {
    const router = renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    fireEvent.keyDown(document.body, { key: "j" });
    fireEvent.keyDown(document.body, { key: "j" });
    fireEvent.keyDown(document.body, { key: "Enter" });
    await waitFor(() =>
      expect(router.state.location.pathname).toBe("/projects/XC/2"),
    );
  });

  it("opens the new issue dialog with C", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    fireEvent.keyDown(document.body, { key: "c" });
    expect(
      await screen.findByRole("dialog", { name: "新建 Issue" }),
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
        body: { status: "todo", afterKey: "XC-2" },
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
    const coding = screen.getByRole("link", {
      name: /交给 Agent/,
    }) as HTMLAnchorElement;
    expect(coding.getAttribute("href")).toBe("/coding?new=1&issue=XC-1");
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

describe("B36 with the backend live", () => {
  it("shows category and checklist progress and filters by category", async () => {
    state.live = true;
    renderAt("/projects/XC");
    const card = await screen.findByText("Issue 1");
    const article = card.closest("article")!;
    expect(within(article).getByText("后端 / 云盘")).toBeTruthy();
    expect(within(article).getByText("3/5")).toBeTruthy();
    // 选一级分类“后端”，包含二级分类“云盘”里的 XC-1，不包含“前端”的 XC-2。
    fireEvent.change(await screen.findByLabelText("分类"), {
      target: { value: "10" },
    });
    await waitFor(() => expect(screen.queryByText("Issue 2")).toBeNull());
    expect(screen.getByText("Issue 1")).toBeTruthy();
  });

  it("opens with ?category= from the sidebar link", async () => {
    state.live = true;
    renderAt("/projects/XC?category=20");
    expect(await screen.findByText("Issue 2")).toBeTruthy();
    expect(screen.queryByText("Issue 1")).toBeNull();
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
