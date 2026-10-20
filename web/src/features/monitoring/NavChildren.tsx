import { useLocation } from "react-router";
import {
  FileCode2,
  Gauge,
  Globe,
  Receipt,
  ScanEye,
  ShieldCheck,
} from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useScripts } from "./api";
import { useMonitoringAttention } from "./badge";
import { monitorTone } from "./lib";

/**
 * 左栏“监控”的二级菜单（用户 2026-10-05 要求）：网站、证书与域名、脚本、订阅，
 * 下面列出每个网站监控，点了打开详情。监控页不再放自己的页签。
 * 红字和左栏图标的数字是同一套规则（B256）：网站是故障数，证书是故障加 7 天内到期，
 * 订阅有 3 天内续费的时候显示要续费的个数，没有就显示总数。脚本是总数。
 */
export default function MonitoringNavChildren({
  onNavigate,
}: NavChildrenProps) {
  const t = useT();
  const location = useLocation();
  const scripts = useScripts();
  const attention = useMonitoringAttention();
  const { monitors, subs } = attention;
  const sites = (monitors.data ?? []).filter((m) => m.kind === "http");
  const renewing = attention.renewing.length;
  const tab = location.pathname.startsWith("/monitoring/")
    ? location.pathname.slice("/monitoring/".length)
    : location.pathname === "/monitoring"
      ? ""
      : null;
  const openId = Number(new URLSearchParams(location.search).get("monitor"));
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/monitoring"
          icon={Globe}
          label={t("Websites")}
          count={attention.downSites.length || null}
          danger
          active={tab === "" && !openId}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/monitoring/certs"
          icon={ShieldCheck}
          label={t("Certificates & domains")}
          count={attention.certs.length || null}
          danger
          active={tab === "certs"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/monitoring/scripts"
          icon={FileCode2}
          label={t("Scripts")}
          count={scripts.data?.length || null}
          active={tab === "scripts"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/monitoring/subscriptions"
          icon={Receipt}
          label={t("Subscriptions")}
          count={renewing || subs.data?.length || null}
          danger={renewing > 0}
          active={tab === "subscriptions"}
          onNavigate={onNavigate}
        />
        {/* B152：AI 额度放在监控下面。数字是读取出错的账号数 */}
        <NavPanelLink
          to="/monitoring/quotas"
          icon={Gauge}
          label={t("AI quotas")}
          count={attention.quotaErrors.length || null}
          danger
          active={tab === "quotas"}
          onNavigate={onNavigate}
        />
        {/* B206：网站自动化。数字是等你处理的任务数 */}
        <NavPanelLink
          to="/monitoring/watchers"
          icon={ScanEye}
          label={t("Web watchers")}
          count={attention.watchersWaiting.length || null}
          danger
          active={tab !== null && tab.startsWith("watchers")}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Websites")}>
        <NavChildLinks
          links={sites.map((m) => ({
            key: m.id,
            to: `/monitoring?monitor=${m.id}`,
            label: m.name,
            mark: <i className={`xc-dot ${monitorTone(m)}`} />,
            active: tab === "" && openId === m.id,
          }))}
          allTo="/monitoring"
          loading={monitors.isPending}
          error={monitors.isError}
          empty={t("No websites yet")}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}
