/*
 * 一个够用的 Markdown 解析器：标题、段落、列表（含任务列表和嵌套）、引用、代码块、
 * 分隔线，以及行内的代码、粗体、斜体、删除线、链接、图片。输出结构化的节点，
 * 由 Markdown.tsx 渲染成 React 元素，不用 innerHTML，所以不会有 XSS。
 */

export type Inline =
  | { type: "text"; text: string }
  | { type: "code"; text: string }
  | { type: "strong" | "em" | "del"; children: Inline[] }
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
  | { type: "hr" };

const fenceRe = /^\s{0,3}(```|~~~)\s*([\w+-]*)\s*$/;
const headingRe = /^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const hrRe = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/;
const quoteRe = /^\s{0,3}>\s?(.*)$/;
const listRe = /^(\s{0,3})([-*+]|\d{1,9}[.)])\s+(.*)$/;
const taskRe = /^\[([ xX])\]\s+(.*)$/;

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
      !listRe.test(lines[i])
    ) {
      para.push(lines[i].trim());
      i++;
    }
    blocks.push({ type: "paragraph", children: joinLines(para) });
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
  /`([^`]+)`|\*\*(.+?)\*\*|__(.+?)__|~~(.+?)~~|\*([^*\s][^*]*?)\*|(?<![\w])_([^_\s][^_]*?)_(?![\w])|(!)?\[([^\]]*)\]\(([^)\s]*)(?:\s+"[^"]*")?\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/g;

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
  }
  if (last < text.length) push({ type: "text", text: text.slice(last) });
  return out;
}

const taskLineRe = /^(\s*(?:>\s*)*(?:[-*+]|\d{1,9}[.)])\s+\[)([ xX])(\])/;

/** 按预览里的顺序列出每个待办的勾选状态，和 Markdown.tsx 的编号一致。 */
function taskStates(blocks: Block[], out: boolean[] = []): boolean[] {
  for (const b of blocks) {
    if (b.type === "quote") taskStates(b.blocks, out);
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
