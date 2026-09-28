// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

const vault = vi.hoisted(() => ({ unlocked: false }));

// openapi-fetch reads globalThis.fetch when the client is created, so the
// mock must exist before the modules are imported.
const calls = vi.hoisted(() => {
  const calls: {
    method: string;
    path: string;
    body: Record<string, unknown> | undefined;
  }[] = [];
  const note = {
    id: 1,
    title: "学习笔记",
    body: "第一行",
    pinned: false,
    tags: ["work"],
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
  };
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
    const path = url.pathname.replace("/api/v1", "");
    const body =
      req.method === "GET"
        ? undefined
        : await req
            .clone()
            .json()
            .catch(() => undefined);
    calls.push({ method: req.method, path: path + url.search, body });
    const json = (data: unknown) =>
      new Response(JSON.stringify(data), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    if (path === "/notes")
      return json({
        items: [
          {
            ...note,
            excerpt: note.body,
            snippet: url.searchParams.get("q") ? "学习笔记" : undefined,
          },
        ],
      });
    if (path === "/vault/status")
      return json({ configured: true, unlocked: vault.unlocked });
    if (path === "/notes" && req.method === "POST")
      return json({ ...note, id: 2, hidden: true });
    if (path === "/notes/tags") return json([{ tag: "work", count: 1 }]);
    if (path === "/notes/1" && req.method === "GET") return json(note);
    if (path === "/notes/1" && req.method === "PATCH") {
      Object.assign(note, body, { updatedAt: new Date().toISOString() });
      return json(note);
    }
    return json({});
  }) as typeof fetch;
  return calls;
});

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
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
  vault.unlocked = false;
});

describe("NotesPage", () => {
  it("autosaves the last text once typing stops", async () => {
    renderAt("/notes/1");
    const body = (await screen.findByLabelText("笔记")) as HTMLTextAreaElement;
    expect(body.value).toBe("第一行");
    for (const text of ["第一行\n第", "第一行\n第二", "第一行\n第二行"])
      fireEvent.change(body, { target: { value: text } });
    expect(screen.getByRole("status").textContent).toBe("保存中…");
    await waitFor(
      () => expect(screen.getByRole("status").textContent).toBe("已保存"),
      { timeout: 3000 },
    );
    const patches = calls.filter((c) => c.method === "PATCH");
    expect(patches).toHaveLength(1);
    expect(patches[0].body).toMatchObject({
      body: "第一行\n第二行",
      title: "学习笔记",
      tags: ["work"],
    });
  });

  it("searches and highlights matches", async () => {
    renderAt("/notes?q=学习");
    const mark = await screen.findByText("学习", { selector: "mark" });
    expect(mark).toBeTruthy();
    expect(calls.some((c) => c.path.startsWith("/notes?q="))).toBe(true);
  });

  it("has no hidden category while the vault is locked", async () => {
    renderAt("/notes?hidden=1");
    await screen.findAllByText("学习笔记");
    expect(screen.queryAllByRole("button", { name: "隐藏" })).toHaveLength(0);
    expect(calls.some((c) => c.path.includes("hidden=true"))).toBe(false);
  });

  it("lists and creates hidden notes once unlocked", async () => {
    vault.unlocked = true;
    renderAt("/notes");
    const [chip] = await screen.findAllByRole("button", { name: "隐藏" });
    fireEvent.click(chip);
    await waitFor(() =>
      expect(calls.some((c) => c.path.includes("hidden=true"))).toBe(true),
    );
    fireEvent.click(screen.getByTitle("新建笔记"));
    await waitFor(() =>
      expect(
        calls.find((c) => c.method === "POST" && c.path === "/notes")?.body,
      ).toEqual({ hidden: true }),
    );
  });
});
