/*
 * 歌词的纯逻辑（B147）。歌词已经由服务端解析成行，这里只算“现在唱到哪一行”。
 */

export interface LyricLine {
  timeMs?: number;
  text: string;
}

/**
 * 当前行的位置：最后一个时间不晚于 ms 的行。还没唱到第一行时返回 -1。
 * 没有时间轴的歌词永远返回 -1。lines 要按时间排好（服务端保证）。
 */
export function activeLine(lines: LyricLine[], ms: number): number {
  let lo = 0;
  let hi = lines.length - 1;
  let found = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    const t = lines[mid].timeMs;
    if (t == null) return -1;
    if (t <= ms) {
      found = mid;
      lo = mid + 1;
    } else hi = mid - 1;
  }
  return found;
}

/** mm:ss，超过一小时写 h:mm:ss。 */
export function formatClock(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}
