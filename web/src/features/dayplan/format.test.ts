import { describe, expect, it } from "vitest";
import type { DayPlanItem } from "./api";
import {
  checkItems,
  addDays,
  dateOffset,
  doneCount,
  minutesOf,
  rangeText,
  sameItems,
} from "./format";

describe("日期", () => {
  it("相对今天的本地日期", () => {
    const base = new Date(2026, 10, 30, 23, 30); // 11 月 30 日
    expect(dateOffset(0, base)).toBe("2026-11-30");
    expect(dateOffset(1, base)).toBe("2026-12-01");
    expect(dateOffset(-1, base)).toBe("2026-11-29");
  });
});

describe("时间", () => {
  it("认 HH:MM", () => {
    expect(minutesOf("09:30")).toBe(570);
    expect(minutesOf("24:00")).toBeNull();
    expect(minutesOf("9:30")).toBeNull();
    expect(minutesOf("")).toBeNull();
  });
  it("时间段显示", () => {
    expect(rangeText("09:00", "10:30")).toBe("09:00–10:30");
    expect(rangeText("", "")).toBe("");
  });
});

describe("检查编辑中的事项", () => {
  it("没问题", () => {
    expect(
      checkItems([
        { title: "a", start: "09:00", end: "10:00" },
        { title: "b" },
        { title: "c", start: "10:00", end: "10:30" },
      ]),
    ).toBe("");
  });
  it("各种错误", () => {
    expect(checkItems([{ title: " " }])).toMatch(/名字/);
    expect(checkItems([{ title: "a", start: "09:00" }])).toMatch(/一起填/);
    expect(checkItems([{ title: "a", start: "10:00", end: "09:00" }])).toMatch(
      /晚于/,
    );
    expect(
      checkItems([
        { title: "a", start: "09:00", end: "10:00" },
        { title: "b", start: "09:30", end: "10:30" },
      ]),
    ).toMatch(/重叠/);
    expect(
      checkItems(Array.from({ length: 9 }, (_, i) => ({ title: `事${i}` }))),
    ).toMatch(/最多 8/);
  });
});

describe("比较", () => {
  const item = (over: Partial<DayPlanItem>): DayPlanItem => ({
    id: 1,
    position: 0,
    title: "a",
    kind: "manual",
    ref: "",
    start: "09:00",
    end: "10:00",
    reason: "",
    done: false,
    ...over,
  });
  it("同样就是同样，改一项就不同", () => {
    expect(
      sameItems(
        [item({})],
        [{ title: "a", start: "09:00", end: "10:00", done: false }],
      ),
    ).toBe(true);
    expect(
      sameItems([item({})], [{ title: "a", start: "09:00", end: "10:30" }]),
    ).toBe(false);
    expect(sameItems([item({})], [])).toBe(false);
  });
  it("数做完的", () => {
    expect(doneCount([item({ done: true }), item({ id: 2 })])).toBe(1);
  });
});

describe("addDays", () => {
  it("adds days on the calendar across months and years", () => {
    expect(addDays("2026-11-30", 1)).toBe("2026-12-01");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(addDays("2028-02-28", 1)).toBe("2028-02-29");
    expect(addDays("2026-03-01", -1)).toBe("2026-02-28");
    expect(addDays("2026-11-11", 0)).toBe("2026-11-11");
  });
});
