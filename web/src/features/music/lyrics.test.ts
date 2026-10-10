import { describe, expect, it } from "vitest";
import { activeLine, formatClock } from "./lyrics";

const lines = [
  { timeMs: 1000, text: "一" },
  { timeMs: 3000, text: "二" },
  { timeMs: 3000, text: "二（翻译）" },
  { timeMs: 8000, text: "三" },
];

describe("当前歌词行", () => {
  it("还没唱到第一行", () => {
    expect(activeLine(lines, 0)).toBe(-1);
    expect(activeLine(lines, 999)).toBe(-1);
  });
  it("取最后一个不晚于当前时间的行", () => {
    expect(activeLine(lines, 1000)).toBe(0);
    expect(activeLine(lines, 2999)).toBe(0);
    expect(activeLine(lines, 3000)).toBe(2);
    expect(activeLine(lines, 7999)).toBe(2);
    expect(activeLine(lines, 99999)).toBe(3);
  });
  it("没有时间轴或没有歌词", () => {
    expect(activeLine([{ text: "a" }, { text: "b" }], 5000)).toBe(-1);
    expect(activeLine([], 5000)).toBe(-1);
  });
});

describe("时间显示", () => {
  it("分秒和小时", () => {
    expect(formatClock(0)).toBe("0:00");
    expect(formatClock(65.9)).toBe("1:05");
    expect(formatClock(3600)).toBe("1:00:00");
    expect(formatClock(3725)).toBe("1:02:05");
  });
  it("不是数字或负数显示 0:00", () => {
    expect(formatClock(NaN)).toBe("0:00");
    expect(formatClock(-3)).toBe("0:00");
    expect(formatClock(Infinity)).toBe("0:00");
  });
});
