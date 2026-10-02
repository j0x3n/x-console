import type { QuoteMode } from "../../stores/preferences-store";

/*
 * B89：今日页问候语后面的每日一句。来源是带“名言”标签的便签。纯函数。
 */

export const QUOTE_TAG = "名言";

export interface QuoteSource {
  id: number;
  pinned: boolean;
  title: string;
  excerpt: string;
}

/** 便签的文字：正文摘要优先，没有时用标题。 */
export function quoteText(q: QuoteSource): string {
  return (q.excerpt || q.title).trim();
}

/** 同一天得到同一个数。 */
function dayHash(day: string): number {
  let h = 0;
  for (const ch of day) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return h;
}

/**
 * 按设置选一条。list 是接口的顺序：置顶的在前，然后按更新时间新的在前。
 * - fixed：置顶的第一条，没有置顶的就是最新的一条
 * - daily：按日期选，同一天都一样
 * - refresh：用传进来的随机数（每次打开页面取一次）
 */
export function pickQuote<T extends QuoteSource>(
  list: T[],
  mode: QuoteMode,
  day: string,
  random: number,
): T | null {
  const usable = list.filter((q) => quoteText(q));
  if (mode === "off" || usable.length === 0) return null;
  if (mode === "fixed") return usable.find((q) => q.pinned) ?? usable[0];
  const i =
    mode === "daily"
      ? dayHash(day) % usable.length
      : Math.floor(random * usable.length) % usable.length;
  return usable[i];
}
