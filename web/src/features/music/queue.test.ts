import { describe, expect, it } from "vitest";
import {
  appendUnique,
  insertAfter,
  moveItem,
  nextMode,
  nextStep,
  playOrder,
  prevStep,
  removeFrom,
  shuffleKey,
} from "./queue";

const ids = [10, 20, 30, 40];
const song = (id: number) => ({ id });

describe("随机顺序", () => {
  it("同一个种子顺序不变，不同种子顺序不同", () => {
    expect(shuffleKey(1, 5)).toBe(shuffleKey(1, 5));
    const a = playOrder(ids, "shuffle", 1);
    const b = playOrder(ids, "shuffle", 2);
    expect([...a].sort()).toEqual(ids);
    expect(playOrder(ids, "shuffle", 1)).toEqual(a);
    expect(a).not.toEqual(b);
  });
  it("队列里加歌不会打乱原来歌之间的相对顺序", () => {
    const before = playOrder(ids, "shuffle", 7);
    const after = playOrder([...ids, 50, 60], "shuffle", 7).filter((i) =>
      ids.includes(i),
    );
    expect(after).toEqual(before);
  });
  it("非随机模式保持队列顺序", () => {
    expect(playOrder(ids, "sequence", 9)).toEqual(ids);
    expect(playOrder(ids, "loop", 9)).toEqual(ids);
  });
});

describe("下一首", () => {
  it("顺序播放：到最后一首停下", () => {
    expect(nextStep(ids, 20, "sequence", 0, "ended")).toEqual({
      id: 30,
      wrapped: false,
    });
    expect(nextStep(ids, 40, "sequence", 0, "ended")).toEqual({
      id: null,
      wrapped: false,
    });
  });
  it("列表循环：最后一首回到开头", () => {
    expect(nextStep(ids, 40, "loop", 0, "ended")).toEqual({
      id: 10,
      wrapped: true,
    });
  });
  it("单曲循环：自然播完重播，手动下一首照常往后", () => {
    expect(nextStep(ids, 20, "one", 0, "ended").id).toBe(20);
    expect(nextStep(ids, 20, "one", 0, "manual").id).toBe(30);
  });
  it("随机：走完一轮后标记绕回", () => {
    const order = playOrder(ids, "shuffle", 3);
    expect(nextStep(ids, order[0], "shuffle", 3, "ended").id).toBe(order[1]);
    const last = nextStep(ids, order[3], "shuffle", 3, "ended");
    expect(last).toEqual({ id: order[0], wrapped: true });
  });
  it("当前歌不在队列里：从头开始。空队列：没有下一首", () => {
    expect(nextStep(ids, 99, "sequence", 0, "manual").id).toBe(10);
    expect(nextStep(ids, null, "sequence", 0, "manual").id).toBe(10);
    expect(nextStep([], 1, "loop", 0, "manual").id).toBeNull();
  });
});

describe("上一首", () => {
  it("往前一首", () => {
    expect(prevStep(ids, 30, "sequence", 0)).toBe(20);
  });
  it("第一首：顺序播放重来这一首，列表循环绕到最后", () => {
    expect(prevStep(ids, 10, "sequence", 0)).toBe(10);
    expect(prevStep(ids, 10, "loop", 0)).toBe(40);
  });
  it("空队列没有上一首", () => {
    expect(prevStep([], null, "loop", 0)).toBeNull();
  });
});

describe("队列编辑", () => {
  it("moveItem 挪动并且不改原数组", () => {
    const list = [1, 2, 3, 4];
    expect(moveItem(list, 0, 2)).toEqual([2, 3, 1, 4]);
    expect(moveItem(list, 3, 0)).toEqual([4, 1, 2, 3]);
    expect(list).toEqual([1, 2, 3, 4]);
    expect(moveItem(list, 1, 1)).toBe(list);
    expect(moveItem(list, -1, 2)).toBe(list);
    expect(moveItem(list, 1, 9)).toBe(list);
  });
  it("下一首播放：插在当前歌后面，已有的歌挪过来而不是出现两份", () => {
    const q = [song(1), song(2), song(3)];
    expect(insertAfter(q, 1, [song(3)]).map((s) => s.id)).toEqual([1, 3, 2]);
    expect(insertAfter(q, 2, [song(4), song(5)]).map((s) => s.id)).toEqual([
      1, 2, 4, 5, 3,
    ]);
    expect(insertAfter(q, null, [song(9)]).map((s) => s.id)).toEqual([
      9, 1, 2, 3,
    ]);
    // 当前歌自己被“下一首播放”时不动
    expect(insertAfter(q, 2, [song(2)]).map((s) => s.id)).toEqual([1, 2, 3]);
  });
  it("加入队列：末尾追加，重复的跳过", () => {
    const q = [song(1), song(2)];
    expect(
      appendUnique(q, [song(2), song(3), song(3)]).map((s) => s.id),
    ).toEqual([1, 2, 3]);
    expect(appendUnique(q, [song(1)])).toBe(q);
  });
  it("删掉正在播的歌：接着播后面一首", () => {
    const q = [song(1), song(2), song(3)];
    expect(removeFrom(q, 2, 2, "sequence", 0)).toEqual({
      list: [song(1), song(3)],
      current: 3,
    });
    expect(removeFrom(q, 3, 3, "sequence", 0).current).toBeNull();
    expect(removeFrom(q, 1, 2, "sequence", 0).current).toBe(2);
    expect(removeFrom([song(1)], 1, 1, "loop", 0)).toEqual({
      list: [],
      current: null,
    });
  });
});

describe("播放模式切换", () => {
  it("四种模式循环", () => {
    expect(nextMode("sequence")).toBe("loop");
    expect(nextMode("loop")).toBe("one");
    expect(nextMode("one")).toBe("shuffle");
    expect(nextMode("shuffle")).toBe("sequence");
  });
});
