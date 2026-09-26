/*
 * 重复规则的常用选项和 RRULE 字符串之间的转换。
 * 服务端按用户时区展开规则，时刻取自第一次的时间，所以这里只拼频率和星期。
 */

export type RepeatPreset =
  | "none"
  | "daily"
  | "weekdays"
  | "weekly"
  | "monthly"
  | "yearly"
  | "custom";

export const repeatPresets: RepeatPreset[] = [
  "none",
  "daily",
  "weekdays",
  "weekly",
  "monthly",
  "yearly",
  "custom",
];

const WEEKDAYS_RULE = "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR";
const DAY_CODES = ["SU", "MO", "TU", "WE", "TH", "FR", "SA"];
const DAY_NAMES_ZH = ["日", "一", "二", "三", "四", "五", "六"];
const DAY_NAMES_EN = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

/** 去掉 RRULE: 前缀和空白，统一大写。 */
export function normalizeRule(rule: string): string {
  return rule
    .trim()
    .replace(/^RRULE:/i, "")
    .toUpperCase();
}

/** 常用选项拼成 RRULE。custom 原样使用用户输入。 */
export function buildRRule(preset: RepeatPreset, custom = ""): string {
  switch (preset) {
    case "none":
      return "";
    case "daily":
      return "FREQ=DAILY";
    case "weekdays":
      return WEEKDAYS_RULE;
    case "weekly":
      return "FREQ=WEEKLY";
    case "monthly":
      return "FREQ=MONTHLY";
    case "yearly":
      return "FREQ=YEARLY";
    case "custom":
      return normalizeRule(custom);
  }
}

/** 编辑已有提醒时，从 RRULE 反推常用选项。 */
export function detectPreset(rule: string): RepeatPreset {
  const r = normalizeRule(rule);
  if (r === "") return "none";
  const known: RepeatPreset[] = [
    "daily",
    "weekdays",
    "weekly",
    "monthly",
    "yearly",
  ];
  return known.find((p) => buildRRule(p) === r) ?? "custom";
}

function parts(rule: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const part of normalizeRule(rule).split(";")) {
    const [key, value] = part.split("=");
    if (key && value !== undefined) out.set(key, value);
  }
  return out;
}

/** 简单检查格式：必须有合法的 FREQ。真正的校验在服务端。 */
export function isValidRule(rule: string): boolean {
  const r = normalizeRule(rule);
  if (r === "") return true;
  if (!/^[A-Z]+=[A-Z0-9,+-]+(;[A-Z]+=[A-Z0-9,+-]+)*$/.test(r)) return false;
  const freq = parts(r).get("FREQ");
  return (
    freq !== undefined &&
    ["YEARLY", "MONTHLY", "WEEKLY", "DAILY", "HOURLY", "MINUTELY"].includes(
      freq,
    )
  );
}

/** 给人看的重复说明，例如“每天”“每周四”“每月 5 号”。 */
export function describeRule(
  rule: string,
  start: Date,
  language: "zh" | "en" = "zh",
): string {
  const r = normalizeRule(rule);
  const zh = language === "zh";
  if (r === "") return zh ? "不重复" : "Once";
  if (r === WEEKDAYS_RULE) return zh ? "工作日" : "Weekdays";
  const p = parts(r);
  const interval = Number(p.get("INTERVAL") ?? "1") || 1;
  const every = (unitZh: string, unitEn: string) =>
    interval === 1
      ? zh
        ? `每${unitZh}`
        : `Every ${unitEn}`
      : zh
        ? `每 ${interval} ${unitZh}`
        : `Every ${interval} ${unitEn}s`;
  const days = (p.get("BYDAY") ?? DAY_CODES[start.getDay()])
    .split(",")
    .map((code) => DAY_CODES.indexOf(code.replace(/^[+-]?\d+/, "")))
    .filter((i) => i >= 0);
  switch (p.get("FREQ")) {
    case "DAILY":
      return every("天", "day");
    case "WEEKLY": {
      const names = days.map((i) => (zh ? DAY_NAMES_ZH : DAY_NAMES_EN)[i]);
      const base = interval === 1 ? (zh ? "每周" : "Weekly on ") : every("周", "week") + (zh ? "的周" : " on ");
      return zh
        ? `${base}${names.join("、")}`
        : `${base}${names.join(", ")}`;
    }
    case "MONTHLY": {
      const day = p.get("BYMONTHDAY") ?? String(start.getDate());
      return zh
        ? `${every("月", "month")} ${day} 号`
        : `${every("month", "month")} on day ${day}`;
    }
    case "YEARLY":
      return zh
        ? `每年 ${start.getMonth() + 1} 月 ${start.getDate()} 日`
        : `Every year on ${start.getMonth() + 1}/${start.getDate()}`;
    case "HOURLY":
      return every("小时", "hour");
    case "MINUTELY":
      return every("分钟", "minute");
  }
  return r;
}

/** Date 转成 <input type="datetime-local"> 需要的本地时间字符串。 */
export function toLocalInput(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}`
  );
}

/** datetime-local 的值按浏览器本地时间解析。无效时返回 null。 */
export function fromLocalInput(value: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value);
  if (!m) return null;
  const date = new Date(
    Number(m[1]),
    Number(m[2]) - 1,
    Number(m[3]),
    Number(m[4]),
    Number(m[5]),
  );
  return Number.isNaN(date.getTime()) ? null : date;
}

/** 提醒时间的短格式：今天 14:05、明天 09:00、10月3日 周六 09:00。 */
export function formatWhen(
  value: string | Date,
  now: Date,
  language: "zh" | "en" = "zh",
): string {
  const date = typeof value === "string" ? new Date(value) : value;
  const pad = (n: number) => String(n).padStart(2, "0");
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  const dayDiff = Math.round(
    (new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime() -
      new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()) /
      86_400_000,
  );
  const zh = language === "zh";
  if (dayDiff === 0) return `${zh ? "今天" : "Today"} ${time}`;
  if (dayDiff === 1) return `${zh ? "明天" : "Tomorrow"} ${time}`;
  if (dayDiff === -1) return `${zh ? "昨天" : "Yesterday"} ${time}`;
  const day = date.toLocaleDateString(zh ? "zh-CN" : "en", {
    month: "short",
    day: "numeric",
    weekday: "short",
    year: date.getFullYear() === now.getFullYear() ? undefined : "numeric",
  });
  return `${day} ${time}`;
}

/** 新建提醒的默认时间：下一个整点后 0 分，至少 5 分钟以后。 */
export function defaultStart(now: Date): Date {
  const d = new Date(now);
  d.setSeconds(0, 0);
  d.setMinutes(0);
  d.setHours(d.getHours() + 1);
  if (d.getTime() - now.getTime() < 5 * 60_000) d.setHours(d.getHours() + 1);
  return d;
}
