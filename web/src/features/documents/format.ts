import {
  BadgeCheck,
  Car,
  CreditCard,
  FileText,
  Plane,
  IdCard,
  Laptop,
  Shield,
  Stamp,
  type LucideIcon,
} from "lucide-react";
import type { DocumentItem, DocumentKind, DocumentStatus } from "./api";

export const KINDS: DocumentKind[] = [
  "passport",
  "id_card",
  "driver_license",
  "visa",
  "contract",
  "insurance",
  "item",
  "other",
];

/** 英文键，中文在 i18n.ts。 */
export const KIND_LABELS: Record<DocumentKind, string> = {
  passport: "Passport",
  id_card: "ID card",
  driver_license: "Driver license",
  visa: "Visa",
  contract: "Contract",
  insurance: "Insurance",
  item: "Item & warranty",
  other: "Other document",
};

export const KIND_ICONS: Record<DocumentKind, LucideIcon> = {
  passport: Plane,
  id_card: IdCard,
  driver_license: Car,
  visa: Stamp,
  contract: FileText,
  insurance: Shield,
  item: Laptop,
  other: BadgeCheck,
};

/** 用户自己加的类型在档案里存成 `c:` 加名称。 */
export const CUSTOM_PREFIX = "c:";

export const isCustomKind = (kind: string) => kind.startsWith(CUSTOM_PREFIX);

/** 类型的显示名：内置的翻译，自己加的直接用名称。 */
export function kindName(t: (key: string) => string, kind: DocumentKind) {
  if (isCustomKind(kind)) return kind.slice(CUSTOM_PREFIX.length);
  return t(KIND_LABELS[kind] ?? KIND_LABELS.other);
}

export function kindIcon(kind: DocumentKind): LucideIcon {
  if (isCustomKind(kind)) return CreditCard;
  return KIND_ICONS[kind] ?? BadgeCheck;
}

/** 状态对应的标签样式：已过期红色，快到期琥珀色，其余不着色。 */
export function statusTone(status: DocumentStatus, daysLeft?: number | null) {
  if (status === "expired") return "danger";
  if (status === "soon")
    return daysLeft != null && daysLeft <= 30 ? "warn" : "info";
  return "";
}

/** 剩余天数的文字，t 是翻译函数。 */
export function daysText(
  t: (key: string) => string,
  daysLeft: number | null | undefined,
): string {
  if (daysLeft == null) return t("No expiry date");
  if (daysLeft < 0) return `${t("Expired")} ${-daysLeft} ${t("days")}`;
  if (daysLeft === 0) return t("Expires today");
  return `${daysLeft} ${t("days left")}`;
}

/** 列表的二级菜单视图。 */
export type View = "all" | "expired" | "soon";

export function matchesView(d: DocumentItem, view: View): boolean {
  if (view === "expired") return d.status === "expired";
  if (view === "soon") return d.status === "soon";
  return true;
}

/** 提醒天数输入框的文字转数组：“90, 30 7” 变成 [90, 30, 7]。 */
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

/** 把日期选择框的值和服务端的日期统一成 YYYY-MM-DD。 */
export function plusDays(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
