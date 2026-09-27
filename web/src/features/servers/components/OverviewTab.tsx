import { useState } from "react";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes } from "../../../lib/time";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useHostMetrics, type HostDetail, type MetricsRange } from "../api";
import { formatRate, formatUptime, niceRateMax, percent } from "../lib";
import MetricChart from "./MetricChart";
import UsageBar from "./UsageBar";

const S1 = "var(--servers-series-1)";
const S2 = "var(--servers-series-2)";
const pct = (v: number) => `${Math.round(v)}%`;

export default function OverviewTab({ host }: { host: HostDetail }) {
  const t = useT();
  const language = useLanguage();
  const [range, setRange] = useState<MetricsRange>("1h");
  const metrics = useHostMetrics(host.id, range);
  const m = host.metrics;
  const info = host.systemInfo;
  const points = metrics.data?.points ?? [];
  const step = metrics.data?.stepSeconds ?? 10;

  return (
    <div className="xc-stack">
      <div className="servers-stats">
        <Stat
          label={t("CPU")}
          value={m ? pct(m.cpu) : "—"}
          sub={info ? `${info.cpuCores} ${t("cores")}` : undefined}
        />
        <Stat
          label={t("Memory")}
          value={m ? pct(percent(m.memUsed, m.memTotal)) : "—"}
          sub={
            m
              ? `${formatBytes(m.memUsed)} / ${formatBytes(m.memTotal)}`
              : undefined
          }
        />
        <Stat
          label={t("Load")}
          value={m ? m.load1.toFixed(2) : "—"}
          sub={m ? `${m.load5.toFixed(2)} · ${m.load15.toFixed(2)}` : undefined}
        />
        <Stat
          label={t("Network")}
          value={m ? `↓ ${formatRate(m.netRx)}` : "—"}
          sub={m ? `↑ ${formatRate(m.netTx)}` : undefined}
        />
        <Stat
          label={t("Uptime")}
          value={m ? formatUptime(m.uptimeSeconds, language === "zh") : "—"}
          sub={m ? `${m.procs} ${t("processes")}` : undefined}
        />
      </div>

      <div className="xc-card">
        <div className="xc-card-head">
          <h2>{t("Trends")}</h2>
          <div className="servers-segmented" role="tablist">
            {(["1h", "24h", "7d"] as MetricsRange[]).map((r) => (
              <button
                key={r}
                role="tab"
                aria-selected={range === r}
                className={range === r ? "active" : ""}
                onClick={() => setRange(r)}
              >
                {t(r === "1h" ? "1 hour" : r === "24h" ? "24 hours" : "7 days")}
              </button>
            ))}
          </div>
        </div>
        {metrics.isPending ? (
          <Loading />
        ) : metrics.isError ? (
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

      <div className="servers-two">
        <div className="xc-card">
          <div className="xc-card-head">
            <h2>{t("Disks")}</h2>
          </div>
          {m && m.disks.length > 0 ? (
            <div className="xc-stack">
              {m.disks.map((d) => (
                <UsageBar
                  key={d.mount}
                  label={d.mount}
                  value={percent(d.used, d.total)}
                  detail={`${formatBytes(d.used)} / ${formatBytes(d.total)}`}
                />
              ))}
            </div>
          ) : (
            <p className="xc-muted">{t("No data yet")}</p>
          )}
          {m && m.cpuPerCore.length > 1 && (
            <>
              <div className="xc-card-head servers-subhead">
                <h3>{t("Per core")}</h3>
              </div>
              <div className="servers-cores">
                {m.cpuPerCore.map((v, i) => (
                  <UsageBar key={i} label={`#${i}`} value={v} />
                ))}
              </div>
            </>
          )}
        </div>
        <div className="xc-card">
          <div className="xc-card-head">
            <h2>{t("System")}</h2>
          </div>
          <dl className="servers-facts">
            <dt>{t("Hostname")}</dt>
            <dd>{info?.hostname || host.hostname}</dd>
            <dt>{t("System")}</dt>
            <dd>
              {info
                ? `${info.platform} ${info.platformVersion}`.trim() || info.os
                : host.os}
            </dd>
            {info?.kernelVersion && (
              <>
                <dt>{t("Kernel")}</dt>
                <dd className="xc-mono">{info.kernelVersion}</dd>
              </>
            )}
            <dt>{t("Architecture")}</dt>
            <dd>{info?.arch || host.arch}</dd>
            {info?.cpuModel && (
              <>
                <dt>{t("CPU")}</dt>
                <dd>{info.cpuModel}</dd>
              </>
            )}
            {info && (
              <>
                <dt>{t("Memory")}</dt>
                <dd>{formatBytes(info.memoryTotal)}</dd>
              </>
            )}
            {host.address && (
              <>
                <dt>{t("Address")}</dt>
                <dd className="xc-mono">{host.address}</dd>
              </>
            )}
            <dt>{t("Agent")}</dt>
            <dd>{host.source === "ssh" ? t("SSH only") : host.version}</dd>
          </dl>
        </div>
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  sub,
}: {
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <div className="servers-stat">
      <span>{label}</span>
      <strong>{value}</strong>
      {sub && <small>{sub}</small>}
    </div>
  );
}
