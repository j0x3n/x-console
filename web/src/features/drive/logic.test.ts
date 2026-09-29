import { describe, expect, it } from "vitest";
import type { DriveItem } from "./api";
import { itemActions } from "./components/ItemMenu";
import {
  archiveBase,
  archiveName,
  canExtract,
  conflictNames,
  daysLeftInTrash,
  fileKind,
  nameError,
  nextSort,
  randomCode,
  shareText,
  sortItems,
  taskPercent,
} from "./logic";
import { toInput } from "./S3SettingsTab";
import { uploadUrl } from "./upload";

const item = (over: Partial<DriveItem>): DriveItem => ({
  id: 1,
  name: "a.txt",
  isDir: false,
  size: 10,
  hidden: false,
  syncState: "off",
  createdAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  ...over,
});

describe("fileKind", () => {
  it("uses the mime type first, then the extension", () => {
    expect(fileKind(item({ isDir: true }))).toBe("folder");
    expect(fileKind(item({ mime: "image/png", name: "x" }))).toBe("image");
    expect(
      fileKind(item({ mime: "application/octet-stream", name: "a.PDF" })),
    ).toBe("pdf");
    expect(fileKind(item({ mime: "video/mp4" }))).toBe("video");
    expect(fileKind(item({ name: "main.go" }))).toBe("text");
    expect(fileKind(item({ name: "Dockerfile" }))).toBe("text");
    expect(fileKind(item({ name: "backup.tar.gz" }))).toBe("archive");
    expect(fileKind(item({ name: "photo.jpeg" }))).toBe("image");
    expect(fileKind(item({ name: "setup.exe" }))).toBe("other");
  });
});

describe("sortItems", () => {
  const list = [
    item({
      id: 1,
      name: "b10.txt",
      size: 5,
      updatedAt: "2026-09-03T00:00:00Z",
    }),
    item({
      id: 2,
      name: "b9.txt",
      size: 50,
      updatedAt: "2026-09-01T00:00:00Z",
    }),
    item({ id: 3, name: "z", isDir: true, size: 0 }),
  ];
  it("keeps folders first and sorts names by number", () => {
    expect(
      sortItems(list, { key: "name", desc: false }).map((i) => i.id),
    ).toEqual([3, 2, 1]);
    expect(
      sortItems(list, { key: "name", desc: true }).map((i) => i.id),
    ).toEqual([3, 1, 2]);
  });
  it("sorts by size and time", () => {
    expect(
      sortItems(list, { key: "size", desc: true }).map((i) => i.id),
    ).toEqual([3, 2, 1]);
    expect(
      sortItems(list, { key: "updatedAt", desc: true }).map((i) => i.id),
    ).toEqual([3, 1, 2]);
  });
  it("flips on the same column and picks a default on a new one", () => {
    expect(nextSort({ key: "name", desc: false }, "name")).toEqual({
      key: "name",
      desc: true,
    });
    expect(nextSort({ key: "name", desc: false }, "size")).toEqual({
      key: "size",
      desc: true,
    });
  });
});

describe("helpers", () => {
  it("counts days left in the trash", () => {
    const now = new Date("2026-09-11T00:00:00Z");
    expect(daysLeftInTrash("2026-09-01T00:00:00Z", now)).toBe(20);
    expect(daysLeftInTrash("2026-08-01T00:00:00Z", now)).toBe(0);
  });
  it("checks names", () => {
    expect(nameError("  ")).toMatch("不能为空");
    expect(nameError("a/b")).toMatch("/");
    expect(nameError("..")).toMatch("不能用");
    expect(nameError("报告.pdf")).toBe("");
  });
  it("offers hide only while the vault is unlocked", () => {
    expect(
      itemActions(item({}), { trash: false, vaultUnlocked: false }),
    ).not.toContain("hide");
    expect(
      itemActions(item({}), { trash: false, vaultUnlocked: true }),
    ).toContain("hide");
    expect(
      itemActions(item({ hidden: true }), {
        trash: false,
        vaultUnlocked: true,
      }),
    ).toContain("unhide");
    expect(
      itemActions(item({ isDir: true }), {
        trash: false,
        vaultUnlocked: false,
      }),
    ).toEqual(["share", "rename", "move", "copy", "compress", "trash"]);
    expect(itemActions(item({}), { trash: true, vaultUnlocked: true })).toEqual(
      ["restore", "delete-forever"],
    );
  });
  it("offers B31 actions only where they make sense", () => {
    const opts = { trash: false, vaultUnlocked: true };
    // 隐藏的东西不能分享。
    expect(itemActions(item({ hidden: true }), opts)).not.toContain("share");
    expect(itemActions(item({}), { ...opts, hiddenView: true })).not.toContain(
      "share",
    );
    // 文件夹能打包下载要看服务端有没有 B31 的接口。
    expect(itemActions(item({ isDir: true }), opts)).not.toContain("download");
    expect(
      itemActions(item({ isDir: true }), { ...opts, batchLive: true }),
    ).toContain("download");
    expect(itemActions(item({ isDir: true }), opts)).not.toContain("versions");
    expect(itemActions(item({ name: "a.tar.gz" }), opts)).toContain("extract");
    expect(itemActions(item({ name: "a.rar" }), opts)).not.toContain("extract");
  });
  it("finds name conflicts and default archive names", () => {
    expect(conflictNames(["a", "b"], new Set(["b", "c"]))).toEqual(["b"]);
    expect(archiveName(["报告.pdf"])).toBe("报告");
    expect(archiveName(["报告.pdf", "图.png"])).toBe("报告 等 2 项");
    expect(archiveBase("日志.tar.gz")).toBe("日志");
    expect(archiveBase("备份.ZIP")).toBe("备份");
    expect(canExtract({ isDir: false, name: "x.tgz" })).toBe(true);
    expect(canExtract({ isDir: true, name: "x.zip" })).toBe(false);
  });
  it("computes task progress", () => {
    const base = { doneBytes: 0, totalBytes: 0, doneItems: 0, totalItems: 0 };
    expect(taskPercent(base)).toBeNull();
    expect(taskPercent({ ...base, doneItems: 1, totalItems: 4 })).toBe(25);
    expect(
      taskPercent({
        ...base,
        doneBytes: 50,
        totalBytes: 200,
        doneItems: 3,
        totalItems: 4,
      }),
    ).toBe(25);
  });
  it("puts the access code into shared text", () => {
    expect(shareText({ url: "https://x/s/a" })).toBe("https://x/s/a");
    expect(shareText({ url: "https://x/s/a", code: "ab12" })).toBe(
      "https://x/s/a\n提取码：ab12",
    );
    expect(randomCode()).toMatch(/^[2-9a-z]{4}$/);
  });
  it("builds the upload address", () => {
    expect(uploadUrl({ parent: null, hidden: false })).toBe(
      "/api/v1/drive/upload",
    );
    expect(uploadUrl({ parent: 7, hidden: true })).toBe(
      "/api/v1/drive/upload?parent=7&hidden=true",
    );
  });
  it("sends the S3 secret only when typed", () => {
    const form = {
      endpoint: " https://s3.example.com ",
      region: "auto",
      bucket: "b",
      prefix: "/backup/",
      accessKeyId: "AK",
      pathStyle: true,
      enabled: true,
      includeHidden: false,
      secretAccessKey: "",
    };
    expect(toInput(form)).toEqual({
      endpoint: "https://s3.example.com",
      region: "auto",
      bucket: "b",
      prefix: "backup",
      accessKeyId: "AK",
      pathStyle: true,
      enabled: true,
      includeHidden: false,
    });
    expect(toInput({ ...form, secretAccessKey: " sk " }).secretAccessKey).toBe(
      "sk",
    );
  });
});
