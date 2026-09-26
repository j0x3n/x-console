import { describe, expect, it } from "vitest";
import {
  addDays,
  allDayEvents,
  dayKey,
  formatDuration,
  formatRemaining,
  layoutDay,
  parseDayKey,
  startOfWeek,
  viewRange,
  type EventLike,
} from "./dates";

const at = (d: number, h: number, m = 0) =>
  new Date(2026, 9, d, h, m).toISOString();

function ev(
  id: string,
  start: string,
  end: string,
  extra: Partial<EventLike> = {},
): EventLike {
  return { id, title: id, start, end, allDay: false, ...extra };
}

describe("ranges", () => {
  it("starts weeks on Monday", () => {
    // 2026-10-25 is a Sunday.
    expect(dayKey(startOfWeek(new Date(2026, 9, 25, 15)))).toBe("2026-10-19");
    expect(dayKey(startOfWeek(new Date(2026, 9, 26)))).toBe("2026-10-26");
  });

  it("builds day and week ranges", () => {
    const week = viewRange("week", new Date(2026, 9, 28, 10));
    expect(week.days.map(dayKey)).toEqual([
      "2026-10-26",
      "2026-10-27",
      "2026-10-28",
      "2026-10-29",
      "2026-10-30",
      "2026-10-31",
      "2026-11-01",
    ]);
    expect(dayKey(week.to)).toBe("2026-11-02");
    const day = viewRange("day", new Date(2026, 9, 28, 10));
    expect(day.days).toHaveLength(1);
    expect(day.to.getTime() - day.from.getTime()).toBeGreaterThan(0);
  });

  it("parses day keys", () => {
    expect(dayKey(parseDayKey("2026-02-03")!)).toBe("2026-02-03");
    expect(parseDayKey("tomorrow")).toBeNull();
    expect(dayKey(addDays(new Date(2026, 11, 31), 1))).toBe("2027-01-01");
  });
});

describe("all-day events", () => {
  it("covers each day up to the exclusive end date", () => {
    const trip = ev("trip", at(30, 0), at(32, 0), {
      allDay: true,
      startDate: "2026-10-30",
      endDate: "2026-11-01",
    });
    const timed = ev("meeting", at(30, 9), at(30, 10));
    expect(allDayEvents([trip, timed], new Date(2026, 9, 30))).toEqual([trip]);
    expect(allDayEvents([trip], new Date(2026, 9, 31))).toEqual([trip]);
    expect(allDayEvents([trip], new Date(2026, 10, 1))).toEqual([]);
  });
});

describe("layoutDay", () => {
  const day = new Date(2026, 9, 27);

  it("places events by minute and splits overlaps into columns", () => {
    const blocks = layoutDay(
      [
        ev("a", at(27, 9), at(27, 10)),
        ev("b", at(27, 9, 30), at(27, 11)),
        ev("c", at(27, 10), at(27, 10, 30)),
        ev("d", at(27, 14), at(27, 15)),
      ],
      day,
    );
    const byId = Object.fromEntries(blocks.map((b) => [b.event.id, b]));
    expect(byId.a).toMatchObject({
      top: 540,
      height: 60,
      column: 0,
      columns: 2,
    });
    expect(byId.b).toMatchObject({
      top: 570,
      height: 90,
      column: 1,
      columns: 2,
    });
    // c starts when a ends, so it reuses the first column.
    expect(byId.c).toMatchObject({ top: 600, column: 0, columns: 2 });
    // d does not overlap anything and takes the full width.
    expect(byId.d).toMatchObject({ column: 0, columns: 1 });
  });

  it("clips events that cross midnight and keeps short events visible", () => {
    const blocks = layoutDay(
      [
        ev("night", at(26, 22), at(27, 2)),
        ev("late", at(27, 23), at(28, 1)),
        ev("ping", at(27, 12), at(27, 12)),
        ev("allday", at(27, 0), at(28, 0), { allDay: true }),
        ev("other", at(28, 9), at(28, 10)),
      ],
      day,
    );
    const byId = Object.fromEntries(blocks.map((b) => [b.event.id, b]));
    expect(Object.keys(byId).sort()).toEqual(["late", "night", "ping"]);
    expect(byId.night).toMatchObject({
      top: 0,
      height: 120,
      continuesBefore: true,
    });
    expect(byId.late).toMatchObject({
      top: 1380,
      height: 60,
      continuesAfter: true,
    });
    expect(byId.ping.height).toBe(15);
  });
});

describe("formatting", () => {
  it("formats remaining time and durations", () => {
    expect(formatRemaining(25 * 60 * 1000)).toBe("25:00");
    expect(formatRemaining(61_500)).toBe("01:02");
    expect(formatRemaining(-5)).toBe("00:00");
    expect(formatDuration(1500, "zh")).toBe("25 分钟");
    expect(formatDuration(3900, "zh")).toBe("1 小时 5 分");
    expect(formatDuration(7200, "en")).toBe("2 h");
  });
});
