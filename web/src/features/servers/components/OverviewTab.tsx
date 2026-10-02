import { useCallback, useEffect, useState } from "react";
import {
  useMetricsInterval,
  useServerEvent,
  type ServerEvent,
} from "../../../api/events";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes } from "../../../lib/time";
import { ErrorState, Loading } from "../../../components/ui/States";
import {
  useHostMetrics,
  type HostDetail,
  type MetricsPoint,
  type MetricsRange,
  type MetricsSample,
} from "../api";
import {
  formatRate,
  formatUptime,
  niceRateMax,
  percent,
  pointFromSample,
} from "../lib";
import MetricChart from "./MetricChart";
import { TrafficCard } from "./Traffic";
import { HostInfoCard } from "./HostInfo";
import AddressList from "./AddressList";
import { relativeTime } from "../../../lib/time";
import { presenceText, presenceTone } from "../../habits/presence";
import UsageBar from "./UsageBar";

const S1 = "var(--servers-series-1)";
const S2 = "var(--servers-series-2)";
const pct = (v: number) => `${Math.round(v)}%`;

/** 刷新周期（B26）。存在 localStorage，所有机器共用。 */
const REFRESH_KEY = "xc.hosts.refresh";
const refreshChoices = [1000, 5000, 30000] as const;
function readRefresh(): number {
  try {
    const v = Number(localStorage.getItem(REFRESH_KEY));
    return (refreshChoices as readonly number[]).includes(v) ? v : 5000;
  } catch {
    return 5000;
  }
}

/** 1 秒刷新时，趋势图只画浏览器里收到的最近 5 分钟。 */
function useLivePoints(hostId: string, enabled: boolean) {
  const [points, setPoints] = useState<MetricsPoint[]>([]);
  const onEvent = useCallback(
    (event: ServerEvent) => {
      if (!enabled) return;
      const data = event.data as { hostId?: string; sample?: MetricsSample };
      if (data.hostId !== hostId || !data.sample) return;
      const point = pointFromSample(data.sample);
      setPoints((prev) => [...prev.slice(-299), point]);
    },
    [hostId, enabled],
  );
  useServerEvent("host.metrics", onEvent);
  useEffect(() => {
    if (!enabled) setPoints([]);
  }, [enabled]);
  return points;
}

export default function OverviewTab({ host }: { host: HostDetail }) {
  const t = useT();
  const language = useLanguage();
  const [range, setRange] = useState<MetricsRange>("1h");
  const [refresh, setRefreshState] = useState(readRefresh);
  const [disksOpen, setDisksOpen] = useState(false);
  const setRefresh = (ms: number) => {
    setRefreshState(ms);
    try {
      localStorage.setItem(REFRESH_KEY, String(ms));
    } catch {
      /* 记不住就算了 */
    }
  };
  useMetricsInterval(host.id, refresh);
  const live = refresh === 1000;
  const livePoints = useLivePoints(host.id, live);
  const metrics = useHostMetrics(host.id, range);
  const m = host.metrics;
  const info = host.systemInfo;
  const points = live ? livePoints : (metrics.data?.points ?? []);
  const step = live ? 1 : (metrics.data?.stepSeconds ?? 10);
  const disks = m?.disks ?? [];
  const fullest = [...disks].sort(
    (a, b) => percent(b.used, b.total) - percent(a.used, a.total),
  )[0];
  const system = info
    ? `${info.platform} ${info.platformVersion}`.trim() || info.os
    : host.os;
  const agent = host.source === "ssh" ? t("SSH only") : host.version;

  return (
    <div className="xc-stack">
      {host.presence && (
        // B83：电脑有没有人在用（看键盘鼠标，不是看开没开机）
        <div className="servers-presence">
          <span className="xc-muted">{t("Computer usage")}</span>
          <span className={`xc-badge ${presenceTone(host.presence.state)}`}>
            {presenceText(host.presence, t)}
          </span>
          {host.presence.state !== "unknown" && (
            <small className="xc-muted">
              {t("since")} {relativeTime(host.presence.since, language)}
            </small>
          )}
        </div>
      )}
      <div className="servers-stats servers-stats-6">
        <Stat
          label={t("CPU")}
          value={m ? pct(m.cpu) : "—"}
          sub={
            info
              ? [info.cpuModel, `${info.cpuCores} ${t("cores")}`]
                  .filter(Boolean)
                  .join(" · ")
              : undefined
          }
          sub2={m ? `${t("Load")} ${m.load1.toFixed(2)}` : undefined}
          title={info?.cpuModel}
        />
        <Stat
          label={t("Memory")}
          value={m ? pct(percent(m.memUsed, m.memTotal)) : "—"}
          sub={
            m
              ? `${formatBytes(m.memUsed)} / ${formatBytes(m.memTotal)}`
              : undefined
          }
          sub2={
            m && m.swapTotal
              ? `${t("Swap")} ${formatBytes(m.swapUsed)} / ${formatBytes(m.swapTotal)}`
              : undefined
          }
        />
        <Stat
          label={t("Disk")}
          value={fullest ? pct(percent(fullest.used, fullest.total)) : "—"}
          sub={
            fullest
              ? `${fullest.mount} · ${disks.length} ${t("disks")}`
              : undefined
          }
          sub2={
            fullest
              ? `${formatBytes(fullest.used)} / ${formatBytes(fullest.total)}`
              : undefined
          }
          onClick={disks.length > 0 ? () => setDisksOpen((v) => !v) : undefined}
          expanded={disksOpen}
        />
        <Stat
          label={t("Network")}
          value={m ? `↓ ${formatRate(m.netRx)}` : "—"}
          sub={m ? `↑ ${formatRate(m.netTx)}` : undefined}
        />
        <TrafficCard hostId={host.id} />
        <Stat
          label={t("Uptime")}
          value={m ? formatUptime(m.uptimeSeconds, language === "zh") : "—"}
          sub={system}
          sub2={[
            info?.kernelVersion && `${t("Kernel")} ${info.kernelVersion}`,
            agent && `${t("Agent")} ${agent}`,
          ]
            .filter(Boolean)
            .join(" · ")}
          title={[
            info?.hostname || host.hostname,
            host.address,
            info && `${formatBytes(info.memoryTotal)} ${t("Memory")}`,
            m && `${m.procs} ${t("processes")}`,
          ]
            .filter(Boolean)
            .join("\n")}
        />
      </div>
      {disksOpen && disks.length > 0 && (
        <div className="xc-card servers-disks">
          {disks.map((d) => (
            <UsageBar
              key={d.mount}
              label={d.mount}
              value={percent(d.used, d.total)}
              detail={`${formatBytes(d.used)} / ${formatBytes(d.total)}`}
            />
          ))}
        </div>
      )}

      <div className="servers-two">
        <HostInfoCard host={host} />
        {!(m?.netInterfaces && m.netInterfaces.length > 0) &&
          host.addresses.length > 0 && (
            <div className="xc-card">
              <div className="xc-card-head">
                <h2>{t("IP addresses")}</h2>
              </div>
              <AddressList addresses={host.addresses} bare />
            </div>
          )}
      </div>

      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Trends")}</h2>
          <div className="servers-trend-controls">
            <div
              className="servers-segmented"
              role="radiogroup"
              aria-label={t("Refresh every")}
              title={t("Refresh every")}
            >
              {refreshChoices.map((ms) => (
                <button
                  key={ms}
                  role="radio"
                  aria-checked={refresh === ms}
                  className={refresh === ms ? "active" : ""}
                  onClick={() => setRefresh(ms)}
                >
                  {ms / 1000} {t("sec")}
                </button>
              ))}
            </div>
            {live ? (
              <span className="xc-muted servers-live-note">
                {t("Last 5 minutes, live")}
              </span>
            ) : (
              <div className="servers-segmented" role="tablist">
                {(["1h", "24h", "7d"] as MetricsRange[]).map((r) => (
                  <button
                    key={r}
                    role="tab"
                    aria-selected={range === r}
                    className={range === r ? "active" : ""}
                    onClick={() => setRange(r)}
                  >
                    {t(
                      r === "1h"
                        ? "1 hour"
                        : r === "24h"
                          ? "24 hours"
                          : "7 days",
                    )}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
        {!live && metrics.isPending ? (
          <Loading />
        ) : !live && metrics.isError ? (
          <ErrorState error={metrics.error} onRetry={() => metrics.refetch()} />
        ) : (
          <div className="servers-charts">
            <MetricChart
              title={t("CPU")}
              points={points}
              stepSeconds={step}
              max={100}
              format={pct}
              series={[{ key: "cpu", label: t("CPU"), color: S1 }]}
              current={m ? pct(m.cpu) : undefined}
            />
            <MetricChart
              title={t("Memory")}
              points={points}
              stepSeconds={step}
              max={100}
              format={pct}
              series={[{ key: "memory", label: t("Memory"), color: S1 }]}
              current={m ? pct(percent(m.memUsed, m.memTotal)) : undefined}
            />
            <MetricChart
              title={t("Disk (fullest)")}
              points={points}
              stepSeconds={step}
              max={100}
              format={pct}
              series={[{ key: "disk", label: t("Disk"), color: S1 }]}
              current={host.disk !== undefined ? pct(host.disk) : undefined}
            />
            <MetricChart
              title={t("Network")}
              points={points}
              stepSeconds={step}
              format={formatRate}
              nice={niceRateMax}
              series={[
                { key: "netRx", label: t("Received"), color: S1 },
                { key: "netTx", label: t("Sent"), color: S2 },
              ]}
            />
          </div>
        )}
      </div>

      {m &&
        (m.cpuPerCore.length > 1 ||
          (m.netInterfaces && m.netInterfaces.length > 0)) && (
          <div className="servers-two">
            {m.cpuPerCore.length > 1 && (
              <div className="xc-card">
                <div className="xc-card-head">
                  <h2>{t("Per core")}</h2>
                </div>
                <div className="servers-cores">
                  {m.cpuPerCore.map((v, i) => (
                    <UsageBar key={i} label={`#${i}`} value={v} />
                  ))}
                </div>
              </div>
            )}
            {m.netInterfaces && m.netInterfaces.length > 0 && (
              <div className="xc-card">
                <div className="xc-card-head">
                  <h2>{t("Network interfaces")}</h2>
                </div>
                <div className="xc-stack">
                  {m.netInterfaces.map((nic) => (
                    <div className="servers-nic" key={nic.name}>
                      <span className="xc-mono">{nic.name}</span>
                      <span className="xc-muted">
                        ↓ {formatRate(nic.rx)} ↑ {formatRate(nic.tx)}
                      </span>
                    </div>
                  ))}
                </div>
                <AddressList addresses={host.addresses} />
              </div>
            )}
          </div>
        )}
    </div>
  );
}

function Stat({
  label,
  value,
  sub,
  sub2,
  title,
  onClick,
  expanded,
}: {
  label: string;
  value: string;
  sub?: string;
  sub2?: string;
  title?: string;
  onClick?: () => void;
  expanded?: boolean;
}) {
  const body = (
    <>
      <span>{label}</span>
      <strong>{value}</strong>
      {sub && <small>{sub}</small>}
      {sub2 && <small>{sub2}</small>}
    </>
  );
  return onClick ? (
    <button
      type="button"
      className="servers-stat clickable"
      title={title}
      aria-expanded={expanded}
      onClick={onClick}
    >
      {body}
    </button>
  ) : (
    <div className="servers-stat" title={title}>
      {body}
    </div>
  );
}
