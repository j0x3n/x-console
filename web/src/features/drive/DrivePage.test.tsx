// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

// openapi-fetch 在建客户端时就取走 fetch，所以要在 import 页面之前替换。
const api = vi.hoisted(() => {
  type Reply = {
    status: number;
    body?: unknown;
    text?: string;
    headers?: Record<string, string>;
  };
  const routes = new Map<
    string,
    (url: URL, body: unknown, req: Request) => Reply
  >();
  const calls: Array<{
    method: string;
    path: string;
    body: unknown;
    headers: Headers;
  }> = [];
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
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input, init);
    const url = new URL(req.url);
    const path = url.pathname.replace("/api/v1", "");
    const text = await req.text();
    let body: unknown = null;
    try {
      body = text ? JSON.parse(text) : null;
    } catch {
      body = text;
    }
    calls.push({
      method: req.method,
      path: path + url.search,
      body,
      headers: req.headers,
    });
    const handler = routes.get(`${req.method} ${path}`);
    const reply: Reply = handler
      ? handler(url, body, req)
      : { status: 404, body: { code: "not_found", message: "not found" } };
    if (reply.text !== undefined)
      return new Response(reply.text, {
        status: reply.status,
        headers: reply.headers,
      });
    if (reply.body === undefined)
      return new Response(null, { status: reply.status });
    return new Response(JSON.stringify(reply.body), {
      status: reply.status,
      headers: { "Content-Type": "application/json", ...reply.headers },
    });
  }) as typeof fetch;
  return { routes, calls };
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
import { EditorView } from "@codemirror/view";
import { routes } from "./routes";

// jsdom 没有 Range 的布局接口，CodeMirror 量光标位置时会用到。
Range.prototype.getClientRects = () =>
  Object.assign([], { item: () => null }) as unknown as DOMRectList;
Range.prototype.getBoundingClientRect = () => new DOMRect();

const base = {
  size: 0,
  hidden: false,
  syncState: "off",
  createdAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
};
const root = [
  {
    ...base,
    id: 2,
    name: "报告.pdf",
    isDir: false,
    size: 2048,
    mime: "application/pdf",
    syncState: "synced",
  },
  { ...base, id: 1, name: "照片", isDir: true },
];
const inside = [
  {
    ...base,
    id: 3,
    name: "猫.jpg",
    isDir: false,
    size: 900,
    mime: "image/jpeg",
  },
];

function live(opts: { unlocked?: boolean } = {}) {
  api.routes.set("GET /drive/usage", () => ({
    status: 200,
    body: { files: 2, bytes: 2048, trashBytes: 0 },
  }));
  api.routes.set("GET /drive/s3/status", () => ({
    status: 200,
    body: { state: "off", pending: 0, synced: 0, failed: 0 },
  }));
  api.routes.set("GET /vault/status", () => ({
    status: 200,
    body: { configured: true, unlocked: !!opts.unlocked },
  }));
  api.routes.set("GET /drive/items", (url) => ({
    status: 200,
    body: {
      items:
        url.searchParams.get("parent") === "1"
          ? inside
          : url.searchParams.get("trashed")
            ? []
            : root,
    },
  }));
  api.routes.set("GET /drive/items/1", () => ({
    status: 200,
    body: { ...root[1], path: [] },
  }));
  api.routes.set("DELETE /drive/items/2", () => ({ status: 204 }));
}

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
  api.routes.clear();
  api.calls.length = 0;
});

describe("DrivePage", () => {
  it("says the drive is not live when the server has no drive API", async () => {
    renderAt("/drive");
    expect(await screen.findByText("云盘还没上线")).toBeTruthy();
  });

  it("lists folders first and opens a folder", async () => {
    live();
    const router = renderAt("/drive");
    const rows = await screen.findAllByRole("row");
    // 表头 + 两行，文件夹在前。
    expect(rows).toHaveLength(3);
    expect(within(rows[1]).getByText("照片")).toBeTruthy();
    expect(within(rows[2]).getByText("报告.pdf")).toBeTruthy();
    fireEvent.click(screen.getByText("照片"));
    expect(await screen.findByText("猫.jpg")).toBeTruthy();
    expect(router.state.location.search).toBe("?folder=1");
  });

  it("selects items and moves them to the trash", async () => {
    // 没挂 ConfirmHost 时 confirmAction 用浏览器自带的确认框。
    vi.stubGlobal("confirm", () => true);
    live();
    renderAt("/drive");
    fireEvent.click(await screen.findByLabelText("选择 报告.pdf"));
    const bar = screen.getByRole("toolbar");
    expect(within(bar).getByText(/已选/).textContent).toContain("1");
    fireEvent.click(within(bar).getByRole("button", { name: /删除/ }));
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === "DELETE" && c.path === "/drive/items/2",
        ),
      ).toBe(true),
    );
    await waitFor(() => expect(screen.queryByRole("toolbar")).toBeNull());
    vi.unstubAllGlobals();
  });

  it("shows the hidden tab only while the vault is unlocked", async () => {
    live();
    renderAt("/drive");
    await screen.findAllByRole("row");
    expect(screen.queryByRole("button", { name: /隐藏内容/ })).toBeNull();
    cleanup();
    live({ unlocked: true });
    renderAt("/drive");
    fireEvent.click(await screen.findByRole("button", { name: /隐藏内容/ }));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.startsWith("/drive/items?hidden=true")),
      ).toBe(true),
    );
  });

  it("uploads picked files to the current folder", async () => {
    live();
    const sent: string[] = [];
    const Xhr = vi.fn(function (this: Record<string, unknown>) {
      this.upload = {};
      this.open = (_m: string, url: string) => sent.push(url);
      this.setRequestHeader = () => {};
      this.send = () => {
        this.status = 201;
        (this.onload as () => void)();
      };
    });
    vi.stubGlobal("XMLHttpRequest", Xhr);
    renderAt("/drive?folder=1");
    await screen.findByText("猫.jpg");
    const input = screen.getByTestId("drive-file-input");
    fireEvent.change(input, {
      target: { files: [new File(["hi"], "a.txt", { type: "text/plain" })] },
    });
    await waitFor(() =>
      expect(sent).toEqual(["/api/v1/drive/upload?parent=1"]),
    );
    expect(await screen.findByText("上传完成")).toBeTruthy();
    vi.unstubAllGlobals();
  });

  it("opens files in the viewer and switches with the arrow keys", async () => {
    live();
    const files = [
      { ...base, id: 10, name: "config.yaml", isDir: false, size: 5 },
      {
        ...base,
        id: 11,
        name: "photo.png",
        isDir: false,
        size: 9,
        mime: "image/png",
      },
    ];
    api.routes.set("GET /drive/items", () => ({
      status: 200,
      body: { items: files },
    }));
    api.routes.set("GET /drive/items/10/content", () => ({
      status: 200,
      text: "a: 1\n",
      headers: { ETag: '"v1"' },
    }));
    renderAt("/drive");
    fireEvent.click(await screen.findByText("config.yaml"));
    const viewer = await screen.findByRole("dialog", { name: "config.yaml" });
    await waitFor(() =>
      expect(viewer.querySelector(".cm-content")?.textContent).toContain(
        "a: 1",
      ),
    );
    expect(within(viewer).getByText(/1 \/ 2/)).toBeTruthy();
    fireEvent.keyDown(document.body, { key: "ArrowRight" });
    const next = await screen.findByRole("dialog", { name: "photo.png" });
    expect(within(next).getByAltText("photo.png")).toBeTruthy();
    // 图片没有编辑入口。
    expect(within(next).queryByRole("button", { name: /编辑/ })).toBeNull();
    fireEvent.keyDown(document.body, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("saves edits with the version and reports conflicts", async () => {
    live();
    api.routes.set("GET /drive/items", () => ({
      status: 200,
      body: {
        items: [
          { ...base, id: 10, name: "config.yaml", isDir: false, size: 5 },
        ],
      },
    }));
    api.routes.set("GET /drive/items/10/content", () => ({
      status: 200,
      text: "a: 1\n",
      headers: { ETag: '"v1"' },
    }));
    let saves = 0;
    api.routes.set("PUT /drive/items/10/content", () =>
      ++saves === 1
        ? {
            status: 200,
            body: { ...base, id: 10, name: "config.yaml", isDir: false },
            headers: { ETag: '"v2"' },
          }
        : {
            status: 409,
            body: { code: "version_conflict", message: "文件在别处改过了" },
          },
    );
    renderAt("/drive");
    await screen.findByText("config.yaml");
    fireEvent.click(screen.getByLabelText("选择 config.yaml"));
    fireEvent.click(
      within(screen.getByRole("toolbar", { name: "已选" })).getByRole(
        "button",
        { name: /编辑/ },
      ),
    );
    const viewer = await screen.findByRole("dialog", { name: "config.yaml" });
    const type = async (text: string) => {
      await waitFor(() =>
        expect(viewer.querySelector(".cm-content")?.textContent).toContain(
          "a:",
        ),
      );
      const view = EditorView.findFromDOM(
        viewer.querySelector(".cm-editor") as HTMLElement,
      )!;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
      });
    };
    await type("a: 2\n");
    const saveButton = within(viewer).getByRole("button", {
      name: "保存",
    }) as HTMLButtonElement;
    await waitFor(() => expect(saveButton.disabled).toBe(false));
    fireEvent.click(saveButton);
    await waitFor(() => expect(saves).toBe(1));
    const first = api.calls.find((c) => c.method === "PUT")!;
    expect(first.body).toBe("a: 2\n");
    expect(first.headers.get("If-Match")).toBe('"v1"');
    expect(await within(viewer).findByText("没有改动")).toBeTruthy();

    await type("a: 3\n");
    await within(viewer).findByText("有改动没保存");
    fireEvent.keyDown(document.body, { key: "s", ctrlKey: true });
    expect(await within(viewer).findByText("文件在别处改过了")).toBeTruthy();
    const second = api.calls.filter((c) => c.method === "PUT")[1];
    expect(second.headers.get("If-Match")).toBe('"v2"');
    expect(within(viewer).getByRole("button", { name: "覆盖" })).toBeTruthy();
  });
});
