import type { DriveItem } from "../api";
import { extension, fileKind } from "../logic";

/** 查看器里的显示方式。 */
export type ViewerKind =
  | "image"
  | "video"
  | "audio"
  | "pdf"
  | "markdown"
  | "log"
  | "text"
  | "other";

/** 超过这个大小的文本不进编辑器，按日志的方式分段读。 */
export const MAX_EDIT_BYTES = 10 * 1024 * 1024;
/** 日志每次读多少。 */
export const LOG_CHUNK_BYTES = 1024 * 1024;

type Item = Pick<DriveItem, "isDir" | "mime" | "name" | "size">;

export function viewerKind(item: Item): ViewerKind {
  const kind = fileKind(item);
  const ext = extension(item.name);
  if (
    kind === "image" ||
    kind === "video" ||
    kind === "audio" ||
    kind === "pdf"
  )
    return kind;
  if (kind !== "text") return "other";
  if (ext === "log" || item.size > MAX_EDIT_BYTES) return "log";
  if (ext === "md" || ext === "markdown") return "markdown";
  return "text";
}

/** 能不能在线编辑：文本，并且不超过 10 MB。 */
export function canEdit(item: Item): boolean {
  if (item.isDir || item.size > MAX_EDIT_BYTES) return false;
  return fileKind(item) === "text";
}

/** 按 UTF-8 解码。解不开时返回 null。 */
export function decodeUtf8(bytes: Uint8Array): string | null {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

/** 列表里上一个、下一个文件的下标，到头了就是 -1。 */
export function neighbor(
  items: Pick<DriveItem, "id">[],
  id: number,
  step: 1 | -1,
): number {
  const i = items.findIndex((x) => x.id === id);
  if (i < 0) return -1;
  const next = i + step;
  return next >= 0 && next < items.length ? next : -1;
}

/**
 * 日志分段读：把一段字节切成完整的行。
 * 不是从文件开头读的，第一行可能只有后半截，按字节放进 head，
 * 留给更早的一段拼上再解码，这样不会把多字节的字切坏。
 */
export function splitChunk(
  bytes: Uint8Array,
  fromStart: boolean,
): { head: Uint8Array; lines: string[] } {
  let head: Uint8Array = new Uint8Array(0);
  let body: Uint8Array = bytes;
  if (!fromStart) {
    const nl = bytes.indexOf(10);
    head = nl < 0 ? bytes : bytes.slice(0, nl);
    body = nl < 0 ? new Uint8Array(0) : bytes.slice(nl + 1);
  }
  const text = new TextDecoder("utf-8").decode(body);
  const lines = text === "" ? [] : text.split("\n");
  if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return { head, lines };
}

/** 两段字节接起来。 */
export function concatBytes(a: Uint8Array, b: Uint8Array): Uint8Array {
  const out = new Uint8Array(a.length + b.length);
  out.set(a);
  out.set(b, a.length);
  return out;
}

/** 图片缩放：限制在 10% 到 800% 之间。 */
export function clampZoom(zoom: number): number {
  return Math.min(8, Math.max(0.1, Math.round(zoom * 100) / 100));
}
