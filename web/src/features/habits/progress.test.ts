import { describe, expect, it } from "vitest";
import {
  barMax,
  describeItem,
  formatAmount,
  heatLevel,
  isoWeekday,
  joinWindow,
  parseTimes,
  plansToSave,
  ratio,
  remindSummary,
  ringGeometry,
  splitWindow,
  weekPlans,
} from "./progress";

describe("progress math", () => {
  it("clamps the completion ratio", () => {
    expect(ratio(4, 8)).toBe(0.5);
    expect(ratio(10, 8)).toBe(1);
    expect(ratio(-1, 8)).toBe(0);
    expect(ratio(1, 0)).toBe(1);
    expect(ratio(0, 0)).toBe(0);
  });

  it("computes the ring offset", () => {
    const { circumference, offset } = ringGeometry(0.25, 10);
    expect(circumference).toBeCloseTo(62.83, 2);
    expect(offset).toBeCloseTo(circumference * 0.75, 5);
    expect(ringGeometry(2, 10).offset).toBe(0);
  });

  it("formats amounts", () => {
    expect(formatAmount(3)).toBe("3");
    expect(formatAmount(0.5)).toBe("0.5");
    expect(formatAmount(1 / 3)).toBe("0.33");
  });

  it("grades heatmap cells", () => {
    expect(heatLevel(0, 8)).toBe(0);
    expect(heatLevel(1, 8)).toBe(1);
    expect(heatLevel(4, 8)).toBe(2);
    expect(heatLevel(7, 8)).toBe(3);
    expect(heatLevel(8, 8)).toBe(4);
    expect(heatLevel(12, 8)).toBe(4);
  });

  it("scales bars to the larger of max and target", () => {
    expect(barMax([1, 2, 3], 8)).toBe(8);
    expect(barMax([1, 12], 8)).toBe(12);
    expect(barMax([], 0)).toBe(1);
  });
});

describe("reminder settings", () => {
  it("splits and joins windows", () => {
    expect(splitWindow("09:00-21:00")).toEqual({
      start: "09:00",
      end: "21:00",
    });
    expect(splitWindow("")).toEqual({ start: "", end: "" });
    expect(joinWindow("09:00", "21:00")).toBe("09:00-21:00");
    expect(joinWindow("", "21:00")).toBe("");
  });

  it("parses time lists", () => {
    expect(parseTimes("20:00, 8:00，12:30 20:00")).toEqual([
      "08:00",
      "12:30",
      "20:00",
    ]);
    expect(parseTimes("")).toEqual([]);
    expect(parseTimes("8am")).toBeNull();
    expect(parseTimes("24:00")).toBeNull();
  });

  it("summarizes reminders", () => {
    const base = {
      remindIntervalMinutes: 60,
      remindWindow: "09:00-21:00",
      remindTimes: ["08:00", "20:00"],
    };
    expect(remindSummary({ ...base, remindMode: "interval" })).toBe(
      "每 60 分钟 · 09:00-21:00",
    );
    expect(remindSummary({ ...base, remindMode: "times" })).toBe(
      "08:00、20:00",
    );
    expect(remindSummary({ ...base, remindMode: "none" })).toBe("");
  });
});

describe("workouts", () => {
  it("numbers weekdays from Monday", () => {
    expect(isoWeekday(new Date(2026, 9, 4))).toBe(7); // Sunday
    expect(isoWeekday(new Date(2026, 9, 5))).toBe(1); // Monday
  });

  it("describes items", () => {
    expect(describeItem({ name: "深蹲", sets: 5, reps: 5, weight: 60 })).toBe(
      "深蹲 5×5 60kg",
    );
    expect(describeItem({ name: "平板支撑", sets: 3 })).toBe("平板支撑 3 组");
    expect(describeItem({ name: "跑步" })).toBe("跑步");
  });

  it("groups plans into a week and drops empty days on save", () => {
    const week = weekPlans([
      { weekday: 1, title: "腿", items: [{ name: "深蹲" }] },
      { weekday: 1, title: "核心", items: [{ name: "卷腹" }] },
      { weekday: 4, title: "", items: [{ name: "卧推" }] },
    ]);
    expect(week).toHaveLength(7);
    expect(week[0].title).toBe("腿 / 核心");
    expect(week[0].items.map((i) => i.name)).toEqual(["深蹲", "卷腹"]);
    week[2].items = [{ name: "  " }];
    const saved = plansToSave(week);
    expect(saved.map((d) => d.weekday)).toEqual([1, 4]);
  });
});
