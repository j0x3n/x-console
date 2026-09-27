import { describe, expect, it } from "vitest";
import { tagColor, TAG_COLORS } from "./tagColor";

describe("tagColor", () => {
  it("uses the saved color, otherwise a stable one from the palette", () => {
    expect(tagColor("work", "#123456")).toBe("#123456");
    expect(TAG_COLORS).toContain(tagColor("work"));
    expect(tagColor("work")).toBe(tagColor("work"));
  });
});
