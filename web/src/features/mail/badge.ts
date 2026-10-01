import type { NavBadge } from "../../lib/navBadges";
import { useMailSummary } from "./api";

/** 左栏“邮件”：所有账号的未读数（B76）。 */
export function useMailBadge(): NavBadge | null {
  const n = useMailSummary().data?.unread ?? 0;
  return n > 0 ? { count: n, title: `${n} 封未读` } : null;
}
