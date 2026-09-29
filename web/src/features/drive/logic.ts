import type { DriveItem } from "./api";

export type FileKind =
  | "folder"
  | "image"
  | "pdf"
  | "video"
  | "audio"
  | "text"
  | "archive"
  | "other";

const TEXT_EXT = new Set(
  "txt md markdown json yaml yml toml ini conf cfg log csv tsv xml html htm css js mjs cjs ts tsx jsx go py rb rs java kt c h cpp hpp cs php sh bash zsh fish ps1 bat sql env gitignore dockerfile makefile lua swift vue svelte".split(
    " ",
  ),
);
const ARCHIVE_EXT = new Set("zip rar 7z tar gz tgz bz2 xz zst".split(" "));

export function extension(name: string): string {
  const base = name.toLowerCase();
  const dot = base.lastIndexOf(".");
  if (dot <= 0) return base === "dockerfile" || base === "makefile" ? base : "";
  return base.slice(dot + 1);
}

/** 按 mime 判断文件类型，mime 不明确时看扩展名。 */
export function fileKind(
  item: Pick<DriveItem, "isDir" | "mime" | "name">,
): FileKind {
  if (item.isDir) return "folder";
  const mime = (item.mime ?? "").toLowerCase();
  const ext = extension(item.name);
  if (mime.startsWith("image/")) return "image";
  if (mime === "application/pdf" || ext === "pdf") return "pdf";
  if (mime.startsWith("video/")) return "video";
  if (mime.startsWith("audio/")) return "audio";
  if (ARCHIVE_EXT.has(ext)) return "archive";
  if (
    mime.startsWith("text/") ||
    mime === "application/json" ||
    mime.endsWith("+json") ||
    mime.endsWith("+xml") ||
    mime === "application/xml" ||
    mime === "application/javascript" ||
    TEXT_EXT.has(ext)
  )
    return "text";
  if (/^(png|jpe?g|gif|webp|avif|bmp|svg)$/.test(ext)) return "image";
  if (/^(mp4|webm|mov|m4v)$/.test(ext)) return "video";
  if (/^(mp3|wav|ogg|m4a|flac|aac)$/.test(ext)) return "audio";
  return "other";
}

export type SortKey = "name" | "size" | "updatedAt";
export interface Sort {
  key: SortKey;
  desc: boolean;
}

const collator = new Intl.Collator("zh-CN", {
  numeric: true,
  sensitivity: "base",
});

/** 文件夹总在前面，再按选的列排序。 */
export function sortItems(items: DriveItem[], sort: Sort): DriveItem[] {
  const dir = sort.desc ? -1 : 1;
  return [...items].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    let c = 0;
    if (sort.key === "size") c = a.size - b.size;
    else if (sort.key === "updatedAt")
      c = a.updatedAt < b.updatedAt ? -1 : a.updatedAt > b.updatedAt ? 1 : 0;
    if (c === 0) c = collator.compare(a.name, b.name);
    return c * dir;
  });
}

/** 点列头：同一列再点一次反过来，换列时名称正序、其他倒序。 */
export function nextSort(current: Sort, key: SortKey): Sort {
  if (current.key === key) return { key, desc: !current.desc };
  return { key, desc: key !== "name" };
}

/** 回收站 30 天后清掉，返回还剩几天。 */
export function daysLeftInTrash(trashedAt: string, now = new Date()): number {
  const end = new Date(trashedAt).getTime() + 30 * 86400_000;
  return Math.max(0, Math.ceil((end - now.getTime()) / 86400_000));
}

/** 新名称的检查。没问题时返回空字符串。 */
export function nameError(name: string): string {
  const n = name.trim();
  if (!n) return "名称不能为空";
  if (n === "." || n === "..") return "不能用这个名称";
  if (/[\\/]/.test(n)) return "名称里不能有 / 或 \\";
  if (n.length > 255) return "名称太长";
  return "";
}

/** 要放进目标文件夹的名字里，哪些已经有了。 */
export function conflictNames(names: string[], taken: Set<string>): string[] {
  return names.filter((n) => taken.has(n));
}

/** 能在线解压的压缩包：zip、tar、tar.gz、tgz。 */
export function canExtract(item: Pick<DriveItem, "isDir" | "name">): boolean {
  if (item.isDir) return false;
  const name = item.name.toLowerCase();
  return /\.(zip|tar|tgz|tar\.gz)$/.test(name);
}

/** 解压到新文件夹时的默认名字：去掉扩展名。 */
export function archiveBase(name: string): string {
  return name.replace(/\.(zip|tar|tgz|tar\.gz)$/i, "") || name;
}

/** 压缩包默认名：只选一个时用它的名字，多个时用第一个加“等”。 */
export function archiveName(names: string[]): string {
  if (names.length === 0) return "archive";
  const first = names[0].replace(/\.[^.]+$/, "") || names[0];
  return names.length === 1 ? first : `${first} 等 ${names.length} 项`;
}

/** 分享链接的提取码：去掉容易看错的 0、O、1、l、I。 */
export function randomCode(length = 4): string {
  const chars = "23456789abcdefghjkmnpqrstuvwxyz";
  const buf = new Uint32Array(length);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => chars[n % chars.length]).join("");
}

/** 任务进度百分比。按字节算，没有字节数时按条目数；还没数完时返回 null。 */
export function taskPercent(task: {
  doneBytes: number;
  totalBytes: number;
  doneItems: number;
  totalItems: number;
}): number | null {
  if (task.totalBytes > 0)
    return Math.min(100, Math.floor((task.doneBytes / task.totalBytes) * 100));
  if (task.totalItems > 0)
    return Math.min(100, Math.floor((task.doneItems / task.totalItems) * 100));
  return null;
}

/** 复制给别人的分享文字：有提取码时带上。 */
export function shareText(share: { url: string; code?: string }): string {
  return share.code ? `${share.url}\n提取码：${share.code}` : share.url;
}
