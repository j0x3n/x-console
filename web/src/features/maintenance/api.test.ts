import { describe, expect, it } from "vitest";
import { uptimeText, versionMismatch } from "./api";

describe("maintenance helpers", () => {
  it("formats how long the server has been up", () => {
    const now = Date.parse("2026-10-02T12:00:00Z");
    expect(uptimeText("2026-10-02T11:52:00Z", now)).toBe("8 分钟");
    expect(uptimeText("2026-10-02T06:48:00Z", now)).toBe("5 小时 12 分");
    expect(uptimeText("2026-09-28T08:00:00Z", now)).toBe("4 天 4 小时");
  });

  it("only flags a mismatch between real versions", () => {
    expect(versionMismatch("a1b2c3d", "a1b2c3d")).toBe(false);
    expect(versionMismatch("a1b2c3d", "e4f5a6b")).toBe(true);
    expect(versionMismatch("dev", "e4f5a6b")).toBe(false);
    expect(versionMismatch("a1b2c3d", undefined)).toBe(false);
  });
});
