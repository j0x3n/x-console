import { describe, expect, it } from "vitest";
import { badgeLabel } from "./navBadges";

describe("badgeLabel", () => {
  it("shows numbers up to 9 and a dot above", () => {
    expect(badgeLabel(1)).toBe("1");
    expect(badgeLabel(9)).toBe("9");
    expect(badgeLabel(10)).toBe("");
  });
});
