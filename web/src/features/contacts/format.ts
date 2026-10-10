import { Briefcase, Heart, User, Users, type LucideIcon } from "lucide-react";
import type {
  ContactEvent,
  ContactEventKind,
  ContactGroup,
  ContactItem,
  ContactStatus,
} from "./api";

export const GROUPS: ContactGroup[] = [
  "family",
  "friend",
  "colleague",
  "other",
];

/** 英文键，中文在 i18n.ts。 */
export const GROUP_LABELS: Record<ContactGroup, string> = {
  family: "Family",
  friend: "Friends",
  colleague: "Colleagues",
  other: "Other",
};

export const GROUP_ICONS: Record<ContactGroup, LucideIcon> = {
  family: Heart,
  friend: Users,
  colleague: Briefcase,
  other: User,
};

export const EVENT_KINDS: ContactEventKind[] = [
  "birthday",
  "anniversary",
  "other",
];

export const EVENT_KIND_LABELS: Record<ContactEventKind, string> = {
  birthday: "Birthday",
  anniversary: "Anniversary",
  other: "Other date",
};

/** 状态对应的标签样式：近期琥珀色，该联系了橙色。 */
export function statusTone(status: ContactStatus) {
  if (status === "soon") return "accent";
  if (status === "overdue") return "warn";
  return "";
}

/** 一个重要日期的说明：“12 天后”“今天”，知道年份时带上几岁或几周年。 */
export function eventText(
  t: (key: string) => string,
  e: Pick<ContactEvent, "kind" | "nextIn" | "years">,
): string {
  const when =
    e.nextIn === 0 ? t("Today") : `${e.nextIn} ${t("days from now")}`;
  if (e.years == null || e.years <= 0) return when;
  const age =
    e.kind === "birthday"
      ? `${e.years} ${t("years old")}`
      : `${e.years} ${t("anniversary years")}`;
  return `${when} · ${age}`;
}

/** 距上次联系的说明。 */
export function sinceText(
  t: (key: string) => string,
  since: number | null | undefined,
): string {
  if (since == null) return t("No contact recorded");
  if (since === 0) return t("Today");
  return `${since} ${t("days ago")}`;
}

/** 列表右边标签的文字，按状态说最急的那一项。 */
export function statusText(
  t: (key: string) => string,
  c: Pick<
    ContactItem,
    "status" | "nextEventIn" | "nextEventLabel" | "contactDueIn"
  >,
): string {
  if (c.status === "soon" && c.nextEventIn != null) {
    const label = c.nextEventLabel ?? "";
    return c.nextEventIn === 0
      ? `${label} ${t("Today")}`
      : `${label} ${c.nextEventIn} ${t("days from now")}`;
  }
  if (c.status === "overdue" && c.contactDueIn != null)
    return `${t("Time to get in touch")} · ${-c.contactDueIn} ${t("days over")}`;
  return t("All good");
}

/** 提醒天数输入框的文字转数组：“7, 1” 变成 [7, 1]。 */
export function parseDays(text: string): number[] | null {
  const parts = text
    .split(/[\s,，、]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  const out: number[] = [];
  for (const p of parts) {
    if (!/^\d+$/.test(p)) return null;
    const n = Number(p);
    if (n < 1 || n > 365) return null;
    if (!out.includes(n)) out.push(n);
  }
  if (out.length > 8) return null;
  return out.sort((a, b) => b - a);
}

export function formatDays(days: number[]): string {
  return days.join(", ");
}

/** 重要日期输入：08-15 或 1990-08-15。 */
export function validDate(text: string): boolean {
  const m = /^(?:(\d{4})-)?(\d{2})-(\d{2})$/.exec(text.trim());
  if (!m) return false;
  const month = Number(m[2]);
  const day = Number(m[3]);
  if (month < 1 || month > 12 || day < 1) return false;
  // 没写年份时按闰年算，所以 02-29 可以
  const year = m[1] ? Number(m[1]) : 2000;
  return day <= new Date(Date.UTC(year, month, 0)).getUTCDate();
}

const pad = (n: number) => String(n).padStart(2, "0");

/** 今天，YYYY-MM-DD。 */
export function today(from = new Date()): string {
  return `${from.getFullYear()}-${pad(from.getMonth() + 1)}-${pad(from.getDate())}`;
}
