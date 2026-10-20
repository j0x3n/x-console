import type { NavBadge } from "../../lib/navBadges";
import { useQuotaAccounts } from "../quotas/api";
import { useWatchers } from "../watchers/api";
import { useMonitors, useSubscriptions } from "./api";
import { countedMonitors } from "./lib";

const SOON_DAYS = 7;
const RENEWAL_DAYS = 3;

/**
 * 监控里要处理的事（B256）。左栏图标的数字和二级菜单的红字都从这里算，两边对得上：
 * 故障网站、故障或 7 天内到期的证书域名、3 天内续费的订阅、读取出错的 AI 额度、等你处理的网站自动化。
 */
export function useMonitoringAttention() {
  const monitors = useMonitors();
  const subs = useSubscriptions(false);
  const quotas = useQuotaAccounts();
  const watchers = useWatchers();
  const counted = countedMonitors(monitors.data ?? []);
  const downSites = counted.filter(
    (m) => m.kind === "http" && m.lastStatus === "down",
  );
  const certs = counted.filter(
    (m) =>
      m.kind !== "http" &&
      (m.lastStatus === "down" ||
        (m.daysLeft != null && m.daysLeft <= SOON_DAYS)),
  );
  const renewing = (subs.data ?? []).filter((s) => s.daysLeft <= RENEWAL_DAYS);
  const quotaErrors = (quotas.data ?? []).filter((a) => a.status === "error");
  const watchersWaiting = (watchers.data ?? []).filter(
    (w) => w.status === "waiting" || w.status === "login",
  );
  return {
    monitors,
    subs,
    downSites,
    certs,
    renewing,
    quotaErrors,
    watchersWaiting,
  };
}

/** 左栏“运维”：监控里要处理的事加起来，再点一次图标跳到第一件。 */
export function useMonitoringBadge(): NavBadge | null {
  const a = useMonitoringAttention();
  const items = [
    {
      n: a.downSites.length,
      text: "个网站故障",
      to:
        a.downSites.length === 1
          ? `/monitoring?monitor=${a.downSites[0].id}`
          : "/monitoring",
    },
    {
      n: a.certs.length,
      text: "个证书或域名故障或快到期",
      to: "/monitoring/certs",
    },
    {
      n: a.renewing.length,
      text: "个订阅快续费",
      to: "/monitoring/subscriptions",
    },
    {
      n: a.quotaErrors.length,
      text: "个 AI 额度读取出错",
      to: "/monitoring/quotas",
    },
    {
      n: a.watchersWaiting.length,
      text: "个网站自动化等你处理",
      to: "/monitoring/watchers",
    },
  ].filter((x) => x.n > 0);
  if (items.length === 0) return null;
  return {
    count: items.reduce((sum, x) => sum + x.n, 0),
    title: items.map((x) => `${x.n} ${x.text}`).join("，"),
    to: items[0].to,
  };
}
