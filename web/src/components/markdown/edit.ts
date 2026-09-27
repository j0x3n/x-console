/*
 * Markdown 编辑框的文字操作：加粗、列表、插入代码块等。纯函数，不依赖 React。
 * 笔记编辑器和公共的 MarkdownEditor 都用它。
 */

export interface Edit {
  text: string;
  start: number;
  end: number;
}

/**
 * 在选中文字两边加标记，比如粗体 **。没有选中时插入占位文字并选中它。
 * 已经包着同样的标记时去掉标记。
 */
export function wrapSelection(
  text: string,
  start: number,
  end: number,
  before: string,
  after = before,
  placeholder = "",
): Edit {
  const selected = text.slice(start, end);
  const outerStart = start - before.length;
  if (
    outerStart >= 0 &&
    text.slice(outerStart, start) === before &&
    text.slice(end, end + after.length) === after
  ) {
    return {
      text:
        text.slice(0, outerStart) + selected + text.slice(end + after.length),
      start: outerStart,
      end: outerStart + selected.length,
    };
  }
  const inner = selected || placeholder;
  return {
    text: text.slice(0, start) + before + inner + after + text.slice(end),
    start: start + before.length,
    end: start + before.length + inner.length,
  };
}

/**
 * 给选中的每一行加前缀（比如 "- "、"> "、"1. "）。所有行都已经有这个前缀时去掉。
 * ordered 为 true 时按 1. 2. 3. 编号。
 */
export function prefixLines(
  text: string,
  start: number,
  end: number,
  prefix: string,
  ordered = false,
): Edit {
  const lineStart = text.lastIndexOf("\n", start - 1) + 1;
  let lineEnd = text.indexOf("\n", end);
  if (lineEnd === -1) lineEnd = text.length;
  const lines = text.slice(lineStart, lineEnd).split("\n");
  const re = ordered
    ? /^\d+\.\s/
    : new RegExp(`^${prefix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`);
  const all = lines.every((l) => re.test(l));
  const next = lines.map((l, i) => {
    if (all) return l.replace(re, "");
    const bare = l.replace(/^(\d+\.\s|[-*+]\s(\[[ xX]\]\s)?|>\s|#{1,6}\s)/, "");
    return (ordered ? `${i + 1}. ` : prefix) + bare;
  });
  const replaced = next.join("\n");
  return {
    text: text.slice(0, lineStart) + replaced + text.slice(lineEnd),
    start: lineStart,
    end: lineStart + replaced.length,
  };
}

/** 在光标处插入一段文字，前后需要时补空行（用于图片、代码块、分隔线）。 */
export function insertBlock(
  text: string,
  start: number,
  end: number,
  block: string,
): Edit {
  const before = text.slice(0, start);
  const after = text.slice(end);
  const lead =
    before === "" || before.endsWith("\n\n")
      ? ""
      : before.endsWith("\n")
        ? "\n"
        : "\n\n";
  const tail =
    after === "" || after.startsWith("\n\n")
      ? ""
      : after.startsWith("\n")
        ? "\n"
        : "\n\n";
  const inserted = lead + block + tail;
  const pos = start + lead.length + block.length;
  return { text: before + inserted + after, start: pos, end: pos };
}

/** 删掉 insertBlock 插进去的一块，连同它前后多出来的空行。 */
export function removeBlock(text: string, block: string): string {
  const i = text.indexOf(block);
  if (i < 0) return text;
  let before = text.slice(0, i);
  let after = text.slice(i + block.length);
  if (before.trim() === "") before = "";
  if (before === "") after = after.replace(/^\n+/, "");
  else if (after.trim() === "") after = "";
  if (after === "") before = before.replace(/\n+$/, "");
  else if (before.endsWith("\n\n") && after.startsWith("\n"))
    after = after.replace(/^\n+/, "");
  return before + after;
}

/* ---- 列表分组 ---- */
