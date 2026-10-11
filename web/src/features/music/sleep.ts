/*
 * 定时暂停的纯逻辑（B149）。
 */

/** 剩余毫秒。已经过了返回 0。 */
export function remainingMs(endsAt: string | undefined, now: number): number {
  if (!endsAt) return 0;
  const t = Date.parse(endsAt);
  return Number.isFinite(t) ? Math.max(0, t - now) : 0;
}

/** 最后 10 秒音量渐小。返回 0 到 1 的倍数，剩余多于 10 秒是 1。 */
export const FADE_MS = 10_000;
export function fadeFactor(remaining: number): number {
  if (remaining >= FADE_MS) return 1;
  if (remaining <= 0) return 0;
  return remaining / FADE_MS;
}

/** 剩余时间的显示：不到 1 小时写 mm:ss，否则 h:mm:ss。 */
export function formatRemaining(ms: number): string {
  const total = Math.ceil(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`;
}

/**
 * 一首歌自然播完时按首数定时怎么办。left 是播完这首之前剩的首数：
 * 只剩 1 首（就是这首）→ 停下并取消；多于 1 首 → 更新剩余数，接着播。
 */
export type TrackStep =
  | { action: "stop" }
  | { action: "continue"; left: number }
  | { action: "ignore" };

export function afterTrackEnded(
  mode: "time" | "tracks" | undefined,
  tracksLeft: number | undefined,
): TrackStep {
  if (mode !== "tracks" || tracksLeft == null || tracksLeft < 1)
    return { action: "ignore" };
  if (tracksLeft <= 1) return { action: "stop" };
  return { action: "continue", left: tracksLeft - 1 };
}
