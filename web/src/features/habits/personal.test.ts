import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import {
  averageWeight,
  bodySeries,
  dateKey,
  exerciseToWorkout,
  personalSession,
  personalSessionId,
  personalWeek,
  personalWorkoutItems,
  validPersonalDate,
} from "./personal";
import type {
  PersonalDay,
  PersonalLibrary,
  PersonalProfile,
} from "./personalApi";

const library: PersonalLibrary = JSON.parse(
  readFileSync(
    new URL(
      "../../../../backend/internal/server/modules/habits/catalog/plan.json",
      import.meta.url,
    ),
    "utf8",
  ),
);
const profile: PersonalProfile = {
  start: "2026-09-07",
  wake: "11:00",
  sleep: "03:00",
  phase: 1,
  baseline: 82,
  stepGoal: 6000,
  runLevel: 0,
  habitIds: {},
};
const day = (date: string, weight: string): PersonalDay => ({
  date,
  weight,
  waist: "",
  sleep: "",
  steps: "",
  restingHr: "",
  energy: "",
  back: "",
  note: "",
  english: "",
  food: "",
  sets: {},
  checks: {},
});
describe("个人计划", () => {
  it("按计划开始日交替 A/B，前两周减少组数", () => {
    expect(personalSessionId("2026-09-07", profile)).toBe("A1");
    expect(personalSessionId("2026-09-14", profile)).toBe("B1");
    expect(personalSessionId("2026-09-13", profile)).toBe("rest");
    expect(personalSession("2026-09-07", profile, library).items[0].sets).toBe(
      2,
    );
    expect(personalSession("2026-09-21", profile, library).items[0].sets).toBe(
      3,
    );
    expect(personalWeek("2026-09-06", profile.start)).toBe(1);
  });
  it("阶段由设置选择，有氧保留走跑间隔", () => {
    expect(personalSessionId("2026-09-11", { ...profile, phase: 2 })).toBe(
      "C2",
    );
    expect(personalSessionId("2026-09-08", { ...profile, phase: 3 })).toBe(
      "U3",
    );
    const cardio = personalSession(
      "2026-09-08",
      { ...profile, runLevel: 1 },
      library,
    );
    expect(cardio.items[1].prescription).toContain("走 5 分＋跑 1 分");
  });
  it("保存区间和距离，不把 8–12 次误记为 8 次", () => {
    const item = exerciseToWorkout(
      library.exercises.find((e) => e.id === "squat")!,
    );
    expect(item.sets).toBe(3);
    expect(item.reps).toBeUndefined();
    expect(item.prescription).toBe("8–12 次");
    expect(
      exerciseToWorkout(library.exercises.find((e) => e.id === "carry")!)
        .prescription,
    ).toContain("米");
    const session = personalSession("2026-09-07", profile, library);
    const d = day("2026-09-07", "");
    d.sets["A1:0:0"] = true;
    expect(personalWorkoutItems(session, library, d)).toEqual([
      {
        name: "高脚杯深蹲",
        exerciseId: "squat",
        sets: 1,
        prescription: "8–12 次",
      },
    ]);
  });
  it("体重均值只用真实记录，日期严格校验", () => {
    expect(
      averageWeight(
        [
          day("2026-09-07", "82"),
          day("2026-09-08", ""),
          day("2026-09-09", "80"),
          day("2026-09-01", "90"),
        ],
        "2026-09-09",
      ),
    ).toBe(81);
    expect(averageWeight([], "2026-09-09")).toBeNull();
    expect(validPersonalDate("2026-02-30")).toBe(false);
    expect(validPersonalDate("2026-10-03")).toBe(true);
    expect(dateKey(new Date("2026-10-02T19:00:00Z"), "Asia/Shanghai")).toBe(
      "2026-10-03",
    );
    expect(dateKey(new Date("2026-10-02T19:00:00Z"), "UTC")).toBe("2026-10-02");
  });

  it("身体趋势只统计范围内记过的值，不补零", () => {
    const rows = [
      { ...day("2026-09-01", "90"), restingHr: "70" },
      { ...day("2026-09-05", "82"), restingHr: "60", steps: "0" },
      { ...day("2026-09-07", ""), restingHr: "0", steps: "8000" },
      { ...day("2026-09-09", "80"), restingHr: "58", sleep: "7.5" },
      { ...day("2026-09-10", "81"), restingHr: "57", sleep: "0" },
    ];
    // 7 天范围从 09-04 到 09-10，09-01 不算
    const w = bodySeries(rows, "weight", 7, "2026-09-10");
    expect(w.points.map((p) => p.date)).toEqual([
      "2026-09-05",
      "2026-09-09",
      "2026-09-10",
    ]);
    expect(w.latest).toBe(81);
    expect(w.min).toBe(80);
    expect(w.max).toBe(82);
    expect(w.average).toBeCloseTo(81);
    expect(w.change).toBe(-1);
    // 心率 0 当作没记，步数 0 是真实记录
    expect(bodySeries(rows, "restingHr", 7, "2026-09-10").points).toHaveLength(
      3,
    );
    expect(bodySeries(rows, "steps", 7, "2026-09-10").points).toHaveLength(2);
    // 睡眠 0 小时也是记录
    expect(bodySeries(rows, "sleep", 7, "2026-09-10").points).toHaveLength(2);
    // 范围拉长到 30 天，把 09-01 也算进来；结束日之后的不算
    expect(bodySeries(rows, "weight", 30, "2026-09-10").points).toHaveLength(4);
    expect(bodySeries(rows, "weight", 30, "2026-09-08").points).toHaveLength(2);
    // 没数据
    const none = bodySeries([], "weight", 30, "2026-09-10");
    expect(none).toMatchObject({ latest: null, average: null, change: null });
  });
});
