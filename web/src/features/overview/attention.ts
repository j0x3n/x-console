import type { Monitor, Subscription } from "../monitoring/api";

/*
 * B59：今日页“监控”卡片要提醒的事：挂掉的网站、快到期的证书和域名、
 * 快续费的订阅。纯函数，按严重程度排好。
 */

export interface AttentionItem {
  key: string;
  tone: "danger" | "warn";
  kind: "site" | "tls" | "domain" | "subscription";
  name: string;
  /** 剩几天；网站挂了时没有 */
  days?: number;
  link: string;
}

export function attentionItems(
  monitors: Monitor[],
  subs: Subscription[],
): AttentionItem[] {
  const out: AttentionItem[] = [];
  for (const m of monitors) {
    if (!m.enabled) continue;
    if (m.kind === "http") {
      if (m.lastStatus === "down")
        out.push({
          key: `m${m.id}`,
          tone: "danger",
          kind: "site",
          name: m.name,
          link: `/monitoring?monitor=${m.id}`,
        });
      continue;
    }
    if (m.daysLeft === undefined) continue;
    const days = Math.floor(m.daysLeft);
    const soon = m.kind === "domain" ? 30 : 14;
    const urgent = m.kind === "domain" ? 7 : 3;
    if (days > soon) continue;
    out.push({
      key: `m${m.id}`,
      tone: days <= urgent ? "danger" : "warn",
      kind: m.kind,
      name: m.name,
      days,
      link: `/monitoring/certs?monitor=${m.id}`,
    });
  }
  for (const s of subs) {
    if (s.archivedAt || s.daysLeft > 7) continue;
    out.push({
      key: `s${s.id}`,
      tone: s.daysLeft <= 3 ? "danger" : "warn",
      kind: "subscription",
      name: s.name,
      days: s.daysLeft,
      link: `/monitoring/subscriptions?subscription=${s.id}`,
    });
  }
  const rank = (x: AttentionItem) =>
    (x.tone === "danger" ? 0 : 1000) + (x.kind === "site" ? -1 : (x.days ?? 0));
  return out.sort((a, b) => rank(a) - rank(b));
}
