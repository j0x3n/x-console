import { describe, expect, it } from "vitest";
import { clip, isExternal, scoreText } from "./format";

describe("knowledge format", () => {
  it("tells web addresses from in-app paths", () => {
    expect(isExternal("https://example.com/a")).toBe(true);
    expect(isExternal("HTTP://example.com")).toBe(true);
    expect(isExternal("/notes/3")).toBe(false);
    expect(isExternal("/journal?date=2026-11-11")).toBe(false);
  });

  it("writes the score as a percentage", () => {
    expect(scoreText(0.834)).toBe("83%");
    expect(scoreText(1)).toBe("100%");
  });

  it("clips long passages by characters and squeezes white space", () => {
    expect(clip("短 \n 文本")).toBe("短 文本");
    expect(clip("字".repeat(300), 10)).toBe(`${"字".repeat(10)}…`);
    expect(clip("字".repeat(10), 10)).toBe("字".repeat(10));
  });
});
