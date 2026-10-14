/*
 * 从步骤正文里认出时间（B209）：“20 分钟”“半小时”“一两分钟”“20 到 40 分钟”。
 * 认出来的时间在做菜页变成计时按钮，点了才开始。
 * 只认秒、分钟、小时。“半年”“12 到 18 个月”这种不认。
 */

export interface Duration {
  /** 原文里的那一段，比如“一两分钟” */
  text: string;
  seconds: number;
  /** 是范围时的上限 */
  maxSeconds?: number;
}

const CN_DIGITS: Record<string, number> = {
  零: 0,
  一: 1,
  二: 2,
  两: 2,
  三: 3,
  四: 4,
  五: 5,
  六: 6,
  七: 7,
  八: 8,
  九: 9,
};

/** 一到九十九的中文数字。认不出返回 null。 */
function parseCnInt(s: string): number | null {
  if (s === "十") return 10;
  const ten = s.indexOf("十");
  if (ten >= 0) {
    if (s.indexOf("十", ten + 1) >= 0) return null;
    const head = s.slice(0, ten);
    const tail = s.slice(ten + 1);
    const tens = head === "" ? 1 : CN_DIGITS[head];
    const ones = tail === "" ? 0 : CN_DIGITS[tail];
    if (tens === undefined || ones === undefined) return null;
    if (head.length > 1 || tail.length > 1) return null;
    return tens * 10 + ones;
  }
  if (s.length === 1) return CN_DIGITS[s] ?? null;
  return null;
}

/**
 * 一个数，或者“一两”“三五”这种两个挨着的中文数字（表示范围）。
 * 返回下限和上限，不是范围时两个一样。
 */
function parseNumber(s: string): { min: number; max: number } | null {
  if (/^\d+(\.\d+)?$/.test(s)) {
    const n = Number(s);
    return { min: n, max: n };
  }
  const n = parseCnInt(s);
  if (n !== null) return { min: n, max: n };
  if (s.length === 2 && !s.includes("十")) {
    const a = CN_DIGITS[s[0]];
    const b = CN_DIGITS[s[1]];
    if (a !== undefined && b !== undefined && a > 0 && b > a)
      return { min: a, max: b };
  }
  return null;
}

const NUM = "(\\d+(?:\\.\\d+)?|[零一二两三四五六七八九十]{1,3})";
const RANGE = "(?:\\s*(?:到|至|-|~|～|－|–|—)\\s*" + NUM + ")?";
const UNIT = "(小时|钟头|分钟|秒钟|秒)";
const PATTERN = new RegExp(
  "(?:" + NUM + RANGE + "\\s*(个)?)?\\s*(半)?\\s*(?:个)?\\s*" + UNIT,
  "g",
);

const UNIT_SECONDS: Record<string, number> = {
  小时: 3600,
  钟头: 3600,
  分钟: 60,
  秒钟: 1,
  秒: 1,
};

/** 一步正文里的全部时间，按出现的顺序，去掉重复的。 */
export function detectDurations(text: string): Duration[] {
  const out: Duration[] = [];
  const seen = new Set<string>();
  for (const m of text.matchAll(PATTERN)) {
    const [whole, a, b, , half, unit] = m;
    if (!a && !half) continue;
    const unitSeconds = UNIT_SECONDS[unit];
    let min: number;
    let max: number;
    if (a) {
      const first = parseNumber(a);
      if (!first) continue;
      min = first.min;
      max = first.max;
      if (b) {
        const second = parseNumber(b);
        if (!second) continue;
        max = second.max;
      }
    } else {
      min = 0;
      max = 0;
    }
    // “半小时”是 0.5，“一个半小时”是 1.5
    if (half) {
      min += 0.5;
      max += 0.5;
    }
    const seconds = Math.round(min * unitSeconds);
    const maxSeconds = Math.round(max * unitSeconds);
    if (seconds < 1 || seconds > 86400 || maxSeconds > 86400) continue;
    const d: Duration = { text: whole.trim(), seconds };
    if (maxSeconds > seconds) d.maxSeconds = maxSeconds;
    const key = `${d.seconds}-${d.maxSeconds ?? ""}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(d);
  }
  return out;
}

/** 计时按钮和剩余时间上的写法：“30 分钟”“1 小时 30 分钟”“45 秒”。 */
export function durationLabel(seconds: number): string {
  if (seconds < 60) return `${seconds} 秒`;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const parts: string[] = [];
  if (h) parts.push(`${h} 小时`);
  if (m) parts.push(`${m} 分钟`);
  if (s) parts.push(`${s} 秒`);
  return parts.join(" ");
}

/** 倒数的写法：“29:58”“1:05:00”。 */
export function clock(seconds: number): string {
  const v = Math.max(0, Math.round(seconds));
  const h = Math.floor(v / 3600);
  const m = Math.floor((v % 3600) / 60);
  const s = v % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}
