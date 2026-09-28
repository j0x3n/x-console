/*
 * 日志级别判断（B28、B29、B31 共用）。有结构化级别时用它，
 * 没有时按每行里的关键字判断，判断不出来的算 other。
 */
export type LogLevel = "error" | "warn" | "info" | "debug" | "other";

const patterns: Array<[LogLevel, RegExp]> = [
  [
    "error",
    /\b(fatal|panic|crit(ical)?|emerg(ency)?|alert|err(or)?|exception|fail(ed|ure)?)\b/i,
  ],
  ["warn", /\b(warn(ing)?)\b/i],
  ["info", /\b(info|notice)\b/i],
  ["debug", /\b(debug|trace|verbose)\b/i],
];

export function detectLevel(text: string): LogLevel {
  for (const [level, re] of patterns) if (re.test(text)) return level;
  return "other";
}

/** journal 的 priority（0 到 7）换成级别。 */
export function levelFromPriority(p: number): LogLevel {
  if (p <= 3) return "error";
  if (p === 4) return "warn";
  if (p <= 6) return "info";
  return "debug";
}

/** 过滤选项：“警告及以上”包含错误。 */
export type LevelFilter = "all" | "error" | "warn" | "info" | "debug";

const rank: Record<LogLevel, number> = {
  error: 0,
  warn: 1,
  info: 2,
  debug: 3,
  other: 4,
};

export function matchesLevel(level: LogLevel, filter: LevelFilter): boolean {
  if (filter === "all") return true;
  // 按关键字判断不出来的行，只在“全部”里显示。
  if (level === "other") return false;
  return rank[level] <= rank[filter];
}

/** 把一行按关键字切开，方便高亮。不区分大小写。 */
export function splitHighlight(
  text: string,
  query: string,
): Array<{ text: string; hit: boolean }> {
  const q = query.trim();
  if (!q) return [{ text, hit: false }];
  const out: Array<{ text: string; hit: boolean }> = [];
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  let i = 0;
  while (i < text.length) {
    const j = lower.indexOf(needle, i);
    if (j < 0) {
      out.push({ text: text.slice(i), hit: false });
      break;
    }
    if (j > i) out.push({ text: text.slice(i, j), hit: false });
    out.push({ text: text.slice(j, j + needle.length), hit: true });
    i = j + needle.length;
  }
  return out;
}
