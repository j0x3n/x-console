import type { NavBadge } from "../../lib/navBadges";
import { useMonitors, useSubscriptions } from "./api";

const SOON_DAYS = 7;

/** 左栏“监控”：故障的监控，7 天内到期的证书、域名和订阅（B76）。 */
export function useMonitoringBadge(): NavBadge | null {
  const monitors = useMonitors().data ?? [];
  const subs = useSubscriptions(false).data ?? [];
  const down = monitors.filter((m) => m.enabled && m.lastStatus === "down");
  const expiring = monitors.filter(
    (m) =>
      m.enabled &&
      m.lastStatus !== "down" &&
      m.daysLeft != null &&
      m.daysLeft <= SOON_DAYS,
  );
  const renewing = subs.filter((s) => s.daysLeft <= SOON_DAYS);
  const n = down.length + expiring.length + renewing.length;
  if (n === 0) return null;
  const parts = [
    down.length ? `${down.length} 个故障` : "",
    expiring.length ? `${expiring.length} 个证书或域名快到期` : "",
    renewing.length ? `${renewing.length} 个订阅快续费` : "",
  ].filter(Boolean);
  return { count: n, title: parts.join("，") };
}
