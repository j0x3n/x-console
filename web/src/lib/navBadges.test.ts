import { describe, expect, it } from "vitest";
import { badgeLabel, combineBadges } from "./navBadges";

describe("badgeLabel", () => {
  it("shows numbers up to 9 and a dot above", () => {
    expect(badgeLabel(1)).toBe("1");
    expect(badgeLabel(9)).toBe("9");
    expect(badgeLabel(10)).toBe("");
  });
});

describe("combineBadges", () => {
  it("adds up members, keeps danger and jumps to the first one with a count", () => {
    expect(
      combineBadges([
        { path: "/calendar", badge: null },
        { path: "/reminders", badge: { count: 2, title: "2 条提醒已到期" } },
        {
          path: "/inbox",
          badge: { count: 3, tone: "warn", title: "3 条没处理" },
        },
      ]),
    ).toEqual({
      count: 5,
      tone: "danger",
      title: "2 条提醒已到期；3 条没处理",
      to: "/reminders",
    });
  });

  it("uses warn when every member is warn, and the member's own target", () => {
    expect(
      combineBadges([
        {
          path: "/inbox",
          badge: {
            count: 1,
            tone: "warn",
            title: "1 条没处理",
            to: "/inbox?x",
          },
        },
      ]),
    ).toMatchObject({ tone: "warn", to: "/inbox?x" });
  });

  it("returns null when nothing has a count", () => {
    expect(
      combineBadges([{ path: "/a", badge: { count: 0, title: "" } }]),
    ).toBeNull();
  });
});
