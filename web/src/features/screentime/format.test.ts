import { describe, expect, it } from "vitest";
import {
  appName,
  dayLabel,
  durationText,
  localToday,
  rangeLabel,
  shiftRange,
  statValue,
} from "./format";

const t = (k: string) => ({ hr: "小时", min: "分钟" })[k] ?? k;

describe("durationText", () => {
  it("writes hours and minutes", () => {
    expect(durationText(t, 125)).toBe("2 小时 5 分钟");
    expect(durationText(t, 120)).toBe("2 小时");
    expect(durationText(t, 35)).toBe("35 分钟");
    expect(durationText(t, 0)).toBe("0 分钟");
  });
});

describe("statValue", () => {
  it("uses minutes below an hour and one decimal of hours above", () => {
    expect(statValue(t, 45)).toEqual({ value: "45", unit: "分钟" });
    expect(statValue(t, 200)).toEqual({ value: "3.3", unit: "小时" });
    expect(statValue(t, 60)).toEqual({ value: "1", unit: "小时" });
  });
});

describe("appName", () => {
  it("drops .exe", () => {
    expect(appName("Code.exe")).toBe("Code");
    expect(appName("wt.EXE")).toBe("wt");
    expect(appName("Steam")).toBe("Steam");
  });
});

describe("shiftRange", () => {
  it("moves by a day, a week or a month", () => {
    expect(shiftRange("2026-10-10", "day", -1)).toBe("2026-10-09");
    expect(shiftRange("2026-10-31", "day", 1)).toBe("2026-11-01");
    expect(shiftRange("2026-10-05", "week", -1)).toBe("2026-09-28");
    expect(shiftRange("2026-10-05", "week", 1)).toBe("2026-10-12");
    expect(shiftRange("2026-10-01", "month", -1)).toBe("2026-09-01");
    expect(shiftRange("2026-12-01", "month", 1)).toBe("2027-01-01");
  });
});

describe("labels", () => {
  it("names a range and a day", () => {
    expect(rangeLabel("month", "2026-10-01", "2026-10-31", "zh")).toBe(
      "2026年10月",
    );
    expect(rangeLabel("day", "2026-10-10", "2026-10-10", "zh")).toContain("10");
    expect(dayLabel("2026-10-05", "month", "zh")).toBe("5");
    expect(dayLabel("2026-10-05", "week", "zh")).toBe("周一");
  });
  it("reads the local date", () => {
    expect(localToday(new Date(2026, 9, 5, 23, 59))).toBe("2026-10-05");
  });
});
