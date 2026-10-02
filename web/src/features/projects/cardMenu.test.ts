import { describe, expect, it } from "vitest";
import { dueShortcuts } from "./cardMenu";

describe("B85 due shortcuts", () => {
  it("offers today only before 18:00", () => {
    const morning = new Date(2026, 9, 2, 9, 0); // 周五
    expect(dueShortcuts(morning).map((d) => d.label)).toEqual([
      "Today 18:00",
      "Tomorrow 18:00",
      "Next Monday 09:00",
    ]);
    const night = new Date(2026, 9, 2, 20, 0);
    expect(dueShortcuts(night)[0].label).toBe("Tomorrow 18:00");
  });
  it("finds next Monday", () => {
    const friday = new Date(2026, 9, 2, 9, 0);
    const monday = dueShortcuts(friday).at(-1)!.at;
    expect(monday.getDay()).toBe(1);
    expect(monday.getDate()).toBe(5);
    const onMonday = new Date(2026, 9, 5, 9, 0);
    expect(dueShortcuts(onMonday).at(-1)!.at.getDate()).toBe(12);
  });
});
