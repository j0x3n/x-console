import { describe, expect, it } from "vitest";
import {
  barWidths,
  currentPeriod,
  fmtChange,
  fmtNum,
  fmtPercent,
  parseNumber,
  quarterLabel,
  shiftPeriod,
  shiftWeek,
  stateTone,
  summarize,
  valueText,
  weekLabel,
  weeksBetween,
} from "./format";

describe("quarters", () => {
  it("finds the quarter of a date", () => {
    expect(currentPeriod(new Date(2026, 0, 1))).toEqual({
      year: 2026,
      quarter: 1,
    });
    expect(currentPeriod(new Date(2026, 2, 31))).toEqual({
      year: 2026,
      quarter: 1,
    });
    expect(currentPeriod(new Date(2026, 3, 1))).toEqual({
      year: 2026,
      quarter: 2,
    });
    expect(currentPeriod(new Date(2026, 9, 11))).toEqual({
      year: 2026,
      quarter: 4,
    });
    expect(currentPeriod(new Date(2026, 11, 31))).toEqual({
      year: 2026,
      quarter: 4,
    });
  });

  it("moves across years", () => {
    expect(shiftPeriod({ year: 2026, quarter: 4 }, 1)).toEqual({
      year: 2027,
      quarter: 1,
    });
    expect(shiftPeriod({ year: 2026, quarter: 1 }, -1)).toEqual({
      year: 2025,
      quarter: 4,
    });
    expect(shiftPeriod({ year: 2026, quarter: 2 }, 5)).toEqual({
      year: 2027,
      quarter: 3,
    });
    expect(shiftPeriod({ year: 2026, quarter: 2 }, 0)).toEqual({
      year: 2026,
      quarter: 2,
    });
  });

  it("labels a quarter", () => {
    expect(quarterLabel({ year: 2026, quarter: 4 })).toBe("2026 年第 4 季度");
  });
});

describe("numbers", () => {
  it("keeps one decimal at most", () => {
    expect(fmtNum(12)).toBe("12");
    expect(fmtNum(12.5)).toBe("12.5");
    expect(fmtNum(12.04)).toBe("12");
    expect(fmtNum(0.25)).toBe("0.3");
  });

  it("shows percent and change", () => {
    expect(fmtPercent(null)).toBe("–");
    expect(fmtPercent(47.55)).toBe("47.6%");
    expect(fmtChange(null)).toBe("–");
    expect(fmtChange(2.5)).toBe("+2.5");
    expect(fmtChange(-1)).toBe("-1");
    expect(fmtChange(0.01)).toBe("0");
  });

  it("writes value against target", () => {
    expect(valueText(41.5, 80, "次")).toBe("41.5 / 80 次");
    expect(valueText(null, 80, "")).toBe("– / 80");
  });

  it("reads a number box", () => {
    expect(parseNumber(" 12.5 ")).toBe(12.5);
    expect(parseNumber("")).toBeNull();
    expect(parseNumber("abc")).toBeNull();
    expect(parseNumber("0")).toBe(0);
  });
});

describe("bar", () => {
  it("clamps the fill and the mark", () => {
    expect(barWidths(45, 50)).toEqual({ fill: 45, mark: 50 });
    expect(barWidths(150, 120)).toEqual({ fill: 100, mark: 100 });
    expect(barWidths(null, -5)).toEqual({ fill: 0, mark: 0 });
  });
});

describe("summary", () => {
  it("counts active goals, averages and counts the ones behind", () => {
    const s = summarize([
      { progress: 40, state: "behind", status: "active" },
      { progress: 60, state: "on_track", status: "active" },
      { progress: null, state: "on_track", status: "active" },
      { progress: 100, state: "done", status: "done" },
    ]);
    expect(s).toEqual({ active: 3, average: 50, behind: 1 });
  });

  it("has no average when nothing is measured", () => {
    expect(summarize([])).toEqual({ active: 0, average: null, behind: 0 });
  });
});

describe("weeks", () => {
  it("labels a week", () => {
    expect(weekLabel("2026-11-09", "2026-11-15")).toBe(
      "11 月 9 日到 11 月 15 日",
    );
  });

  it("moves by weeks across month and year ends", () => {
    expect(shiftWeek("2026-11-09", -1)).toBe("2026-11-02");
    expect(shiftWeek("2026-12-28", 1)).toBe("2027-01-04");
    expect(shiftWeek("2027-01-04", -1)).toBe("2026-12-28");
  });

  it("counts weeks between two Mondays", () => {
    expect(weeksBetween("2026-11-09", "2026-11-23")).toBe(2);
    expect(weeksBetween("2026-11-23", "2026-11-09")).toBe(-2);
  });
});

describe("state", () => {
  it("has a tone for each state", () => {
    expect(stateTone("done")).toBe("ok");
    expect(stateTone("behind")).toBe("warn");
    expect(stateTone("missed")).toBe("danger");
    expect(stateTone("on_track")).toBe("info");
    expect(stateTone("upcoming")).toBe("");
  });
});
