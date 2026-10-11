import { describe, expect, it } from "vitest";
import {
  MANUAL_STATES,
  STATE_LABELS,
  STATE_NAMES,
  STATE_TONES,
  manualBadge,
  sinceText,
  sleepLine,
  validHomeEntity,
  validSleepEntity,
} from "./logic";
import type { MeDay, MeState } from "./api";

const t = (k: string) => k;
const time = (d: Date) =>
  `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
const date = (d: Date) => `${d.getMonth() + 1}/${d.getDate()}`;
const at = (day: number, h: number, m = 0) =>
  new Date(2026, 9, day, h, m).toISOString();
const now = new Date(2026, 9, 11, 15, 0);

describe("states", () => {
  it("每种状态都有名称和颜色，手动设定里没有“不确定”", () => {
    for (const s of STATE_NAMES) {
      expect(STATE_LABELS[s]).toBeTruthy();
      expect(s in STATE_TONES).toBe(true);
    }
    expect(MANUAL_STATES).toHaveLength(5);
    expect(MANUAL_STATES).not.toContain("unknown");
  });
});

describe("sinceText", () => {
  it("今天只写时间，昨天加“Yesterday”，更早的加日期", () => {
    expect(sinceText(t, at(11, 9, 5), now, time, date)).toBe("09:05");
    expect(sinceText(t, at(10, 22, 30), now, time, date)).toBe(
      "Yesterday 22:30",
    );
    expect(sinceText(t, at(8, 7, 0), now, time, date)).toBe("10/8 07:00");
  });
});

describe("manualBadge", () => {
  const base: MeState = {
    state: "out",
    since: at(11, 14),
    guessed: false,
    manual: true,
    source: "manual",
  };
  it("不是手动的不显示", () => {
    expect(manualBadge(t, { ...base, manual: false }, now, time, date)).toBe(
      "",
    );
  });
  it("没有到期时间时只写手动", () => {
    expect(manualBadge(t, base, now, time, date)).toBe("Manual");
  });
  it("有到期时间时写到几点", () => {
    expect(
      manualBadge(t, { ...base, manualUntil: at(11, 16, 30) }, now, time, date),
    ).toBe("Manual · until 16:30");
  });
});

describe("sleepLine", () => {
  const day = (extra: Partial<MeDay>): MeDay => ({
    date: "2026-10-12",
    segments: [],
    ...extra,
  });
  it("写出几点睡、几点起和睡了多久", () => {
    const line = sleepLine(
      t,
      day({
        sleptAt: at(12, 3, 40),
        wokeAt: at(12, 11, 55),
        sleepMinutes: 495,
      }),
      time,
    );
    expect(line).toBe("03:40 → 11:55 · 8 hr 15 min");
  });
  it("整点小时和不到一小时", () => {
    expect(
      sleepLine(
        t,
        day({ sleptAt: at(12, 1), wokeAt: at(12, 9), sleepMinutes: 480 }),
        time,
      ),
    ).toBe("01:00 → 09:00 · 8 hr");
    expect(
      sleepLine(
        t,
        day({ sleptAt: at(12, 1), wokeAt: at(12, 1, 40), sleepMinutes: 40 }),
        time,
      ),
    ).toBe("01:00 → 01:40 · 40 min");
  });
  it("没有睡眠时是空的", () => {
    expect(sleepLine(t, day({}), time)).toBe("");
  });
});

describe("entity names", () => {
  it("在家的实体只收 person 和 device_tracker，空表示不用", () => {
    expect(validHomeEntity("")).toBe(true);
    expect(validHomeEntity(" person.me ")).toBe(true);
    expect(validHomeEntity("device_tracker.iphone_1")).toBe(true);
    expect(validHomeEntity("light.desk")).toBe(false);
    expect(validHomeEntity("Person.Me")).toBe(false);
    expect(validHomeEntity("person.")).toBe(false);
  });
  it("睡眠专注的实体只收 binary_sensor 和 input_boolean", () => {
    expect(validSleepEntity("")).toBe(true);
    expect(validSleepEntity("binary_sensor.iphone_focus")).toBe(true);
    expect(validSleepEntity("input_boolean.sleeping")).toBe(true);
    expect(validSleepEntity("person.me")).toBe(false);
  });
});
