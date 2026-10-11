import { describe, expect, it } from "vitest";
import type { CardQueueItem } from "./api";
import { BLANK, makeBlank } from "./cloze";
import {
  currentItem,
  durationText,
  intervalText,
  isFinished,
  keyAction,
  rate,
  reveal,
  sessionStats,
  skip,
  startSession,
} from "./session";

const item = (id: number): CardQueueItem => ({
  card: {
    id,
    deck: "默认",
    question: `问题 ${id}`,
    answer: `答案 ${id}`,
    sourceTitle: "",
    sourceLink: "",
    suspended: false,
    state: "new",
    due: "2026-10-12T00:00:00Z",
    stability: 0,
    difficulty: 0,
    reps: 0,
    lapses: 0,
    createdAt: "2026-10-12T00:00:00Z",
    updatedAt: "2026-10-12T00:00:00Z",
  },
  intervals: { again: 1, hard: 2, good: 3, easy: 8 },
});

const t = (s: string) =>
  ({ days: "天", months: "个月", years: "年", minutes: "分钟", seconds: "秒" })[
    s
  ] ?? s;

describe("review session", () => {
  it("walks through the cards: reveal, rate, next", () => {
    let s = startSession([item(1), item(2)], 1000);
    expect(currentItem(s)?.card.id).toBe(1);
    expect(s.revealed).toBe(false);
    // Rating before the answer is shown does nothing.
    expect(rate(s, 3, 2000)).toBe(s);
    s = reveal(s);
    expect(s.revealed).toBe(true);
    s = rate(s, 3, 2000);
    expect(currentItem(s)?.card.id).toBe(2);
    expect(s.revealed).toBe(false);
    expect(s.counts).toEqual([0, 0, 1, 0]);
    s = rate(reveal(s), 1, 5000);
    expect(isFinished(s)).toBe(true);
    expect(s.finishedAt).toBe(5000);
    expect(currentItem(s)).toBeNull();
    expect(sessionStats(s)).toEqual({
      total: 2,
      counts: [1, 0, 1, 0],
      seconds: 4,
      passed: 0.5,
    });
    // A finished session ignores everything.
    expect(rate(s, 4, 6000)).toBe(s);
    expect(reveal(s)).toBe(s);
  });

  it("starts finished when the queue is empty", () => {
    const s = startSession([], 10);
    expect(isFinished(s)).toBe(true);
    expect(sessionStats(s).total).toBe(0);
    expect(sessionStats(s).passed).toBe(0);
  });

  it("skips a card without counting it", () => {
    let s = startSession([item(1), item(2)], 0);
    s = skip(s, 100);
    expect(currentItem(s)?.card.id).toBe(2);
    expect(s.counts).toEqual([0, 0, 0, 0]);
    s = skip(s, 200);
    expect(isFinished(s)).toBe(true);
    expect(s.finishedAt).toBe(200);
  });

  it("maps keys: space reveals then means Good, 1 to 4 rate after the answer", () => {
    expect(keyAction(" ", false)).toEqual({ type: "reveal" });
    expect(keyAction(" ", true)).toEqual({ type: "rate", rating: 3 });
    expect(keyAction("1", false)).toBeNull();
    for (const k of ["1", "2", "3", "4"])
      expect(keyAction(k, true)).toEqual({ type: "rate", rating: Number(k) });
    expect(keyAction("5", true)).toBeNull();
    expect(keyAction("a", true)).toBeNull();
    expect(keyAction("Enter", false)).toBeNull();
  });

  it("writes intervals and durations", () => {
    expect(intervalText(1, t)).toBe("1 天");
    expect(intervalText(29, t)).toBe("29 天");
    expect(intervalText(30, t)).toBe("1 个月");
    expect(intervalText(90, t)).toBe("3 个月");
    expect(intervalText(365, t)).toBe("1 年");
    expect(intervalText(730, t)).toBe("2 年");
    expect(durationText(45, t)).toBe("45 秒");
    expect(durationText(200, t)).toBe("3 分钟 20 秒");
  });
});

describe("fill in the blank", () => {
  it("replaces the selection and keeps it as the answer", () => {
    expect(makeBlank("WAL 把写入追加到日志文件", 10, 14)).toEqual({
      question: `WAL 把写入追加到${BLANK}`,
      answer: "日志文件",
    });
  });

  it("keeps the spaces around the selection", () => {
    expect(makeBlank("use the WAL mode", 7, 12)).toEqual({
      question: `use the ${BLANK} mode`,
      answer: "WAL",
    });
  });

  it("works with a backwards selection and ignores blanks", () => {
    expect(makeBlank("abcdef", 4, 2)?.answer).toBe("cd");
    expect(makeBlank("abc   def", 3, 6)).toBeNull();
    expect(makeBlank("abc", 2, 2)).toBeNull();
    expect(makeBlank("abc", 1, 99)?.answer).toBe("bc");
  });
});
