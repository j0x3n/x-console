/*
 * 一个够用的 Markdown 解析器：标题、段落、列表（含任务列表和嵌套）、引用、代码块、
 * 分隔线，以及行内的代码、粗体、斜体、删除线、链接、图片。输出结构化的节点，
 * 由 Markdown.tsx 渲染成 React 元素，不用 innerHTML，所以不会有 XSS。
 * B74 加了：表格、提示块（:::note 等）、隐藏块（:::hidden）、高亮（==文字==）、
 * 单独一行的网址显示成链接卡片。视频、音频用图片的写法，渲染时按后缀区分。
 */

export type Inline =
  | { type: "text"; text: string }
  | { type: "code"; text: string }
  | { type: "strong" | "em" | "del" | "mark"; children: Inline[] }
  | { type: "link"; href: string; children: Inline[] }
  | { type: "image"; src: string; alt: string }
  | { type: "br" };

export interface ListItem {
  checked: boolean | null;
  blocks: Block[];
}

export type Block =
  | { type: "heading"; level: number; children: Inline[] }
  | { type: "paragraph"; children: Inline[] }
  | { type: "code"; lang: string; text: string }
  | { type: "quote"; blocks: Block[] }
  | { type: "list"; ordered: boolean; start: number; items: ListItem[] }
  | { type: "hr" }
  | {
      type: "table";
      align: ("left" | "center" | "right" | null)[];
      header: Inline[][];
      rows: Inline[][][];
    }
  | { type: "container"; kind: ContainerKind; title: string; blocks: Block[] }
  | { type: "linkcard"; href: string };

export type ContainerKind = "hidden" | "note" | "tip" | "warn" | "danger";

const fenceRe = /^\s{0,3}(```|~~~)\s*([\w+-]*)\s*$/;
const headingRe = /^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const hrRe = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/;
const quoteRe = /^\s{0,3}>\s?(.*)$/;
const listRe = /^(\s{0,3})([-*+]|\d{1,9}[.)])\s+(.*)$/;
const taskRe = /^\[([ xX])\]\s+(.*)$/;
const containerRe = /^\s{0,3}:::\s*(hidden|note|tip|warn|danger)\b\s*(.*)$/;
const containerEndRe = /^\s{0,3}:::\s*$/;
const tableSepRe = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/;
const bareUrlRe = /^https?:\/\/[^\s<>]+$/;

/** 表格的一行切成单元格。行首行尾的 | 可以省略，\| 是字面的竖线。 */
export function splitRow(line: string): string[] {
  let row = line.trim();
  if (row.startsWith("|")) row = row.slice(1);
  if (row.endsWith("|") && !row.endsWith("\\|")) row = row.slice(0, -1);
  const cells: string[] = [];
  let cur = "";
  for (let i = 0; i < row.length; i++) {
    if (row[i] === "\\" && row[i + 1] === "|") {
      cur += "|";
      i++;
    } else if (row[i] === "|") {
      cells.push(cur.trim());
      cur = "";
    } else cur += row[i];
  }
  cells.push(cur.trim());
  return cells;
}

function isTableStart(lines: string[], i: number): boolean {
  return (
    lines[i].includes("|") &&
    i + 1 < lines.length &&
    tableSepRe.test(lines[i + 1]) &&
    lines[i + 1].includes("-")
  );
}

export function parseMarkdown(source: string): Block[] {
  return parseBlocks(source.replace(/\r\n?/g, "\n").split("\n"));
}

function parseBlocks(lines: string[]): Block[] {
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === "") {
      i++;
      continue;
    }
    const fence = fenceRe.exec(line);
    if (fence) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trim().startsWith(fence[1])) {
        body.push(lines[i]);
        i++;
      }
      i++; // closing fence
      blocks.push({ type: "code", lang: fence[2], text: body.join("\n") });
      continue;
    }
    const container = containerRe.exec(line);
    if (container) {
      // 找配对的 :::，里面可以再嵌套提示块和代码块
      const body: string[] = [];
      let depth = 0;
      let inFence: string | null = null;
      i++;
      while (i < lines.length) {
        const l = lines[i];
        const f = fenceRe.exec(l);
        if (inFence) {
          if (l.trim().startsWith(inFence)) inFence = null;
        } else if (f) inFence = f[1];
        else if (containerRe.test(l)) depth++;
        else if (containerEndRe.test(l)) {
          if (depth === 0) break;
          depth--;
        }
        body.push(l);
        i++;
      }
      i++; // closing :::
      blocks.push({
        type: "container",
        kind: container[1] as ContainerKind,
        title: container[2].trim(),
        blocks: parseBlocks(body),
      });
      continue;
    }
    if (isTableStart(lines, i)) {
      const header = splitRow(line);
      const align = splitRow(lines[i + 1]).map((c) =>
        c.startsWith(":") && c.endsWith(":")
          ? ("center" as const)
          : c.endsWith(":")
            ? ("right" as const)
            : c.startsWith(":")
              ? ("left" as const)
              : null,
      );
      i += 2;
      const rows: Inline[][][] = [];
      while (
        i < lines.length &&
        lines[i].trim() !== "" &&
        lines[i].includes("|")
      ) {
        const cells = splitRow(lines[i]);
        rows.push(header.map((_, k) => parseInline(cells[k] ?? "")));
        i++;
      }
      blocks.push({
        type: "table",
        align: header.map((_, k) => align[k] ?? null),
        header: header.map((c) => parseInline(c)),
        rows,
      });
      continue;
    }
    const heading = headingRe.exec(line);
    if (heading) {
      blocks.push({
        type: "heading",
        level: heading[1].length,
        children: parseInline(heading[2]),
      });
      i++;
      continue;
    }
    if (hrRe.test(line)) {
      blocks.push({ type: "hr" });
      i++;
      continue;
    }
    if (quoteRe.test(line)) {
      const body: string[] = [];
      while (i < lines.length && lines[i].trim() !== "") {
        const m = quoteRe.exec(lines[i]);
        body.push(m ? m[1] : lines[i]);
        i++;
      }
      blocks.push({ type: "quote", blocks: parseBlocks(body) });
      continue;
    }
    const item = listRe.exec(line);
    if (item) {
      const [list, next] = parseList(lines, i);
      blocks.push(list);
      i = next;
      continue;
    }
    const para: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !fenceRe.test(lines[i]) &&
      !headingRe.test(lines[i]) &&
      !hrRe.test(lines[i]) &&
      !quoteRe.test(lines[i]) &&
      !listRe.test(lines[i]) &&
      !containerRe.test(lines[i]) &&
      !isTableStart(lines, i)
    ) {
      para.push(lines[i].trim());
      i++;
    }
    // 一段里只有一个裸网址：显示成链接卡片（不去抓网页标题）
    if (para.length === 1 && bareUrlRe.test(para[0]))
      blocks.push({ type: "linkcard", href: para[0] });
    else blocks.push({ type: "paragraph", children: joinLines(para) });
  }
  return blocks;
}

function parseList(lines: string[], start: number): [Block, number] {
  const first = listRe.exec(lines[start])!;
  const ordered = /\d/.test(first[2]);
  const indent = first[1].length;
  const items: ListItem[] = [];
  let i = start;
  while (i < lines.length) {
    const m = listRe.exec(lines[i]);
    if (!m || m[1].length !== indent || /\d/.test(m[2]) !== ordered) break;
    const content = [m[3]];
    const childIndent = indent + m[2].length + 1;
    i++;
    // Continuation: indented lines, or blank lines followed by indented ones.
    while (i < lines.length) {
      const line = lines[i];
      const lead = line.length - line.trimStart().length;
      if (line.trim() === "") {
        const nextLine = lines[i + 1];
        if (
          nextLine !== undefined &&
          nextLine.trim() !== "" &&
          nextLine.length - nextLine.trimStart().length >= childIndent
        ) {
          content.push("");
          i++;
          continue;
        }
        break;
      }
      if (lead >= Math.min(childIndent, indent + 2)) {
        content.push(line.slice(Math.min(lead, childIndent)));
        i++;
        continue;
      }
      break;
    }
    const task = taskRe.exec(content[0]);
    let checked: boolean | null = null;
    if (task) {
      checked = task[1] !== " ";
      content[0] = task[2];
    }
    items.push({ checked, blocks: parseBlocks(content) });
    // A blank line between items of the same list is allowed.
    if (
      lines[i]?.trim() === "" &&
      lines[i + 1] !== undefined &&
      listRe.exec(lines[i + 1])?.[1].length === indent
    )
      i++;
  }
  return [
    {
      type: "list",
      ordered,
      start: ordered ? parseInt(first[2], 10) : 1,
      items,
    },
    i,
  ];
}

function joinLines(lines: string[]): Inline[] {
  const out: Inline[] = [];
  lines.forEach((line, index) => {
    if (index > 0) out.push({ type: "br" });
    out.push(...parseInline(line));
  });
  return out;
}

/** 图片地址只允许 http、https 和站内路径。 */
export function safeSrc(src: string): string | null {
  const url = src.trim();
  if (/^https?:/i.test(url)) return url;
  if (url.startsWith("/") && !url.startsWith("//")) return url;
  return null;
}

/** 只允许 http、https、mailto 和站内路径。 */
export function safeHref(href: string): string | null {
  const url = href.trim();
  if (/^(https?:|mailto:)/i.test(url)) return url;
  if (url.startsWith("/") && !url.startsWith("//")) return url;
  if (url.startsWith("#")) return url;
  return null;
}

const inlineRe =
  /`([^`]+)`|\*\*(.+?)\*\*|__(.+?)__|~~(.+?)~~|\*([^*\s][^*]*?)\*|(?<![\w])_([^_\s][^_]*?)_(?![\w])|(!)?\[([^\]]*)\]\(([^)\s]*)(?:\s+"[^"]*")?\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])|==([^=\s](?:[^=]*?[^=\s])?)==/g;

export function parseInline(text: string): Inline[] {
  const out: Inline[] = [];
  let last = 0;
  const push = (node: Inline) => {
    const prev = out[out.length - 1];
    if (node.type === "text" && prev?.type === "text") prev.text += node.text;
    else out.push(node);
  };
  for (const m of text.matchAll(inlineRe)) {
    const index = m.index ?? 0;
    if (index > last) push({ type: "text", text: text.slice(last, index) });
    last = index + m[0].length;
    if (m[1] !== undefined) push({ type: "code", text: m[1] });
    else if (m[2] !== undefined || m[3] !== undefined)
      push({ type: "strong", children: parseInline(m[2] ?? m[3]) });
    else if (m[4] !== undefined)
      push({ type: "del", children: parseInline(m[4]) });
    else if (m[5] !== undefined || m[6] !== undefined)
      push({ type: "em", children: parseInline(m[5] ?? m[6]) });
    else if (m[9] !== undefined && m[7] === "!") {
      const src = safeSrc(m[9]);
      if (src) push({ type: "image", src, alt: m[8] });
      else push({ type: "text", text: m[8] });
    } else if (m[9] !== undefined) {
      const href = safeHref(m[9]);
      const label = m[8] || m[9];
      if (href) push({ type: "link", href, children: parseInline(label) });
      else push({ type: "text", text: label });
    } else if (m[10] !== undefined)
      push({
        type: "link",
        href: m[10],
        children: [{ type: "text", text: m[10] }],
      });
    else if (m[11] !== undefined)
      push({ type: "mark", children: parseInline(m[11]) });
  }
  if (last < text.length) push({ type: "text", text: text.slice(last) });
  return out;
}

const taskLineRe = /^(\s*(?:>\s*)*(?:[-*+]|\d{1,9}[.)])\s+\[)([ xX])(\])/;

/** 按预览里的顺序列出每个待办的勾选状态，和 Markdown.tsx 的编号一致。 */
function taskStates(blocks: Block[], out: boolean[] = []): boolean[] {
  for (const b of blocks) {
    if (b.type === "quote" || b.type === "container") taskStates(b.blocks, out);
    if (b.type !== "list") continue;
    for (const item of b.items) {
      if (item.checked !== null) out.push(item.checked);
      taskStates(item.blocks, out);
    }
  }
  return out;
}

/**
 * 切换源码里第 index 个待办（从 0 开始，按预览里的顺序）。
 * 预览里点勾选框时用它改正文。
 * 像待办的行不一定被解析成待办（比如代码块里的、没有文字的 `- [ ]`），
 * 所以逐行试着切换，重新解析后只有第 index 个待办变了才算找对了行。
 */
export function toggleTask(source: string, index: number): string {
  const before = taskStates(parseMarkdown(source));
  if (index < 0 || index >= before.length) return source;
  const lines = source.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const m = taskLineRe.exec(lines[i]);
    if (!m) continue;
    const original = lines[i];
    lines[i] =
      m[1] + (m[2] === " " ? "x" : " ") + m[3] + original.slice(m[0].length);
    const next = lines.join("\n");
    const after = taskStates(parseMarkdown(next));
    if (
      after.length === before.length &&
      after.every((c, k) => (k === index ? c !== before[k] : c === before[k]))
    )
      return next;
    lines[i] = original;
  }
  return source;
}

/** 正文里的图片地址，按出现顺序。 */
export function imageSources(source: string): string[] {
  const out: string[] = [];
  for (const m of source.matchAll(/!\[[^\]]*\]\(([^)\s]+)[^)]*\)/g)) {
    const src = safeSrc(m[1]);
    if (src) out.push(src);
  }
  return out;
}

/** 按后缀判断图片写法里的视频、音频（B74）。地址没后缀时看名字。 */
export function mediaKind(src: string, alt = ""): "video" | "audio" | null {
  const ext = (s: string) =>
    s
      .split(/[?#]/)[0]
      .match(/\.([a-z0-9]+)$/i)?.[1]
      ?.toLowerCase() ?? "";
  for (const e of [ext(src), ext(alt)]) {
    if (["mp4", "webm", "mov", "m4v"].includes(e)) return "video";
    if (["mp3", "m4a", "ogg", "wav", "flac", "aac"].includes(e)) return "audio";
  }
  return null;
}
