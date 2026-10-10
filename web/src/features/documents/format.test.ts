import { describe, expect, it } from "vitest";
import {
  daysText,
  formatDays,
  matchesView,
  parseDays,
  statusTone,
} from "./format";
import type { DocumentItem } from "./api";

const t = (s: string) => s;

describe("证件档案的显示逻辑", () => {
  it("剩余天数的文字", () => {
    expect(daysText(t, null)).toBe("No expiry date");
    expect(daysText(t, 0)).toBe("Expires today");
    expect(daysText(t, 20)).toBe("20 days left");
    expect(daysText(t, -3)).toBe("Expired 3 days");
  });

  it("状态标签的颜色", () => {
    expect(statusTone("expired", -1)).toBe("danger");
    expect(statusTone("soon", 20)).toBe("warn");
    expect(statusTone("soon", 80)).toBe("info");
    expect(statusTone("ok", 400)).toBe("");
    expect(statusTone("none", null)).toBe("");
  });

  it("提醒天数输入：去重、从大到小、拒绝不合法的", () => {
    expect(parseDays("7, 90 30，30")).toEqual([90, 30, 7]);
    expect(parseDays("")).toEqual([]);
    expect(parseDays("0")).toBeNull();
    expect(parseDays("3651")).toBeNull();
    expect(parseDays("abc")).toBeNull();
    expect(parseDays("1 2 3 4 5 6 7 8 9")).toBeNull();
    expect(formatDays([90, 30, 7])).toBe("90, 30, 7");
  });

  it("视图筛选", () => {
    const d = (status: DocumentItem["status"]) => ({ status }) as DocumentItem;
    expect(matchesView(d("expired"), "expired")).toBe(true);
    expect(matchesView(d("soon"), "expired")).toBe(false);
    expect(matchesView(d("soon"), "soon")).toBe(true);
    expect(matchesView(d("none"), "all")).toBe(true);
  });
});
