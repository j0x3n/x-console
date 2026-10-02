import { usePageStatus } from "../../../stores/page-title";
import { useState, type DragEvent } from "react";
import { Link } from "react-router";
import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Pencil,
  TerminalSquare,
} from "lucide-react";
import MoreMenu, { type MoreMenuItem } from "../../../components/ui/MoreMenu";
import { formatBytes } from "../../../lib/time";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import type { Host } from "../api";
import { formatRate, formatUptime } from "../lib";
import UsageBar from "./UsageBar";
import CountryFlag from "./CountryFlag";
import { HostInfoDialog } from "./HostInfo";

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

export interface CardDrag {
  dragging: boolean;
  over: boolean;
  onDragStart: () => void;
  onDragOver: () => void;
  onDrop: () => void;
  onDragEnd: () => void;
}

/**
 * 列表里的一台机器：在线状态和 CPU、内存、磁盘小条。
 * 名称左边是国旗，后面是“客户”标签和前两个标签（B82）。
 * 桌面上拖动排序，手机上“更多”里上移、下移。
 */
export default function HostCard({
  host,
  drag,
  onMove,
}: {
  host: Host;
  drag?: CardDrag;
  /** 上移、下移。到头了传 undefined。 */
  onMove?: { up?: () => void; down?: () => void };
}) {
  const t = useT();
  const language = useLanguage();
  const [editing, setEditing] = useState(false);
  const m = host.metrics;
  const items: MoreMenuItem[] = [
    {
      key: "edit",
      label: t("Edit info"),
      icon: <Pencil size={14} />,
      onSelect: () => setEditing(true),
    },
  ];
  if (onMove?.up)
    items.push({
      key: "up",
      label: t("Move up"),
      icon: <ArrowUp size={14} />,
      onSelect: onMove.up,
    });
  if (onMove?.down)
    items.push({
      key: "down",
      label: t("Move down"),
      icon: <ArrowDown size={14} />,
      onSelect: onMove.down,
    });
  const dragProps = drag
    ? {
        draggable: true,
        onDragStart: (e: DragEvent) => {
          e.dataTransfer.effectAllowed = "move";
          e.dataTransfer.setData("text/plain", host.id);
          drag.onDragStart();
        },
        onDragOver: (e: DragEvent) => {
          e.preventDefault();
          drag.onDragOver();
        },
        onDrop: (e: DragEvent) => {
          e.preventDefault();
          drag.onDrop();
        },
        onDragEnd: drag.onDragEnd,
      }
    : {};
  const tags = host.info?.tags ?? [];
  return (
    <>
      <Link
        to={`/servers/${encodeURIComponent(host.id)}`}
        className={`xc-card servers-card${host.online ? "" : " offline"}${drag?.dragging ? " dragging" : ""}${drag?.over ? " drag-over" : ""}`}
        {...dragProps}
      >
        <div className="servers-card-head">
          <div>
            <span className="servers-card-name">
              {host.country && <CountryFlag country={host.country} />}
              <strong>{host.name}</strong>
              {host.info?.ownership === "client" && (
                <span className="xc-badge info">
                  {t("Client machine")}
                  {host.info.client ? `：${host.info.client}` : ""}
                </span>
              )}
              {tags.slice(0, 2).map((tag) => (
                <span key={tag} className="xc-badge">
                  {tag}
                </span>
              ))}
            </span>
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
          <span className="servers-card-side">
            <HostStatus host={host} />
            {/* 菜单在链接里面：拦住点击，不让它跳转 */}
            <span
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
              }}
            >
              <MoreMenu
                items={items}
                title={host.name}
                label={`${t("More")}：${host.name}`}
              />
            </span>
          </span>
        </div>
        <div className="xc-stack servers-card-bars">
          <UsageBar label={t("CPU")} value={host.cpu} />
          <UsageBar label={t("Memory")} value={host.memory} />
          <UsageBar label={t("Disk")} value={host.disk} />
          {host.traffic && host.traffic.limitBytes > 0 && (
            <UsageBar
              label={t("Traffic")}
              value={(host.traffic.usedBytes / host.traffic.limitBytes) * 100}
              detail={`${formatBytes(host.traffic.usedBytes)} / ${formatBytes(host.traffic.limitBytes)}`}
            />
          )}
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
      {editing && (
        <HostInfoDialog host={host} onClose={() => setEditing(false)} />
      )}
    </>
  );
}
