import { useEffect, useMemo, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Database,
  RefreshCw,
  Search,
  Trash2,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  formatBytes,
  formatDate,
  formatTime,
  relativeTime,
} from "../../lib/time";

import {
  uptimeText,
  useCleanupJob,
  useMetrics,
  useOverview,
  useRefreshOverview,
  useScan,
  useServerVersion,
  useStartCleanup,
  useStartScan,
  useVacuum,
  versionMismatch,
  webVersion,
  type CleanupGroup,
  type MaintenancePoint,
  type StorageUsage,
} from "./api";
import "./i18n";
import "./maintenance.css";

/**
 * 设置 → 维护（B79）：版本、网站进程和机器的资源、各存储的占用、清理没用的文件。
 */
export default function MaintenanceTab() {
  const t = useT();
  const overview = useOverview();
  if (overview.isPending) return <Loading />;
  if (overview.isError)
    return isNotLive(overview.error) ? (
      <NotLive name={t("Maintenance")} icon={<Database size={28} />} />
    ) : (
      <ErrorState error={overview.error} onRetry={() => overview.refetch()} />
    );
  const o = overview.data;
  return (
    <div className="mt-page">
      <Resources process={o.process} machine={o.machine} />
      <div className="mt-columns">
        <VersionCard
          server={o.version}
          builtAt={o.builtAt}
          startedAt={o.startedAt}
          goVersion={o.goVersion}
          dataDir={o.dataDir}
          geo={o.geoAttribution}
          geoUrl={o.geoAttributionUrl}
        />
        <StorageCard rows={o.storage} />
      </div>
      <CleanupCard />
    </div>
  );
}

function Spark({ values }: { values: number[] }) {
  if (values.length < 2) return null;
  const max = Math.max(...values, 1e-9);
  const min = Math.min(...values, 0);
  const w = 120;
  const h = 28;
  const span = max - min || 1;
  const pts = values
    .map(
      (v, i) =>
        `${((i / (values.length - 1)) * w).toFixed(1)},${(h - ((v - min) / span) * (h - 2) - 1).toFixed(1)}`,
    )
    .join(" ");
  return (
    <svg
      className="mt-spark"
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      aria-hidden
    >
      <polyline points={pts} />
    </svg>
  );
}

function Resources({
  process,
  machine,
}: {
  process: MaintenancePoint;
  machine: {
    cpu: number;
    memoryTotal: number;
    memoryUsed: number;
    diskTotal: number;
    diskFree: number;
  };
}) {
  const t = useT();
  const metrics = useMetrics();
  const points = metrics.data ?? [];
  const now = points[points.length - 1] ?? process;
  return (
    <>
      <StatStrip label={t("Web process")}>
        <StatCard
          label={t("CPU, last minute")}
          value={`${now.cpu.toFixed(1)}`}
          unit="%"
        >
          <Spark values={points.map((p) => p.cpu)} />
        </StatCard>
        <StatCard label={t("Process memory")} value={formatBytes(now.rss)}>
          <Spark values={points.map((p) => p.rss)} />
        </StatCard>
        <StatCard
          label={t("Go heap")}
          value={formatBytes(now.heap)}
          foot={`${t("Goroutines")} ${now.goroutines}`}
        >
          <Spark values={points.map((p) => p.heap)} />
        </StatCard>
        <StatCard
          label={t("Connections")}
          value={now.browserConnections + now.agentConnections}
          foot={t("Browsers {n} · agents {m}")
            .replace("{n}", String(now.browserConnections))
            .replace("{m}", String(now.agentConnections))}
        />
      </StatStrip>
      <StatStrip label={t("This machine")}>
        <StatCard
          label={t("Machine CPU")}
          value={machine.cpu.toFixed(1)}
          unit="%"
          tone={
            machine.cpu >= 90
              ? "danger"
              : machine.cpu >= 70
                ? "warn"
                : undefined
          }
        />
        <StatCard
          label={t("Machine memory")}
          value={formatBytes(machine.memoryUsed)}
          foot={`/ ${formatBytes(machine.memoryTotal)}`}
        />
        <StatCard
          label={t("Disk free")}
          value={formatBytes(machine.diskFree)}
          foot={`/ ${formatBytes(machine.diskTotal)}`}
          tone={
            machine.diskTotal > 0 && machine.diskFree / machine.diskTotal < 0.1
              ? "warn"
              : undefined
          }
        />
      </StatStrip>
    </>
  );
}

function VersionCard({
  server,
  builtAt,
  startedAt,
  goVersion,
  dataDir,
  geo,
  geoUrl,
}: {
  server: string;
  builtAt: string;
  startedAt: string;
  goVersion: string;
  dataDir: string;
  geo?: string;
  geoUrl?: string;
}) {
  const t = useT();
  const language = useLanguage();
  const when = (v: string) => {
    const d = new Date(v);
    return Number.isNaN(d.getTime())
      ? v || "—"
      : `${formatDate(d, language)} ${formatTime(d, language)}`;
  };
  const stale = versionMismatch(webVersion.version, server);
  return (
    <section className="xc-card mt-version">
      <div className="xc-card-head">
        <h3>{t("Version info")}</h3>
      </div>
      <dl className="mt-dl">
        <dt>{t("Web version")}</dt>
        <dd className="xc-mono">
          {webVersion.version}
          {stale && (
            <button
              type="button"
              className="mt-stale"
              onClick={() => window.location.reload()}
            >
              {t("Page is out of date. Refresh it.")}
            </button>
          )}
        </dd>
        <dt>{t("Server version")}</dt>
        <dd className="xc-mono">{server}</dd>
        <dt>{t("Built at")}</dt>
        <dd>{when(builtAt)}</dd>
        <dt>{t("Server uptime")}</dt>
        <dd title={when(startedAt)}>{uptimeText(startedAt)}</dd>
        <dt>{t("Go version")}</dt>
        <dd className="xc-mono">{goVersion}</dd>
        <dt>{t("Data directory")}</dt>
        <dd className="xc-mono mt-ellipsis" title={dataDir}>
          {dataDir}
        </dd>
      </dl>
      {geo && (
        <small className="mt-geo">
          {geoUrl ? (
            <a href={geoUrl} target="_blank" rel="noreferrer">
              {geo}
            </a>
          ) : (
            geo
          )}
        </small>
      )}
    </section>
  );
}

// 后端给的位置和公共上传所属模块是英文代号，这里换成中文。
const LOCATIONS: Record<string, string> = {
  local: "本地",
  s3: "S3",
  external: "外部",
};
const MODULES: Record<string, string> = {
  calendar: "日程",
  coding: "Agent 任务",
  projects: "项目",
  reminders: "提醒",
};
function storageLabel(r: StorageUsage) {
  const m = r.label.match(/^(.*) ([a-z]+)$/);
  return m && MODULES[m[2]] ? `${m[1]} · ${MODULES[m[2]]}` : r.label;
}

function StorageCard({ rows }: { rows: StorageUsage[] }) {
  const t = useT();
  const refresh = useRefreshOverview();
  const vacuum = useVacuum();
  const total = rows.reduce((n, r) => n + (r.available ? r.bytes : 0), 0);
  const tidy = async () => {
    if (
      !(await confirmAction({
        title: t("Tidy the database?"),
        description: "执行 VACUUM 和 PRAGMA optimize。整理时网站会卡几秒。",
        confirmLabel: t("Tidy database"),
      }))
    )
      return;
    vacuum.mutate(undefined, {
      onSuccess: (r) =>
        toast(
          t("Database tidied. Freed {size}.").replace(
            "{size}",
            formatBytes(Math.max(0, r.freedBytes)),
          ),
        ),
      onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
    });
  };
  return (
    <section className="xc-card mt-storage">
      <div className="xc-card-head">
        <h3>{t("Storage usage")}</h3>
        <span className="xc-spacer" />
        <button
          type="button"
          className="xc-btn ghost small"
          disabled={refresh.isPending}
          onClick={() =>
            refresh.mutate(undefined, {
              onError: (e) =>
                toast({ message: errorMessage(e), tone: "error" }),
            })
          }
        >
          <RefreshCw size={14} className={refresh.isPending ? "mt-spin" : ""} />
          {t("Recount")}
        </button>
      </div>
      <div className="mt-table" role="table" aria-label={t("Storage usage")}>
        {rows.map((r) => (
          <div key={r.key} className="mt-row" role="row">
            <span role="cell" className="mt-name">
              <span className="mt-ellipsis" title={r.label}>
                {storageLabel(r)}
              </span>
              <small>{LOCATIONS[r.location] ?? r.location}</small>
            </span>
            {r.available ? (
              <>
                <span role="cell" className="mt-num">
                  {formatBytes(r.bytes)}
                </span>
                <span role="cell" className="mt-num mt-files">
                  {r.files.toLocaleString()}
                </span>
                <span role="cell" className="mt-bar" aria-hidden>
                  <i
                    style={{
                      width: `${total > 0 && r.bytes > 0 ? Math.max(1, (r.bytes / total) * 100) : 0}%`,
                    }}
                  />
                </span>
              </>
            ) : (
              <span role="cell" className="mt-na" title={r.note}>
                {t("Not countable")}
                {r.note ? ` · ${r.note}` : ""}
              </span>
            )}
          </div>
        ))}
      </div>
      <div className="mt-card-foot">
        <button
          type="button"
          className="xc-btn small"
          disabled={vacuum.isPending}
          onClick={() => void tidy()}
        >
          <Database size={14} /> {t("Tidy database")}
        </button>
      </div>
    </section>
  );
}

function CleanupCard() {
  const t = useT();
  const language = useLanguage();
  const scan = useScan();
  const job = useCleanupJob();
  const startScan = useStartScan();
  const startCleanup = useStartCleanup();
  const [picked, setPicked] = useState<Record<string, boolean>>({});
  const [open, setOpen] = useState<string | null>(null);
  const s = scan.data;
  const groups = useMemo(() => s?.groups ?? [], [s]);
  // 新的扫描结果出来后，按服务端给的默认勾选。
  useEffect(() => {
    setPicked(Object.fromEntries(groups.map((g) => [g.kind, g.selected])));
  }, [s?.id, groups.length]);
  const chosen = groups.filter((g) => picked[g.kind]);
  const size = chosen.reduce((n, g) => n + g.bytes, 0);
  const count = chosen.reduce((n, g) => n + g.count, 0);
  const cleaning = job.data?.state === "running";
  const scanning = s?.state === "running";

  const clean = async () => {
    if (
      !(await confirmAction({
        title: t("Clean up {n} items?").replace("{n}", String(count)),
        description: `会删掉选中的 ${chosen.length} 类，共 ${formatBytes(size)}。存储在 S3 时同时删 S3 上的对象。删掉的找不回来。`,
        confirmLabel: t("Cleanup"),
        danger: true,
      }))
    )
      return;
    startCleanup.mutate(
      { kinds: chosen.map((g) => g.kind), scanId: s?.id },
      { onError: (e) => toast({ message: errorMessage(e), tone: "error" }) },
    );
  };

  const kindLabel = (g: CleanupGroup) => t(g.kind);
  const result = job.data;
  return (
    <section className="xc-card mt-cleanup">
      <div className="xc-card-head">
        <h3>{t("Cleanup")}</h3>
        {s?.finishedAt && s.state !== "running" && (
          <small className="xc-muted">
            {relativeTime(s.finishedAt, language)}
          </small>
        )}
        <span className="xc-spacer" />
        <button
          type="button"
          className="xc-btn small"
          disabled={scanning || cleaning || startScan.isPending}
          onClick={() =>
            startScan.mutate(undefined, {
              onError: (e) =>
                toast({ message: errorMessage(e), tone: "error" }),
            })
          }
        >
          <Search size={14} />
          {scanning
            ? t("Scanning…")
            : s?.state === "done"
              ? t("Scan again")
              : t("Scan for junk")}
        </button>
      </div>
      {scan.isPending ? (
        <Loading />
      ) : scan.isError ? (
        <ErrorState error={scan.error} onRetry={() => scan.refetch()} />
      ) : !s || s.state === "idle" ? (
        <p className="xc-muted mt-empty">
          {t("Run a scan to find files and records that are no longer used.")}
        </p>
      ) : (
        <>
          {s.error && <p className="xc-error-text">{s.error}</p>}
          {groups.length === 0 && s.state === "done" ? (
            <p className="xc-muted mt-empty">{t("Nothing to clean up")}</p>
          ) : (
            <ul className="mt-groups">
              {groups.map((g) => (
                <li key={g.kind}>
                  <div className="mt-group">
                    <label className="xc-check">
                      <input
                        type="checkbox"
                        checked={!!picked[g.kind]}
                        disabled={cleaning}
                        onChange={(e) =>
                          setPicked((p) => ({
                            ...p,
                            [g.kind]: e.target.checked,
                          }))
                        }
                      />
                      <span className="mt-ellipsis">{kindLabel(g)}</span>
                    </label>
                    {!g.selected && (
                      <span className="xc-badge">
                        {t("Not deleted by default")}
                      </span>
                    )}
                    <span className="xc-spacer" />
                    <span className="mt-num">{g.count.toLocaleString()}</span>
                    <span className="mt-num mt-size">
                      {formatBytes(g.bytes)}
                    </span>
                    <button
                      type="button"
                      className="xc-btn ghost small"
                      aria-expanded={open === g.kind}
                      aria-label={
                        open === g.kind ? t("Hide items") : t("Show items")
                      }
                      title={
                        open === g.kind ? t("Hide items") : t("Show items")
                      }
                      onClick={() => setOpen(open === g.kind ? null : g.kind)}
                    >
                      {open === g.kind ? (
                        <ChevronDown size={14} />
                      ) : (
                        <ChevronRight size={14} />
                      )}
                    </button>
                  </div>
                  {open === g.kind && (
                    <div className="mt-items">
                      {g.items.map((i) => (
                        <div key={i.id} className="mt-item">
                          <span className="mt-ellipsis" title={i.name}>
                            {i.name}
                          </span>
                          <small className="mt-ellipsis" title={i.reason}>
                            {i.reason}
                          </small>
                          <span className="mt-num">{formatBytes(i.bytes)}</span>
                        </div>
                      ))}
                      {g.count > g.items.length && (
                        <small className="xc-muted">
                          {t("Only the first 50 are shown.")}
                        </small>
                      )}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
      {(cleaning || result?.state === "done" || result?.state === "failed") &&
        result && (
          <div
            className={`mt-result${result.state === "failed" ? " failed" : ""}`}
          >
            {cleaning ? (
              <>
                <span>{t("Cleaning up…")}</span>
                <progress max={Math.max(1, result.total)} value={result.done} />
                <small>
                  {result.done} / {result.total}
                </small>
              </>
            ) : (
              <span>
                {t("Cleaned up {n} files and freed {size}.")
                  .replace("{n}", String(result.result.deleted))
                  .replace("{size}", formatBytes(result.result.bytes))}
                {result.error ? ` ${result.error}` : ""}
              </span>
            )}
          </div>
        )}
      {groups.length > 0 && s?.state === "done" && (
        <div className="mt-card-foot">
          <button
            type="button"
            className="xc-btn danger small"
            disabled={chosen.length === 0 || cleaning || startCleanup.isPending}
            onClick={() => void clean()}
          >
            <Trash2 size={14} />
            {t("Clean up selected ({size})").replace(
              "{size}",
              formatBytes(size),
            )}
          </button>
        </div>
      )}
    </section>
  );
}
