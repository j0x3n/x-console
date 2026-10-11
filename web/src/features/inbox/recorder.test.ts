import { describe, expect, it } from "vitest";
import { pickMime } from "./recorder";

describe("录音格式", () => {
  it("Chrome 支持 webm 就用带 opus 的 webm", () => {
    expect(pickMime((t) => t.startsWith("audio/webm"))).toBe(
      "audio/webm;codecs=opus",
    );
  });
  it("Safari 只支持 mp4", () => {
    expect(pickMime((t) => t === "audio/mp4")).toBe("audio/mp4");
  });
  it("一种都不支持时让浏览器自己挑", () => {
    expect(pickMime(() => false)).toBeUndefined();
  });
});
