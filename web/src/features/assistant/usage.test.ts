import { describe, expect, it } from "vitest";
import { change, formatChange, formatRate, niceMax, rangeDates } from "./usage";

const now = new Date(2026, 2, 15, 10, 0); // 2026-03-15

describe("rangeDates", () => {
  it("covers today, 7 and 30 days including today", () => {
    expect(rangeDates("today", now)).toEqual({
      from: "2026-03-15",
      to: "2026-03-15",
    });
    expect(rangeDates("7d", now)).toEqual({
      from: "2026-03-09",
      to: "2026-03-15",
    });
    expect(rangeDates("30d", now)).toEqual({
      from: "2026-02-14",
      to: "2026-03-15",
    });
  });
  it("handles this and last month across a year", () => {
    expect(rangeDates("month", now)).toEqual({
      from: "2026-03-01",
      to: "2026-03-15",
    });
    expect(rangeDates("lastMonth", new Date(2026, 0, 5))).toEqual({
      from: "2025-12-01",
      to: "2025-12-31",
    });
  });
});

describe("numbers", () => {
  it("formats change and rate", () => {
    expect(formatChange(change(120, 100))).toBe("↑20%");
    expect(formatChange(change(50, 100))).toBe("↓50%");
    expect(formatChange(change(5, 0))).toBe("");
    expect(formatRate(0.8123)).toBe("81.2%");
    expect(formatRate(undefined)).toBe("-");
  });
  it("rounds the axis up to a tidy number", () => {
    expect(niceMax(0)).toBe(1);
    expect(niceMax(730)).toBe(1000);
    expect(niceMax(1800)).toBe(2000);
    expect(niceMax(2400)).toBe(2500);
  });
});
