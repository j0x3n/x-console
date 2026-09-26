// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch reads globalThis.fetch when the client is created, so the
// mock must exist before the modules are imported.
const calls = vi.hoisted(() => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  const issue = (n: number, status: string, sortOrder: number) => ({
    id: n, key: `XC-${n}`, projectId: 1, projectKey: "XC", number: n, title: `Issue ${n}`,
    description: "", status, priority: 0, sortOrder, labels: [], externalSource: "", externalId: "",
    createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z",
  });
  const issues = [issue(1, "todo", 1), issue(2, "todo", 2), issue(3, "done", 1)];
  const project = { id: 1, key: "XC", name: "X Console", description: "", color: "", icon: "",
    createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z", issueCount: 3, openCount: 2 };
  // Node's Request needs absolute URLs; the app uses relative ones.
  const NativeRequest = globalThis.Request;
  globalThis.Request = class extends NativeRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(typeof input === "string" && input.startsWith("/") ? `http://localhost${input}` : input, init);
    }
  } as typeof Request;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof NativeRequest ? input : new Request(String(input), init);
    const url = new URL(req.url);
    const body = req.method === "GET" ? undefined : await req.clone().json().catch(() => undefined);
    calls.push({ method: req.method, url: url.pathname + url.search, body });
    const json = (data: unknown) =>
      new Response(JSON.stringify(data), { status: 200, headers: { "Content-Type": "application/json" } });
    const path = url.pathname.replace("/api/v1", "");
    if (path === "/projects") return json(url.searchParams.get("archived") ? [] : [project]);
    if (path === "/issues") return json({ items: issues });
    if (path.endsWith("/comments") || path.endsWith("/links")) return json([]);
    if (path.startsWith("/issues/") && req.method === "GET")
      return json(issues.find((i) => i.key === path.split("/")[2]));
    if (path.endsWith("/labels") || path.endsWith("/milestones")) return json([]);
    if (path.endsWith("/move")) {
      const move = body as { status: string; afterKey?: string; beforeKey?: string };
      const target = issues.find((i) => i.key === path.split("/")[2])!;
      const after = issues.find((i) => i.key === move.afterKey);
      const before = issues.find((i) => i.key === move.beforeKey);
      target.status = move.status;
      target.sortOrder = after ? after.sortOrder + 1 : before ? before.sortOrder - 1 : 0;
      return json(target);
    }
    if (path.startsWith("/issues/") && req.method === "PATCH") {
      const key = path.split("/")[2];
      return json({ ...issues.find((i) => i.key === key), ...(body as object) });
    }
    return json({});
  }) as typeof fetch;
  return calls;
});

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
    await waitFor(() => expect(router.state.location.pathname).toBe("/projects/XC/2"));
  });

  it("opens the new issue dialog with C", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    fireEvent.keyDown(document.body, { key: "c" });
    expect(await screen.findByRole("dialog", { name: "新建 Issue" })).toBeTruthy();
  });
});

describe("Board drag and drop", () => {
  it("moves a card below another one, optimistically", async () => {
    renderAt("/projects/XC");
    await screen.findByText("Issue 1");
    const card = (key: string) => document.querySelector(`[data-issue-key="${key}"]`)!;
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
    const order = [...todo.querySelectorAll("[data-issue-key]")].map((el) => el.getAttribute("data-issue-key"));
    expect(order).toEqual(["XC-2", "XC-1"]);
  });
});

describe("IssuePage", () => {
  it("shows the issue, a coding link, and edits with the keyboard", async () => {
    renderAt("/projects/XC/1");
    expect(await screen.findByRole("heading", { name: "Issue 1" })).toBeTruthy();
    const coding = screen.getByRole("link", { name: /交给编码助手/ }) as HTMLAnchorElement;
    expect(coding.getAttribute("href")).toBe("/coding?new=1&issue=XC-1");
    fireEvent.keyDown(document.body, { key: "4" });
    await waitFor(() =>
      expect(calls.find((c) => c.method === "PATCH")).toMatchObject({ body: { status: "in_review" } }),
    );
    fireEvent.keyDown(document.body, { key: "e" });
    const input = (await screen.findByLabelText("标题")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Renamed" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(calls.filter((c) => c.method === "PATCH").at(-1)).toMatchObject({ body: { title: "Renamed" } }),
    );
  });
});
