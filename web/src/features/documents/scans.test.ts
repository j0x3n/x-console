import { beforeEach, describe, expect, it, vi } from "vitest";

const calls = vi.hoisted(
  () => [] as Array<{ path: string; init?: RequestInit }>,
);
const replies = vi.hoisted(() => ({
  folders: [] as unknown[],
}));

vi.mock("../../api/client", () => ({
  apiFetch: async (path: string, init?: RequestInit) => {
    calls.push({ path, init });
    if (path.startsWith("/drive/items"))
      return new Response(JSON.stringify({ items: replies.folders }));
    if (path === "/drive/folders")
      return new Response(JSON.stringify({ id: 50, name: "证件档案" }));
    if (path.startsWith("/drive/upload"))
      return new Response(
        JSON.stringify({ items: [{ id: 77, name: "护照.jpg" }] }),
      );
    return new Response("{}", { status: 404 });
  },
}));

import { scanFolder, uploadScan } from "./scans";

beforeEach(() => {
  calls.length = 0;
  replies.folders = [];
});

describe("扫描件上传", () => {
  it("文件夹已经有了就直接用", async () => {
    replies.folders = [
      { id: 9, name: "别的", isDir: true },
      { id: 12, name: "证件档案", isDir: true },
    ];
    expect(await scanFolder(false)).toBe(12);
    expect(calls.map((c) => c.path)).toEqual(["/drive/items"]);
  });

  it("没有就新建；隐藏内容解锁时建在隐藏空间", async () => {
    expect(await scanFolder(true)).toBe(50);
    expect(calls[0]!.path).toBe("/drive/items?hidden=true");
    expect(JSON.parse(String(calls[1]!.init?.body))).toEqual({
      name: "证件档案",
      hidden: true,
    });
  });

  it("回收站里的同名文件夹不算", async () => {
    replies.folders = [
      {
        id: 12,
        name: "证件档案",
        isDir: true,
        trashedAt: "2026-10-01T00:00:00Z",
      },
    ];
    expect(await scanFolder(false)).toBe(50);
  });

  it("上传到文件夹里，返回云盘条目", async () => {
    replies.folders = [{ id: 12, name: "证件档案", isDir: true }];
    const item = await uploadScan(new File(["x"], "护照.jpg"), false);
    expect(item.id).toBe(77);
    const upload = calls.find((c) => c.path.startsWith("/drive/upload"))!;
    expect(upload.path).toBe("/drive/upload?parent=12");
    expect(upload.init?.body).toBeInstanceOf(FormData);
  });

  it("隐藏内容解锁时上传成隐藏文件", async () => {
    replies.folders = [{ id: 12, name: "证件档案", isDir: true }];
    await uploadScan(new File(["x"], "护照.jpg"), true);
    expect(calls.at(-1)!.path).toBe("/drive/upload?parent=12&hidden=true");
  });
});
