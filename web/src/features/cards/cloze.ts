/** 填空卡里被挖掉的位置。 */
export const BLANK = "____";

/**
 * 把问题里选中的 [start, end) 换成空格，返回新的问题和被挖掉的字。
 * 选中的是空白或超出范围时返回 null。
 */
export function makeBlank(
  question: string,
  start: number,
  end: number,
): { question: string; answer: string } | null {
  const from = Math.max(0, Math.min(start, end));
  const to = Math.min(question.length, Math.max(start, end));
  const picked = question.slice(from, to);
  const answer = picked.trim();
  if (!answer) return null;
  // 选中时带进来的前后空白留在空格外面
  const lead = picked.length - picked.trimStart().length;
  const tail = picked.length - picked.trimEnd().length;
  return {
    question:
      question.slice(0, from + lead) + BLANK + question.slice(to - tail),
    answer,
  };
}

/** 选中的文字太短不出按钮。 */
export const MIN_SELECTION = 6;
/** 发给 AI 和放进对话框的最长字数，和服务端一致。 */
export const MAX_SELECTION = 4000;
