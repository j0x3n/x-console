import { parseMarkdown, type Block } from "../../components/markdown/mdparse";
import { caretFor } from "./logic";

/*
 * B242 笔记导出图片：选项存取、选中内容截取、倍数计算、文件名。
 * 截图本身在 components/ImageExportDialog.tsx。
 */

/** 图片里放哪些内容。 */
export interface ImageParts {
  title: boolean;
  date: boolean;
  tags: boolean;
  hidden: boolean;
  brand: boolean;
}

export const DEFAULT_PARTS: ImageParts = {
  title: true,
  date: true,
  tags: false,
  hidden: false,
  brand: false,
};

const PARTS_KEY = "xc.notes.imageExport";

export function readParts(): ImageParts {
  try {
    const raw = localStorage.getItem(PARTS_KEY);
    if (!raw) return DEFAULT_PARTS;
    const saved = JSON.parse(raw) as Partial<Record<keyof ImageParts, unknown>>;
    const out = { ...DEFAULT_PARTS };
    for (const key of Object.keys(out) as (keyof ImageParts)[])
      if (typeof saved[key] === "boolean") out[key] = saved[key];
    return out;
  } catch {
    return DEFAULT_PARTS;
  }
}

export function saveParts(parts: ImageParts) {
  try {
    localStorage.setItem(PARTS_KEY, JSON.stringify(parts));
  } catch {
    /* 存不了就只在这次有效 */
  }
}

/**
 * B246 宽表格在图片里怎么放：cards 每行变成一块“表头：内容”，图片宽度不变；
 * wide 保持表格，图片跟着表格加宽，最宽 WIDE_SHEET_WIDTH。
 */
export type TableMode = "cards" | "wide";

const TABLE_MODE_KEY = "xc.notes.imageExport.tables";

export function readTableMode(): TableMode {
  try {
    return localStorage.getItem(TABLE_MODE_KEY) === "wide" ? "wide" : "cards";
  } catch {
    return "cards";
  }
}

export function saveTableMode(mode: TableMode) {
  try {
    localStorage.setItem(TABLE_MODE_KEY, mode);
  } catch {
    /* 存不了就只在这次有效 */
  }
}

/** 列数到这么多才算宽表格。3 列以内在 400 宽里放得下，照常画成表格。 */
export const CARD_MIN_COLUMNS = 4;

/** 正文里有没有宽表格，有才显示“表格”选项。 */
export function hasWideTable(source: string): boolean {
  const walk = (blocks: Block[]): boolean =>
    blocks.some((b) => {
      if (b.type === "table") return b.header.length >= CARD_MIN_COLUMNS;
      if (b.type === "quote" || b.type === "container") return walk(b.blocks);
      if (b.type === "list") return b.items.some((item) => walk(item.blocks));
      return false;
    });
  return walk(parseMarkdown(source));
}

/** 排版宽度（CSS 像素，含四周空白）。 */
export const SHEET_WIDTH = 400;
/** 加宽表格时最宽多少。 */
export const WIDE_SHEET_WIDTH = 800;
/** 排版四周的空白，和 notes.css 里 .notes-img-sheet 的 padding 一样。 */
export const SHEET_PADDING = 24;

/** 加宽表格时的排版宽度：表格多宽就多宽，在 SHEET_WIDTH 和 WIDE_SHEET_WIDTH 之间。 */
export function wideSheetWidth(tableWidth: number): number {
  return Math.min(
    WIDE_SHEET_WIDTH,
    Math.max(SHEET_WIDTH, Math.ceil(tableWidth) + SHEET_PADDING * 2),
  );
}
/** 400 × 2.7 = 1080 像素宽。 */
export const MAX_SCALE = 2.7;
export const MIN_SCALE = 1.5;
/** iOS Safari 的 canvas 上限约 1677 万像素，留一点余量。 */
export const MAX_PIXELS = 16_000_000;

/** 按排版高度算导出倍数。降到 MIN_SCALE 还放不下时返回 null。 */
export function pickScale(height: number, width = SHEET_WIDTH): number | null {
  if (height <= 0) return MAX_SCALE;
  const fit = Math.sqrt(MAX_PIXELS / (width * height));
  if (fit >= MAX_SCALE) return MAX_SCALE;
  if (fit < MIN_SCALE) return null;
  return Math.floor(fit * 100) / 100;
}

/** 图片里的日期：本地时间的 2026-10-17。 */
export function exportDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 文件名：笔记标题-20261017.png。标题为空时用 fallback。 */
export function imageFileName(
  title: string,
  now: Date,
  fallback: string,
): string {
  const name =
    title
      .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, "-")
      .replace(/\s+/g, " ")
      .trim()
      .slice(0, 60) || fallback;
  const pad = (n: number) => String(n).padStart(2, "0");
  const day = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`;
  return `${name}-${day}.png`;
}

/** 编辑框里选中的 Markdown 原文。编辑框没有焦点或没选中时返回空字符串。 */
export function textareaSelection(el: HTMLTextAreaElement | null): string {
  if (!el || document.activeElement !== el) return "";
  if (el.selectionEnd <= el.selectionStart) return "";
  return el.value.slice(el.selectionStart, el.selectionEnd);
}

/**
 * 阅读模式下页面上选中的部分，换回 Markdown 原文。
 * 选区两头按文字找回原文的位置，再扩到整行，免得把 Markdown 的写法切断。
 * 找不到位置时退回选中的纯文字。没选中、选区不在 root 里时返回空字符串。
 */
export function previewSelection(
  body: string,
  root: HTMLElement | null,
): string {
  const sel = window.getSelection();
  if (!root || !sel || sel.isCollapsed || sel.rangeCount === 0) return "";
  const { anchorNode, focusNode } = sel;
  if (!anchorNode || !focusNode) return "";
  if (!root.contains(anchorNode) || !root.contains(focusNode)) return "";
  const text = sel.toString();
  if (!text.trim()) return "";
  const at = (node: Node, offset: number) =>
    node.nodeType === Node.TEXT_NODE
      ? caretFor(body, node.textContent ?? "", offset)
      : -1;
  const a = at(anchorNode, sel.anchorOffset);
  const b = at(focusNode, sel.focusOffset);
  if (a < 0 || b < 0 || (a === body.length && b === body.length)) return text;
  const start = body.lastIndexOf("\n", Math.min(a, b) - 1) + 1;
  const endAt = body.indexOf("\n", Math.max(a, b));
  const out = body.slice(start, endAt < 0 ? body.length : endAt);
  return out.trim() ? out : text;
}
