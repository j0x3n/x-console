import type { MailCategory, MailTriage } from "./api";

/** 分拣类别的显示：名字和徽标颜色。 */
export const CATEGORY_LABELS: Record<
  MailCategory,
  { label: string; tone: string }
> = {
  action: { label: "Mail needs action", tone: "warn" },
  notice: { label: "Mail notice", tone: "info" },
  promo: { label: "Mail promo", tone: "" },
};

/** B220：要处理的邮件里需要我做决定的，标“待决定”，其余照类别标。 */
export function triageLabel(t: MailTriage): { label: string; tone: string } {
  if (t.category === "action" && t.decision)
    return { label: "Mail needs decision", tone: "accent" };
  return CATEGORY_LABELS[t.category];
}

export const CATEGORY_FILTERS: Array<{
  value: MailCategory | "all";
  label: string;
}> = [
  { value: "all", label: "Mail all kinds" },
  { value: "action", label: "Mail needs action" },
  { value: "notice", label: "Mail notice" },
  { value: "promo", label: "Mail promo" },
];

/** 地址里的 c 参数转成类别，不认识的当成全部。 */
export function categoryFromParam(value: string | null): MailCategory | null {
  return value === "action" || value === "notice" || value === "promo"
    ? value
    : null;
}
