import { describe, expect, it } from "vitest";
import {
  buildRRule,
  defaultStart,
  describeRule,
  detectPreset,
  formatWhen,
  fromLocalInput,
  isValidRule,
  repeatPresets,
  toLocalInput,
} from "./rrule";

// 2026-10-01 is a Thursday.
const thursday = new Date(2026, 9, 1, 9, 0);

describe("rrule presets", () => {
  it("round-trips every preset", () => {
    for (const preset of repeatPresets) {
      if (preset === "custom") continue;
      expect(detectPreset(buildRRule(preset))).toBe(preset);
    }
  });

  it("builds rules the server understands", () => {
    expect(buildRRule("none")).toBe("");
    expect(buildRRule("daily")).toBe("FREQ=DAILY");
    expect(buildRRule("weekdays")).toBe("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR");
    expect(buildRRule("custom", " rrule:freq=weekly;byday=mo,we ")).toBe(
      "FREQ=WEEKLY;BYDAY=MO,WE",
    );
  });

  it("treats unknown rules as custom", () => {
    expect(detectPreset("FREQ=WEEKLY;BYDAY=SA,SU")).toBe("custom");
    expect(detectPreset("")).toBe("none");
  });

  it("validates the basic shape", () => {
    expect(isValidRule("")).toBe(true);
    expect(isValidRule("FREQ=DAILY;INTERVAL=2")).toBe(true);
    expect(isValidRule("FREQ=MONTHLY;BYDAY=-1FR")).toBe(true);
    expect(isValidRule("every day")).toBe(false);
    expect(isValidRule("INTERVAL=2")).toBe(false);
    expect(isValidRule("FREQ=SOMETIMES")).toBe(false);
  });
});

describe("describeRule", () => {
  it("describes common rules in Chinese", () => {
    expect(describeRule("", thursday)).toBe("不重复");
    expect(describeRule("FREQ=DAILY", thursday)).toBe("每天");
    expect(describeRule("FREQ=DAILY;INTERVAL=3", thursday)).toBe("每 3 天");
    expect(describeRule(buildRRule("weekdays"), thursday)).toBe("工作日");
    expect(describeRule("FREQ=WEEKLY", thursday)).toBe("每周四");
    expect(describeRule("FREQ=WEEKLY;BYDAY=MO,WE", thursday)).toBe(
      "每周一、三",
    );
    expect(describeRule("FREQ=MONTHLY", thursday)).toBe("每月 1 号");
    expect(describeRule("FREQ=YEARLY", thursday)).toBe("每年 10 月 1 日");
  });

  it("describes rules in English", () => {
    expect(describeRule("FREQ=WEEKLY;BYDAY=MO,WE", thursday, "en")).toBe(
      "Weekly on Mon, Wed",
    );
    expect(describeRule("FREQ=DAILY", thursday, "en")).toBe("Every day");
  });
});

describe("formatWhen", () => {
  const now = new Date(2026, 9, 1, 12, 0);
  it("uses today, tomorrow and yesterday", () => {
    expect(formatWhen(new Date(2026, 9, 1, 14, 5), now)).toBe("今天 14:05");
    expect(formatWhen(new Date(2026, 9, 2, 9, 0), now)).toBe("明天 09:00");
    expect(formatWhen(new Date(2026, 8, 30, 23, 0), now, "en")).toBe(
      "Yesterday 23:00",
    );
  });
  it("shows the date for other days", () => {
    expect(formatWhen(new Date(2026, 9, 5, 8, 30), now)).toMatch(/08:30$/);
    expect(formatWhen(new Date(2026, 9, 5, 8, 30), now)).toContain("5");
  });
});

describe("local datetime input", () => {
  it("round-trips a local time", () => {
    const value = toLocalInput(thursday);
    expect(value).toBe("2026-10-01T09:00");
    expect(fromLocalInput(value)?.getTime()).toBe(thursday.getTime());
    expect(fromLocalInput("nope")).toBeNull();
  });

  it("defaults to the next full hour at least 5 minutes away", () => {
    expect(defaultStart(new Date(2026, 9, 1, 9, 20)).getHours()).toBe(10);
    const late = defaultStart(new Date(2026, 9, 1, 9, 58));
    expect(late.getHours()).toBe(11);
    expect(late.getMinutes()).toBe(0);
  });
});
