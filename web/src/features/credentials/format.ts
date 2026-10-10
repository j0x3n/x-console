import {
  FileKey,
  KeyRound,
  Lock,
  Terminal,
  Ticket,
  type LucideIcon,
} from "lucide-react";
import type { CredentialItem, CredentialKind, CredentialStatus } from "./api";

export const KINDS: CredentialKind[] = [
  "api_key",
  "access_token",
  "ssh_key",
  "signing_key",
  "other",
];

/** 英文键，中文在 i18n.ts。 */
export const KIND_LABELS: Record<CredentialKind, string> = {
  api_key: "API secret",
  access_token: "Access token",
  ssh_key: "SSH key",
  signing_key: "Signing key",
  other: "Other key",
};

export const KIND_ICONS: Record<CredentialKind, LucideIcon> = {
  api_key: KeyRound,
  access_token: Ticket,
  ssh_key: Terminal,
  signing_key: FileKey,
  other: Lock,
};

/** 状态对应的标签样式：已过期红色，快到期和久未更换琥珀色。 */
export function statusTone(status: CredentialStatus) {
  if (status === "expired") return "danger";
  if (status === "soon" || status === "stale") return "warn";
  return "";
}

/** 列表右边标签的文字，t 是翻译函数。按状态说最急的那一项。 */
export function dueText(
  t: (key: string) => string,
  c: Pick<CredentialItem, "status" | "expiresIn" | "rotateDueIn">,
): string {
  const { expiresIn, rotateDueIn } = c;
  if (c.status === "stale" && rotateDueIn != null)
    return rotateDueIn === 0
      ? t("Rotation due today")
      : `${t("Rotation overdue")} ${-rotateDueIn} ${t("days")}`;
  if (expiresIn != null) {
    if (expiresIn < 0) return `${t("Expired")} ${-expiresIn} ${t("days")}`;
    if (expiresIn === 0) return t("Expires today");
    return `${expiresIn} ${t("days left")}`;
  }
  if (rotateDueIn != null) return `${rotateDueIn} ${t("days until rotation")}`;
  return t("No expiry date");
}

/** 提醒天数输入框的文字转数组：“30, 7” 变成 [30, 7]。 */
export function parseDays(text: string): number[] | null {
  const parts = text
    .split(/[\s,，、]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  const out: number[] = [];
  for (const p of parts) {
    if (!/^\d+$/.test(p)) return null;
    const n = Number(p);
    if (n < 1 || n > 3650) return null;
    if (!out.includes(n)) out.push(n);
  }
  if (out.length > 8) return null;
  return out.sort((a, b) => b - a);
}

export function formatDays(days: number[]): string {
  return days.join(", ");
}

/** “用在哪里”文本框：一行一项，去掉空行和重复。 */
export function parseLines(text: string): string[] {
  const out: string[] = [];
  for (const line of text.split(/\r?\n/)) {
    const item = line.trim();
    if (item && !out.includes(item)) out.push(item);
  }
  return out;
}

const pad = (n: number) => String(n).padStart(2, "0");
function dateText(d: Date) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 今天往后（或往前）n 天，YYYY-MM-DD。 */
export function plusDays(days: number, from = new Date()): string {
  const d = new Date(from.getFullYear(), from.getMonth(), from.getDate());
  d.setDate(d.getDate() + days);
  return dateText(d);
}

const dayNumber = (date: string) => Date.parse(`${date}T12:00:00Z`) / 86400000;

/**
 * 更换时给新到期日一个默认值：沿用上一次的有效期长度。
 * 上一次没有到期日或没有起点就返回空。
 */
export function suggestExpiry(
  c: Pick<CredentialItem, "expiresOn" | "rotatedOn" | "createdOn">,
  rotatedOn: string,
): string {
  const base = c.rotatedOn || c.createdOn;
  if (!c.expiresOn || !base) return "";
  const length = Math.round(dayNumber(c.expiresOn) - dayNumber(base));
  if (length <= 0) return "";
  const d = new Date(`${rotatedOn}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + length);
  return d.toISOString().slice(0, 10);
}
