import type { NavBadge } from "../../lib/navBadges";
import { useHosts } from "./api";

/** 左栏“服务器”：离线的和正在告警的机器（B76）。 */
export function useServersBadge(): NavBadge | null {
  const hosts = useHosts("server").data ?? [];
  const offline = hosts.filter((h) => !h.online).length;
  const alerting = hosts.filter((h) => h.online && h.activeAlerts > 0).length;
  const n = offline + alerting;
  if (n === 0) return null;
  const parts = [
    offline ? `${offline} 台离线` : "",
    alerting ? `${alerting} 台告警` : "",
  ].filter(Boolean);
  return { count: n, title: parts.join("，") };
}
