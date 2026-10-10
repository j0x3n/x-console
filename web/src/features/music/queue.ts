/*
 * 播放队列的纯逻辑（B147）：播放顺序、上一首下一首、拖动排序。
 * 随机模式不改队列本身，只给每首歌按种子算一个排序值。这样队列里加歌、删歌
 * 不会打乱已有的顺序，也不用另存一份原始队列。
 */

export type PlayMode = "sequence" | "loop" | "one" | "shuffle";

export const PLAY_MODES: PlayMode[] = ["sequence", "loop", "one", "shuffle"];

/** 随机模式下一首歌的排序值。同一个种子和编号永远得到同一个值。 */
export function shuffleKey(seed: number, id: number): number {
  let h = (seed ^ Math.imul(id, 0x9e3779b1)) >>> 0;
  h = Math.imul(h ^ (h >>> 16), 0x85ebca6b) >>> 0;
  h = Math.imul(h ^ (h >>> 13), 0xc2b2ae35) >>> 0;
  return (h ^ (h >>> 16)) >>> 0;
}

/** 实际的播放顺序（歌曲编号）。随机模式按排序值，其他模式就是队列顺序。 */
export function playOrder(
  ids: number[],
  mode: PlayMode,
  seed: number,
): number[] {
  if (mode !== "shuffle") return ids;
  return [...ids].sort(
    (a, b) => shuffleKey(seed, a) - shuffleKey(seed, b) || a - b,
  );
}

export interface Step {
  /** 下一首的编号。null 表示播完了，停下。 */
  id: number | null;
  /** 绕回了开头。随机模式要换一个种子，下一轮的顺序才不一样。 */
  wrapped: boolean;
}

/**
 * 下一首。reason 是 "ended"（自然播完）或 "manual"（点了下一首）：
 * 单曲循环自然播完时重播这一首，手动点下一首时照常往后走。
 */
export function nextStep(
  ids: number[],
  current: number | null,
  mode: PlayMode,
  seed: number,
  reason: "ended" | "manual",
): Step {
  if (ids.length === 0) return { id: null, wrapped: false };
  if (mode === "one" && reason === "ended" && current != null)
    return { id: current, wrapped: false };
  const order = playOrder(ids, mode, seed);
  const pos = current == null ? -1 : order.indexOf(current);
  if (pos >= 0 && pos < order.length - 1)
    return { id: order[pos + 1], wrapped: false };
  if (pos < 0) return { id: order[0], wrapped: false };
  // 最后一首之后：顺序播放停下，其他模式回到开头。
  if (mode === "sequence") return { id: null, wrapped: false };
  return { id: order[0], wrapped: true };
}

/**
 * 上一首。第一首时：顺序播放回到这首的开头（返回同一首），其他模式绕到最后一首。
 */
export function prevStep(
  ids: number[],
  current: number | null,
  mode: PlayMode,
  seed: number,
): number | null {
  if (ids.length === 0) return null;
  const order = playOrder(ids, mode, seed);
  const pos = current == null ? -1 : order.indexOf(current);
  if (pos > 0) return order[pos - 1];
  if (pos === 0) return mode === "sequence" ? current : order[order.length - 1];
  return order[0];
}

/** 把第 from 项挪到第 to 项的位置。越界或不动时原样返回。 */
export function moveItem<T>(list: T[], from: number, to: number): T[] {
  if (
    from === to ||
    from < 0 ||
    to < 0 ||
    from >= list.length ||
    to >= list.length
  )
    return list;
  const next = [...list];
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

/** 在 anchor 这一项后面插入 items，已经在队列里的歌先拿掉再插，不会出现两份。 */
export function insertAfter<T extends { id: number }>(
  list: T[],
  anchor: number | null,
  items: T[],
): T[] {
  const incoming = new Set(items.map((i) => i.id));
  const rest = list.filter((i) => !incoming.has(i.id) || i.id === anchor);
  const at = anchor == null ? -1 : rest.findIndex((i) => i.id === anchor);
  const unique = items.filter(
    (item, i) =>
      item.id !== anchor && items.findIndex((x) => x.id === item.id) === i,
  );
  return [...rest.slice(0, at + 1), ...unique, ...rest.slice(at + 1)];
}

/** 加到队列末尾，已经在队列里的歌跳过。 */
export function appendUnique<T extends { id: number }>(
  list: T[],
  items: T[],
): T[] {
  const have = new Set(list.map((i) => i.id));
  const add = items.filter(
    (item, i) =>
      !have.has(item.id) && items.findIndex((x) => x.id === item.id) === i,
  );
  return add.length ? [...list, ...add] : list;
}

/** 从队列里拿掉一首。拿掉的是正在播的歌时，返回接着播哪一首（可能没有）。 */
export function removeFrom<T extends { id: number }>(
  list: T[],
  id: number,
  current: number | null,
  mode: PlayMode,
  seed: number,
): { list: T[]; current: number | null } {
  const next = list.filter((i) => i.id !== id);
  if (id !== current) return { list: next, current };
  const ids = list.map((i) => i.id);
  const step = nextStep(
    ids,
    id,
    mode === "one" ? "loop" : mode,
    seed,
    "manual",
  );
  const after = step.id != null && step.id !== id ? step.id : null;
  return { list: next, current: after };
}

const MODE_LABELS: Record<PlayMode, string> = {
  sequence: "Play in order",
  loop: "Repeat the queue",
  one: "Repeat one song",
  shuffle: "Shuffle",
};

export function modeLabel(mode: PlayMode): string {
  return MODE_LABELS[mode];
}

export function nextMode(mode: PlayMode): PlayMode {
  return PLAY_MODES[(PLAY_MODES.indexOf(mode) + 1) % PLAY_MODES.length];
}
