import { usePageStatus } from "../../../stores/page-title";
import { Link } from "react-router";
import { AlertTriangle, TerminalSquare } from "lucide-react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import type { Host } from "../api";
import { formatRate, formatUptime } from "../lib";
import UsageBar from "./UsageBar";

export function HostStatus({
  host,
}: {
  host: Pick<Host, "online" | "lastSeenAt" | "activeAlerts">;
}) {
  const t = useT();
  const language = useLanguage();
  return (
    <span className="xc-row">
      {host.activeAlerts > 0 && (
        <span className="xc-badge danger">
          <AlertTriangle size={11} /> {host.activeAlerts} {t("alerts")}
        </span>
      )}
      <span className={`xc-badge ${host.online ? "ok" : ""}`}>
        <span className={`xc-dot ${host.online ? "ok" : ""}`} />
        {host.online
          ? t("Online")
          : host.lastSeenAt
            ? `${t("Offline")} · ${relativeTime(host.lastSeenAt, language)}`
            : t("Offline")}
      </span>
    </span>
  );
}

/**
 * 详情页用：在线状态显示成左上角标题后面的小点，页头只在有告警或离线时显示标签。
 */
export function HostHeadStatus({
  host,
}: {
  host: Pick<Host, "online" | "lastSeenAt" | "activeAlerts">;
}) {
  const t = useT();
  const language = useLanguage();
  const offline = host.lastSeenAt
    ? `${t("Offline")} · ${relativeTime(host.lastSeenAt, language)}`
    : t("Offline");
  usePageStatus(
    host.online ? "ok" : "danger",
    host.online ? t("Online") : offline,
  );
  if (host.online && host.activeAlerts === 0) return null;
  return (
    <span className="xc-row">
      {host.activeAlerts > 0 && (
        <span className="xc-badge danger">
          <AlertTriangle size={11} /> {host.activeAlerts} {t("alerts")}
        </span>
      )}
      {!host.online && <span className="xc-badge">{offline}</span>}
    </span>
  );
}

/** 列表里的一台机器：在线状态和 CPU、内存、磁盘小条。 */
export default function HostCard({ host }: { host: Host }) {
  const t = useT();
  const language = useLanguage();
  const m = host.metrics;
  return (
    <Link
      to={`/servers/${encodeURIComponent(host.id)}`}
      className={`xc-card servers-card${host.online ? "" : " offline"}`}
    >
      <div className="servers-card-head">
        <div>
          <strong>{host.name}</strong>
          <small className="xc-muted">
            {host.source === "ssh" ? (
              <>
                <TerminalSquare size={11} /> SSH · {host.hostname}
              </>
            ) : (
              `${host.hostname} · ${host.os}`
            )}
          </small>
        </div>
        <HostStatus host={host} />
      </div>
      <div className="xc-stack servers-card-bars">
        <UsageBar label={t("CPU")} value={host.cpu} />
        <UsageBar label={t("Memory")} value={host.memory} />
        <UsageBar label={t("Disk")} value={host.disk} />
      </div>
      <div className="servers-card-foot xc-muted">
        {m ? (
          <>
            <span>
              {t("Load")} {m.load1.toFixed(2)}
            </span>
            <span>
              ↓ {formatRate(m.netRx)} ↑ {formatRate(m.netTx)}
            </span>
            <span>{formatUptime(m.uptimeSeconds, language === "zh")}</span>
          </>
        ) : (
          <span>{t("Waiting for the first metrics")}</span>
        )}
      </div>
    </Link>
  );
}
