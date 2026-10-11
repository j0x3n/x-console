import { useLocation } from "react-router";
import { FileCode2, Gauge, Globe, Receipt, ShieldCheck } from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useMonitors, useScripts, useSubscriptions } from "./api";
import { monitorTone } from "./lib";
import { useQuotaAccounts } from "../quotas/api";

/**
 * 左栏“监控”的二级菜单（用户 2026-10-05 要求）：网站、证书与域名、脚本、订阅，
 * 下面列出每个网站监控，点了打开详情。监控页不再放自己的页签。
 * 网站和证书的数量是故障数，红色；脚本和订阅是总数。
 */
export default function MonitoringNavChildren({
  onNavigate,
}: NavChildrenProps) {
  const t = useT();
  const location = useLocation();
  const monitors = useMonitors();
  const scripts = useScripts();
  const subs = useSubscriptions(false);
  const quotas = useQuotaAccounts();
  const list = monitors.data ?? [];
  const down = list.filter((m) => m.enabled && m.lastStatus === "down");
  const sites = list.filter((m) => m.kind === "http");
  // 只在有故障时显示红色数字
  const downSites = down.filter((m) => m.kind === "http").length;
  const downCerts = down.length - downSites;
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
          count={downSites || null}
          danger
          active={tab === "" && !openId}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/monitoring/certs"
          icon={ShieldCheck}
          label={t("Certificates & domains")}
          count={downCerts || null}
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
          count={subs.data?.length || null}
          active={tab === "subscriptions"}
          onNavigate={onNavigate}
        />
        {/* B152：AI 额度放在监控下面。数字是读取出错的账号数 */}
        <NavPanelLink
          to="/monitoring/quotas"
          icon={Gauge}
          label={t("AI quotas")}
          count={
            (quotas.data ?? []).filter((a) => a.status === "error").length ||
            null
          }
          danger
          active={tab === "quotas"}
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
