import type { CardQueueItem } from "./api";

/*
 * 一次复习的状态。纯逻辑，不依赖 React：页面把按键和按钮都转成这里的动作。
 * 评分在服务端记，这里只管走到哪一张、显示不显示答案和统计。
 */

export type Rating = 1 | 2 | 3 | 4;

export interface Session {
  items: CardQueueItem[];
  /** 当前是第几张（从 0 开始）。等于 items.length 说明评完了 */
  index: number;
  revealed: boolean;
  /** 重来、困难、良好、简单各评了几次 */
  counts: [number, number, number, number];
  startedAt: number;
  finishedAt: number | null;
}

export function startSession(items: CardQueueItem[], now: number): Session {
  return {
    items,
    index: 0,
    revealed: false,
    counts: [0, 0, 0, 0],
    startedAt: now,
    finishedAt: items.length === 0 ? now : null,
  };
}

export function currentItem(s: Session): CardQueueItem | null {
  return s.items[s.index] ?? null;
}

export function isFinished(s: Session): boolean {
  return s.index >= s.items.length;
}

export function reveal(s: Session): Session {
  return isFinished(s) || s.revealed ? s : { ...s, revealed: true };
}

/** 评一次分，换到下一张。没显示答案时不能评分。 */
export function rate(s: Session, rating: Rating, now: number): Session {
  if (isFinished(s) || !s.revealed) return s;
  const counts = [...s.counts] as Session["counts"];
  counts[rating - 1] += 1;
  const index = s.index + 1;
  return {
    ...s,
    index,
    revealed: false,
    counts,
    finishedAt: index >= s.items.length ? now : null,
  };
}

/** 跳过当前这张，不计入统计。这张卡在别处已经评过时用。 */
export function skip(s: Session, now: number): Session {
  if (isFinished(s)) return s;
  const index = s.index + 1;
  return {
    ...s,
    index,
    revealed: false,
    finishedAt: index >= s.items.length ? now : null,
  };
}

export type KeyAction = { type: "reveal" } | { type: "rate"; rating: Rating };

/**
 * 按键对应的动作。空格：没显示答案时显示答案，显示后等于“良好”。
 * 1 到 4：重来、困难、良好、简单，只在显示答案后有用。
 */
export function keyAction(key: string, revealed: boolean): KeyAction | null {
  if (key === " ")
    return revealed ? { type: "rate", rating: 3 } : { type: "reveal" };
  if (revealed && (key === "1" || key === "2" || key === "3" || key === "4"))
    return { type: "rate", rating: Number(key) as Rating };
  return null;
}

export function sessionStats(s: Session) {
  const total = s.counts.reduce((a, b) => a + b, 0);
  const end = s.finishedAt ?? s.startedAt;
  return {
    total,
    counts: s.counts,
    seconds: Math.max(0, Math.round((end - s.startedAt) / 1000)),
    /** 评了“困难”以上的占比，0 到 1 */
    passed: total === 0 ? 0 : (total - s.counts[0]) / total,
  };
}

/** 下次复习在几天以后，写成“3 天”“2 个月”“1 年”。 */
export function intervalText(
  days: number,
  t: (text: string) => string,
): string {
  if (days < 30) return `${days} ${t("days")}`;
  if (days < 365) return `${Math.round(days / 30)} ${t("months")}`;
  return `${Math.round((days / 365) * 10) / 10} ${t("years")}`;
}

/** 用时写成“3 分 20 秒”。 */
export function durationText(
  seconds: number,
  t: (text: string) => string,
): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m === 0) return `${s} ${t("seconds")}`;
  return `${m} ${t("minutes")} ${s} ${t("seconds")}`;
}
