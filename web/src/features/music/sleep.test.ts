import { describe, expect, it } from "vitest";
import {
  afterTrackEnded,
  fadeFactor,
  formatRemaining,
  remainingMs,
} from "./sleep";

describe("定时暂停", () => {
  it("剩余时间不会是负数", () => {
    const now = Date.parse("2026-10-11T10:00:00Z");
    expect(remainingMs("2026-10-11T10:30:00Z", now)).toBe(30 * 60 * 1000);
    expect(remainingMs("2026-10-11T09:00:00Z", now)).toBe(0);
    expect(remainingMs(undefined, now)).toBe(0);
    expect(remainingMs("坏的", now)).toBe(0);
  });
  it("最后 10 秒音量渐小，之前不变", () => {
    expect(fadeFactor(60_000)).toBe(1);
    expect(fadeFactor(10_000)).toBe(1);
    expect(fadeFactor(5_000)).toBe(0.5);
    expect(fadeFactor(0)).toBe(0);
    expect(fadeFactor(-5)).toBe(0);
  });
  it("剩余时间的写法", () => {
    expect(formatRemaining(0)).toBe("00:00");
    expect(formatRemaining(61_000)).toBe("01:01");
    expect(formatRemaining(59_100)).toBe("01:00");
    expect(formatRemaining(3_725_000)).toBe("1:02:05");
  });
  it("按首数：最后一首播完停下，否则减一继续", () => {
    expect(afterTrackEnded("tracks", 1)).toEqual({ action: "stop" });
    expect(afterTrackEnded("tracks", 3)).toEqual({
      action: "continue",
      left: 2,
    });
    expect(afterTrackEnded("time", undefined)).toEqual({ action: "ignore" });
    expect(afterTrackEnded(undefined, undefined)).toEqual({ action: "ignore" });
    expect(afterTrackEnded("tracks", 0)).toEqual({ action: "ignore" });
  });
});
