/* 资料问答页用的纯函数。 */

export const SOURCE_LABELS: Record<string, string> = {
  notes: "Ask source notes",
  readlater: "Ask source readlater",
  journal: "Ask source journal",
  cards: "Ask source cards",
};

/** 出处链接是网页地址还是应用内路径。 */
export function isExternal(link: string): boolean {
  return /^https?:\/\//i.test(link);
}

/** 相似度写成百分数。 */
export function scoreText(score: number): string {
  return `${Math.round(score * 100)}%`;
}

/** 片段太长时只显示开头。 */
export function clip(text: string, max = 160): string {
  const chars = Array.from(text.replace(/\s+/g, " ").trim());
  return chars.length <= max
    ? chars.join("")
    : `${chars.slice(0, max).join("")}…`;
}
