import { describe, expect, it } from "vitest";
import { joinMemo, splitMemo } from "./memoText";

describe("Keep style memo text", () => {
  it("leaves text without images alone", () => {
    const body = "  第一行\n\n- [ ] 待办\n";
    expect(splitMemo(body)).toEqual({ images: [], text: body });
    expect(joinMemo([], body)).toBe(body);
  });
  it("moves image lines to the top and back", () => {
    const body = "![猫](/api/v1/notes/attachments/3)\n\n喂猫\n![](/x.png)";
    const { images, text } = splitMemo(body);
    expect(images.map((x) => x.src)).toEqual([
      "/api/v1/notes/attachments/3",
      "/x.png",
    ]);
    expect(text).toBe("喂猫");
    expect(
      joinMemo(
        images.map((x) => x.line),
        text,
      ),
    ).toBe("![猫](/api/v1/notes/attachments/3)\n\n![](/x.png)\n\n喂猫");
  });
  it("keeps an image inside a sentence as text", () => {
    expect(splitMemo("看 ![](/a.png) 这个").images).toHaveLength(0);
  });
});
