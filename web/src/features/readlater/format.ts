import type { ReadItem } from "./api";

const LINK = /https?:\/\/[^\s<>"'`，。！？；：、（）【】《》「」『』“”‘’]+/;

/** 分享进来的内容里找出第一个网址。网址可能在 url 参数里，也可能夹在文字里。 */
export function linkFromShare(params: URLSearchParams): string | null {
  for (const key of ["url", "text", "title"]) {
    const found = LINK.exec(params.get(key) ?? "");
    if (found) return found[0].replace(/[.,;:!?)\]}>]+$/, "");
  }
  return null;
}

/** 来源的英文键，中文在 i18n.ts。 */
export const SOURCE_LABELS: Record<ReadItem["source"], string> = {
  web: "Added here",
  share: "Shared from phone",
  telegram: "Telegram",
  ai: "AI assistant",
};

/** 标签输入框里的文字拆成标签：逗号、顿号、换行都算分隔。 */
export function parseTags(text: string): string[] {
  return text
    .split(/[,，、\n]/)
    .map((s) => s.trim().replace(/^#/, ""))
    .filter(Boolean);
}
