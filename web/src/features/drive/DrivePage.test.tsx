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

  it("browses a drive account on its own tab (B68, B69)", async () => {
    live();
    api.routes.set("GET /storage/remotes", (url) => ({
      status: 200,
      body: {
        items:
          url.searchParams.get("drive") === "true"
            ? [
                {
                  id: 7,
                  kind: "webdav",
                  name: "坚果云",
                  showInDrive: true,
                  ready: true,
                  usedByBackup: false,
                  createdAt: "2026-10-01T00:00:00Z",
                  webdav: {
                    url: "https://dav.jianguoyun.com/dav/",
                    username: "me",
                    passwordSet: true,
                  },
                },
              ]
            : [],
      },
    }));
    api.routes.set("GET /storage/remotes/7/items", (url) => ({
      status: 200,
      body:
        url.searchParams.get("ref") === "照片"
          ? {
              ref: "照片",
              trail: [{ ref: "照片", name: "照片" }],
              items: [
                {
                  ref: "照片/a.jpg",
                  name: "a.jpg",
                  isDir: false,
                  size: 2048,
                  downloadable: true,
                },
              ],
            }
          : {
              ref: "",
              trail: [],
              items: [
                { ref: "照片", name: "照片", isDir: true, downloadable: false },
              ],
            },
    }));
    const router = renderAt("/drive");
    fireEvent.click(await screen.findByRole("button", { name: /坚果云/ }));
    const row = await screen.findByRole("button", { name: "照片" });
    // 网盘标签里没有上传和搜索
    expect(screen.queryByRole("button", { name: /上传/ })).toBeNull();
    fireEvent.click(row);
    expect(
      await screen.findByRole("button", { name: "下载 a.jpg" }),
    ).toBeTruthy();
    expect(router.state.location.search).toBe(
      "?view=remote&remote=7&ref=%E7%85%A7%E7%89%87",
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

  it("copies selected items and asks what to do with same names", async () => {
    live();
    api.routes.set("GET /drive/tasks", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /drive/batch/copy", () => ({
      status: 202,
      body: {
        id: "t1",
        kind: "copy",
        state: "running",
        title: "复制 1 项到 /",
        doneItems: 0,
        totalItems: 1,
        doneBytes: 0,
        totalBytes: 2048,
        createdAt: "2026-09-29T00:00:00Z",
      },
    }));
    renderAt("/drive");
    fireEvent.click(await screen.findByLabelText("选择 报告.pdf"));
    fireEvent.click(
      within(screen.getByRole("toolbar", { name: "已选" })).getByRole(
        "button",
        { name: /复制到/ },
      ),
    );
    // 根目录里已经有“报告.pdf”，复制到根目录会重名。
    fireEvent.click(await screen.findByRole("button", { name: "复制到这里" }));
    expect(await screen.findByText("有重名的文件")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "保留两个" }));
    await waitFor(() =>
      expect(
        api.calls.find(
          (c) => c.method === "POST" && c.path === "/drive/batch/copy",
        )?.body,
      ).toEqual({ ids: [2], targetId: 0, conflict: "rename" }),
    );
    expect(await screen.findByText("复制 1 项到 /")).toBeTruthy();
  });

  it("moves one by one when the server has no batch move yet", async () => {
    live();
    api.routes.set("PATCH /drive/items/2", () => ({
      status: 200,
      body: { ...root[0], parentId: 1 },
    }));
    renderAt("/drive");
    fireEvent.click(await screen.findByLabelText("选择 报告.pdf"));
    fireEvent.click(
      within(screen.getByRole("toolbar", { name: "已选" })).getByRole(
        "button",
        { name: /移动/ },
      ),
    );
    const dialog = await screen.findByRole("dialog", { name: "移动到" });
    fireEvent.click(await within(dialog).findByText("照片"));
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.startsWith("/drive/items?parent=1")),
      ).toBe(true),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "移到这里" }));
    await waitFor(() =>
      expect(
        api.calls.find(
          (c) => c.method === "PATCH" && c.path === "/drive/items/2",
        )?.body,
      ).toEqual({ parentId: 1 }),
    );
    expect(api.calls.some((c) => c.path === "/drive/batch/move")).toBe(false);
  });

  it("says share links are not live on the shares tab", async () => {
    live();
    renderAt("/drive");
    fireEvent.click(await screen.findByRole("button", { name: /分享/ }));
    expect(await screen.findByText("分享链接还没上线")).toBeTruthy();
  });

  it("creates a share link with an access code", async () => {
    live();
    const writeText = vi.fn(() => Promise.resolve());
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    api.routes.set("GET /drive/shares", () => ({
      status: 200,
      body: { items: [] },
    }));
    api.routes.set("POST /drive/shares", (_url, body) => ({
      status: 201,
      body: {
        id: 5,
        itemId: 2,
        itemName: "报告.pdf",
        isDir: false,
        token: "abc",
        url: "https://x.example/s/abc",
        code: (body as { code?: string }).code,
        visits: 0,
        downloads: 0,
        active: true,
        createdAt: "2026-09-29T00:00:00Z",
      },
    }));
    renderAt("/drive");
    fireEvent.click(await screen.findByLabelText(/: 报告\.pdf$/));
    fireEvent.click(await screen.findByRole("menuitem", { name: /分享/ }));
    const dialog = await screen.findByRole("dialog", { name: "分享" });
    fireEvent.click(within(dialog).getByRole("button", { name: /生成链接/ }));
    await waitFor(() => expect(writeText).toHaveBeenCalled());
    const sent = api.calls.find(
      (c) => c.method === "POST" && c.path === "/drive/shares",
    )!.body as Record<string, unknown>;
    expect(sent.itemId).toBe(2);
    expect(sent.expiresIn).toBe("7d");
    expect(String(sent.code)).toMatch(/^[2-9a-z]{6}$/);
    expect(String(writeText.mock.calls[0])).toContain("密码");
  });
});
