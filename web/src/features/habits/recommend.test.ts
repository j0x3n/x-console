import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import type { PersonalLibrary } from "./personalApi";
import {
  EXTRA_RECOMMENDATIONS,
  FITNESS_PROGRAMS,
  TEMPLATE_RECOMMENDATIONS,
  libraryCategory,
  programForHabit,
  programHabitInput,
  sameName,
} from "./recommend";

const library: PersonalLibrary = JSON.parse(
  readFileSync(
    new URL(
      "../../../../backend/internal/server/modules/habits/catalog/plan.json",
      import.meta.url,
    ),
    "utf8",
  ),
);

describe("健身方案", () => {
  it("每个动作都在动作库里", () => {
    const ids = new Set(library.exercises.map((e) => e.id));
    for (const p of FITNESS_PROGRAMS)
      for (const item of p.items) expect(ids, p.id).toContain(item.exerciseId);
  });

  it("身体问题和每天的基础动作都有", () => {
    expect(
      FITNESS_PROGRAMS.filter((p) => p.kind === "problem").length,
    ).toBeGreaterThan(5);
    expect(
      FITNESS_PROGRAMS.filter((p) => p.kind === "daily").length,
    ).toBeGreaterThan(5);
  });

  it("id 和习惯名不重复，按习惯名能找回方案", () => {
    const ids = FITNESS_PROGRAMS.map((p) => p.id);
    const names = FITNESS_PROGRAMS.map((p) => p.habitName);
    expect(new Set(ids).size).toBe(ids.length);
    expect(new Set(names).size).toBe(names.length);
    for (const p of FITNESS_PROGRAMS)
      expect(programForHabit(p.habitName)).toBe(p);
  });

  it("加入习惯时按默认时间提醒", () => {
    const input = programHabitInput(FITNESS_PROGRAMS[0]);
    expect(input.remindMode).toBe("times");
    expect(input.remindTimes).toEqual([FITNESS_PROGRAMS[0].remindAt]);
    expect(input.remindTimes![0]).toMatch(/^\d\d:\d\d$/);
  });
});

describe("推荐习惯", () => {
  it("名字不和健康提醒、个人计划、健身方案重复", () => {
    const names = [
      ...TEMPLATE_RECOMMENDATIONS.map((r) => r.name),
      ...library.habits.map((h) => h.name),
      ...EXTRA_RECOMMENDATIONS.map((r) => r.name),
      ...FITNESS_PROGRAMS.map((p) => p.habitName),
    ].map((n) => n.replace(/\s+/g, ""));
    expect(new Set(names).size).toBe(names.length);
  });

  it("卡片名就是新建出来的习惯名", () => {
    for (const r of EXTRA_RECOMMENDATIONS) expect(r.input.name).toBe(r.name);
  });

  it("个人计划的模板按内容分类", () => {
    expect(libraryCategory({ category: "food" })).toBe("food");
    expect(libraryCategory({ category: "english" })).toBe("learn");
    expect(libraryCategory({ category: "daily" })).toBe("routine");
    expect(libraryCategory({ category: "daily", exerciseId: "chin" })).toBe(
      "body",
    );
  });

  it("比较名字时忽略空格", () => {
    expect(sameName("读书 20 分钟", "读书20分钟")).toBe(true);
    expect(sameName("读书", "写日记")).toBe(false);
  });
});
