import { useState } from "react";
import { Pause, Pencil, Play, RefreshCw, Trash2 } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { relativeTime } from "../../../lib/time";
import {
  useCheckMonitor,
  useDeleteMonitor,
  useMonitorResults,
  useSaveMonitor,
  type Monitor,
  type ResultRange,
} from "../api";
import { expiryTone, monitorTone } from "../lib";
import { longDate, shortDateTime, showError } from "./common";
import HistoryChart from "./HistoryChart";
import MonitorDialog from "./MonitorDialog";
import { StatusBadge } from "./StatusBadge";
import { confirmAction } from "../../../components/ui/ConfirmDialog";

const ranges: ResultRange[] = ["24h", "7d", "30d"];

interface Props {
  monitor: Monitor | null;
  onClose: () => void;
}

/** 一个监控的详情：历史曲线、最近的检查、操作按钮。 */
export default function MonitorDetail({ monitor, onClose }: Props) {
  const t = useT();
  const language = useLanguage();
  const [range, setRange] = useState<ResultRange>("24h");
  const [editing, setEditing] = useState(false);
  const results = useMonitorResults(monitor?.id ?? null, range);
  const check = useCheckMonitor();
  const save = useSaveMonitor();
  const remove = useDeleteMonitor();
  if (!monitor) return null;

  const latest = results.data?.items.at(-1);
  const recent = [...(results.data?.items ?? [])].reverse().slice(0, 20);
  const detail = latest?.detail;

  const runCheck = () =>
    check.mutate(monitor.id, {
      onSuccess: (r) => {
        toast(r.ok ? t("Check passed") : `${t("Check failed")}: ${r.error}`);
        results.refetch();
      },
      onError: showError,
    });

  return (
    <>
      <Dialog open={!editing} onClose={onClose} title={monitor.name} wide>
        <div className="monitoring-detail-head">
          <span className="xc-mono monitoring-target">{monitor.target}</span>
          <StatusBadge tone={monitorTone(monitor)} monitor={monitor} />
        </div>
        {monitor.kind !== "http" && monitor.expiresAt && (
          <div className="monitoring-facts">
            <div>
              <small>{t("Expires")}</small>
              <strong>{longDate(monitor.expiresAt, language)}</strong>
            </div>
            <div>
              <small>{t("Days left")}</small>
              <strong
                className={`monitoring-tone-${expiryTone(monitor.kind, monitor.daysLeft)}`}
              >
                {Math.floor(monitor.daysLeft ?? 0)}
              </strong>
            </div>
            {detail?.issuer && (
              <div>
                <small>{t("Issuer")}</small>
                <strong>{detail.issuer}</strong>
              </div>
            )}
            {detail?.registrar && (
              <div>
                <small>{t("Registrar")}</small>
                <strong>{detail.registrar}</strong>
              </div>
            )}
          </div>
        )}
        <div className="monitoring-detail-bar">
          <nav className="xc-tabs monitoring-range">
            {ranges.map((r) => (
              <button
                key={r}
                className={r === range ? "active" : ""}
                onClick={() => setRange(r)}
              >
                {r}
              </button>
            ))}
          </nav>
          {results.data && (
            <span className="xc-muted">
              {t("Availability")} {results.data.uptime}% · {t("Average")}{" "}
              {Math.round(results.data.avgLatencyMs)} ms · {results.data.total}{" "}
              {t("checks")}
            </span>
          )}
        </div>
        {results.isPending ? (
          <Loading />
        ) : results.isError ? (
          <ErrorState error={results.error} onRetry={() => results.refetch()} />
        ) : (
          <>
            <HistoryChart items={results.data.items} />
            {recent.length > 0 && (
              <div className="xc-table-wrap monitoring-results">
                <table className="xc-table">
                  <thead>
                    <tr>
                      <th>{t("Time")}</th>
                      <th>{t("Result")}</th>
                      <th>{t("Code")}</th>
                      <th>{t("Latency")}</th>
                      <th>{t("Error")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {recent.map((r, i) => (
                      <tr key={r.id ?? i}>
                        <td>{shortDateTime(r.at, language)}</td>
                        <td>
                          <span
                            className={`xc-dot ${r.ok ? "ok" : "danger"}`}
                          />{" "}
                          {r.ok ? t("OK") : t("Failed")}
                        </td>
                        <td>{r.statusCode ?? "—"}</td>
                        <td>{r.latencyMs} ms</td>
                        <td className="monitoring-error-cell">
                          {r.error || "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
        {monitor.lastCheckedAt && (
          <p className="xc-muted monitoring-small">
            {t("Last checked")} {relativeTime(monitor.lastCheckedAt, language)}
          </p>
        )}
        <div className="xc-dialog-actions monitoring-actions">
          <button
            className="xc-btn ghost danger"
            disabled={remove.isPending}
            onClick={async () =>
              (await confirmAction({
                title: `${t("Delete")}“${monitor.name}”？`,
              })) &&
              remove.mutate(monitor.id, {
                onSuccess: () => {
                  toast(t("Deleted"));
                  onClose();
                },
                onError: showError,
              })
            }
          >
            <Trash2 size={14} /> {t("Delete")}
          </button>
          <span className="xc-spacer" />
          <button
            className="xc-btn"
            disabled={save.isPending}
            onClick={() =>
              save.mutate(
                { id: monitor.id, patch: { enabled: !monitor.enabled } },
                {
                  onSuccess: () =>
                    toast(monitor.enabled ? t("Paused") : t("Resumed")),
                  onError: showError,
                },
              )
            }
          >
            {monitor.enabled ? <Pause size={14} /> : <Play size={14} />}{" "}
            {monitor.enabled ? t("Pause") : t("Resume")}
          </button>
          <button className="xc-btn" onClick={() => setEditing(true)}>
            <Pencil size={14} /> {t("Edit")}
          </button>
          <button
            className="xc-btn primary"
            disabled={check.isPending}
            onClick={runCheck}
          >
            <RefreshCw
              size={14}
              className={check.isPending ? "monitoring-spin" : ""}
            />{" "}
            {t("Check now")}
          </button>
        </div>
      </Dialog>
      <MonitorDialog
        open={editing}
        onClose={() => setEditing(false)}
        monitor={monitor}
        kinds={[monitor.kind]}
      />
    </>
  );
}
