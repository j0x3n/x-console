import { describe, expect, it } from "vitest";
import {
  dueText,
  formatDays,
  parseDays,
  parseLines,
  plusDays,
  statusTone,
  suggestExpiry,
} from "./format";

const t = (s: string) => s;

describe("credentials format", () => {
  it("提醒天数：去重、从大到小、拒绝越界", () => {
    expect(parseDays("7, 30 30，90")).toEqual([90, 30, 7]);
    expect(parseDays("")).toEqual([]);
    expect(parseDays("0")).toBeNull();
    expect(parseDays("3651")).toBeNull();
    expect(parseDays("a")).toBeNull();
    expect(parseDays("1 2 3 4 5 6 7 8 9")).toBeNull();
    expect(formatDays([30, 7])).toBe("30, 7");
  });

  it("用在哪里：一行一项，去空行和重复", () => {
    expect(parseLines(" 服务器 a \n\n项目 b\r\n服务器 a\n")).toEqual([
      "服务器 a",
      "项目 b",
    ]);
  });

  it("状态标签：过期红色，快到期和久未更换琥珀色", () => {
    expect(statusTone("expired")).toBe("danger");
    expect(statusTone("soon")).toBe("warn");
    expect(statusTone("stale")).toBe("warn");
    expect(statusTone("ok")).toBe("");
    expect(statusTone("none")).toBe("");
  });

  it("标签文字说最急的一项", () => {
    expect(dueText(t, { status: "expired", expiresIn: -3 })).toBe(
      "Expired 3 days",
    );
    expect(dueText(t, { status: "soon", expiresIn: 12 })).toBe("12 days left");
    expect(dueText(t, { status: "soon", expiresIn: 0 })).toBe("Expires today");
    expect(
      dueText(t, { status: "stale", expiresIn: 200, rotateDueIn: -10 }),
    ).toBe("Rotation overdue 10 days");
    expect(
      dueText(t, { status: "stale", expiresIn: null, rotateDueIn: 0 }),
    ).toBe("Rotation due today");
    expect(dueText(t, { status: "ok", expiresIn: null, rotateDueIn: 40 })).toBe(
      "40 days until rotation",
    );
    expect(dueText(t, { status: "none" })).toBe("No expiry date");
  });

  it("更换时新到期日沿用上一次的有效期长度", () => {
    expect(
      suggestExpiry(
        { createdOn: "2026-01-01", rotatedOn: "", expiresOn: "2026-04-11" },
        "2026-04-10",
      ),
    ).toBe("2026-07-19");
    expect(
      suggestExpiry(
        {
          createdOn: "2026-01-01",
          rotatedOn: "2026-03-01",
          expiresOn: "2026-05-30",
        },
        "2026-05-30",
      ),
    ).toBe("2026-08-28");
    expect(
      suggestExpiry(
        { createdOn: "", rotatedOn: "", expiresOn: "2026-05-30" },
        "2026-05-30",
      ),
    ).toBe("");
    expect(
      suggestExpiry(
        { createdOn: "2026-01-01", rotatedOn: "", expiresOn: "" },
        "2026-05-30",
      ),
    ).toBe("");
  });

  it("plusDays 用本地日期", () => {
    expect(plusDays(1, new Date(2026, 9, 31, 23, 30))).toBe("2026-11-01");
    expect(plusDays(-1, new Date(2026, 0, 1))).toBe("2025-12-31");
  });
});
