/* 笔记模块的纯函数：搜索片段、标题、标签输入。 */

export interface SnippetPart {
  text: string;
  hit: boolean;
}

/** 服务端用 U+E000 和 U+E001 包住命中的文字，这里拆成片段。 */
export function snippetParts(snippet: string): SnippetPart[] {
  const parts: SnippetPart[] = [];
  let rest = snippet;
  while (rest) {
    const open = rest.indexOf("");
    if (open === -1) {
      parts.push({ text: rest, hit: false });
      break;
    }
    if (open > 0) parts.push({ text: rest.slice(0, open), hit: false });
    const close = rest.indexOf("", open + 1);
    const end = close === -1 ? rest.length : close;
    parts.push({ text: rest.slice(open + 1, end), hit: true });
    rest = close === -1 ? "" : rest.slice(close + 1);
  }
  return parts.filter((p) => p.text !== "");
}

/** 列表里显示的标题：有标题用标题，没有就用正文第一行。 */
export function noteTitle(title: string, excerptOrBody: string): string {
  if (title.trim()) return title.trim();
  const first = excerptOrBody
    .split("\n")
    .map((line) => line.replace(/^[#>\-*+\s]+/, "").trim())
    .find(Boolean);
  return first ? first.slice(0, 60) : "";
}

/** 把输入的标签文字拆开："#a, b  c" → ["a", "b", "c"]，去重。 */
export function parseTags(input: string): string[] {
  const out: string[] = [];
  for (const raw of input.split(/[,，\s]+/)) {
    const tag = raw.replace(/^#+/, "").trim();
    if (tag && !out.includes(tag)) out.push(tag);
  }
  return out;
}

/** 两组标签是否一样（不看顺序）。 */
export function sameTags(a: string[], b: string[]) {
  if (a.length !== b.length) return false;
  const set = new Set(a);
  return b.every((t) => set.has(t));
}

/** datetime-local 输入框用的本地时间，默认明天早上 9 点。 */
export function defaultReminderTime(now = new Date()): string {
  const d = new Date(now);
  d.setDate(d.getDate() + 1);
  d.setHours(9, 0, 0, 0);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/* ---- 编辑器工具条 ---- */

export {
  insertBlock,
  prefixLines,
  removeBlock,
  wrapSelection,
  type Edit,
} from "../../components/markdown/edit";

export type DateGroup = "today" | "yesterday" | "week" | "month" | "earlier";

export const DATE_GROUP_LABELS: Record<DateGroup, string> = {
  today: "Today",
  yesterday: "Yesterday",
  week: "Earlier this week",
  month: "Earlier this month",
  earlier: "Earlier",
};

/** 按更新时间分组。一周从周一算。 */
export function dateGroup(value: string | Date, now = new Date()): DateGroup {
  const d = typeof value === "string" ? new Date(value) : value;
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((day(now) - day(d)) / 86_400_000);
  if (diff <= 0) return "today";
  if (diff === 1) return "yesterday";
  const weekday = (now.getDay() + 6) % 7; // 周一是 0
  if (diff <= weekday) return "week";
  if (d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth()) return "month";
  return "earlier";
}

/** 字数：中文按字算，英文按词算。 */
export function countWords(text: string): number {
  const cjk = text.match(/[㐀-鿿豈-﫿]/g)?.length ?? 0;
  const words = text.replace(/[㐀-鿿豈-﫿]/g, " ").match(/[A-Za-z0-9_'-]+/g)?.length ?? 0;
  return cjk + words;
}

/** 上传中的占位文字，完成后换成真正的图片或链接。 */
export function uploadPlaceholder(name: string, id: string): string {
  return `![上传中 ${name.replace(/[[\]]/g, "")} ${id}…]()`;
}

/** 上传完成后插入的 Markdown：图片用 ![]()，其他文件用 []()。 */
export function attachmentMarkdown(a: { name: string; mime: string; url: string }): string {
  const name = a.name.replace(/[[\]]/g, "");
  return a.mime.startsWith("image/") && a.mime !== "image/svg+xml"
    ? `![${name}](${a.url})`
    : `[${name}](${a.url})`;
}
