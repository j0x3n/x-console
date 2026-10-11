import { describe, expect, it } from "vitest";
import { formatMinutes, targetPercent, weekLabels } from "./format";

describe("skills format", () => {
  it("writes minutes as hours and minutes", () => {
    expect(formatMinutes(0)).toBe("0 分钟");
    expect(formatMinutes(45)).toBe("45 分钟");
    expect(formatMinutes(60)).toBe("1 小时");
    expect(formatMinutes(150)).toBe("2 小时 30 分钟");
  });

  it("caps the target percent at 100 and ignores a missing target", () => {
    expect(targetPercent(60, 120)).toBe(50);
    expect(targetPercent(200, 120)).toBe(100);
    expect(targetPercent(0, 120)).toBe(0);
    expect(targetPercent(60, undefined)).toBeNull();
    expect(targetPercent(60, 0)).toBeNull();
  });

  it("labels the weeks from the oldest to this week", () => {
    expect(weekLabels(3)).toEqual(["-2", "-1", "0"]);
  });
});
