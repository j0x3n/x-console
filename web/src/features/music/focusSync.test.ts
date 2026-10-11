import { describe, expect, it } from "vitest";
import { planFocusEnd, planFocusStart, type FocusSettings } from "./focusSync";

const off: FocusSettings = {
  autoPlay: false,
  playlistId: 0,
  autoPause: false,
  onlyFocusList: false,
};
const here = { visible: true, playing: false, hasCurrent: true };

describe("专注开始", () => {
  it("默认什么都不做", () => {
    expect(planFocusStart(off, here)).toEqual({ kind: "none" });
  });
  it("开了自动播放：没指定列表就接着放当前队列", () => {
    expect(planFocusStart({ ...off, autoPlay: true }, here)).toEqual({
      kind: "resume",
    });
    expect(
      planFocusStart(
        { ...off, autoPlay: true },
        { ...here, hasCurrent: false },
      ),
    ).toEqual({ kind: "none" });
  });
  it("指定了列表：没在听歌时放这个列表", () => {
    expect(
      planFocusStart({ ...off, autoPlay: true, playlistId: 4 }, here),
    ).toEqual({ kind: "playlist", playlistId: 4, swapQueue: false });
  });
  it("已经在听歌时不打断，除非要求专注期间只用专注列表", () => {
    const playing = { ...here, playing: true };
    expect(
      planFocusStart({ ...off, autoPlay: true, playlistId: 4 }, playing),
    ).toEqual({
      kind: "none",
    });
    expect(
      planFocusStart(
        { ...off, autoPlay: true, playlistId: 4, onlyFocusList: true },
        playing,
      ),
    ).toEqual({ kind: "playlist", playlistId: 4, swapQueue: true });
  });
  it("页面没在看时不动，免得多个页面一起出声", () => {
    expect(
      planFocusStart(
        { ...off, autoPlay: true, playlistId: 4 },
        { ...here, visible: false },
      ),
    ).toEqual({ kind: "none" });
  });
  it("没开自动播放时，只用专注列表也不生效", () => {
    expect(
      planFocusStart({ ...off, playlistId: 4, onlyFocusList: true }, here),
    ).toEqual({
      kind: "none",
    });
  });
});

describe("专注结束", () => {
  it("开了自动暂停：正在放就暂停", () => {
    expect(
      planFocusEnd(
        { ...off, autoPause: true },
        { swapped: false, playing: true },
      ),
    ).toEqual({
      pause: true,
      restoreQueue: false,
    });
    expect(
      planFocusEnd(
        { ...off, autoPause: true },
        { swapped: false, playing: false },
      ),
    ).toEqual({
      pause: false,
      restoreQueue: false,
    });
  });
  it("没开自动暂停：什么都不暂停", () => {
    expect(planFocusEnd(off, { swapped: false, playing: true }).pause).toBe(
      false,
    );
  });
  it("换过队列：暂停了或没在放就换回，还在放就先不换", () => {
    expect(
      planFocusEnd(
        { ...off, autoPause: true },
        { swapped: true, playing: true },
      ).restoreQueue,
    ).toBe(true);
    expect(
      planFocusEnd(off, { swapped: true, playing: false }).restoreQueue,
    ).toBe(true);
    expect(
      planFocusEnd(off, { swapped: true, playing: true }).restoreQueue,
    ).toBe(false);
    expect(
      planFocusEnd(
        { ...off, autoPause: true },
        { swapped: false, playing: true },
      ).restoreQueue,
    ).toBe(false);
  });
});
