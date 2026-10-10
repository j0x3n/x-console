import { describe, expect, it } from "vitest";
import {
  eventText,
  parseDays,
  sinceText,
  statusText,
  statusTone,
  today,
  validDate,
} from "./format";

const t = (k: string) =>
  ({
    Today: "今天",
    "days from now": "天后",
    "years old": "岁",
    "anniversary years": "周年",
    "days ago": "天前",
    "No contact recorded": "还没记过",
    "Time to get in touch": "该联系了",
    "days over": "天",
    "All good": "正常",
  })[k] ?? k;

describe("contacts format", () => {
  it("解析提醒天数：去重、从大到小、拒绝非法值", () => {
    expect(parseDays("1, 7")).toEqual([7, 1]);
    expect(parseDays("7，7 、 3")).toEqual([7, 3]);
    expect(parseDays("")).toEqual([]);
    expect(parseDays("0")).toBeNull();
    expect(parseDays("366")).toBeNull();
    expect(parseDays("a")).toBeNull();
    expect(parseDays("1 2 3 4 5 6 7 8 9")).toBeNull();
  });

  it("日期可以不写年份，02-29 只在没写年份或闰年可用", () => {
    expect(validDate("08-15")).toBe(true);
    expect(validDate("1990-08-15")).toBe(true);
    expect(validDate("02-29")).toBe(true);
    expect(validDate("2023-02-29")).toBe(false);
    expect(validDate("2024-02-29")).toBe(true);
    expect(validDate("13-01")).toBe(false);
    expect(validDate("8-15")).toBe(false);
  });

  it("日子说明带上几岁或几周年", () => {
    expect(eventText(t, { kind: "birthday", nextIn: 0, years: 36 })).toBe(
      "今天 · 36 岁",
    );
    expect(eventText(t, { kind: "anniversary", nextIn: 5, years: 10 })).toBe(
      "5 天后 · 10 周年",
    );
    expect(eventText(t, { kind: "birthday", nextIn: 5 })).toBe("5 天后");
  });

  it("上次联系和状态文字", () => {
    expect(sinceText(t, null)).toBe("还没记过");
    expect(sinceText(t, 0)).toBe("今天");
    expect(sinceText(t, 12)).toBe("12 天前");
    expect(
      statusText(t, {
        status: "soon",
        nextEventIn: 3,
        nextEventLabel: "生日",
        contactDueIn: null,
      }),
    ).toBe("生日 3 天后");
    expect(
      statusText(t, {
        status: "overdue",
        nextEventIn: null,
        contactDueIn: -4,
      }),
    ).toBe("该联系了 · 4 天");
    expect(statusText(t, { status: "ok" })).toBe("正常");
    expect(statusTone("soon")).toBe("accent");
    expect(statusTone("overdue")).toBe("warn");
    expect(statusTone("ok")).toBe("");
  });

  it("今天按本地日期", () => {
    expect(today(new Date(2026, 9, 5))).toBe("2026-10-05");
  });
});
