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
